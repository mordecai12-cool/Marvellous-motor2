package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type ShipmentEvent struct {
	ID        int64  `json:"id"`
	Stage     string `json:"stage"`
	Location  string `json:"location"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

func shipmentLabel(stage string) string {
	labels := map[string]string{"payment_pending": "Awaiting payment review", "processing": "Preparing your vehicle", "dispatched": "Shipment dispatched", "in_transit": "In transit", "arrived_at_hub": "At a delivery hub", "out_for_delivery": "On the final delivery leg", "delivery_exception": "Delivery update needed", "delivered": "Delivered & handed over"}
	return labels[stage]
}
func shipmentRank(stage string) int {
	return map[string]int{"in_transit": 1, "arrived_at_hub": 2, "out_for_delivery": 3, "delivered": 4}[stage]
}
func (e ShipmentEvent) Label() string     { return shipmentLabel(e.Stage) }
func (e ShipmentEvent) TimeLabel() string { return displayTime(e.CreatedAt) }

type ShipmentSnapshot struct {
	EstimatedArrival string          `json:"estimated_arrival"`
	TimeRemaining    string          `json:"time_remaining"`
	OrderID          int64           `json:"order_id"`
	Stage            string          `json:"stage"`
	Label            string          `json:"label"`
	Carrier          string          `json:"carrier"`
	Reference        string          `json:"reference"`
	PlannedDate      string          `json:"planned_date"`
	Location         string          `json:"location"`
	LastReported     string          `json:"last_reported"`
	Driver           string          `json:"driver"`
	Events           []ShipmentEvent `json:"events"`
	Progress         int             `json:"progress"`
	CheckedAt        string          `json:"checked_at"`
}

func loadShipment(db *sql.DB, order ManagedOrder) (ShipmentSnapshot, error) {
	s := ShipmentSnapshot{EstimatedArrival: order.ArrivalLabel(), TimeRemaining: order.TimeRemaining(), OrderID: order.ID, Stage: order.Status, Carrier: order.Delivery.Partner, Reference: order.Delivery.Reference, PlannedDate: order.ScheduledLabel(), Driver: order.Delivery.Driver, Events: []ShipmentEvent{}, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	rows, err := db.Query("SELECT id,stage,location,note,CAST(created_at AS TEXT) FROM shipment_events WHERE order_id=? ORDER BY id DESC LIMIT 100", order.ID)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var e ShipmentEvent
		if err = rows.Scan(&e.ID, &e.Stage, &e.Location, &e.Note, &e.CreatedAt); err != nil {
			return s, err
		}
		s.Events = append(s.Events, e)
	}
	if err = rows.Err(); err != nil {
		return s, err
	}
	rows.Close()
	var highest int
	err = db.QueryRow("SELECT COALESCE(MAX(CASE stage WHEN 'in_transit' THEN 1 WHEN 'arrived_at_hub' THEN 2 WHEN 'out_for_delivery' THEN 3 ELSE 0 END),0) FROM shipment_events WHERE order_id=?", order.ID).Scan(&highest)
	if err != nil {
		return s, err
	}
	if order.Status == "out_for_delivery" {
		s.Stage = "dispatched"
		s.Progress = highest + 1
		if len(s.Events) > 0 {
			s.Stage = s.Events[0].Stage
		}
		for _, e := range s.Events {
			if p := shipmentRank(e.Stage) + 1; e.Stage != "delivery_exception" && p > s.Progress {
				s.Progress = p
			}
		}
	}
	if order.Status == "delivered" {
		s.Progress = 5
	}
	if len(s.Events) > 0 {
		s.Location = s.Events[0].Location
		s.LastReported = s.Events[0].TimeLabel()
	}
	// Include the real dispatch and handover records alongside transit checkpoints.
	orderEvents, err := loadOrderEvents(db, order.ID)
	if err != nil {
		return s, err
	}
	for _, event := range orderEvents {
		if event.FromStatus == event.ToStatus {
			continue
		}
		stage := ""
		if event.ToStatus == "out_for_delivery" {
			stage = "dispatched"
		}
		if event.ToStatus == "delivered" {
			stage = "delivered"
		}
		if stage != "" {
			s.Events = append(s.Events, ShipmentEvent{ID: -event.ID, Stage: stage, Note: event.Note, CreatedAt: event.CreatedAt})
		}
	}
	sort.SliceStable(s.Events, func(i, j int) bool {
		a, b := s.Events[i], s.Events[j]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		if a.Stage == "delivered" || b.Stage == "delivered" {
			return a.Stage == "delivered" && b.Stage != "delivered"
		}
		return a.ID > b.ID
	})
	if len(s.Events) > 100 {
		s.Events = s.Events[:100]
	}
	s.Label = shipmentLabel(s.Stage)
	return s, nil
}
func ownedShipmentOrder(db *sql.DB, w http.ResponseWriter, r *http.Request, api bool) (ManagedOrder, bool) {
	c, err := serviceCustomer(db, r)
	if err != nil {
		if api {
			serviceError(w, 401, "Sign in to track your shipment.")
		} else {
			http.Redirect(w, r, "/login.html", 303)
		}
		return ManagedOrder{}, false
	}
	id, err := serviceQueryID(r, "order-id")
	if err != nil || id < 1 {
		if api {
			serviceError(w, 404, "Shipment not found.")
		} else {
			http.NotFound(w, r)
		}
		return ManagedOrder{}, false
	}
	orders, err := loadOrders(db, "WHERE o.id=? AND o.customer_id=?", id, c.ID)
	if err != nil {
		if api {
			serviceError(w, 500, "Could not load shipment.")
		} else {
			http.Error(w, "Could not load shipment", 500)
		}
		return ManagedOrder{}, false
	}
	if len(orders) == 0 {
		if api {
			serviceError(w, 404, "Shipment not found.")
		} else {
			http.NotFound(w, r)
		}
		return ManagedOrder{}, false
	}
	return orders[0], true
}
func serveShipmentPage(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	order, ok := ownedShipmentOrder(db, w, r, false)
	if !ok {
		return
	}
	s, err := loadShipment(db, order)
	if err != nil {
		http.Error(w, "Could not load shipment", 500)
		return
	}
	renderServicePage(w, "shipment-tracking.html", struct {
		ShowroomData
		Order    ManagedOrder
		Shipment ShipmentSnapshot
	}{ShowroomData{SignedIn: true}, order, s})
}
func shipmentAPI(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		serviceError(w, 405, "Method not allowed.")
		return
	}
	order, ok := ownedShipmentOrder(db, w, r, true)
	if !ok {
		return
	}
	s, err := loadShipment(db, order)
	if err != nil {
		serviceError(w, 500, "Could not load shipment.")
		return
	}
	serviceJSON(w, 200, s)
}
func recordShipmentCheckpoint(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return
	}
	id, err := requestID(r, "order-id")
	stage := r.FormValue("stage")
	location := strings.TrimSpace(r.FormValue("location"))
	note := strings.TrimSpace(r.FormValue("note"))
	if err != nil || (stage != "in_transit" && stage != "arrived_at_hub" && stage != "out_for_delivery" && stage != "delivery_exception") || len(location) == 0 || utf8.RuneCountInString(location) > 160 || len(note) == 0 || utf8.RuneCountInString(note) > 1000 || !utf8.ValidString(location+note) {
		http.Error(w, "Choose a shipment stage, location and a short customer update", 400)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not update shipment", 500)
		return
	}
	defer tx.Rollback()
	var state string
	if err = tx.QueryRow("SELECT status FROM orders WHERE id=?", id).Scan(&state); err != nil {
		http.NotFound(w, r)
		return
	}
	if state != "out_for_delivery" {
		http.Error(w, "Shipment checkpoints require a dispatched order; completed deliveries cannot be changed", 409)
		return
	}
	var highest int
	err = tx.QueryRow("SELECT COALESCE(MAX(CASE stage WHEN 'in_transit' THEN 1 WHEN 'arrived_at_hub' THEN 2 WHEN 'out_for_delivery' THEN 3 ELSE 0 END),0) FROM shipment_events WHERE order_id=?", id).Scan(&highest)
	if err != nil {
		http.Error(w, "Could not check shipment stage", 500)
		return
	}
	if stage != "delivery_exception" && shipmentRank(stage) < highest {
		http.Error(w, "A shipment cannot move back to an earlier stage", 409)
		return
	}
	if _, err = tx.Exec("INSERT INTO shipment_events(order_id,stage,location,note,actor_id,actor_name) VALUES(?,?,?,?,?,?)", id, stage, location, note, staff.ID, staff.Name); err != nil {
		http.Error(w, "Could not record shipment", 500)
		return
	}
	if err = addActivity(tx, staff, "Shipment checkpoint: "+shipmentLabel(stage), fmt.Sprintf("Order #%d · %s", id, location)); err != nil {
		http.Error(w, "Could not record shipment activity", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save shipment", 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/order?id=%d&notice=checkpoint", id), 303)
}
