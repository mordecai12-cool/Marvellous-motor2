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
