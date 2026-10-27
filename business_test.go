package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDuplicateRegistrationRetainsFormAndContactDetails(t *testing.T) {
	db := serviceFixture(t)
	fields := url.Values{"name": {"Existing Buyer"}, "email": {" FIRST@EXAMPLE.TEST "}, "phone": {"08012345678"}, "password": {"Registration123"}, "confirm-password": {"Registration123"}}
	for _, email := range []string{" FIRST@EXAMPLE.TEST ", "OWNER@EXAMPLE.TEST"} {
		fields.Set("email", email)
		w := httptest.NewRecorder()
		handleRegister(db, w, adminPost(t, "/register", fields, 0))
		if w.Code != 409 || !strings.Contains(w.Body.String(), "details already exist") || !strings.Contains(w.Body.String(), `action="/register"`) || !strings.Contains(w.Body.String(), `value="Existing Buyer"`) {
			t.Fatal("Duplicate registration lost the form", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), `value="Registration123"`) {
			t.Fatal("Password was echoed back")
		}
	}
	fields.Set("email", "new-buyer@example.test")
	w := httptest.NewRecorder()
	handleRegister(db, w, adminPost(t, "/register", fields, 0))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	fields.Set("email", " NEW-BUYER@EXAMPLE.TEST ")
	w = httptest.NewRecorder()
	handleRegister(db, w, adminPost(t, "/register", fields, 0))
	if w.Code != 409 {
		t.Fatal("Case variant created another account")
	}
}
func TestNewAdminScreensRequireStaffAndRender(t *testing.T) {
	db := adminFixture(t)
	handlers := map[string]func(http.ResponseWriter, *http.Request){"inventory": func(w http.ResponseWriter, r *http.Request) { manageInventory(db, w, r) }, "pricing": func(w http.ResponseWriter, r *http.Request) { managePricing(db, w, r) }, "appointments": func(w http.ResponseWriter, r *http.Request) { manageAppointments(db, w, r) }, "sales": func(w http.ResponseWriter, r *http.Request) { manageSales(db, w, r) }, "dealership": func(w http.ResponseWriter, r *http.Request) { manageDealership(db, w, r) }}
	for name, handler := range handlers {
		r := httptest.NewRequest("GET", "/admin/"+name, nil)
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 303 {
			t.Fatal("Anonymous workspace access", name, w.Code)
		}
		r = httptest.NewRequest("GET", "/admin/"+name, nil)
		r.AddCookie(staffCookie(t, 2))
		w = httptest.NewRecorder()
		handler(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "ops-sidebar") {
			t.Fatal(name, w.Code, w.Body.String())
		}
	}
}
func testFleetFields() url.Values {
	return url.Values{"brand": {"Test Motors"}, "model": {"Touring"}, "trim": {"Premium"}, "year": {"2024"}, "price": {"10000000"}, "mileage": {"30000"}, "stock_status": {"Draft"}, "condition": {"Used"}, "body": {"Sedan"}, "fuel": {"Petrol"}, "engine_layout": {"V6"}, "engine": {"3.5L V6"}, "transmission": {"Automatic"}, "drive": {"FWD"}, "location": {"Lagos, Nigeria"}, "origin": {"Foreign used"}, "description": {"A real stock record for isolated testing."}, "previous_owners": {"1"}, "categories": {"Luxury"}, "condition_report": {"Small scratch disclosed on rear bumper"}, "service_history": {"Service invoices available"}, "documents": {"Ownership papers available"}, "offer_name": {"Test savings"}, "offer_discount": {"1000000"}}
}
func fleetMultipart(t *testing.T, fields url.Values, files map[string]string) *http.Request {
	t.Helper()
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	for key, values := range fields {
		for _, v := range values {
			m.WriteField(key, v)
		}
	}
	for field, name := range files {
		p, err := m.CreateFormFile(field, name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, ".png") {
			p.Write(complaintPNG(t))
		} else {
			p.Write([]byte("%PDF-1.4\nTest inspection report\n%%EOF"))
		}
	}
	m.Close()
	r := httptest.NewRequest("POST", "/admin/inventory", &b)
	r.Header.Set("Content-Type", m.FormDataContentType())
	return r
}
func TestFleetDraftPublishingAndPrivateDocuments(t *testing.T) {
	db := serviceFixture(t)
	t.Setenv("MM_UPLOADS_DIR", t.TempDir())
	staff := Staff{ID: 2, Name: "Staff", Role: "admin", Status: "approved"}
	fields := testFleetFields()
	r := fleetMultipart(t, fields, map[string]string{"photos": "stock.png", "files": "inspection.pdf"})
	r.ParseMultipartForm(1 << 20)
	defer r.MultipartForm.RemoveAll()
	v, err := saveFleetVehicle(db, r, staff)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fleetVehicle(db, v.ID, false); err == nil {
		t.Fatal("Draft was published")
	}
	request := httptest.NewRequest("GET", v.PhotoItems[0].Image, nil)
	w := httptest.NewRecorder()
	serveVehicleAsset(db, w, request)
	if w.Code != 404 {
		t.Fatal("Draft photo exposed")
	}
	fields.Set("id", v.ID)
	fields.Set("stock_status", "Available")
	fields.Set("actual_photos", "on")
	v, err = saveFleetVehicle(db, adminPost(t, "/admin/inventory", fields, 2), staff)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Photos()) != 1 || !v.ActualPhotos || v.ConditionReport == "" {
		t.Fatal("Actual condition/photo details lost")
	}
	w = httptest.NewRecorder()
	serveVehicleAsset(db, w, httptest.NewRequest("GET", v.PhotoItems[0].Image, nil))
	if w.Code != 200 {
		t.Fatal("Published photo missing", w.Code)
	}
	w = httptest.NewRecorder()
	serveVehicleAsset(db, w, httptest.NewRequest("GET", v.DocumentItems[0].URL, nil))
	if w.Code != 404 {
		t.Fatal("Private ownership document exposed")
	}
	request = httptest.NewRequest("GET", v.DocumentItems[0].URL, nil)
	request.AddCookie(staffCookie(t, 2))
	w = httptest.NewRecorder()
	serveVehicleAsset(db, w, request)
	if w.Code != 200 {
		t.Fatal("Admin cannot open private inspection document")
	}
	db.Exec("INSERT INTO order_pricing(order_id,vehicle_id,quote) VALUES(1,?,'{}')", v.ID)
	for _, tc := range []struct {
		customer int64
		code     int
	}{{1, 200}, {2, 404}} {
		req := httptest.NewRequest("GET", v.DocumentItems[0].URL, nil)
		req.AddCookie(customerCookie(t, tc.customer, ""))
		out := httptest.NewRecorder()
		serveVehicleAsset(db, out, req)
		if out.Code != tc.code {
			t.Fatal("Private buyer document access", tc, out.Code)
		}
	}
	list, err := fleetList(db, false)
	if err != nil || len(list) != len(vehicles)+1 {
		t.Fatal("Published stock not in catalogue", len(list), err)
	}
	db.Exec("UPDATE orders SET status='delivered' WHERE id=1")
	fields.Set("stock_status", "Sold")
	v, err = saveFleetVehicle(db, adminPost(t, "/admin/inventory", fields, 2), staff)
	if err != nil || v.Orderable() {
		t.Fatal("Sold stock remains orderable")
	}
}
func TestAutomaticQuoteIncludesFeesOffersAndValueBasedCharges(t *testing.T) {
	db := serviceFixture(t)
	_, err := db.Exec("UPDATE delivery_rates SET delivery=200000,fees=25000,insurance=10000,insurance_bps=100,duty_bps=1000,vat_bps=500,taxes=5000,port_fees=30000 WHERE origin='Lagos' AND state='Lagos'")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := fleetVehicle(db, "toyota-camry", false)
	v.Price = 10000000
	v.OfferName = "Savings"
	v.OfferDiscount = 1000000
	q, err := quoteVehicle(db, v, "Nigeria", "Lagos", "Ikeja")
	if err != nil {
		t.Fatal(err)
	}
	// Net car=9m; insurance=100k; tax base=9.3m; duty=930k; VAT=511.5k.
	expected := int64(10801500) * 100
	if q.Total != expected || q.Offer != "Savings" || !q.CanOrder || len(q.Lines) != 9 {
		t.Fatalf("Quote: %+v, want %d", q, expected)
	}
	expired := v
	expired.OfferEnd = "2000-01-01"
	q2, err := quoteVehicle(db, expired, "Nigeria", "Lagos", "Ikeja")
	if err != nil || q2.Offer != "" || q2.Total <= q.Total {
		t.Fatal("Expired discount applied")
	}
	if _, err = quoteVehicle(db, v, "Nigeria", "Unknown", "Somewhere"); err == nil {
		t.Fatal("Unknown destination received invented charges")
	}
	db.Exec("UPDATE delivery_rates SET complete=0,estimated=1 WHERE origin='Lagos' AND state='Lagos'")
	q, err = quoteVehicle(db, v, "Nigeria", "Lagos", "Ikeja")
	if err != nil || q.CanOrder || q.Complete {
		t.Fatal("Incomplete quote enabled checkout")
	}
	db.Exec("UPDATE delivery_rates SET complete=1,estimated=0,expires='2000-01-01' WHERE origin='Lagos' AND state='Lagos'")
	if _, err = quoteVehicle(db, v, "Nigeria", "Lagos", "Ikeja"); err == nil {
		t.Fatal("Expired route used")
	}
}
func TestQuoteSnapshotReservationAndCancellation(t *testing.T) {
	db := serviceFixture(t)
	db.Exec("UPDATE delivery_rates SET delivery=200000,fees=25000 WHERE origin='Lagos' AND state='Lagos'")
	v, _ := fleetVehicle(db, "toyota-camry", false)
	quote, err := quoteVehicle(db, v, "Nigeria", "Lagos", "Ikeja")
	if err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"vehicle-id": {v.ID}, "address": {"1 Test Road"}, "city": {"Ikeja"}, "state": {"Lagos"}, "country": {"Nigeria"}, "price": {"1"}, "quote-token": {quote.Token}}
	w := httptest.NewRecorder()
	handleOrder(db, w, servicePost(t, "/order", fields, 1))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	var id int64
	db.QueryRow("SELECT MAX(id) FROM orders").Scan(&id)
	saved := storedOrderQuote(db, id)
	if saved.Total != quote.Total {
		t.Fatal("Server quote was not frozen")
	}
	db.Exec("UPDATE delivery_rates SET delivery=900000 WHERE origin='Lagos' AND state='Lagos'")
	if storedOrderQuote(db, id).Total != quote.Total {
		t.Fatal("Existing order price changed with route")
	}
	w = httptest.NewRecorder()
	handleOrder(db, w, servicePost(t, "/order", fields, 2))
	if w.Code != 422 && w.Code != 409 {
		t.Fatal("Second customer ordered reserved stock", w.Code)
	}
	var price float64
	db.QueryRow("SELECT car_price FROM orders WHERE id=?", id).Scan(&price)
	if int64(price*100) != quote.Total {
		t.Fatal("Client supplied price accepted")
	}
	w = httptest.NewRecorder()
	cancelVehicleOrder(db, w, adminPost(t, "/admin/orders/cancel", url.Values{"order-id": {fmt.Sprint(id)}}, 2))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	v, err = fleetVehicle(db, v.ID, false)
	if err != nil || !v.Orderable() {
		t.Fatal("Cancellation did not release stock")
	}
	w = httptest.NewRecorder()
	handleOrder(db, w, servicePost(t, "/order", fields, 1))
	if w.Code != 409 {
		t.Fatal("Stale quote token was accepted", w.Code)
	}
}
func TestReferenceRatesAreClearlyIncompleteInProduction(t *testing.T) {
	db := adminFixture(t)
	t.Setenv("MM_ENV", "production")
	seedLocalBusinessDemo(db)
	rates, err := loadRates(db)
	if err != nil || len(rates) < 6 {
		t.Fatal("Reference routes missing", err)
	}
	v, _ := fleetVehicle(db, "toyota-camry", false)
	q, err := quoteVehicle(db, v, "United Kingdom", "England", "London")
	if err != nil || q.Complete || q.CanOrder || !q.Estimated || !strings.Contains(q.Notes, "GBP 2,200") {
		t.Fatal("International freight presented as all-in", q, err)
	}
	fields := url.Values{"origin": {"Lagos"}, "country": {"France"}, "state": {""}, "city": {""}, "body": {"All"}, "label": {"Confirmed route"}, "delivery": {"4000000"}, "taxes": {"500000"}, "fees": {"100000"}, "active": {"on"}, "complete": {"on"}, "insurance_percent": {"1.25"}, "duty_percent": {"10"}, "vat_percent": {"5"}}
	if err := saveRate(db, adminPost(t, "/admin/pricing", fields, 1), Staff{ID: 1, Name: "Owner"}); err != nil {
		t.Fatal(err)
	}
	q, err = quoteVehicle(db, v, "France", "Ile-de-France", "Paris")
	if err != nil || !q.CanOrder {
		t.Fatal("Admin-confirmed international route not orderable", err)
	}
}
func TestAppointmentsPreventDoubleBookingsAndProtectCalendars(t *testing.T) {
	db := serviceFixture(t)
	staff := Staff{ID: 2, Name: "Staff"}
	start := time.Now().In(dealershipTime()).Add(48 * time.Hour).Truncate(time.Hour)
	end := start.Add(time.Hour)
	fields := url.Values{"action": {"slot"}, "kind": {"test-drive"}, "vehicle": {"toyota-camry"}, "start": {start.Format("2006-01-02T15:04")}, "end": {end.Format("2006-01-02T15:04")}, "location": {"Test showroom"}}
	if err := saveAppointmentAdmin(db, adminPost(t, "/admin/appointments", fields, 2), staff); err != nil {
		t.Fatal(err)
	}
	if err := saveAppointmentAdmin(db, adminPost(t, "/admin/appointments", fields, 2), staff); err == nil {
		t.Fatal("Overlapping slot accepted")
	}
	booking := url.Values{"slot": {"1"}, "vehicle": {"toyota-camry"}, "note": {"Please show the inspection history."}}
	if err := bookVisit(db, servicePost(t, "/appointments", booking, 1), ServiceCustomer{ID: 1, Name: "First", Email: "first@example.test"}); err != nil {
		t.Fatal(err)
	}
	if err := bookVisit(db, servicePost(t, "/appointments", booking, 2), ServiceCustomer{ID: 2, Name: "Second", Email: "second@example.test"}); err == nil {
		t.Fatal("Slot double-booked")
	}
	leads, _ := loadSales(db)
	if len(leads) != 1 || leads[0].Stage != "Appointment" {
		t.Fatal("Booking not in sales follow-ups")
	}
	fields = url.Values{"action": {"status"}, "id": {"1"}, "status": {"confirmed"}}
	if err := saveAppointmentAdmin(db, adminPost(t, "/admin/appointments", fields, 2), staff); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		customer int64
		code     int
	}{{1, 200}, {2, 404}} {
		r := httptest.NewRequest("GET", "/appointments/calendar?id=1", nil)
		r.AddCookie(customerCookie(t, tc.customer, ""))
		w := httptest.NewRecorder()
		appointmentCalendar(db, w, r)
		if w.Code != tc.code {
			t.Fatal("Private calendar access", tc, w.Code)
		}
		if w.Code == 200 && !strings.Contains(w.Body.String(), "BEGIN:VALARM") {
			t.Fatal("Calendar reminder missing")
		}
	}
}
func TestSalesAssignmentAndComplaintWorkflow(t *testing.T) {
	db := serviceFixture(t)
	db.Exec("INSERT INTO enquiries(id,kind,name,email,vehicle,message) VALUES(1,'test-drive','Lead','lead@example.test','Toyota Camry','Please call me about this car.')")
	fields := url.Values{"id": {"1"}, "stage": {"Negotiating"}, "assigned": {"2"}, "followup": {"2026-01-01"}, "note": {"Follow up about delivery."}}
	if err := saveSalesLead(db, adminPost(t, "/admin/sales", fields, 2), Staff{ID: 2, Name: "Staff"}); err != nil {
		t.Fatal(err)
	}
	leads, _ := loadSales(db)
	if len(leads) != 1 || !leads[0].Due() || leads[0].AssignedID != 2 {
		t.Fatal(leads)
	}
	fields.Set("assigned", "3")
	if err := saveSalesLead(db, adminPost(t, "/admin/sales", fields, 2), Staff{ID: 2, Name: "Staff"}); err == nil {
		t.Fatal("Lead assigned to pending staff")
	}
	if w := customerMessage(t, db, 1, "Complaint about my delivery.", "case-customer-message-001"); w.Code != 201 {
		t.Fatal(w.Code)
	}
	fields = url.Values{"conversation-id": {"1"}, "case-status": {"investigating"}, "assigned": {"2"}}
	w := httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/case", fields, 2), "case")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	state := supportState(t, db, 1)
	if state.Conversation.CaseReference != "MM-S000001" || state.Conversation.CaseStatus != "investigating" || state.Conversation.AssignedID != 2 {
		t.Fatal(state.Conversation)
	}
	fields.Set("case-status", "resolved")
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/api/admin/support/case", fields, 2), "case")
	customerMessage(t, db, 1, "I still need help please.", "case-customer-message-002")
	state = supportState(t, db, 1)
	if state.Conversation.CaseStatus != "open" || state.Conversation.Status != "open" {
		t.Fatal("Customer follow-up did not reopen case")
	}
}
func TestOnlyDeliveredBuyersCanConsentToPublishedReviews(t *testing.T) {
	db := serviceFixture(t)
	db.Exec("UPDATE orders SET status='delivered' WHERE id=1")
	fields := url.Values{"order": {"1"}, "rating": {"5"}, "body": {"The delivery and handover were excellent."}, "consent": {"on"}}
	w := httptest.NewRecorder()
	customerReview(db, w, servicePost(t, "/account/reviews", fields, 2))
	var count int
	db.QueryRow("SELECT COUNT(*) FROM customer_reviews").Scan(&count)
	if count != 0 {
		t.Fatal("Another customer's order reviewed")
	}
	w = httptest.NewRecorder()
	customerReview(db, w, servicePost(t, "/account/reviews", fields, 1))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	list, _ := loadReviews(db, true)
	if len(list) != 0 {
		t.Fatal("Unmoderated review published")
	}
	w = httptest.NewRecorder()
	manageDealership(db, w, adminPost(t, "/admin/dealership", url.Values{"action": {"review"}, "id": {"1"}, "status": {"published"}}, 2))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	list, _ = loadReviews(db, true)
	if len(list) != 1 || list[0].Body != fields.Get("body") {
		t.Fatal("Consented review missing or modified")
	}
}
func TestFailedFleetUploadLeavesNoFiles(t *testing.T) {
	db := adminFixture(t)
	dir := t.TempDir()
	t.Setenv("MM_UPLOADS_DIR", dir)
	fields := testFleetFields()
	fields.Set("stock_status", "Available")
	fields.Set("description", "")
	r := fleetMultipart(t, fields, map[string]string{"photos": "stock.png"})
	r.ParseMultipartForm(1 << 20)
	defer r.MultipartForm.RemoveAll()
	if _, err := saveFleetVehicle(db, r, Staff{ID: 2, Name: "Staff"}); err == nil {
		t.Fatal("Incomplete published listing accepted")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "vehicles"))
	if err != nil || len(entries) != 0 {
		t.Fatal("Failed vehicle save left files", err)
	}
}
func TestPercentValidation(t *testing.T) {
	for _, raw := range []string{"-1", "100.01", "1.234", "nan", "1.-1"} {
		if _, err := percentField(adminPost(t, "/", url.Values{"rate": {raw}}, 0), "rate"); err == nil {
			t.Fatal("Invalid percentage accepted", raw)
		}
	}
	for _, n := range []int64{0, 100, 125, 10000} {
		raw := percentLabel(n)
		value, err := percentField(adminPost(t, "/", url.Values{"rate": {raw}}, 0), "rate")
		if err != nil || value != n {
			t.Fatal(strconv.FormatInt(n, 10), value, err)
		}
	}
}

func TestDuplicateAdminCreationKeepsTeamInterface(t *testing.T) {
	db := adminFixture(t)
	fields := url.Values{"name": {"Existing Staff"}, "email": {"STAFF@EXAMPLE.TEST"}, "password": {"SomeStrongPassword123"}}
	w := httptest.NewRecorder()
	manageTeam(db, w, adminPost(t, "/admin/team/create", fields, 1), "create")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "details already exist") || !strings.Contains(w.Body.String(), "ops-sidebar") {
		t.Fatal(w.Code, w.Body.String())
	}
}
