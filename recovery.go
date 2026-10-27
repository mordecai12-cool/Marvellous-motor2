package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type RecoveryCustomer struct {
	ID                            int64
	Name, Email, Phone, Requested string
}

func recoveryHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func customerEpoch(db *sql.DB, id int64) int64 {
	var epoch int64
	_ = db.QueryRow("SELECT epoch FROM customer_auth_epochs WHERE customer_id=?", id).Scan(&epoch)
	return epoch
}

// Clear the decoded session in the request registry as well as the browser cookie,
// so every downstream handler rejects sessions revoked by a password reset.
func recoverySessionMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := store.Get(r, "customer-session")
		if err == nil {
			if id, ok := session.Values["customer_id"].(int64); ok {
				epoch, _ := session.Values["auth_epoch"].(int64)
				var exists bool
				lookupErr := db.QueryRow("SELECT EXISTS(SELECT 1 FROM customers WHERE id=?)", id).Scan(&exists)
				if lookupErr != nil || !exists || epoch != customerEpoch(db, id) {
					session.Values = map[interface{}]interface{}{}
					session.Options.MaxAge = -1
					_ = session.Save(r, w)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
func serveRecoveryRequest(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderCustomerAuth(w, 200, "forgot-password.html", CustomerAuthPage{})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	// One entry per known customer; repeat requests never invalidate a valid link.
	_, err := db.Exec(`INSERT INTO account_recovery_requests(customer_id) SELECT id FROM customers WHERE lower(email)=? ON CONFLICT(customer_id) DO UPDATE SET requested_at=CURRENT_TIMESTAMP`, email)
	if err != nil {
		http.Error(w, "Could not record recovery request", 500)
		return
	}
	renderCustomerAuth(w, 200, "forgot-password.html", CustomerAuthPage{Notice: "If this customer account exists, your recovery request is now with the admin team. Contact customer care using the contact details already on your account so staff can verify your identity. No email has been sent automatically."})
}
func manageRecovery(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	d, ok := operationsData(db, w, r, "recovery", "Help customers sign in.", "Verify the account holder, then share a private recovery link through their existing contact details.")
	if !ok {
		return
	}
	p := d
	status := 200
	if r.Method == http.MethodPost {
		id, _ := strconv.ParseInt(r.FormValue("customer"), 10, 64)
		reason := strings.TrimSpace(r.FormValue("reason"))
		var hash string
		err := db.QueryRow("SELECT password FROM admins WHERE id=?", d.Staff.ID).Scan(&hash)
		if id < 1 || r.FormValue("verified") != "yes" || len(reason) < 10 || len(reason) > 500 || err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("admin-password"))) != nil {
			p.Error = "Confirm the customer's identity, enter a verification note (10–500 characters), and confirm your admin password."
			status = 400
		} else {
			var person RecoveryCustomer
			err = db.QueryRow("SELECT id,name,email,phone FROM customers WHERE id=?", id).Scan(&person.ID, &person.Name, &person.Email, &person.Phone)
			if err != nil {
				http.Error(w, "Customer not found", 404)
				return
			}
			raw := make([]byte, 32)
			if _, err = rand.Read(raw); err != nil {
				http.Error(w, "Could not create recovery link", 500)
				return
			}
			token := hex.EncodeToString(raw)
			tx, e := db.Begin()
			if e != nil {
				http.Error(w, "Could not create recovery link", 500)
				return
			}
			defer tx.Rollback()
			_, e = tx.Exec("DELETE FROM account_recovery_links WHERE customer_id=?", id)
			if e == nil {
				_, e = tx.Exec("INSERT INTO account_recovery_links(token_hash,customer_id,expires_at) VALUES(?,?,?)", recoveryHash(token), id, time.Now().UTC().Add(30*time.Minute).Format(time.RFC3339))
			}
			if e == nil {
				e = addActivity(tx, d.Staff, "Customer recovery link issued", fmt.Sprintf("Customer MM-C%06d. Identity verification: %s", id, reason))
			}
			if e == nil {
				e = tx.Commit()
			}
			if e != nil {
				http.Error(w, "Could not save recovery link", 500)
				return
			}
			p.RecoveryLink = "/reset-password?token=" + token
			p.RecoveryRecipient = person.Email
			p.Notice = "Recovery link created. It expires in 30 minutes and works once. A new link replaces the previous link."
		}
	} else if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	rows, err := db.Query(`SELECT c.id,c.name,c.email,c.phone,COALESCE(r.requested_at,'') FROM customers c LEFT JOIN account_recovery_requests r ON r.customer_id=c.id ORDER BY r.requested_at DESC,c.name`)
	if err != nil {
		http.Error(w, "Could not load customers", 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c RecoveryCustomer
		if err = rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Requested); err != nil {
			http.Error(w, "Could not load customers", 500)
			return
		}
		p.RecoveryPeople = append(p.RecoveryPeople, c)
	}
	if rows.Err() != nil {
		http.Error(w, "Could not load customers", 500)
		return
	}
	renderAdminStatus(w, status, "admin-recovery.html", p)
}
func redeemRecoveryLink(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	token := r.URL.Query().Get("token")
	if r.Method == http.MethodPost {
		token = r.FormValue("token")
	}
	var id int64
	var expiry string
	err := db.QueryRow("SELECT customer_id,expires_at FROM account_recovery_links WHERE token_hash=?", recoveryHash(token)).Scan(&id, &expiry)
	deadline, e := time.Parse(time.RFC3339, expiry)
	if len(token) != 64 || err != nil || e != nil || !time.Now().Before(deadline) {
		http.Error(w, "This recovery link is invalid or expired. Ask customer care for a new one.", 400)
		return
	}
	data := struct{ Token, Error string }{Token: token}
	if r.Method == http.MethodPost {
		password := r.FormValue("password")
		if len(password) < 8 || len(password) > 72 || password != r.FormValue("confirm-password") {
			data.Error = "Enter matching passwords with 8–72 bytes."
			renderBusinessPage(w, 400, "reset-password.html", data)
			return
		}
		var old string
		if err = db.QueryRow("SELECT password FROM customers WHERE id=?", id).Scan(&old); err != nil {
			http.Error(w, "Could not load account", 500)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(old), []byte(password)) == nil {
			data.Error = "Choose a password different from your current password."
			renderBusinessPage(w, 400, "reset-password.html", data)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Could not secure password", 500)
			return
		}
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Could not reset password", 500)
			return
		}
		defer tx.Rollback()
		result, err := tx.Exec("DELETE FROM account_recovery_links WHERE token_hash=? AND expires_at>?", recoveryHash(token), time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			http.Error(w, "Could not reset password", 500)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			http.Error(w, "Recovery link has already been used or expired", 400)
			return
		}
		result, err = tx.Exec("UPDATE customers SET password=? WHERE id=? AND password=?", string(hash), id, old)
		if err == nil {
			changed, e := result.RowsAffected()
			if e != nil || changed != 1 {
				http.Error(w, "Your password changed during this request. Ask for a new recovery link.", 409)
				return
			}
		}
		if err == nil {
			_, err = tx.Exec("INSERT INTO customer_auth_epochs(customer_id,epoch) VALUES(?,1) ON CONFLICT(customer_id) DO UPDATE SET epoch=epoch+1", id)
		}
		for _, table := range []string{"account_recovery_links", "account_recovery_requests", "password_reset_tokens"} {
			if err == nil {
				_, err = tx.Exec("DELETE FROM "+table+" WHERE customer_id=?", id)
			}
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			http.Error(w, "Could not reset password", 500)
			return
		}
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	renderBusinessPage(w, 200, "reset-password.html", data)
}
