package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestAssistedRecoveryIsPrivateSingleUseAndRevokesSessions(t *testing.T) {
	db := serviceFixture(t)
	if _, err := db.Exec("CREATE TABLE password_reset_tokens(customer_id INTEGER,token TEXT)"); err != nil {
		t.Fatal(err)
	}
	// Requests disclose neither account existence nor a reset secret.
	var response string
	for _, email := range []string{"first@example.test", "missing@example.test"} {
		w := httptest.NewRecorder()
		serveRecoveryRequest(db, w, adminPost(t, "/forgot-password", url.Values{"email": {email}}, 0))
		if w.Code != 200 || strings.Contains(w.Body.String(), "token=") {
			t.Fatal(w.Code, w.Body.String())
		}
		if response != "" && response != w.Body.String() {
			t.Fatal("Account existence disclosed")
		}
		response = w.Body.String()
	}
	fields := url.Values{"customer": {"1"}, "verified": {"yes"}, "reason": {"Verified via the existing contact channel."}, "admin-password": {"wrong"}}
	w := httptest.NewRecorder()
	manageRecovery(db, w, adminPost(t, "/admin/recovery", fields, 1))
	if w.Code != 400 {
		t.Fatal("Reset issued without admin reauthentication", w.Code)
	}
	// A known fixture admin hash is set explicitly for this security test.
	hash, _ := bcrypt.GenerateFromPassword([]byte("RecoveryAdmin123"), bcrypt.MinCost)
	db.Exec("UPDATE admins SET password=? WHERE id=1", string(hash))
	fields.Set("admin-password", "RecoveryAdmin123")
	w = httptest.NewRecorder()
	manageRecovery(db, w, adminPost(t, "/admin/recovery", fields, 1))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	body := w.Body.String()
	start := strings.Index(body, "/reset-password?token=")
	if start < 0 {
		t.Fatal("No private recovery link", body)
	}
	token := body[start+len("/reset-password?token="):][:64]
	var stored string
	if e := db.QueryRow("SELECT token_hash FROM account_recovery_links WHERE customer_id=1").Scan(&stored); e != nil || stored == token || stored != recoveryHash(token) {
		t.Fatal("Recovery token not securely stored", e)
	}
	oldRequest := httptest.NewRequest("GET", "/customer-dashboard.html", nil)
	oldRequest.AddCookie(customerCookie(t, 1, ""))
	// Issuing the link alone must not revoke existing access.
	seen := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, _ := store.Get(r, "customer-session")
		seen = s.Values["customer_id"] != nil
	})
	recoverySessionMiddleware(db, next).ServeHTTP(httptest.NewRecorder(), oldRequest)
	if !seen {
		t.Fatal("Issuing link revoked session")
	}
	values := url.Values{"token": {token}, "password": {"RecoveredBuyer123"}, "confirm-password": {"RecoveredBuyer123"}}
	w = httptest.NewRecorder()
	redeemRecoveryLink(db, w, adminPost(t, "/reset-password", values, 0))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	var password string
	db.QueryRow("SELECT password FROM customers WHERE id=1").Scan(&password)
	if bcrypt.CompareHashAndPassword([]byte(password), []byte("RecoveredBuyer123")) != nil {
		t.Fatal("Password not updated")
	}
	w = httptest.NewRecorder()
	redeemRecoveryLink(db, w, adminPost(t, "/reset-password", values, 0))
	if w.Code != 400 {
		t.Fatal("Recovery link replay accepted")
	}
	oldRequest = httptest.NewRequest("GET", "/customer-dashboard.html", nil)
	oldRequest.AddCookie(customerCookie(t, 1, ""))
	recoverySessionMiddleware(db, next).ServeHTTP(httptest.NewRecorder(), oldRequest)
	if seen {
		t.Fatal("Previous session still active")
	}
	// Even a cryptographically valid secret is rejected after expiry.
	expired := strings.Repeat("a", 64)
	db.Exec("INSERT INTO account_recovery_links(token_hash,customer_id,expires_at) VALUES(?,1,?)", recoveryHash(expired), time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	w = httptest.NewRecorder()
	redeemRecoveryLink(db, w, httptest.NewRequest("GET", "/reset-password?token="+expired, nil))
	if w.Code != 400 {
		t.Fatal("Expired recovery link accepted")
	}
	w = httptest.NewRecorder()
	manageRecovery(db, w, httptest.NewRequest("GET", "/admin/recovery", nil))
	if w.Code != 303 {
		t.Fatal("Anonymous recovery admin access")
	}
}
