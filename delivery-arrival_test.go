package main

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestArrivalRemainingStates(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, dealershipTime())
	for _, tc := range []struct{ date, arrival, status, want string }{
		{"", "", "processing", "Arrival estimate not set yet"},
		{"2026-10-27", "", "processing", "Expected today"},
		{"2026-10-28", "", "processing", "Expected tomorrow"},
		{"2026-10-30", "", "processing", "Expected in 3 days"},
		{"2026-10-26", "", "processing", "Awaiting an updated arrival estimate"},
		{"", now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339), "out_for_delivery", "About 2 hours, 30 minutes remaining"},
		{"", now.Add(-time.Minute).Format(time.RFC3339), "out_for_delivery", "Awaiting an updated arrival estimate"},
		{"2026-10-30", "", "delivered", "Your vehicle has arrived"},
	} {
		o := ManagedOrder{Status: tc.status, Delivery: DeliveryDetails{ScheduledDate: tc.date, ArrivalAt: tc.arrival}}
		if got := o.arrivalRemaining(now); got != tc.want {
			t.Errorf("%+v: got %q", tc, got)
		}
	}
}

func TestAdminArrivalSavedAndVisibleOnlyToOrderOwner(t *testing.T) {
	db := serviceFixture(t)
	fields := url.Values{"order-id": {"1"}, "scheduled-date": {"2027-01-20"}, "arrival-time": {"15:30"}, "partner": {"Test Carrier"}, "reference": {"TEST-ETA"}}
	w := httptest.NewRecorder()
	saveDelivery(db, w, adminPost(t, "/admin/delivery/save", fields, 0))
	if w.Code != 403 {
		t.Fatal("Anonymous user set arrival", w.Code)
	}
	w = httptest.NewRecorder()
	r := servicePost(t, "/admin/delivery/save", fields, 1)
	saveDelivery(db, w, r)
	if w.Code != 403 {
		t.Fatal("Customer set arrival", w.Code)
	}
	w = httptest.NewRecorder()
	saveDelivery(db, w, adminPost(t, "/admin/delivery/save", fields, 2))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	var saved string
	if err := db.QueryRow("SELECT arrival_at FROM delivery_arrivals WHERE order_id=1").Scan(&saved); err != nil || saved != "2027-01-20T14:30:00Z" {
		t.Fatal("Nigeria timezone was not saved correctly", saved, err)
	}
	if err := initAdminPlatform(db); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("GET", "/api/shipment?order-id=1", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	shipmentAPI(db, w, r)
	var shipment ShipmentSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &shipment); err != nil || w.Code != 200 || !strings.Contains(shipment.EstimatedArrival, "3:30 PM WAT") || !strings.Contains(shipment.TimeRemaining, "remaining") {
		t.Fatal(w.Code, shipment, err)
	}
	r = httptest.NewRequest("GET", "/api/shipment?order-id=1", nil)
	r.AddCookie(customerCookie(t, 2, ""))
	w = httptest.NewRecorder()
	shipmentAPI(db, w, r)
	if w.Code != 404 {
		t.Fatal("Another customer read arrival details", w.Code)
	}
	fields.Set("arrival-time", "bad")
	w = httptest.NewRecorder()
	saveDelivery(db, w, adminPost(t, "/admin/delivery/save", fields, 2))
	if w.Code != 400 {
		t.Fatal("Invalid arrival accepted", w.Code)
	}
	var unchanged string
	db.QueryRow("SELECT arrival_at FROM delivery_arrivals WHERE order_id=1").Scan(&unchanged)
	if unchanged != saved {
		t.Fatal("Invalid arrival changed saved estimate")
	}
	fields.Set("scheduled-date", "")
	fields.Set("arrival-time", "15:00")
	w = httptest.NewRecorder()
	saveDelivery(db, w, adminPost(t, "/admin/delivery/save", fields, 2))
	if w.Code != 400 {
		t.Fatal("Time without a date accepted", w.Code)
	}
	fields.Set("arrival-time", "")
	w = httptest.NewRecorder()
	saveDelivery(db, w, adminPost(t, "/admin/delivery/save", fields, 2))
	if w.Code != 303 {
		t.Fatal("Admin could not clear estimate", w.Code)
	}
	orders, _ := loadOrders(db, "WHERE o.id=1")
	if orders[0].TimeRemaining() != "Arrival estimate not set yet" {
		t.Fatal("Cleared estimate still displayed")
	}
}
