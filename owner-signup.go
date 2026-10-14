package main

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// The durable singleton is consumed in the same transaction as account creation.
// Disabling or deleting an account never reopens public registration.
func initOwnerSignup(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS admin_setup_state(id INTEGER PRIMARY KEY CHECK(id=1),consumed_at INTEGER);
 INSERT OR IGNORE INTO admin_setup_state(id) VALUES(1);`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE admin_setup_state SET consumed_at=COALESCE(consumed_at,?) WHERE id=1 AND EXISTS(SELECT 1 FROM admins WHERE lower(email)=? AND status!='setup_pending')`, time.Now().Unix(), deploymentSecondOwnerEmail)
	return err
}
func serveReservedOwnerSignup(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	data := StaffSignupPage{Email: deploymentSecondOwnerEmail, Setup: true, Valid: true}
	if cookie, err := r.Cookie("mm-csrf"); err == nil {
		data.CSRF = cookie.Value
	}
	var open bool
	err := db.QueryRow(`SELECT consumed_at IS NULL AND NOT EXISTS(SELECT 1 FROM admins WHERE lower(email)=? AND status!='setup_pending') FROM admin_setup_state WHERE id=1`, deploymentSecondOwnerEmail).Scan(&open)
	if err != nil {
		http.Error(w, "Could not check signup availability", 500)
		return
	}
	if !open {
		data.Valid = false
		data.Setup = false
		data.Error = "The additional signup slot has been used. New administrators need an invitation from an owner."
		renderStaffSignup(w, 403, data)
		return
	}
	if r.Method == http.MethodGet {
		renderStaffSignup(w, 200, data)
		return
	}
	data.Name = strings.TrimSpace(r.FormValue("name"))
	password := r.FormValue("password")
	if strings.ToLower(strings.TrimSpace(r.FormValue("email"))) != deploymentSecondOwnerEmail {
		data.Error = "This last signup slot is reserved for Marvellous Otit's email address."
	} else if !utf8.ValidString(data.Name) || utf8.RuneCountInString(data.Name) < 2 || utf8.RuneCountInString(data.Name) > 100 || len(password) < 12 || len(password) > 72 {
		data.Error = "Enter your full name and a password with 12–72 characters."
	} else if password != r.FormValue("confirm-password") {
		data.Error = "The passwords do not match."
	}
	if data.Error != "" {
		renderStaffSignup(w, 400, data)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Could not secure account", 500)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not create account", 500)
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE admin_setup_state SET consumed_at=? WHERE id=1 AND consumed_at IS NULL`, time.Now().Unix())
	if err != nil {
		http.Error(w, "Could not claim signup slot", 500)
		return
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		http.Error(w, "The last signup slot has already been used", 409)
		return
	}
	result, err = tx.Exec(`UPDATE admins SET name=?,password=?,role='super_admin',status='approved' WHERE lower(email)=? AND status='setup_pending'`, data.Name, string(hash), data.Email)
	if err != nil {
		http.Error(w, "Could not activate account", 500)
		return
	}
	n, err = result.RowsAffected()
	if err != nil {
		http.Error(w, "Could not activate account", 500)
		return
	}
	var id int64
	if n == 0 {
		result, err = tx.Exec(`INSERT INTO admins(name,email,password,role,status) VALUES(?,?,?,'super_admin','approved')`, data.Name, data.Email, string(hash))
		if err != nil {
			http.Error(w, "This account already exists. Contact an owner.", 409)
			return
		}
		id, err = result.LastInsertId()
	} else {
		err = tx.QueryRow(`SELECT id FROM admins WHERE lower(email)=?`, data.Email).Scan(&id)
	}
	if err != nil {
		http.Error(w, "Could not record signup", 500)
		return
	}
	if err = addActivity(tx, Staff{ID: id, Name: data.Name, Email: data.Email, Role: "super_admin", Status: "approved"}, "Reserved owner signup activated", data.Email); err != nil {
		http.Error(w, "Could not record signup", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save account", 500)
		return
	}
	http.Redirect(w, r, "/admin-login.html?notice=account-created", 303)
}
