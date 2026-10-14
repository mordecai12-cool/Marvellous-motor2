package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func adminFixture(t *testing.T) *sql.DB {
	t.Helper()
	t.Setenv("MM_OWNER_ONLY", "false")
	db := testDatabase(t)
	_, err := db.Exec("INSERT INTO admins(id,name,email,password,role,status) VALUES(1,'Test Owner','owner@example.test','unused','super_admin','approved'),(2,'Test Staff','staff@example.test','unused','admin','approved'),(3,'Pending Staff','pending@example.test','unused','admin','pending')")
	if err != nil {
		t.Fatal(err)
	}
	return db
}
func staffCookie(t *testing.T, id int64) *http.Cookie {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	session, _ := store.Get(r, "admin-session")
	session.Values["admin_id"] = id
	session.Values["admin_role"] = "super_admin"
	if err := session.Save(r, w); err != nil {
		t.Fatal(err)
	}
	return w.Result().Cookies()[0]
}
func adminPost(t *testing.T, path string, values url.Values, id int64) *http.Request {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if id > 0 {
		r.AddCookie(staffCookie(t, id))
	}
	return r
}
func TestAdminAccessRechecksAccountStatusAndRole(t *testing.T) {
	db := adminFixture(t)
	handler := adminAccessMiddleware(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tc := range []struct {
		id   int64
		want int
	}{{0, 303}, {1, 204}, {2, 204}, {3, 303}} {
		r := httptest.NewRequest("GET", "/admin/orders", nil)
		if tc.id > 0 {
			r.AddCookie(staffCookie(t, tc.id))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("staff %d: got %d, want %d", tc.id, w.Code, tc.want)
		}
	}
	r := httptest.NewRequest("GET", "/admin/orders", nil)
	r.AddCookie(staffCookie(t, 2))
	s, err := currentStaff(db, r)
	if err != nil || s.Role != "admin" {
		t.Fatal("Cookie role must not override database role")
	}
	if _, err = db.Exec("UPDATE admins SET status='disabled' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatal("Disabled staff session remains usable")
	}
	for _, path := range []string{"/admin-request.html", "/admin-request", "/admin-request-status"} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 403 {
			t.Fatalf("Public admin access route remains open: %s", path)
		}
	}
}
func TestStaffCreationIsOwnerOnly(t *testing.T) {
	db := adminFixture(t)
	fields := url.Values{"name": {"New Staff"}, "email": {"new@example.test"}, "password": {"TestOnlyStrongPassword!26"}}
	w := httptest.NewRecorder()
	manageTeam(db, w, adminPost(t, "/admin/team/create", fields, 2), "create")
	if w.Code != 403 {
		t.Fatal("Ordinary staff can create administrators")
	}
	w = httptest.NewRecorder()
	manageTeam(db, w, adminPost(t, "/admin/team/create", fields, 1), "create")
	if w.Code != 303 {
		t.Fatalf("Owner cannot create staff: %d %s", w.Code, w.Body.String())
	}
	var role, status, password string
	if err := db.QueryRow("SELECT role,status,password FROM admins WHERE email='new@example.test'").Scan(&role, &status, &password); err != nil {
		t.Fatal(err)
	}
	if role != "admin" || status != "approved" || password == fields.Get("password") {
		t.Fatal("New staff account is not correctly secured")
	}
	w = httptest.NewRecorder()
	manageTeam(db, w, adminPost(t, "/admin/team/update", url.Values{"admin-id": {"1"}, "status": {"disabled"}}, 1), "update")
	if w.Code != 403 {
		t.Fatal("Owner can disable their own owner account")
	}
}
func TestPaymentReviewIsAtomicAndCannotBeReplayed(t *testing.T) {
	db := adminFixture(t)
	_, err := db.Exec("INSERT INTO orders(id,customer_id,status,car_price) VALUES(1,1,'payment_pending',25000000),(2,1,'payment_pending',25000000);INSERT INTO payments(id,order_id,amount_paid,payment_status) VALUES(1,1,25000000,'pending'),(2,2,100,'pending')")
	if err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"payment-id": {"1"}}
	w := httptest.NewRecorder()
	reviewPayment(db, w, adminPost(t, "/admin/payments/verify", fields, 1), "verified")
	if w.Code != 303 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var state string
	if err = db.QueryRow("SELECT status FROM orders WHERE id=1").Scan(&state); err != nil || state != "processing" {
		t.Fatal("Approved order did not move to preparation")
	}
	w = httptest.NewRecorder()
	reviewPayment(db, w, adminPost(t, "/admin/payments/verify", fields, 1), "verified")
	if w.Code != 409 {
		t.Fatal("Payment approval can be replayed")
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM order_events WHERE order_id=1").Scan(&count)
	if count != 1 {
		t.Fatal("Repeated approval created duplicate events")
	}
	fields.Set("payment-id", "2")
	w = httptest.NewRecorder()
	reviewPayment(db, w, adminPost(t, "/admin/payments/verify", fields, 1), "verified")
	if w.Code != 409 {
		t.Fatal("Underpayment approved")
	}
	db.QueryRow("SELECT status FROM orders WHERE id=2").Scan(&state)
	if state != "payment_pending" {
		t.Fatal("Rejected review altered order state")
	}
}
func TestDeliverySchedulingDispatchAndHandover(t *testing.T) {
	db := adminFixture(t)
	_, err := db.Exec("INSERT INTO orders(id,customer_id,status,car_price) VALUES(1,1,'processing',25000000)")
	if err != nil {
		t.Fatal(err)
	}
	id := url.Values{"order-id": {"1"}}
	w := httptest.NewRecorder()
	transitionDelivery(db, w, adminPost(t, "/admin/orders/delivery", id, 1), "out_for_delivery")
	if w.Code != 409 {
		t.Fatal("Dispatch allowed without a partner and reference")
	}
	details := url.Values{"order-id": {"1"}, "scheduled-date": {"2026-10-20"}, "partner": {"Test Logistics"}, "driver": {"Test Driver"}, "driver-phone": {"123456789"}, "reference": {"TEST-DISPATCH-001"}, "note": {"Your fictional delivery is scheduled."}}
	w = httptest.NewRecorder()
	saveDelivery(db, w, adminPost(t, "/admin/delivery/save", details, 1))
	if w.Code != 303 {
		t.Fatalf("Scheduling failed: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	transitionDelivery(db, w, adminPost(t, "/admin/orders/delivery", id, 1), "out_for_delivery")
	if w.Code != 303 {
		t.Fatal("Dispatch failed")
	}
	w = httptest.NewRecorder()
	transitionDelivery(db, w, adminPost(t, "/admin/orders/delivered", id, 1), "delivered")
	if w.Code != 303 {
		t.Fatal("Handover failed")
	}
	w = httptest.NewRecorder()
	transitionDelivery(db, w, adminPost(t, "/admin/orders/delivered", id, 1), "delivered")
	if w.Code != 409 {
		t.Fatal("Handover can be repeated")
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM order_events WHERE order_id=1").Scan(&count)
	if count != 3 {
		t.Fatalf("Expected 3 recorded customer updates, got %d", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM admin_activity").Scan(&count)
	if count != 3 {
		t.Fatal("Delivery staff actions not logged")
	}
	w = httptest.NewRecorder()
	saveDelivery(db, w, adminPost(t, "/admin/delivery/save", details, 1))
	if w.Code != 409 {
		t.Fatal("Delivered arrangements can be altered")
	}
}
func TestCustomerDeliveryIsPrivateAndReviewStartsIncomplete(t *testing.T) {
	db := adminFixture(t)
	_, err := db.Exec("INSERT INTO orders(id,customer_id,name,email,phone,address,city,state,car_name,car_price,status) VALUES(1,1,'Test Customer','test@example.test','12345','Test Road','Lagos','Lagos','2025 Toyota Camry Hybrid XSE (New)',48500000,'payment_pending')")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/order-status?order-id=1", nil)
	r.AddCookie(customerCookie(t, 2, ""))
	w := httptest.NewRecorder()
	serveCustomerDelivery(db, w, r, false)
	if w.Code != 404 {
		t.Fatal("Customer can view someone else's delivery")
	}
	r = httptest.NewRequest("GET", "/order-status?order-id=1", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	serveCustomerDelivery(db, w, r, false)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "progress-step current") || strings.Contains(w.Body.String(), "Evidence reviewed") {
		t.Fatal("Pending payment incorrectly appears reviewed")
	}
}
func TestAdminScreensRenderAndTeamIsRestricted(t *testing.T) {
	db := adminFixture(t)
	for _, page := range []string{"overview", "orders", "deliveries", "payments", "customers", "team", "activity"} {
		r := httptest.NewRequest("GET", "/admin/"+page, nil)
		r.AddCookie(staffCookie(t, 1))
		w := httptest.NewRecorder()
		serveAdminPage(db, w, r, page)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "ops-sidebar") {
			t.Fatalf("Screen %s: %d %s", page, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "/admin/team", nil)
	r.AddCookie(staffCookie(t, 2))
	w := httptest.NewRecorder()
	serveAdminPage(db, w, r, "team")
	if w.Code != 403 {
		t.Fatal("Staff can access owner staff management")
	}
}

func TestPaymentEvidenceCanResumeOnlyForOwningPendingOrder(t *testing.T) {
	db := adminFixture(t)
	_, err := db.Exec("INSERT INTO orders(id,customer_id,name,email,phone,address,city,state,car_name,car_price,status) VALUES(1,1,'Test Customer','test@example.test','12345','Test Road','Lagos','Lagos','2025 Toyota Camry Hybrid XSE (New)',48500000,'payment_pending')")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/order-payment?order-id=1", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w := httptest.NewRecorder()
	serveCustomerPayment(db, w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "₦48,500,000") {
		t.Fatalf("Cannot resume evidence submission: %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/order-payment?order-id=1", nil)
	r.AddCookie(customerCookie(t, 2, ""))
	w = httptest.NewRecorder()
	serveCustomerPayment(db, w, r)
	if w.Code != 404 {
		t.Fatal("Other customer can submit against this order")
	}
	db.Exec("UPDATE orders SET status='delivered' WHERE id=1")
	r = httptest.NewRequest("GET", "/order-payment?order-id=1", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	serveCustomerPayment(db, w, r)
	if w.Code != 303 {
		t.Fatal("Completed order accepts another receipt")
	}
}
