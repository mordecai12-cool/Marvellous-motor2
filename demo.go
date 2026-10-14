package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// Demo credentials are generated on this machine and never served by HTTP.
func seedLocalDemo(db *sql.DB) {
	if os.Getenv("MM_DEMO_MODE") != "true" {
		return
	}
	type credentials struct {
		CustomerEmail string `json:"customer_email"`
		AdminEmail    string `json:"admin_email"`
		Password      string `json:"password"`
	}
	account := credentials{CustomerEmail: "customer@marvellous.local", AdminEmail: "admin@marvellous.local"}
	if raw, err := os.ReadFile(".local/demo-accounts.json"); err == nil {
		if err := json.Unmarshal(raw, &account); err != nil {
			log.Fatal("Invalid local demo credentials")
		}
	} else {
		bytes := make([]byte, 12)
		if _, err := rand.Read(bytes); err != nil {
			log.Fatal(err)
		}
		account.Password = "Local-" + hex.EncodeToString(bytes)
		raw, _ := json.MarshalIndent(account, "", "  ")
		if err := os.WriteFile(".local/demo-accounts.json", raw, 0600); err != nil {
			log.Fatal(err)
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(account.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	if _, err = db.Exec("INSERT OR IGNORE INTO customers(name,email,phone,password) VALUES(?,?,?,?)", "Demo Explorer", account.CustomerEmail, "08000000000", string(hash)); err != nil {
		log.Fatal(err)
	}
	if _, err = db.Exec("INSERT OR IGNORE INTO admins(name,email,password,role,status) VALUES(?,?,?,?,?)", "Local Demo Administrator", account.AdminEmail, string(hash), "super_admin", "approved"); err != nil {
		log.Fatal(err)
	}
	log.Println("Fictional demo accounts enabled. Credentials are in .local/demo-accounts.json")
}

func handleHealth(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	if err := db.PingContext(r.Context()); err != nil {
		http.Error(w, "Database unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","mode":"local-preview"}`))
}
