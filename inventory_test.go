package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE customers(id INTEGER PRIMARY KEY, name TEXT, email TEXT UNIQUE, phone TEXT, password TEXT,created_at TEXT DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE orders(id INTEGER PRIMARY KEY AUTOINCREMENT,customer_id INTEGER,name TEXT,email TEXT,phone TEXT,address TEXT,city TEXT,state TEXT,status TEXT,car_name TEXT,car_price INTEGER,created_at TEXT DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE payments(id INTEGER PRIMARY KEY,order_id INTEGER,amount_paid REAL,payment_reference TEXT,receipt TEXT,payment_status TEXT,created_at TEXT DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE admins(id INTEGER PRIMARY KEY,name TEXT,email TEXT UNIQUE,password TEXT,role TEXT,status TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	if err = initRequestsDB(db); err != nil {
		t.Fatal(err)
	}
	if err = initAdminPlatform(db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO delivery_rates(origin,country,state,city,delivery,fees,label,complete,estimated) VALUES('Lagos','Nigeria','Lagos','',0,0,'Test rate',1,0),('Abuja','Nigeria','Abuja','',0,0,'Test rate',1,0),('Abuja','Nigeria','Lagos','',0,0,'Test rate',1,0)")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func customerCookie(t *testing.T, id int64, selected string) *http.Cookie {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	session, _ := store.Get(r, "customer-session")
	session.Values["customer_id"] = id
	session.Values["customer_name"] = "Test driver"
	if selected != "" {
		session.Values["selected_vehicle"] = selected
	}
	if err := session.Save(r, w); err != nil {
		t.Fatal(err)
	}
	return w.Result().Cookies()[0]
}

func TestInventoryIntegrity(t *testing.T) {
	if len(vehicles) < 16 {
		t.Fatalf("Expected expanded inventory, got %d", len(vehicles))
	}
	ids := map[string]bool{}
	newCount, usedCount := 0, 0
	for _, v := range vehicles {
		if ids[v.ID] || v.ID == "" {
			t.Fatalf("Duplicate/missing vehicle ID %q", v.ID)
		}
		ids[v.ID] = true
		if v.Price <= 0 || v.Mileage < 0 || len(v.Features) < 3 || v.Brand == "" || v.Model == "" {
			t.Fatalf("Incomplete vehicle %s", v.ID)
		}
		if _, err := os.Stat(strings.TrimPrefix(v.Image, "/")); err != nil {
			t.Fatalf("Missing photo for %s: %v", v.ID, err)
		}
		if v.Condition == "New" {
			newCount++
			if v.Mileage != 0 {
				t.Fatal("New car has used mileage")
			}
		} else if v.Condition == "Used" {
			usedCount++
		} else {
			t.Fatal("Invalid condition")
		}
	}
	if newCount == 0 || usedCount == 0 {
		t.Fatal("Both new and used vehicles required")
	}
}

func TestShowroomPagesAndAPI(t *testing.T) {
	for _, path := range []string{"/car-details.html?id=lexus-rx350", "/car-details.html?id=does-not-exist"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		handleVehicle(w, r)
		if strings.Contains(path, "does-not-exist") {
			if w.Code != 404 {
				t.Fatal(w.Code)
			}
			continue
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), "₦85,000,000") || !strings.Contains(w.Body.String(), "vehicle=lexus-rx350") {
			t.Fatal("Detail page does not reflect selected vehicle")
		}
		if strings.Contains(w.Body.String(), "ZgotmplZ") {
			t.Fatal("Invalid template output")
		}
	}
	w := httptest.NewRecorder()
	handleCatalogue(w, httptest.NewRequest("GET", "/cars.html", nil))
	if strings.Count(w.Body.String(), "class=\"vehicle-card\"") != len(vehicles) {
		t.Fatal("Catalogue does not include all cars")
	}
	w = httptest.NewRecorder()
	handleVehicleAPI(w, httptest.NewRequest("GET", "/api/vehicles", nil))
	var got []Vehicle
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != len(vehicles) {
		t.Fatalf("Invalid vehicle API: %v", err)
	}
	w = httptest.NewRecorder()
	handleVehicleAPI(w, httptest.NewRequest("POST", "/api/vehicles", nil))
	if w.Code != 405 {
		t.Fatal("API must be read-only")
	}
}

func TestOrderUsesSelectedVehicleAndServerPrice(t *testing.T) {
	db := testDatabase(t)
	_, err := db.Exec("INSERT INTO customers(id,name,email,phone) VALUES(1,'Test Driver','driver@example.test','123456789')")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"vehicle-id": {"bmw-x5"}, "address": {"1 Sample Road"}, "city": {"Lagos"}, "state": {"Lagos"}, "price": {"1"}}
	r := httptest.NewRequest("POST", "/order", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(customerCookie(t, 1, ""))
	w := httptest.NewRecorder()
	handleOrder(db, w, r)
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var name string
	var price int
	if err = db.QueryRow("SELECT car_name,car_price FROM orders").Scan(&name, &price); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(name, "BMW X5") || price != 112000000 {
		t.Fatalf("Wrong order: %s %d", name, price)
	}
	if !strings.Contains(w.Body.String(), "BMW X5") || !strings.Contains(w.Body.String(), "₦112,000,000") {
		t.Fatal("Payment summary has wrong vehicle")
	}
	form.Set("vehicle-id", "missing")
	r = httptest.NewRequest("POST", "/order", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	handleOrder(db, w, r)
	if w.Code != 400 {
		t.Fatal("Unknown vehicle must be rejected")
	}
}

func TestOrderSelectionSurvivesSignInRedirect(t *testing.T) {
	w := httptest.NewRecorder()
	handleStartOrder(w, httptest.NewRequest("GET", "/start-order?vehicle=porsche-911", nil))
	if w.Code != 303 || w.Header().Get("Location") != "/login.html" {
		t.Fatal("Anonymous order should redirect to sign in")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(w.Result().Cookies()[0])
	session, err := store.Get(r, "customer-session")
	if err != nil || session.Values["selected_vehicle"] != "porsche-911" {
		t.Fatal("Selected vehicle lost during sign in")
	}
	r = httptest.NewRequest("GET", "/start-order?vehicle=porsche-911", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	handleStartOrder(w, r)
	if w.Header().Get("Location") != "/order.html?vehicle=porsche-911" {
		t.Fatal("Signed-in order uses wrong vehicle")
	}
}

func TestCSRFAndOriginProtection(t *testing.T) {
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	token := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, token, origin string
		status              int
	}{{"no token", "", "", 403}, {"wrong token", strings.Repeat("b", 64), "", 403}, {"cross origin", token, "https://evil.example", 403}, {"valid", token, "http://example.com", 204}} {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{"csrf-token": {tc.token}}
			r := httptest.NewRequest("POST", "http://example.com/order", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Origin", tc.origin)
			r.AddCookie(&http.Cookie{Name: "mm-csrf", Value: token})
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestReceiptsRequireOwnership(t *testing.T) {
	db := testDatabase(t)
	_, err := db.Exec("INSERT INTO orders(id,customer_id,status) VALUES(1,1,'payment_pending'); INSERT INTO payments(id,order_id,receipt) VALUES(1,1,'sample-receipt.pdf')")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/uploads/sample-receipt.pdf", nil)
	w := httptest.NewRecorder()
	handlePrivateReceipt(db, w, r)
	if w.Code != 401 {
		t.Fatal("Anonymous receipt access allowed")
	}
	r = httptest.NewRequest("GET", "/uploads/sample-receipt.pdf", nil)
	r.AddCookie(customerCookie(t, 2, ""))
	w = httptest.NewRecorder()
	handlePrivateReceipt(db, w, r)
	if w.Code != 404 {
		t.Fatal("Another customer's receipt is accessible")
	}
	r = httptest.NewRequest("POST", "/payment", strings.NewReader("order-id=1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(customerCookie(t, 2, ""))
	w = httptest.NewRecorder()
	handlePayment(db, w, r)
	if w.Code != 404 {
		t.Fatal("Another customer's payment can be submitted")
	}
}

func TestRequestValidationAndAccountIsolation(t *testing.T) {
	db := testDatabase(t)
	form := url.Values{"kind": {"import"}, "name": {"Demo Driver"}, "email": {"driver@example.test"}, "vehicle": {"Toyota RAV4"}, "market": {"Japan"}, "budget": {"USD 30000"}, "message": {"Please source a hybrid Toyota RAV4."}}
	r := httptest.NewRequest("POST", "/request", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(customerCookie(t, 1, ""))
	w := httptest.NewRecorder()
	handleRequest(db, w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "saved successfully") {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	own, err := readRequests(db, "WHERE customer_id = ?", 1)
	if err != nil || len(own) != 1 || own[0].Status != "new" {
		t.Fatal("Request not saved correctly")
	}
	other, err := readRequests(db, "WHERE customer_id = ?", 2)
	if err != nil || len(other) != 0 {
		t.Fatal("Requests not isolated by customer")
	}
	form.Set("email", "invalid")
	r = httptest.NewRequest("POST", "/request", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	handleRequest(db, w, r)
	if w.Code != 400 {
		t.Fatal("Invalid enquiry accepted")
	}
}

func TestRegistrationRejectsWeakPasswords(t *testing.T) {
	db := testDatabase(t)
	form := url.Values{"name": {"Demo Driver"}, "email": {"driver@example.test"}, "phone": {"12345678"}, "password": {"weak"}, "confirm-password": {"weak"}}
	r := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleRegister(db, w, r)
	if w.Code != 400 {
		t.Fatal("Weak password accepted")
	}
}

func TestEncodedTraversalCannotReachPrivateFiles(t *testing.T) {
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, path := range []string{"/images/%2e%2e%2fmarvellous.db", "/images/..%2fmain.go", "/fonts/%2e%2e%2f.local/session-key"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("Encoded traversal accepted: %s, status %d", path, w.Code)
		}
	}
}

func TestEngineConfigurationsAndCollectionsMatchSampleVehicles(t *testing.T) {
	for id, want := range map[string]string{"lexus-preowned": "V6", "bmw-x5": "I6", "porsche-911": "Flat6", "tesla-model-y": "Electric", "toyota-camry": "I4"} {
		v, ok := findVehicle(id)
		if !ok || v.EngineLayout != want {
			t.Fatalf("%s: engine configuration %s, want %s", id, v.EngineLayout, want)
		}
	}
	for _, v := range vehicles {
		if len(v.Categories) == 0 || v.EngineLayout == "" {
			t.Fatalf("Missing browsing details for %s", v.ID)
		}
	}
	w := httptest.NewRecorder()
	handleCatalogue(w, httptest.NewRequest("GET", "/cars.html?category=Sports&engine=Flat6", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `name="category"`) || !strings.Contains(w.Body.String(), `data-engine-layout="V6"`) {
		t.Fatal("Catalogue missing engine or collection filters")
	}
}
