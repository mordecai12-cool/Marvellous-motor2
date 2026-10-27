package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestBenueCheckoutAndAdminOverridesSurviveStartup(t *testing.T) {
	db := serviceFixture(t)
	t.Setenv("MM_ENV", "production")
	for _, id := range []string{"mercedes-c300", "bmw-x5", "toyota-camry"} {
		v, err := fleetVehicle(db, id, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, city := range []string{"Otukpo", "Makurdi", "Gboko", "Katsina-Ala"} {
			q, err := quoteVehicle(db, v, "Nigeria", "Benue", city)
			if err != nil || !q.CanOrder || q.Estimated || q.Lines[1].Amount != 200000 {
				t.Fatalf("%s / %s: %+v %v", id, city, q, err)
			}
		}
	}
	v, _ := fleetVehicle(db, "mercedes-c300", false)
	q, _ := quoteVehicle(db, v, "Nigeria", "Benue", "Otukpo")
	fields := url.Values{"vehicle-id": {v.ID}, "address": {"1 Test Street"}, "city": {"Otukpo"}, "state": {"Benue"}, "country": {"Nigeria"}, "quote-token": {q.Token}}
	w := httptest.NewRecorder()
	placeVehicleOrder(db, w, servicePost(t, "/order", fields, 1))
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/order-payment?order-id=") {
		t.Fatal(w.Code, w.Body.String())
	}
	paymentURL := w.Header().Get("Location")
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("GET", paymentURL, nil)
		r.AddCookie(customerCookie(t, 1, ""))
		w = httptest.NewRecorder()
		serveCustomerPayment(db, w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "8106333322") || !strings.Contains(w.Body.String(), "Opay") || !strings.Contains(w.Body.String(), "Otiti Mordecai Mavellouse") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM orders WHERE car_name LIKE '%C 300%'").Scan(&count)
	if count != 1 {
		t.Fatal("Refreshing payment duplicated the order", count)
	}
	r := httptest.NewRequest("GET", paymentURL, nil)
	r.AddCookie(customerCookie(t, 2, ""))
	w = httptest.NewRecorder()
	serveCustomerPayment(db, w, r)
	if w.Code != 404 {
		t.Fatal("Other customer read payment", w.Code)
	}
	fields = url.Values{"id": {fmt.Sprint(q.RateID)}, "body": {"All"}, "origin": {"All"}, "country": {"Nigeria"}, "state": {"Benue"}, "delivery": {"3500"}, "label": {"Benue delivery"}, "active": {"on"}, "complete": {"on"}}
	if err := saveRate(db, adminPost(t, "/admin/pricing", fields, 2), Staff{ID: 2, Name: "Test Staff"}); err != nil {
		t.Fatal(err)
	}
	if err := initBusiness(db); err != nil {
		t.Fatal(err)
	}
	v, _ = fleetVehicle(db, "toyota-camry", false)
	changed, err := quoteVehicle(db, v, "Nigeria", "Benue", "Otukpo")
	if err != nil || changed.Lines[1].Amount != 350000 {
		t.Fatal("Restart lost admin tariff", changed, err)
	}
	parsedURL, _ := url.Parse(paymentURL)
	orderID, _ := strconv.ParseInt(parsedURL.Query().Get("order-id"), 10, 64)
	if storedOrderQuote(db, orderID).Total != q.Total {
		t.Fatal("Rate update changed existing order")
	}
	db.Exec("UPDATE delivery_rates SET active=0 WHERE id=?", q.RateID)
	if err := initBusiness(db); err != nil {
		t.Fatal(err)
	}
	if _, err := quoteVehicle(db, v, "Nigeria", "Benue", "Otukpo"); err == nil {
		t.Fatal("Restart re-enabled disabled rate")
	}
}

