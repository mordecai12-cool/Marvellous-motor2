package main

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

type BankAccount struct{ BankName, AccountName, AccountNumber string }

func (b BankAccount) Configured() bool {
	return b.BankName != "" && b.AccountName != "" && b.AccountNumber != ""
}
func initSettings(db *sql.DB) error {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS payment_account (
            id INTEGER PRIMARY KEY CHECK(id=1),
            bank_name TEXT NOT NULL DEFAULT '',
            account_name TEXT NOT NULL DEFAULT '',
            account_number TEXT NOT NULL DEFAULT ''
        );
        INSERT OR IGNORE INTO payment_account(id,bank_name,account_name,account_number)
        VALUES(1,'Opay','Otiti Mordecai Mavellouse','8106333322');
        UPDATE payment_account SET bank_name='Opay',account_name='Otiti Mordecai Mavellouse',account_number='8106333322'
        WHERE id=1 AND bank_name='' AND account_name='' AND account_number='';
        CREATE TABLE IF NOT EXISTS account_deletion_audit (
            audit_id TEXT PRIMARY KEY,
            deleted_at DATETIME NOT NULL
        );`)
	return err
}
func loadBankAccount(db *sql.DB) (b BankAccount, err error) {
	err = db.QueryRow("SELECT bank_name,account_name,account_number FROM payment_account WHERE id=1").Scan(&b.BankName, &b.AccountName, &b.AccountNumber)
	return
}
func registerSettings(db *sql.DB) {
	http.HandleFunc("/admin/settings", func(w http.ResponseWriter, r *http.Request) { serveSettings(db, w, r, true) })
	http.HandleFunc("/account/settings", func(w http.ResponseWriter, r *http.Request) { serveSettings(db, w, r, false) })
}

// The account identity always comes from the authenticated session, never the form.
func changeAccountPassword(db *sql.DB, r *http.Request, table string, id int64) (string, int) {
	current, next := r.FormValue("current-password"), r.FormValue("new-password")
	if len(current) > 72 || len(next) < 8 || len(next) > 72 {
		return "Use a new password with 8–72 bytes.", 400
	}
	if next != r.FormValue("confirm-password") {
		return "The new passwords do not match.", 400
	}
	if current == next {
		return "Choose a password different from your current password.", 400
	}
	var old string
	if err := db.QueryRow("SELECT password FROM "+table+" WHERE id=?", id).Scan(&old); err != nil {
		return "Could not load your account.", 500
	}
	if bcrypt.CompareHashAndPassword([]byte(old), []byte(current)) != nil {
		return "Your current password is incorrect.", 400
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return "Could not secure your new password.", 500
	}
	tx, err := db.Begin()
	if err != nil {
		return "Could not save your password.", 500
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE "+table+" SET password=? WHERE id=? AND password=?", string(hash), id, old)
	if err != nil {
		return "Could not save your password.", 500
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return "Your password changed during this request. Please try again.", 409
	}
	if table == "customers" {
		if _, err = tx.Exec("DELETE FROM password_reset_tokens WHERE customer_id=?", id); err != nil {
			return "Could not update password recovery.", 500
		}
		for _, recoveryTable := range []string{"account_recovery_links", "account_recovery_requests"} {
			if _, err = tx.Exec("DELETE FROM "+recoveryTable+" WHERE customer_id=?", id); err != nil {
				return "Could not update password recovery.", 500
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return "Could not save your password.", 500
	}
	return "", 303
}

var accountNumberPattern = regexp.MustCompile(`^[0-9]{10}$`)

func serveSettings(db *sql.DB, w http.ResponseWriter, r *http.Request, admin bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var id int64
	table, path := "customers", "/account/settings"
	var staff Staff
	var customer ServiceCustomer
	var err error
	if admin {
		var ok bool
		staff, ok = requireStaff(db, w, r)
		if !ok {
			return
		}
		id = staff.ID
		table = "admins"
		path = "/admin/settings"
	} else {
		customer, err = serviceCustomer(db, r)
		if err != nil {
			http.Redirect(w, r, "/login.html", 303)
			return
		}
		id = customer.ID
	}
	b, err := loadBankAccount(db)
	if err != nil {
		http.Error(w, "Could not load payment settings", 500)
		return
	}
	message := ""
	status := 200
	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "delete-account":
			if admin {
				http.Error(w, "Use customer settings to delete a customer account", 403)
				return
			}
			var deleted bool
			message, status, deleted = deleteCustomerAccount(db, r, id)
			if deleted {
				handleLogout(w, r)
				return
			}
		case "password":
			message, status = changeAccountPassword(db, r, table, id)
		case "bank":
			if !admin {
				http.Error(w, "Administrator access required", 403)
				return
			}
			b = BankAccount{strings.TrimSpace(r.FormValue("bank-name")), strings.TrimSpace(r.FormValue("account-name")), strings.TrimSpace(r.FormValue("account-number"))}
			if !utf8.ValidString(b.BankName+b.AccountName) || len(b.BankName) < 2 || len(b.BankName) > 100 || len(b.AccountName) < 2 || len(b.AccountName) > 150 || !accountNumberPattern.MatchString(b.AccountNumber) {
				message = "Enter a bank name, account holder name and a 10-digit account number."
				status = 400
			} else {
				tx, e := db.Begin()
				if e != nil {
					http.Error(w, "Could not save payment settings", 500)
					return
				}
				defer tx.Rollback()
				_, e = tx.Exec("UPDATE payment_account SET bank_name=?,account_name=?,account_number=? WHERE id=1", b.BankName, b.AccountName, b.AccountNumber)
				if e == nil {
					e = addActivity(tx, staff, "Payment bank account updated", b.BankName+" / "+b.AccountNumber)
				}
				if e == nil {
					e = tx.Commit()
				}
				if e != nil {
					http.Error(w, "Could not save payment settings", 500)
					return
				}
				status = 303
			}
		default:
			http.Error(w, "Invalid settings action", 400)
			return
		}
		if status == 303 {
			http.Redirect(w, r, path+"?notice="+r.FormValue("action"), 303)
			return
		}
	}
	notice := ""
	switch r.URL.Query().Get("notice") {
	case "password":
		notice = "Your password has been changed. Use the new password next time you sign in."
	case "bank":
		notice = "Payment account saved. Customers will see the updated details at checkout."
	}
	if admin {
		metrics, e := loadMetrics(db)
		if e != nil {
			http.Error(w, "Could not load settings", 500)
			return
		}
		renderAdminStatus(w, status, "admin-settings.html", AdminPage{Staff: staff, Page: "settings", Title: "Your settings.", Subtitle: "Manage your password and the bank account shown at checkout.", Metrics: metrics, Error: message, Notice: notice, Bank: b})
	} else {
		renderCustomerAccount(w, status, "customer-settings.html", CustomerAccountPage{ShowroomData: ShowroomData{SignedIn: true, Page: "settings"}, CustomerID: id, Name: customer.Name, Email: customer.Email, Error: message, Notice: notice})
	}
}
