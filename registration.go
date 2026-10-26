package main

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"
)

func serveRegistration(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderCustomerAuth(w, 200, "register.html", CustomerAuthPage{})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	data := CustomerAuthPage{Name: strings.TrimSpace(r.FormValue("name")), Email: strings.ToLower(strings.TrimSpace(r.FormValue("email"))), Phone: strings.TrimSpace(r.FormValue("phone"))}
	fail := func(status int, message string) {
		data.Error = message
		renderCustomerAuth(w, status, "register.html", data)
	}
	parsed, err := mail.ParseAddress(data.Email)
	if err != nil || parsed.Address != data.Email || len(data.Email) > 254 || !utf8.ValidString(data.Name) || len(data.Name) < 2 || len(data.Name) > 100 || !profilePhone.MatchString(data.Phone) {
		fail(400, "Enter your full name, a valid email address and phone number.")
		return
	}
	password := r.FormValue("password")
	if len(password) < 8 || len(password) > 72 {
		fail(400, "Your password must contain 8–72 bytes.")
		return
	}
	if password != r.FormValue("confirm-password") {
		fail(400, "The passwords do not match. Please enter them again.")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		fail(500, "We could not open registration. Please try again.")
		return
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM customers WHERE lower(email)=? UNION ALL SELECT 1 FROM admins WHERE lower(email)=?)`, data.Email, data.Email).Scan(&exists); err != nil {
		fail(500, "We could not check your account. Please try again.")
		return
	}
	if exists {
		fail(409, "These details already exist. An account with this email is already registered. Sign in below, or use Forgot Password on the sign-in page.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fail(500, "We could not secure your password. Please try again.")
		return
	}
	_, err = tx.Exec("INSERT INTO customers(name,email,phone,password) VALUES(?,?,?,?)", data.Name, data.Email, data.Phone, string(hash))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			fail(409, "These details already exist. Please sign in with this email.")
		} else {
			fail(500, "Your account could not be saved. Please try again.")
		}
		return
	}
	if err = tx.Commit(); err != nil {
		fail(500, "Your account could not be saved. Please try again.")
		return
	}
	http.Redirect(w, r, "/login.html?notice=registered", 303)
}
