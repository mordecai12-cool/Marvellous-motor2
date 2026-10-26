package main

import (
	"database/sql"
	"fmt"
	"net/http"
)

type CaseStaff struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func availableCaseStaff(db *sql.DB) ([]CaseStaff, error) {
	rows, err := db.Query("SELECT id,name,email,role,status FROM admins WHERE status='approved'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []CaseStaff{}
	for rows.Next() {
		var s Staff
		if err = rows.Scan(&s.ID, &s.Name, &s.Email, &s.Role, &s.Status); err != nil {
			return nil, err
		}
		if authorizedStaff(s) {
			list = append(list, CaseStaff{ID: s.ID, Name: s.Name})
		}
	}
	return list, rows.Err()
}
func updateSupportCase(db *sql.DB, w http.ResponseWriter, r *http.Request, staff Staff) {
	id, e := requestID(r, "conversation-id")
	assigned, e2 := optionalInteger(r, "assigned", 1<<50)
	status := r.FormValue("case-status")
	if e != nil || e2 != nil || !oneOf(status, "open", "investigating", "awaiting_customer", "resolved") {
		serviceError(w, 400, "Choose a valid complaint status and assignee.")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		serviceError(w, 500, "Could not update case.")
		return
	}
	defer tx.Rollback()
	var exists bool
	if e = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM support_conversations WHERE id=?)", id).Scan(&exists); e != nil || !exists {
		serviceError(w, 404, "Conversation not found.")
		return
	}
	if assigned > 0 {
		var s Staff
		if e = tx.QueryRow("SELECT id,name,email,role,status FROM admins WHERE id=?", assigned).Scan(&s.ID, &s.Name, &s.Email, &s.Role, &s.Status); e != nil || !authorizedStaff(s) {
			serviceError(w, 400, "Assign an approved administrator.")
			return
		}
	}
	_, err = tx.Exec("INSERT INTO support_cases(conversation_id,status,assigned_to) VALUES(?,?,?) ON CONFLICT(conversation_id) DO UPDATE SET status=excluded.status,assigned_to=excluded.assigned_to,updated_at=CURRENT_TIMESTAMP", id, status, nullableID(assigned))
	state := "open"
	if status == "resolved" {
		state = "resolved"
	}
	if err == nil {
		_, err = tx.Exec("UPDATE support_conversations SET status=? WHERE id=?", state, id)
	}
	if err == nil {
		err = addActivity(tx, staff, "Complaint "+status, fmt.Sprintf("MM-S%06d", id))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		serviceError(w, 500, "Could not save complaint update.")
		return
	}
	serviceJSON(w, 200, map[string]bool{"ok": true})
}
