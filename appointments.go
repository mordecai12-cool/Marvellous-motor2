package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type VisitSlot struct {
	ID                                                       int64
	VehicleID, VehicleName, Kind, StartsAt, EndsAt, Location string
	Active                                                   bool
}

func (s VisitSlot) TimeLabel() string {
	t, err := time.Parse(time.RFC3339, s.StartsAt)
	if err != nil {
		return s.StartsAt
	}
	return t.In(dealershipTime()).Format("Mon 2 Jan 2006, 15:04 WAT")
}
func (s VisitSlot) KindLabel() string {
	if s.Kind == "inspection" {
		return "Inspection"
	}
	return "Test drive"
}

type Appointment struct {
	ID, CustomerID, SlotID                                                                      int64
	CustomerName, Email, VehicleID, VehicleName, Kind, StartsAt, EndsAt, Location, Status, Note string
}

func (a Appointment) TimeLabel() string { return (VisitSlot{StartsAt: a.StartsAt}).TimeLabel() }
func (a Appointment) Reference() string { return fmt.Sprintf("MM-V%06d", a.ID) }
func (a Appointment) CanCancel() bool   { return a.Status == "requested" || a.Status == "confirmed" }
func loadVisitSlots(db *sql.DB, available bool) ([]VisitSlot, error) {
	query := `SELECT s.id,s.vehicle_id,s.kind,s.starts_at,s.ends_at,s.location,s.active,COALESCE(v.payload,'') FROM visit_slots s LEFT JOIN fleet_vehicles v ON v.id=s.vehicle_id`
	if available {
		query += ` WHERE s.active=1 AND s.starts_at>? AND NOT EXISTS(SELECT 1 FROM appointments a WHERE a.slot_id=s.id AND a.status!='cancelled')`
	}
	query += " ORDER BY s.starts_at DESC LIMIT 100"
	args := []any{}
	if available {
		args = append(args, time.Now().UTC().Format(time.RFC3339))
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []VisitSlot{}
	for rows.Next() {
		var s VisitSlot
		var raw string
		if err = rows.Scan(&s.ID, &s.VehicleID, &s.Kind, &s.StartsAt, &s.EndsAt, &s.Location, &s.Active, &raw); err != nil {
			return nil, err
		}
		var v Vehicle
		json.Unmarshal([]byte(raw), &v)
		s.VehicleName = v.Name()
		if s.VehicleID == "" {
			s.VehicleName = "Any available vehicle"
		}
		list = append(list, s)
	}
	return list, rows.Err()
}
func loadAppointments(db *sql.DB, customer int64) ([]Appointment, error) {
	query := `SELECT a.id,a.customer_id,a.slot_id,c.name,c.email,a.vehicle_id,v.payload,s.kind,s.starts_at,s.ends_at,s.location,a.status,a.note FROM appointments a JOIN customers c ON c.id=a.customer_id JOIN visit_slots s ON s.id=a.slot_id JOIN fleet_vehicles v ON v.id=a.vehicle_id`
	args := []any{}
	if customer > 0 {
		query += " WHERE a.customer_id=?"
		args = append(args, customer)
	}
	query += " ORDER BY s.starts_at DESC LIMIT 100"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Appointment{}
	for rows.Next() {
		var a Appointment
		var raw string
		if err = rows.Scan(&a.ID, &a.CustomerID, &a.SlotID, &a.CustomerName, &a.Email, &a.VehicleID, &raw, &a.Kind, &a.StartsAt, &a.EndsAt, &a.Location, &a.Status, &a.Note); err != nil {
			return nil, err
		}
		var v Vehicle
		json.Unmarshal([]byte(raw), &v)
		a.VehicleName = v.Name()
		list = append(list, a)
	}
	return list, rows.Err()
}
func manageAppointments(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	d, ok := operationsData(db, w, r, "appointments", "Visits worth looking forward to.", "Publish test-drive and inspection slots, confirm bookings and keep customers informed.")
	if !ok {
		return
	}
	if r.Method == "POST" {
		if err := saveAppointmentAdmin(db, r, d.Staff); err != nil {
			d.Error = err.Error()
		} else {
			http.Redirect(w, r, "/admin/appointments?saved=1", 303)
			return
		}
	} else if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d.Fleet, _ = fleetList(db, false)
	d.Slots, _ = loadVisitSlots(db, false)
	d.Appointments, _ = loadAppointments(db, 0)
	renderAdmin(w, "admin-appointments.html", d)
}
func saveAppointmentAdmin(db *sql.DB, r *http.Request, staff Staff) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	switch r.FormValue("action") {
	case "slot":
		kind := r.FormValue("kind")
		location := strings.TrimSpace(r.FormValue("location"))
		vehicle := r.FormValue("vehicle")
		start, e1 := time.ParseInLocation("2006-01-02T15:04", r.FormValue("start"), dealershipTime())
		end, e2 := time.ParseInLocation("2006-01-02T15:04", r.FormValue("end"), dealershipTime())
		if !oneOf(kind, "test-drive", "inspection") || len(location) < 3 || len(location) > 200 || e1 != nil || e2 != nil || !start.After(time.Now()) || !end.After(start) || end.Sub(start) > 8*time.Hour {
			return errors.New("Choose a future appointment lasting up to eight hours, a location and a visit type. Times use Nigeria time (WAT).")
		}
		if vehicle != "" {
			var exists bool
			if e := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM fleet_vehicles WHERE id=? AND status!='Draft' AND status!='Sold')", vehicle).Scan(&exists); e != nil || !exists {
				return errors.New("Choose an available showroom vehicle.")
			}
		}
		// One staff team cannot host overlapping slots at the same location.
		var overlaps bool
		if e := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM visit_slots WHERE active=1 AND lower(location)=lower(?) AND starts_at<? AND ends_at>?)", location, end.UTC().Format(time.RFC3339), start.UTC().Format(time.RFC3339)).Scan(&overlaps); e != nil {
			return e
		}
		if overlaps {
			return errors.New("This location already has an overlapping appointment slot.")
		}
		_, err = tx.Exec("INSERT INTO visit_slots(vehicle_id,kind,starts_at,ends_at,location) VALUES(?,?,?,?,?)", vehicle, kind, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), location)
	case "status":
		id, e := requestID(r, "id")
		status := r.FormValue("status")
		if e != nil || !oneOf(status, "confirmed", "completed", "cancelled") {
			return errors.New("Choose a valid appointment update.")
		}
		var customer int64
		var previous string
		if e = tx.QueryRow("SELECT customer_id,status FROM appointments WHERE id=?", id).Scan(&customer, &previous); e != nil {
			return errors.New("Appointment not found.")
		}
		if previous == "cancelled" || previous == "completed" {
			return errors.New("This appointment is already closed.")
		}
		_, err = tx.Exec("UPDATE appointments SET status=? WHERE id=?", status, id)
		if err == nil {
			_, err = tx.Exec("INSERT OR IGNORE INTO appointment_notifications(customer_id,appointment_id,kind,message) VALUES(?,?,?,?)", customer, id, status, fmt.Sprintf("Visit MM-V%06d is now %s. Open your visits page for details.", id, status))
		}
	case "close-slot":
		id, e := requestID(r, "id")
		if e != nil {
			return e
		}
		var booked bool
		tx.QueryRow("SELECT EXISTS(SELECT 1 FROM appointments WHERE slot_id=? AND status!='cancelled')", id).Scan(&booked)
		if booked {
			return errors.New("Cancel or complete the booked appointment before closing this slot.")
		}
		_, err = tx.Exec("UPDATE visit_slots SET active=0 WHERE id=?", id)
	default:
		return errors.New("Unknown appointment action.")
	}
	if err != nil {
		return err
	}
	if err = addActivity(tx, staff, "Appointment "+r.FormValue("action"), r.FormValue("id")); err != nil {
		return err
	}
	return tx.Commit()
}
func customerAppointments(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	d := publicCommerceData(db, r, "appointments")
	customer, authErr := serviceCustomer(db, r)
	if r.Method == "POST" {
		if authErr != nil {
			http.Redirect(w, r, "/login.html", 303)
			return
		}
		var err error
		if r.FormValue("action") == "cancel" {
			id, e := requestID(r, "id")
			if e != nil {
				err = e
			} else {
				result, e := db.Exec("UPDATE appointments SET status='cancelled' WHERE id=? AND customer_id=? AND status IN ('requested','confirmed')", id, customer.ID)
				err = e
				if err == nil {
					n, _ := result.RowsAffected()
					if n != 1 {
						err = errors.New("This booking cannot be cancelled.")
					}
				}
			}
		} else {
			err = bookVisit(db, r, customer)
		}
		if err != nil {
			d.Error = err.Error()
		} else {
			http.Redirect(w, r, "/appointments?saved=1", 303)
			return
		}
	} else if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d.Slots, _ = loadVisitSlots(db, true)
	if authErr == nil {
		d.Appointments, _ = loadAppointments(db, customer.ID)
		for _, a := range d.Appointments {
			t, e := time.Parse(time.RFC3339, a.StartsAt)
			if e == nil && a.CanCancel() && t.After(time.Now()) && t.Before(time.Now().Add(24*time.Hour)) {
				db.Exec("INSERT OR IGNORE INTO appointment_notifications(customer_id,appointment_id,kind,message) VALUES(?,?,'reminder',?)", customer.ID, a.ID, "Reminder: "+a.Reference()+" is within the next 24 hours, "+a.TimeLabel()+" at "+a.Location)
			}
		}
		rows, e := db.Query("SELECT message FROM appointment_notifications WHERE customer_id=? ORDER BY id DESC LIMIT 8", customer.ID)
		if e == nil {
			for rows.Next() {
				var message string
				rows.Scan(&message)
				d.Notifications = append(d.Notifications, message)
			}
			rows.Close()
		}
	}
	if r.URL.Query().Get("saved") == "1" {
		d.Notice = "Your appointment update has been saved. Add confirmed visits to your calendar for reminders."
	}
	renderBusinessPage(w, 200, "appointments.html", d)
}
func bookVisit(db *sql.DB, r *http.Request, c ServiceCustomer) error {
	slot, e := requestID(r, "slot")
	if e != nil {
		return errors.New("Choose an available visit slot.")
	}
	vehicle := r.FormValue("vehicle")
	note := strings.TrimSpace(r.FormValue("note"))
	if len(note) > 2000 {
		return errors.New("Keep your visit notes under 2,000 characters.")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var allowed, kind, start, location string
	if err = tx.QueryRow("SELECT vehicle_id,kind,starts_at,location FROM visit_slots WHERE id=? AND active=1 AND starts_at>?", slot, time.Now().UTC().Format(time.RFC3339)).Scan(&allowed, &kind, &start, &location); err != nil {
		return errors.New("This appointment slot is no longer available.")
	}
	if allowed != "" && allowed != vehicle {
		return errors.New("This slot is for a different vehicle.")
	}
	var raw string
	if err = tx.QueryRow("SELECT payload FROM fleet_vehicles WHERE id=? AND status='Available'", vehicle).Scan(&raw); err != nil {
		return errors.New("Choose an available vehicle.")
	}
	var v Vehicle
	json.Unmarshal([]byte(raw), &v)
	result, err := tx.Exec("INSERT INTO appointments(slot_id,customer_id,vehicle_id,note) VALUES(?,?,?,?)", slot, c.ID, vehicle, note)
	if err != nil {
		return errors.New("Someone has just booked this slot. Please select another time.")
	}
	id, _ := result.LastInsertId()
	var phone string
	tx.QueryRow("SELECT phone FROM customers WHERE id=?", c.ID).Scan(&phone)
	result, err = tx.Exec("INSERT INTO enquiries(customer_id,kind,name,email,phone,vehicle,message) VALUES(?,?,?,?,?,?,?)", c.ID, kind, c.Name, c.Email, phone, v.Name(), "Appointment requested for "+start+" at "+location+". "+note)
	if err != nil {
		return err
	}
	lead, _ := result.LastInsertId()
	if _, err = tx.Exec("INSERT INTO sales_followups(enquiry_id,stage,followup,note) VALUES(?,'Appointment',?,?)", lead, start[:10], "Visit "+fmt.Sprint(id)); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO appointment_notifications(customer_id,appointment_id,kind,message) VALUES(?,?,'requested',?)", c.ID, id, fmt.Sprintf("Visit MM-V%06d requested. Our team will confirm the appointment here.", id)); err != nil {
		return err
	}
	return tx.Commit()
}
func appointmentCalendar(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	c, err := serviceCustomer(db, r)
	if err != nil {
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	id, err := serviceQueryID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	list, err := loadAppointments(db, c.ID)
	if err != nil {
		http.Error(w, "Could not load appointment", 500)
		return
	}
	for _, a := range list {
		if a.ID != id {
			continue
		}
		if a.Status != "confirmed" {
			http.Error(w, "Calendar reminders are available after the team confirms your visit.", 409)
			return
		}
		start, _ := time.Parse(time.RFC3339, a.StartsAt)
		end, _ := time.Parse(time.RFC3339, a.EndsAt)
		escape := strings.NewReplacer("\\", "\\\\", "\r", "", "\n", "\\n", ",", "\\,", ";", "\\;")
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=visit-"+strconv.FormatInt(id, 10)+".ics")
		fmt.Fprintf(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Marvellous Motors//Visits//EN\r\nBEGIN:VEVENT\r\nUID:visit-%d@marvellous-motors\r\nDTSTAMP:%s\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:%s\r\nLOCATION:%s\r\nDESCRIPTION:%s\r\nBEGIN:VALARM\r\nTRIGGER:-PT1H\r\nACTION:DISPLAY\r\nDESCRIPTION:Marvellous Motors visit reminder\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", id, time.Now().UTC().Format("20060102T150405Z"), start.UTC().Format("20060102T150405Z"), end.UTC().Format("20060102T150405Z"), escape.Replace(a.Kind+": "+a.VehicleName), escape.Replace(a.Location), escape.Replace(a.Reference()+" · "+a.Note))
		return
	}
	http.NotFound(w, r)
}
