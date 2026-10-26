package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

type SalesLead struct {
	ID, AssignedID                                                     int64
	Name, Email, Phone, Kind, Vehicle, Stage, Assignee, Followup, Note string
}

func (s SalesLead) Due() bool {
	return s.Stage != "Sold" && s.Followup != "" && s.Followup <= time.Now().In(dealershipTime()).Format("2006-01-02")
}
func loadSales(db *sql.DB) ([]SalesLead, error) {
	rows, err := db.Query(`SELECT e.id,e.name,e.email,e.phone,e.kind,e.vehicle,COALESCE(f.stage,'New'),COALESCE(f.assigned_to,0),COALESCE(a.name,''),COALESCE(f.followup,''),COALESCE(f.note,'') FROM enquiries e LEFT JOIN sales_followups f ON f.enquiry_id=e.id LEFT JOIN admins a ON a.id=f.assigned_to ORDER BY CASE WHEN f.followup!='' THEN 0 ELSE 1 END,f.followup,e.id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []SalesLead{}
	for rows.Next() {
		var v SalesLead
		if err = rows.Scan(&v.ID, &v.Name, &v.Email, &v.Phone, &v.Kind, &v.Vehicle, &v.Stage, &v.AssignedID, &v.Assignee, &v.Followup, &v.Note); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
func manageSales(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	d, ok := operationsData(db, w, r, "sales", "Every enquiry has a next step.", "Assign a salesperson, plan the next follow-up and move customers from interest to sale.")
	if !ok {
		return
	}
	if r.Method == "POST" {
		if err := saveSalesLead(db, r, d.Staff); err != nil {
			d.Error = err.Error()
		} else {
			http.Redirect(w, r, "/admin/sales?saved=1", 303)
			return
		}
	} else if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d.Leads, _ = loadSales(db)
	d.StaffOptions, _ = availableCaseStaff(db)
	renderAdmin(w, "admin-sales.html", d)
}
func saveSalesLead(db *sql.DB, r *http.Request, staff Staff) error {
	id, e := requestID(r, "id")
	stage := r.FormValue("stage")
	assigned, e2 := optionalInteger(r, "assigned", 1<<50)
	followup := r.FormValue("followup")
	note := strings.TrimSpace(r.FormValue("note"))
	if e != nil || e2 != nil || !oneOf(stage, "New", "Contacted", "Appointment", "Negotiating", "Sold") || !validDate(followup) || len(note) > 2000 {
		return errors.New("Choose a valid stage, follow-up date and note of at most 2,000 characters.")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM enquiries WHERE id=?)", id).Scan(&exists); err != nil || !exists {
		return errors.New("Enquiry not found.")
	}
	if assigned > 0 {
		var role, status, email string
		if err = tx.QueryRow("SELECT role,status,email FROM admins WHERE id=?", assigned).Scan(&role, &status, &email); err != nil || !authorizedStaff(Staff{Role: role, Status: status, Email: email}) {
			return errors.New("Assign an approved administrator.")
		}
	}
	_, err = tx.Exec("INSERT INTO sales_followups(enquiry_id,stage,assigned_to,followup,note) VALUES(?,?,?,?,?) ON CONFLICT(enquiry_id) DO UPDATE SET stage=excluded.stage,assigned_to=excluded.assigned_to,followup=excluded.followup,note=excluded.note,updated_at=CURRENT_TIMESTAMP", id, stage, nullableID(assigned), followup, note)
	if err != nil {
		return err
	}
	if err = addActivity(tx, staff, "Sales lead moved to "+stage, r.FormValue("id")); err != nil {
		return err
	}
	return tx.Commit()
}
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
