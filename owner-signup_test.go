package main

import (
	"golang.org/x/crypto/bcrypt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestReservedSignupActivatesImmediatelyAndNeverReopens(t *testing.T) {
	t.Setenv("MM_OWNER_ONLY", "true")
	db := testDatabase(t)
	values := url.Values{"name": {"Marvellous Otit"}, "email": {deploymentSecondOwnerEmail}, "password": {"Fictional-Test-Password!26"}, "confirm-password": {"Fictional-Test-Password!26"}}
	w := httptest.NewRecorder()
	serveStaffSignup(db, w, adminPost(t, "/admin-signup", values, 0))
	if w.Code != 303 {
		t.Fatalf("Signup %d: %s", w.Code, w.Body.String())
	}
	var id int64
	var hash, status, role string
	if err := db.QueryRow("SELECT id,password,status,role FROM admins WHERE email=?", deploymentSecondOwnerEmail).Scan(&id, &hash, &status, &role); err != nil {
		t.Fatal(err)
	}
	if status != "approved" || role != "super_admin" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(values.Get("password"))) != nil {
		t.Fatal("Signup must immediately activate a hashed-password owner")
	}
	r := httptest.NewRequest("GET", "/admin/support", nil)
	r.AddCookie(staffCookie(t, id))
	if _, err := currentStaff(db, r); err != nil {
		t.Fatal("New owner must have access without approval", err)
	}
	for _, method := range []string{"GET", "POST"} {
		w = httptest.NewRecorder()
		if method == "GET" {
			serveStaffSignup(db, w, httptest.NewRequest(method, "/admin-signup.html", nil))
		} else {
			serveStaffSignup(db, w, adminPost(t, "/admin-signup", values, 0))
		}
		if w.Code != 403 {
			t.Fatalf("Repeat signup %s: %d", method, w.Code)
		}
	}
	db.Exec("DELETE FROM admins WHERE id=?", id)
	if err := initOwnerSignup(db); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	serveStaffSignup(db, w, httptest.NewRequest("GET", "/admin-signup.html", nil))
	if w.Code != 403 {
		t.Fatal("Deleting account reopened slot")
	}
}
func TestReservedSignupValidationDoesNotConsumeSlot(t *testing.T) {
	t.Setenv("MM_OWNER_ONLY", "true")
	db := testDatabase(t)
	for _, v := range []url.Values{
		{"name": {"Wrong Person"}, "email": {"stranger@example.test"}, "password": {"Fictional-Password!26"}, "confirm-password": {"Fictional-Password!26"}},
		{"name": {"Marvellous Otit"}, "email": {deploymentSecondOwnerEmail}, "password": {"Fictional-Password!26"}, "confirm-password": {"Mismatch-Password!26"}},
	} {
		w := httptest.NewRecorder()
		serveStaffSignup(db, w, adminPost(t, "/admin-signup", v, 0))
		if w.Code != 400 {
			t.Fatalf("Validation %d", w.Code)
		}
	}
	var available bool
	db.QueryRow("SELECT consumed_at IS NULL FROM admin_setup_state").Scan(&available)
	if !available {
		t.Fatal("Invalid input consumed slot")
	}
}
func TestProductionOwnersCanInviteAndDisableOrdinaryStaff(t *testing.T) {
	db := adminFixture(t)
	t.Setenv("MM_OWNER_ONLY", "true")
	db.Exec("UPDATE admins SET email=? WHERE id=1", deploymentOwnerEmail)
	w := httptest.NewRecorder()
	createStaffInvitation(db, w, adminPost(t, "/admin/team/invite", url.Values{"email": {"new-staff@example.test"}}, 1))
	if w.Code != 200 {
		t.Fatalf("Owner invitation %d: %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	createStaffInvitation(db, w, adminPost(t, "/admin/team/invite", url.Values{"email": {"intruder@example.test"}}, 2))
	if w.Code != 403 {
		t.Fatal("Staff could invite")
	}
	w = httptest.NewRecorder()
	manageTeam(db, w, adminPost(t, "/admin/team/update", url.Values{"admin-id": {"2"}, "status": {"disabled"}}, 1), "update")
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/admin/support", nil)
	r.AddCookie(staffCookie(t, 2))
	if _, err := currentStaff(db, r); err == nil {
		t.Fatal("Disabled session still authorized")
	}
}
func TestProductionStaffPresenceMatchesAuthorization(t *testing.T) {
	db := adminFixture(t)
	t.Setenv("MM_OWNER_ONLY", "true")
	db.Exec("INSERT INTO support_presence(staff_id,last_seen) VALUES(2,strftime('%s','now'))")
	online, err := supportOnline(db)
	if err != nil || !online {
		t.Fatal("Approved ordinary staff must appear online", err)
	}
	db.Exec("UPDATE admins SET status='disabled' WHERE id=2")
	online, err = supportOnline(db)
	if err != nil || online {
		t.Fatal("Disabled staff presence must disappear", err)
	}
}
func TestCustomerAccountIDAndProfileOwnership(t *testing.T) {
	db := testDatabase(t)
	db.Exec("INSERT INTO customers(id,name,email,phone) VALUES(1,'First Driver','first@example.test','08012345678'),(2,'Second Driver','second@example.test','08098765432')")
	for _, page := range []string{"dashboard", "profile", "edit-profile"} {
		r := httptest.NewRequest("GET", "/customer-profile", nil)
		r.AddCookie(customerCookie(t, 1, ""))
		w := httptest.NewRecorder()
		serveCustomerAccount(db, w, r, page)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "MM-C000001") || strings.Contains(w.Body.String(), "second@example.test") {
			t.Fatalf("Account %s %d %s", page, w.Code, w.Body.String())
		}
	}
	values := url.Values{"name": {"Updated First Driver"}, "phone": {"+234 801 234 5678"}, "email": {"second@example.test"}, "customer-id": {"2"}}
	r := adminPost(t, "/edit-profile", values, 0)
	r.AddCookie(customerCookie(t, 1, ""))
	w := httptest.NewRecorder()
	serveCustomerAccount(db, w, r, "edit-profile")
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	var name, email string
	db.QueryRow("SELECT name,email FROM customers WHERE id=1").Scan(&name, &email)
	if name != "Updated First Driver" || email != "first@example.test" {
		t.Fatal("Profile ownership or immutable email broken")
	}
	db.QueryRow("SELECT name FROM customers WHERE id=2").Scan(&name)
	if name != "Second Driver" {
		t.Fatal("Changed another customer")
	}
	values.Set("phone", "invalid")
	r = adminPost(t, "/edit-profile", values, 0)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	serveCustomerAccount(db, w, r, "edit-profile")
	if w.Code != 400 {
		t.Fatal("Invalid phone accepted")
	}
	db.Exec("DELETE FROM customers WHERE id=1")
	r = httptest.NewRequest("GET", "/customer-dashboard.html", nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	serveCustomerAccount(db, w, r, "dashboard")
	if w.Code != 303 {
		t.Fatal("Deleted customer can access profile")
	}
}
