package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type StaffInvitation struct {
	ID                int64
	Email             string
	ExpiresAt         int64
	UsedAt, RevokedAt sql.NullInt64
}

func (i StaffInvitation) State() string {
	if i.UsedAt.Valid {
		return "Accepted"
	}
	if i.RevokedAt.Valid {
		return "Revoked"
	}
	if i.ExpiresAt <= time.Now().Unix() {
		return "Expired"
	}
	return "Pending signup"
}
func (i StaffInvitation) Active() bool { return i.State() == "Pending signup" }
func (i StaffInvitation) ExpiryLabel() string {
	return time.Unix(i.ExpiresAt, 0).UTC().Format("02 Jan 2006 · 15:04 UTC")
}
func loadStaffInvitations(db *sql.DB) ([]StaffInvitation, error) {
	rows, err := db.Query("SELECT id,email,expires_at,used_at,revoked_at FROM staff_invitations ORDER BY id DESC LIMIT 25")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invitations []StaffInvitation
	for rows.Next() {
		var i StaffInvitation
		if err = rows.Scan(&i.ID, &i.Email, &i.ExpiresAt, &i.UsedAt, &i.RevokedAt); err != nil {
			return nil, err
		}
		invitations = append(invitations, i)
	}
	return invitations, rows.Err()
}
func invitationHash(token string) (string, bool) {
	if len(token) != 64 {
		return "", false
	}
	if _, err := hex.DecodeString(token); err != nil {
		return "", false
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:]), true
}
func adminForInvitation(db *sql.DB, w http.ResponseWriter, r *http.Request) (Staff, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return Staff{}, false
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return Staff{}, false
	}
	return staff, true
}
func createStaffInvitation(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	staff, ok := adminForInvitation(db, w, r)
	if !ok {
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))

	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		http.Error(w, "Enter a valid work email address", http.StatusBadRequest)
		return
	}
	random := make([]byte, 32)
	if _, err = rand.Read(random); err != nil {
		http.Error(w, "Could not create invitation", 500)
		return
	}
	token := hex.EncodeToString(random)
	hash, _ := invitationHash(token)
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not create invitation", 500)
		return
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM admins WHERE lower(email)=?)", email).Scan(&exists); err != nil {
		http.Error(w, "Could not check staff account", 500)
		return
	}
	if exists {
		http.Error(w, "This email already has a staff account. Manage its access in the team list.", 409)
		return
	}
	now := time.Now().Unix()
	// Replacing an invitation immediately invalidates any earlier link for that email.
	if _, err = tx.Exec("UPDATE staff_invitations SET revoked_at=? WHERE email=? AND used_at IS NULL AND revoked_at IS NULL", now, email); err != nil {
		http.Error(w, "Could not replace invitation", 500)
		return
	}
	if _, err = tx.Exec("INSERT INTO staff_invitations(email,token_hash,created_by,expires_at) VALUES(?,?,?,?)", email, hash, staff.ID, now+int64((48*time.Hour).Seconds())); err != nil {
		http.Error(w, "Could not save invitation", 500)
		return
	}
	if err = addActivity(tx, staff, "Staff invitation created", email); err != nil {
		http.Error(w, "Could not record invitation", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save invitation", 500)
		return
	}
	// The raw token is shown once to the owner and never stored or put in an activity log.
	data := AdminPage{Staff: staff, Page: "team", Title: "Your invitation is ready.", Subtitle: "Share this link privately with the person you invited.", InviteURL: "/admin-signup.html?token=" + token, InviteEmail: email}
	data.Invitations, err = loadStaffInvitations(db)
	if err != nil {
		http.Error(w, "Invitation created, but its details could not be loaded", 500)
		return
	}
	renderAdmin(w, "admin-invitation.html", data)
}
func revokeStaffInvitation(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	staff, ok := adminForInvitation(db, w, r)
	if !ok {
		return
	}
	id, err := requestID(r, "invitation-id")
	if err != nil {
		http.Error(w, "Invalid invitation", 400)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not revoke invitation", 500)
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE staff_invitations SET revoked_at=? WHERE id=? AND used_at IS NULL AND revoked_at IS NULL AND expires_at>?", time.Now().Unix(), id, time.Now().Unix())
	if err != nil {
		http.Error(w, "Could not revoke invitation", 500)
		return
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		http.Error(w, "This invitation is no longer active", 409)
		return
	}
	if err = addActivity(tx, staff, "Staff invitation revoked", fmt.Sprintf("Invitation #%d", id)); err != nil {
		http.Error(w, "Could not record invitation change", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save invitation change", 500)
		return
	}
	http.Redirect(w, r, "/admin/team?notice=invite-revoked", 303)
}

type StaffSignupPage struct {
	Token, Email, Name, Error, CSRF string
	Valid, Setup                    bool
}

func renderStaffSignup(w http.ResponseWriter, status int, data StaffSignupPage) {
	t, err := template.ParseFiles("admin-signup.html")
	if err != nil {
		http.Error(w, "Could not load staff signup", 500)
		return
	}
	var output bytes.Buffer
	if err = t.Execute(&output, data); err != nil {
		http.Error(w, "Could not render staff signup", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "strict-origin")
	w.WriteHeader(status)
	w.Write(output.Bytes())
}
func serveStaffSignup(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if r.Method == http.MethodPost {
		token = r.FormValue("token")
	}
	if ownerOnlyAccess() && strings.TrimSpace(token) == "" {
		serveReservedOwnerSignup(db, w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	data := StaffSignupPage{}
	if cookie, err := r.Cookie("mm-csrf"); err == nil {
		data.CSRF = cookie.Value
	}
	data.Token = strings.TrimSpace(r.URL.Query().Get("token"))
	if r.Method == http.MethodPost {
		data.Token = strings.TrimSpace(r.FormValue("token"))
	}
	if data.Token == "" && r.Method == http.MethodGet {
		renderStaffSignup(w, 200, data)
		return
	}
	hash, valid := invitationHash(data.Token)
	var invitationID int64
	var creator Staff
	var err error
	if valid {
		err = db.QueryRow(`SELECT i.id,i.email,a.id,a.name,a.email,a.role,a.status FROM staff_invitations i JOIN admins a ON a.id=i.created_by WHERE i.token_hash=? AND i.used_at IS NULL AND i.revoked_at IS NULL AND i.expires_at>? AND a.role IN ('super_admin','admin') AND a.status='approved'`, hash, time.Now().Unix()).Scan(&invitationID, &data.Email, &creator.ID, &creator.Name, &creator.Email, &creator.Role, &creator.Status)
	}
	if !valid || err != nil {
		data.Token = ""
		data.Error = "This invitation is invalid, expired or already used. Ask an administrator for a new link."
		renderStaffSignup(w, 400, data)
		return
	}
	data.Valid = true
	if ownerOnlyAccess() && !authorizedStaff(creator) {
		data.Valid = false
		data.Token = ""
		data.Email = ""
		data.Error = "This invitation is not authorized for owner access. Contact the platform owner."
		renderStaffSignup(w, 403, data)
		return
	}
	if r.Method == http.MethodGet {
		renderStaffSignup(w, 200, data)
		return
	}
	data.Name = strings.TrimSpace(r.FormValue("name"))
	password := r.FormValue("password")
	if len(data.Name) < 2 || len(data.Name) > 100 || len(password) < 12 || len(password) > 72 {
		data.Error = "Enter your full name and a password with 12–72 characters."
		renderStaffSignup(w, 400, data)
		return
	}
	if password != r.FormValue("confirm-password") {
		data.Error = "The passwords do not match. Please enter them again."
		renderStaffSignup(w, 400, data)
		return
	}
	if strings.ToLower(strings.TrimSpace(r.FormValue("email"))) != data.Email {
		data.Error = "Use the work email assigned to this invitation."
		renderStaffSignup(w, 400, data)
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Could not secure your account", 500)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not create your account", 500)
		return
	}
	defer tx.Rollback()
	// Claim the invitation within the same transaction that creates the account.
	// Re-check expiry, revocation, single use and the inviting admin's current authority here.
	result, err := tx.Exec(`UPDATE staff_invitations SET used_at=? WHERE id=? AND used_at IS NULL AND revoked_at IS NULL AND expires_at>? AND EXISTS(SELECT 1 FROM admins a WHERE a.id=staff_invitations.created_by AND a.role IN ('super_admin','admin') AND a.status='approved')`, time.Now().Unix(), invitationID, time.Now().Unix())
	if err != nil {
		http.Error(w, "Could not accept invitation", 500)
		return
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		data.Valid = false
		data.Token = ""
		data.Error = "This invitation is no longer available. Ask the owner for a new link."
		renderStaffSignup(w, 409, data)
		return
	}
	var exists bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM admins WHERE lower(email)=?)", data.Email).Scan(&exists); err != nil {
		http.Error(w, "Could not check staff account", 500)
		return
	}
	if exists {
		data.Error = "This email already has a staff account. Contact the owner to restore access."
		renderStaffSignup(w, 409, data)
		return
	}
	role := "admin"
	if ownerOnlyAccess() && allowedOwnerEmail(data.Email) {
		role = "super_admin"
	}
	if _, err = tx.Exec("INSERT INTO admins(name,email,password,role,status) VALUES(?,?,?,?,'approved')", data.Name, data.Email, string(passwordHash), role); err != nil {
		http.Error(w, "Could not create your account. Please contact the owner.", 409)
		return
	}
	if err = addActivity(tx, creator, "Invited staff account activated", data.Email); err != nil {
		http.Error(w, "Could not record account activation", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save your account", 500)
		return
	}
	http.Redirect(w, r, "/admin-login.html?notice=account-created", 303)
}