func TestCustomerDeletionRequiresPasswordAndRemovesPrivateData(t *testing.T) {
	db := serviceFixture(t)
	if _, err := db.Exec("CREATE TABLE password_reset_tokens(customer_id INTEGER,token TEXT)"); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("FictionalPassword!26"), bcrypt.DefaultCost)
	db.Exec("UPDATE customers SET password=? WHERE id=1", string(hash))
	t.Setenv("MM_UPLOADS_DIR", t.TempDir())
	dir := filepath.Join(uploadDirectory(), "support")
	os.MkdirAll(dir, 0700)
	key := "deletion-test.png"
	file := filepath.Join(dir, key)
	os.WriteFile(file, []byte("test attachment"), 0600)
	_, err := db.Exec(`INSERT INTO support_conversations(id,customer_id,created_at,updated_at) VALUES(101,1,'2026-10-27T00:00:00Z','2026-10-27T00:00:00Z');
 INSERT INTO support_messages(id,conversation_id,sender_kind,sender_id,sender_name,body,client_id,created_at) VALUES(101,101,'customer',1,'Test Customer','File attached','delete-test','2026-10-27T00:00:00Z');
 INSERT INTO support_attachments(message_id,name,media_type,size,storage_key,digest) VALUES(101,'test.png','image/png',15,'deletion-test.png','test-digest');
 INSERT INTO support_cases(conversation_id) VALUES(101);
 INSERT INTO enquiries(id,customer_id,kind,name,email,message) VALUES(101,1,'general','Test Customer','first@example.test','An enquiry');
 INSERT INTO sales_followups(enquiry_id) VALUES(101);
 INSERT INTO enquiry_events(enquiry_id,admin_id,previous_status,next_status) VALUES(101,2,'new','closed');
 INSERT INTO account_recovery_requests(customer_id) VALUES(1);
 INSERT INTO account_recovery_links(token_hash,customer_id,expires_at) VALUES('delete-test',1,'2099-01-01');
 INSERT INTO password_reset_tokens(customer_id,token) VALUES(1,'delete-test');`)
	if err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"action": {"delete-account"}, "confirm-delete": {"yes"}, "delete-password": {"wrong"}, "customer-id": {"2"}}
	w := httptest.NewRecorder()
	serveSettings(db, w, servicePost(t, "/account/settings", fields, 1), false)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM customers WHERE id=1").Scan(&count)
	if count != 1 {
		t.Fatal("Wrong password deleted customer")
	}
	// Even a URL containing deletion fields must never mutate an account.
	get := httptest.NewRequest("GET", "/account/settings?notice=delete-account&confirm-delete=yes&delete-password=FictionalPassword!26", nil)
	get.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	serveSettings(db, w, get, false)
	db.QueryRow("SELECT COUNT(*) FROM customers WHERE id=1").Scan(&count)
	if w.Code != 200 || count != 1 {
		t.Fatal("GET changed the account", w.Code)
	}
	fields.Set("delete-password", "FictionalPassword!26")
	fields.Del("confirm-delete")
	w = httptest.NewRecorder()
	serveSettings(db, w, servicePost(t, "/account/settings", fields, 1), false)
	if w.Code != 400 {
		t.Fatal("Deletion omitted confirmation", w.Code)
	}
	fields.Set("confirm-delete", "yes")
	w = httptest.NewRecorder()
	serveSettings(db, w, servicePost(t, "/account/settings", fields, 1), false)
	if w.Code != 303 || w.Header().Get("Location") != "/login.html" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, table := range []string{"customers", "enquiries", "support_conversations", "password_reset_tokens", "account_recovery_links", "account_recovery_requests"} {
		column := "customer_id"
		if table == "customers" {
			column = "id"
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + column + "=1").Scan(&count); err != nil || count != 0 {
			t.Fatal("Personal data left in", table, count, err)
		}
	}
	db.QueryRow("SELECT COUNT(*) FROM customers WHERE id=2").Scan(&count)
	if count != 1 {
		t.Fatal("Deleted another customer")
	}
	db.QueryRow("SELECT COUNT(*) FROM orders WHERE id=1 AND customer_id IS NULL").Scan(&count)
	if count != 1 {
		t.Fatal("Order record was not retained and unlinked")
	}
	db.QueryRow("SELECT COUNT(*) FROM account_deletion_audit").Scan(&count)
	if count != 1 {
		t.Fatal("Missing deletion audit")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("Private attachment left on disk", err)
	}
	r := httptest.NewRequest("GET", "/customer-dashboard.html", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	recoverySessionMiddleware(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, _ := store.Get(r, "customer-session")
		if s.Values["customer_id"] != nil {
			t.Error("Deleted account retained a session")
		}
	})).ServeHTTP(w, r)
}

func TestRegistrationAndProfilePersistAcrossDatabaseReopen(t *testing.T) {
	db := testDatabase(t)
	fields := url.Values{"name": {"Persistent Driver"}, "email": {"persistent@example.test"}, "phone": {"08012345678"}, "password": {"FictionalPassword!26"}, "confirm-password": {"FictionalPassword!26"}}
	w := httptest.NewRecorder()
	serveRegistration(db, w, adminPost(t, "/register", fields, 0))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	var id int64
	var hash string
	if err := db.QueryRow("SELECT id,password FROM customers WHERE email=?", fields.Get("email")).Scan(&id, &hash); err != nil {
		t.Fatal(err)
	}
	if hash == fields.Get("password") || bcrypt.CompareHashAndPassword([]byte(hash), []byte(fields.Get("password"))) != nil {
		t.Fatal("Password was not securely saved")
	}
	w = httptest.NewRecorder()
	serveRegistration(db, w, adminPost(t, "/register", fields, 0))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "details already exist") {
		t.Fatal("Duplicate signup did not show clear feedback", w.Code)
	}
	r := adminPost(t, "/edit-profile", url.Values{"name": {"Updated Driver"}, "phone": {"+234 801 234 5678"}, "customer-id": {"9999"}}, 0)
	r.AddCookie(customerCookie(t, id, ""))
	w = httptest.NewRecorder()
	serveCustomerAccount(db, w, r, "edit-profile")
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	var sequence int
	var databaseName, path string
	if err := db.QueryRow("PRAGMA database_list").Scan(&sequence, &databaseName, &path); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var name, phone string
	if err := reopened.QueryRow("SELECT name,phone FROM customers WHERE id=?", id).Scan(&name, &phone); err != nil || name != "Updated Driver" || phone != "+234 801 234 5678" {
		t.Fatal("Profile did not persist", name, phone, err)
	}
	if err := initSettings(reopened); err != nil {
		t.Fatal(err)
	}
	bank, err := loadBankAccount(reopened)
	if err != nil || bank.AccountNumber != "8106333322" {
		t.Fatal("Payment details did not persist", err)
	}
}
