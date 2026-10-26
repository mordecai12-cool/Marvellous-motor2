package main

import (
	"bytes"
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"net/http"
)

type CustomerAuthPage struct{ Error, Notice, Name, Email, Phone string }

func renderCustomerAuth(w http.ResponseWriter, status int, file string, data CustomerAuthPage) {
	t, err := template.ParseFiles(file)
	if err != nil {
		http.Error(w, "Could not load account page", 500)
		return
	}
	var body bytes.Buffer
	if err = t.Execute(&body, data); err != nil {
		http.Error(w, "Could not render account page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(body.Bytes())
}
func serveCustomerSignIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	renderCustomerAuth(w, 200, "login.html", CustomerAuthPage{})
}

// Return true once an admin sign-in has been handled, including session errors.
// Customer credentials never grant staff access, even when the email is shared.
func signInAdmin(db *sql.DB, w http.ResponseWriter, r *http.Request, email, password string) bool {
	var staff Staff
	var hash string
	err := db.QueryRow("SELECT id,name,email,password,role,status FROM admins WHERE lower(email)=?", email).Scan(&staff.ID, &staff.Name, &staff.Email, &hash, &staff.Role, &staff.Status)
	if err != nil || !authorizedStaff(staff) || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return false
	}
	session, err := store.Get(r, "admin-session")
	if err != nil {
		http.Error(w, "Could not create session", 500)
		return true
	}
	session.Values["admin_id"] = staff.ID
	session.Values["admin_name"] = staff.Name
	session.Values["admin_role"] = staff.Role
	if err = session.Save(r, w); err != nil {
		http.Error(w, "Could not save session", 500)
		return true
	}
	http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
	return true
}
