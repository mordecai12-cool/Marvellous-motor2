package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
)

// deleteCustomerAccount returns an error message,
// HTTP status, and whether deletion succeeded.
func deleteCustomerAccount(
	db *sql.DB,
	r *http.Request,
	id int64,
) (string, int, bool) {
	if r.FormValue("confirm-delete") != "yes" {
		return "Please confirm account deletion.", 400, false
	}

	password := r.FormValue("delete-password")
	if password == "" {
		return "Enter your current password.", 400, false
	}

	var hash string
	err := db.QueryRow(
		"SELECT password FROM customers WHERE id=?", id,
	).Scan(&hash)
	if err != nil {
		return "Customer account not found.", 404, false
	}

	if bcrypt.CompareHashAndPassword(
		[]byte(hash), []byte(password),
	) != nil {
		return "Incorrect password.", 400, false
	}

	tx, err := db.Begin()
	if err != nil {
		return "Could not start deletion.", 500, false
	}
	defer tx.Rollback()

	// Order and payment snapshots stay available to staff, but no longer
	// authorize any customer session to access them.
	if _, err = tx.Exec("UPDATE orders SET customer_id=NULL WHERE customer_id=?", id); err != nil {
		return "Could not preserve your order records safely.", 500, false
	}

	// Collect private attachment locations before deleting
	// their database records.
	rows, err := tx.Query(`
		SELECT a.storage_key
		FROM support_attachments a
		JOIN support_messages m ON m.id=a.message_id
		JOIN support_conversations c
			ON c.id=m.conversation_id
		WHERE c.customer_id=?
	`, id)
	if err != nil {
		return "Could not inspect support files.", 500, false
	}

	var attachments []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return "Could not inspect support files.", 500, false
		}
		attachments = append(attachments, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "Could not inspect support files.", 500, false
	}

	// Remove dependent private records first.
	queries := []string{
		`DELETE FROM sales_followups WHERE enquiry_id IN (SELECT id FROM enquiries WHERE customer_id=?)`,
		`DELETE FROM enquiry_events WHERE enquiry_id IN (SELECT id FROM enquiries WHERE customer_id=?)`,
		`DELETE FROM enquiries WHERE customer_id=?`,
		`DELETE FROM support_cases
		 WHERE conversation_id IN (
		   SELECT id FROM support_conversations
		   WHERE customer_id=?)`,

		`DELETE FROM support_attachments
		 WHERE message_id IN (
		   SELECT m.id FROM support_messages m
		   JOIN support_conversations c
		   ON c.id=m.conversation_id
		   WHERE c.customer_id=?)`,

		`DELETE FROM support_messages
		 WHERE conversation_id IN (
		   SELECT id FROM support_conversations
		   WHERE customer_id=?)`,

		`DELETE FROM support_conversations
		 WHERE customer_id=?`,

		`DELETE FROM customer_reviews WHERE customer_id=?`,
		`DELETE FROM appointment_notifications WHERE customer_id=?`,
		`DELETE FROM appointments WHERE customer_id=?`,
		`DELETE FROM password_reset_tokens WHERE customer_id=?`,
		`DELETE FROM account_recovery_links WHERE customer_id=?`,
		`DELETE FROM account_recovery_requests WHERE customer_id=?`,
		`DELETE FROM customer_auth_epochs WHERE customer_id=?`,
	}

	for _, query := range queries {
		if _, err = tx.Exec(query, id); err != nil {
			log.Printf("Account deletion database error: %v", err)
			return "Could not safely delete account data.", 500, false
		}
	}

	result, err := tx.Exec(
		"DELETE FROM customers WHERE id=? AND password=?",
		id, hash,
	)
	if err != nil {
		return "Could not remove customer account.", 500, false
	}

	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return "Account changed during deletion. Try again.", 409, false
	}

	// Anonymous deletion audit record.
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return "Could not record deletion.", 500, false
	}
	_, err = tx.Exec(
		`INSERT INTO account_deletion_audit
		 (audit_id, deleted_at)
		 VALUES (?, CURRENT_TIMESTAMP)`,
		hex.EncodeToString(token),
	)
	if err != nil {
		return "Could not record deletion.", 500, false
	}

	if err = tx.Commit(); err != nil {
		return "Could not complete account deletion.", 500, false
	}

	// Database deletion has committed.
	// Remove the corresponding files from private storage.
	for _, key := range attachments {
		if key == "" || filepath.Base(key) != key {
			log.Print("Invalid support attachment key during cleanup")
			continue
		}
		file := filepath.Join(uploadDirectory(), "support", key)
		if err = os.Remove(file); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			log.Printf("Support attachment cleanup failed: %v", err)
		}
	}

	return "", 303, true
}
