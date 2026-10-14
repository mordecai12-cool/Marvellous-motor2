package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Local account configuration contains a hash, never a plaintext password.
// It creates an absent owner without resetting existing access on restart.
func seedConfiguredLocalOwner(db *sql.DB, filename string) error {
	raw, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var account struct {
		Name         string `json:"name"`
		Email        string `json:"email"`
		PasswordHash string `json:"password_hash"`
	}
	if err = json.Unmarshal(raw, &account); err != nil {
		return err
	}
	account.Email = strings.ToLower(strings.TrimSpace(account.Email))
	parsed, err := mail.ParseAddress(account.Email)
	if err != nil || parsed.Address != account.Email || strings.TrimSpace(account.Name) == "" {
		return errors.New("invalid local owner configuration")
	}
	cost, err := bcrypt.Cost([]byte(account.PasswordHash))
	if err != nil || cost < bcrypt.DefaultCost {
		return errors.New("invalid local owner password hash")
	}
	_, err = db.Exec("INSERT INTO admins(name,email,password,role,status) SELECT ?,?,?,'super_admin','approved' WHERE NOT EXISTS(SELECT 1 FROM admins WHERE lower(email)=?)", account.Name, account.Email, account.PasswordHash, account.Email)
	return err
}
