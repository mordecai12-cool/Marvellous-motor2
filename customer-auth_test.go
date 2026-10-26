package main

import (
	"golang.org/x/crypto/bcrypt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCustomerSignInNormalizesEmailAndPreservesErrorPage(t *testing.T) {
	db := testDatabase(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("Fictional-Password!26"), bcrypt.MinCost)
	db.Exec("INSERT INTO customers(id,name,email,phone,password) VALUES(1,'Test Driver','driver@example.test','08012345678',?)", string(hash))
	w := httptest.NewRecorder()
	handleLogin(db, w, adminPost(t, "/login", url.Values{"email": {" DRIVER@EXAMPLE.TEST "}, "password": {"Fictional-Password!26"}}, 0))
	if w.Code != 303 || w.Header().Get("Location") != "/customer-dashboard.html" {
		t.Fatal("Case-insensitive sign in failed", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handleLogin(db, w, adminPost(t, "/login", url.Values{"email": {"driver@example.test"}, "password": {"incorrect"}}, 0))
	if w.Code != 401 || !strings.Contains(w.Body.String(), `role="alert"`) || !strings.Contains(w.Body.String(), `action="/login"`) {
		t.Fatal("Invalid sign in must retain the styled form")
	}
}
func TestRecoveryResponseDoesNotRevealCustomerExistence(t *testing.T) {
	db := testDatabase(t)
	db.Exec("CREATE TABLE password_reset_tokens(id INTEGER PRIMARY KEY,customer_id INTEGER,token TEXT,expires_at DATETIME)")
	db.Exec("INSERT INTO customers(id,name,email,phone) VALUES(1,'Driver','driver@example.test','08012345678')")
	var bodies []string
	for _, email := range []string{"driver@example.test", "missing@example.test"} {
		w := httptest.NewRecorder()
		handleForgotPassword(db, w, adminPost(t, "/forgot-password", url.Values{"email": {email}}, 0))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		bodies = append(bodies, w.Body.String())
	}
	if bodies[0] != bodies[1] {
		t.Fatal("Recovery revealed account existence")
	}
}

func TestSharedLoginRecognizesBothBuiltInAdmins(t *testing.T) {
	db := testDatabase(t)
	if err := seedDeploymentOwner(db); err != nil {
		t.Fatal(err)
	}
	for _, account := range []struct{ email, password string }{{deploymentOwnerEmail, "Ikese2020"}, {deploymentSecondOwnerEmail, "password"}} {
		w := httptest.NewRecorder()
		handleLogin(db, w, adminPost(t, "/login", url.Values{"email": {" " + strings.ToUpper(account.email) + " "}, "password": {account.password}}, 0))
		if w.Code != 303 || w.Header().Get("Location") != "/admin.html" {
			t.Fatal("Shared login did not redirect admin", w.Code, w.Body.String())
		}
		r := httptest.NewRequest("GET", "/admin.html", nil)
		for _, cookie := range w.Result().Cookies() {
			r.AddCookie(cookie)
		}
		staff, err := currentStaff(db, r)
		if err != nil || staff.Email != account.email {
			t.Fatal("Shared login did not create a valid admin session", err)
		}
	}
}

func TestSharedLoginCannotGrantAdminAccessToWrongOrDisabledCredentials(t *testing.T) {
	db := adminFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("StaffPassword123"), bcrypt.MinCost)
	db.Exec("UPDATE admins SET password=? WHERE id=2", string(hash))
	// A customer may share an email with staff, but has independent credentials.
	customerHash, _ := bcrypt.GenerateFromPassword([]byte("CustomerPassword123"), bcrypt.MinCost)
	db.Exec("INSERT INTO customers(id,name,email,password) VALUES(1,'Customer','staff@example.test',?)", string(customerHash))
	for _, tc := range []struct {
		password, status, location string
		code                       int
	}{
		{"StaffPassword123", "approved", "/admin.html", 303},
		{"wrong-password", "approved", "", 401},
		{"StaffPassword123", "disabled", "", 401},
		{"StaffPassword123", "pending", "", 401},
		{"CustomerPassword123", "approved", "/customer-dashboard.html", 303},
	} {
		db.Exec("UPDATE admins SET status=? WHERE id=2", tc.status)
		w := httptest.NewRecorder()
		handleLogin(db, w, adminPost(t, "/login", url.Values{"email": {"staff@example.test"}, "password": {tc.password}}, 0))
		if w.Code != tc.code || w.Header().Get("Location") != tc.location {
			t.Fatalf("%s: got %d %s", tc.status, w.Code, w.Header().Get("Location"))
		}
		if tc.location != "/admin.html" {
			for _, cookie := range w.Result().Cookies() {
				if cookie.Name == "admin-session" {
					t.Fatal("Invalid credentials or customer login granted admin session")
				}
			}
		}
	}
}
