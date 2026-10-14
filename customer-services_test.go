package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func serviceFixture(t *testing.T) *sql.DB {
	db := adminFixture(t)
	_, err := db.Exec("INSERT INTO customers(id,name,email,phone,password) VALUES(1,'First Customer','first@example.test','000','unused'),(2,'Second Customer','second@example.test','000','unused'); INSERT INTO orders(id,customer_id,name,email,phone,address,city,state,car_name,car_price,status) VALUES(1,1,'First Customer','first@example.test','000','Test Road','Lagos','Lagos','2025 Toyota Camry Hybrid XSE (New)',48500000,'processing'),(2,2,'Second Customer','second@example.test','000','Other Road','Abuja','Abuja','2025 Toyota Camry Hybrid XSE (New)',48500000,'processing'); INSERT INTO delivery_details(order_id,partner,reference) VALUES(1,'Test Carrier','TEST-DISPATCH-1')")
	if err != nil {
		t.Fatal(err)
	}
	return db
}
func servicePost(t *testing.T, path string, fields url.Values, customer int64) *http.Request {
	r := adminPost(t, path, fields, 0)
	if customer > 0 {
		r.AddCookie(customerCookie(t, customer, ""))
	}
	return r
}
func customerMessage(t *testing.T, db *sql.DB, customer int64, message, nonce string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	customerSupportAPI(db, w, servicePost(t, "/api/support/send", url.Values{"message": {message}, "client-id": {nonce}}, customer), "send")
	return w
}
func supportState(t *testing.T, db *sql.DB, customer int64) SupportSnapshot {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/support", nil)
	r.AddCookie(customerCookie(t, customer, ""))
	w := httptest.NewRecorder()
	customerSupportAPI(db, w, r, "get")
	if w.Code != 200 {
		t.Fatalf("Cannot read support: %d %s", w.Code, w.Body.String())
	}
	var state SupportSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	return state
}
func TestSupportConversationsArePrivateAndMessageRetriesAreSafe(t *testing.T) {
	db := serviceFixture(t)
	for _, want := range []int{201, 200} {
		w := customerMessage(t, db, 1, "Help with my receipt", "customer-send-0001")
		if w.Code != want {
			t.Fatalf("Message retry: %d", w.Code)
		}
	}
	state := supportState(t, db, 1)
	if len(state.Messages) != 1 || state.Messages[0].SenderKind != "customer" {
		t.Fatal("Retry duplicated message or changed sender")
	}
	other := supportState(t, db, 2)
	if other.Conversation != nil || len(other.Messages) != 0 {
		t.Fatal("Another customer sees private conversation")
	}
	w := customerMessage(t, db, 1, "Changed message", "customer-send-0001")
	if w.Code != 409 {
		t.Fatal("A retry token can replace an existing message")
	}
	r := httptest.NewRequest("GET", "/api/support", nil)
	w = httptest.NewRecorder()
	customerSupportAPI(db, w, r, "get")
	if w.Code != 401 {
		t.Fatal("Anonymous user can read customer chat")
	}
}
func TestSupportCannotLinkAnotherCustomersOrderOrImpersonateStaff(t *testing.T) {
	db := serviceFixture(t)
	fields := url.Values{"message": {"Shipment question"}, "client-id": {"customer-send-0002"}, "order-id": {"2"}, "sender-kind": {"staff"}, "sender-id": {"1"}}
	w := httptest.NewRecorder()
	customerSupportAPI(db, w, servicePost(t, "/api/support/send", fields, 1), "send")
	if w.Code != 404 {
		t.Fatal("Customer can reference another customer's order")
	}
	fields.Set("order-id", "1")
	fields.Set("message", "<script>alert('test')</script>")
	w = httptest.NewRecorder()
	customerSupportAPI(db, w, servicePost(t, "/api/support/send", fields, 1), "send")
	if w.Code != 201 {
		t.Fatal("Valid owned message was not saved")
	}
	state := supportState(t, db, 1)
	if state.Messages[0].SenderKind != "customer" || state.Messages[0].OrderID != 1 || state.Messages[0].Body != fields.Get("message") {
		t.Fatal("Sender forged or message content lost")
	}
}
func TestSupportReadMarkersResolutionAndCustomerReopen(t *testing.T) {
	db := serviceFixture(t)
	customerMessage(t, db, 1, "Help please", "customer-send-0003")
	state := supportState(t, db, 1)
	id := state.Conversation.ID
	fields := url.Values{"conversation-id": {fmt.Sprint(id)}, "message": {"We can help you."}, "client-id": {"staff-reply-000001"}}
	w := httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/send", fields, 2), "send")
	if w.Code != 201 {
		t.Fatalf("Staff reply failed: %d", w.Code)
	}
	state = supportState(t, db, 1)
	if len(state.Messages) != 2 || state.Conversation.UnreadCustomer != 1 || state.Conversation.UnreadStaff != 1 {
		t.Fatal("Replies or unread counts incorrect")
	}
	last := state.Messages[1].ID
	w = httptest.NewRecorder()
	customerSupportAPI(db, w, servicePost(t, "/api/support/read", url.Values{"message-id": {fmt.Sprint(last + 100)}}, 1), "read")
	if w.Code != 400 {
		t.Fatal("Customer marks future messages as read")
	}
	w = httptest.NewRecorder()
	customerSupportAPI(db, w, servicePost(t, "/api/support/read", url.Values{"message-id": {fmt.Sprint(last)}}, 1), "read")
	if w.Code != 200 || supportState(t, db, 1).Conversation.UnreadCustomer != 0 {
		t.Fatal("Read receipt was not recorded")
	}
	status := url.Values{"conversation-id": {fmt.Sprint(id)}, "status": {"resolved"}}
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/status", status, 2), "status")
	if w.Code != 200 {
		t.Fatal("Cannot resolve conversation")
	}
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/send", fields, 2), "send")
	if w.Code != 200 {
		t.Fatal("Saved staff message cannot be retried after resolution")
	}
	fields.Set("client-id", "staff-reply-000002")
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/send", fields, 2), "send")
	if w.Code != 409 {
		t.Fatal("Resolved conversation accepts new staff reply")
	}
	w = customerMessage(t, db, 1, "I have a follow-up.", "customer-send-0004")
	if w.Code != 201 {
		t.Fatal("Customer cannot follow up")
	}
	state = supportState(t, db, 1)
	if state.Conversation.ID != id || state.Conversation.Status != "open" || len(state.Messages) != 3 {
		t.Fatal("Follow-up lost history or failed to reopen")
	}
}
func TestSupportPresenceAndAccessUseLiveStaffAuthority(t *testing.T) {
	db := serviceFixture(t)
	w := httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/presence", url.Values{"available": {"true"}}, 2), "presence")
	online, err := supportOnline(db)
	if w.Code != 200 || err != nil || !online {
		t.Fatal("Available staff not shown online")
	}
	db.Exec("UPDATE support_presence SET last_seen=?", time.Now().Add(-61*time.Second).Unix())
	online, _ = supportOnline(db)
	if online {
		t.Fatal("Expired presence still shows online")
	}
	db.Exec("UPDATE support_presence SET last_seen=?", time.Now().Unix())
	db.Exec("UPDATE admins SET status='disabled' WHERE id=2")
	online, _ = supportOnline(db)
	if online {
		t.Fatal("Disabled staff still shown available")
	}
	r := httptest.NewRequest("GET", "/api/admin/support", nil)
	r.AddCookie(staffCookie(t, 2))
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, r, "get")
	if w.Code != 401 {
		t.Fatal("Disabled staff session can read support inbox")
	}
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/presence", url.Values{"available": {"true"}}, 2), "presence")
	if w.Code != 401 {
		t.Fatal("Disabled staff can renew presence")
	}
}
func TestSupportHistoryPaginationAndSendRateLimit(t *testing.T) {
	db := serviceFixture(t)
	customerMessage(t, db, 1, "Start history", "customer-send-0005")
	id := supportState(t, db, 1).Conversation.ID
	for i := 0; i < 105; i++ {
		_, err := db.Exec("INSERT INTO support_messages(conversation_id,sender_kind,sender_id,sender_name,body,client_id,created_at) VALUES(?,'customer',1,'First Customer',?,?,?)", id, fmt.Sprintf("Historical message %d", i), fmt.Sprintf("history-message-%04d", i), time.Now().Add(-time.Hour).Unix())
		if err != nil {
			t.Fatal(err)
		}
	}
	state := supportState(t, db, 1)
	if len(state.Messages) != 100 || !state.HasOlder {
		t.Fatal("History is not bounded or pagination missing")
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/support?before=%d", state.Messages[0].ID), nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w := httptest.NewRecorder()
	customerSupportAPI(db, w, r, "get")
	var earlier SupportSnapshot
	json.Unmarshal(w.Body.Bytes(), &earlier)
	if w.Code != 200 || len(earlier.Messages) != 6 || earlier.HasOlder {
		t.Fatal("Earlier history lost or duplicates latest page")
	}
	db.Exec("UPDATE support_messages SET created_at=?", time.Now().Unix())
	w = customerMessage(t, db, 1, "Another send", "customer-send-0006")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("Message burst not limited")
	}
}
func TestSupportWritesRequireCSRFAndExistingCustomerAccount(t *testing.T) {
	db := serviceFixture(t)
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { customerSupportAPI(db, w, r, "send") }))
	r := servicePost(t, "/api/support/send", url.Values{"message": {"Hello"}, "client-id": {"customer-send-0007"}}, 1)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("Support send accepts missing CSRF")
	}
	db.Exec("DELETE FROM customers WHERE id=1")
	w = customerMessage(t, db, 1, "Hello", "customer-send-0008")
	if w.Code != 401 {
		t.Fatal("Deleted customer session still sends private messages")
	}
}
func TestShipmentIsPrivateAndRefreshesRecordedCheckpoints(t *testing.T) {
	db := serviceFixture(t)
	db.Exec("UPDATE orders SET status='out_for_delivery' WHERE id=1")
	fields := url.Values{"order-id": {"1"}, "stage": {"in_transit"}, "location": {"Ibadan distribution hub"}, "note": {"Vehicle is moving toward your delivery city."}}
	w := httptest.NewRecorder()
	recordShipmentCheckpoint(db, w, adminPost(t, "/admin/shipment/update", fields, 2))
	if w.Code != 303 {
		t.Fatalf("Checkpoint failed: %d", w.Code)
	}
	for _, customer := range []int64{0, 1, 2} {
		r := httptest.NewRequest("GET", "/api/shipment?order-id=1", nil)
		if customer > 0 {
			r.AddCookie(customerCookie(t, customer, ""))
		}
		w = httptest.NewRecorder()
		shipmentAPI(db, w, r)
		want := 401
		if customer == 1 {
			want = 200
		} else if customer == 2 {
			want = 404
		}
		if w.Code != want {
			t.Fatalf("Shipment privacy for %d: %d", customer, w.Code)
		}
		if customer == 1 {
			var s ShipmentSnapshot
			json.Unmarshal(w.Body.Bytes(), &s)
			if s.Stage != "in_transit" || s.Location != fields.Get("location") || s.Reference != "TEST-DISPATCH-1" || s.Progress != 2 {
				t.Fatal("Snapshot does not reflect confirmed checkpoint")
			}
		}
	}
	r := httptest.NewRequest("GET", "/shipment-tracking?order-id=1", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	serveShipmentPage(db, w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "data-shipment-tracker") {
		t.Fatal("Shipment page does not render")
	}
}
func TestShipmentCannotRegressOrChangeBeforeDispatchOrAfterHandover(t *testing.T) {
	db := serviceFixture(t)
	fields := url.Values{"order-id": {"1"}, "stage": {"in_transit"}, "location": {"Confirmed hub"}, "note": {"A confirmed customer update."}}
	w := httptest.NewRecorder()
	recordShipmentCheckpoint(db, w, adminPost(t, "/admin/shipment/update", fields, 2))
	if w.Code != 409 {
		t.Fatal("Undispatched vehicle accepts shipment update")
	}
	db.Exec("UPDATE orders SET status='out_for_delivery' WHERE id=1")
	fields.Set("stage", "out_for_delivery")
	w = httptest.NewRecorder()
	recordShipmentCheckpoint(db, w, adminPost(t, "/admin/shipment/update", fields, 2))
	if w.Code != 303 {
		t.Fatal("Final leg checkpoint fails")
	}
	fields.Set("stage", "in_transit")
	w = httptest.NewRecorder()
	recordShipmentCheckpoint(db, w, adminPost(t, "/admin/shipment/update", fields, 2))
	if w.Code != 409 {
		t.Fatal("Shipment can move backward")
	}
	fields.Set("stage", "delivery_exception")
	w = httptest.NewRecorder()
	recordShipmentCheckpoint(db, w, adminPost(t, "/admin/shipment/update", fields, 2))
	if w.Code != 303 {
		t.Fatal("Cannot report delivery issue")
	}
	orders, _ := loadOrders(db, "WHERE o.id=1")
	snapshot, err := loadShipment(db, orders[0])
	if err != nil || snapshot.Stage != "delivery_exception" || snapshot.Progress != 4 {
		t.Fatal("Exception loses confirmed progress")
	}
	db.Exec("UPDATE orders SET status='delivered' WHERE id=1")
	w = httptest.NewRecorder()
	recordShipmentCheckpoint(db, w, adminPost(t, "/admin/shipment/update", fields, 2))
	if w.Code != 409 {
		t.Fatal("Handover-complete shipment can be edited")
	}
	fields.Set("location", "")
	w = httptest.NewRecorder()
	recordShipmentCheckpoint(db, w, adminPost(t, "/admin/shipment/update", fields, 2))
	if w.Code != 400 {
		t.Fatal("Empty checkpoint is accepted")
	}
}
func TestShipmentHistoryIncludesActualDispatchAndHandover(t *testing.T) {
	db := serviceFixture(t)
	w := httptest.NewRecorder()
	transitionDelivery(db, w, adminPost(t, "/admin/orders/delivery", url.Values{"order-id": {"1"}}, 2), "out_for_delivery")
	if w.Code != 303 {
		t.Fatal("Could not dispatch fixture")
	}
	w = httptest.NewRecorder()
	transitionDelivery(db, w, adminPost(t, "/admin/orders/delivered", url.Values{"order-id": {"1"}}, 2), "delivered")
	if w.Code != 303 {
		t.Fatal("Could not hand over fixture")
	}
	orders, _ := loadOrders(db, "WHERE o.id=1")
	s, err := loadShipment(db, orders[0])
	if err != nil || s.Stage != "delivered" || s.Progress != 5 || len(s.Events) != 2 || s.Events[0].Stage != "delivered" || s.Events[1].Stage != "dispatched" {
		t.Fatalf("Actual lifecycle history missing: %+v %v", s, err)
	}
}
func TestLocalOwnerConfigurationStoresHashAndPreservesExistingAccess(t *testing.T) {
	db := adminFixture(t)
	path := filepath.Join(t.TempDir(), "owner.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte("FictionalTestPassword!26"), bcrypt.DefaultCost)
	raw, _ := json.Marshal(map[string]string{"name": "Configured Owner", "email": "configured@example.test", "password_hash": string(hash)})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := seedConfiguredLocalOwner(db, path); err != nil {
		t.Fatal(err)
	}
	var role, password string
	db.QueryRow("SELECT role,password FROM admins WHERE email='configured@example.test'").Scan(&role, &password)
	if role != "super_admin" || password != string(hash) {
		t.Fatal("Local owner not secured")
	}
	db.Exec("UPDATE admins SET status='disabled' WHERE email='configured@example.test'")
	if err := seedConfiguredLocalOwner(db, path); err != nil {
		t.Fatal(err)
	}
	var status string
	db.QueryRow("SELECT status FROM admins WHERE email='configured@example.test'").Scan(&status)
	if status != "disabled" {
		t.Fatal("Restart overrides owner access state")
	}
}

func TestDeploymentOwnerBootstrapWorksWithoutLocalConfiguration(t *testing.T) {
	db := adminFixture(t)
	if err := seedDeploymentOwner(db); err != nil {
		t.Fatal(err)
	}
	var role, status, hash string
	if err := db.QueryRow("SELECT role,status,password FROM admins WHERE email=?", deploymentOwnerEmail).Scan(&role, &status, &hash); err != nil {
		t.Fatal(err)
	}
	if role != "super_admin" || status != "approved" || hash != deploymentOwnerPasswordHash {
		t.Fatal("Deployment owner is missing or misconfigured")
	}
	if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost < bcrypt.DefaultCost {
		t.Fatal("Deployment credential is not a bcrypt hash")
	}
	db.Exec("UPDATE admins SET status='disabled' WHERE email=?", deploymentOwnerEmail)
	if err := seedDeploymentOwner(db); err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM admins WHERE email=?", deploymentOwnerEmail).Scan(&count)
	db.QueryRow("SELECT status FROM admins WHERE email=?", deploymentOwnerEmail).Scan(&status)
	if count != 1 || status != "disabled" {
		t.Fatal("Startup duplicates or resets existing owner access")
	}
}
