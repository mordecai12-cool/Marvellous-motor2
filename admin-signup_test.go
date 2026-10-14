package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func inviteForTest(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	w := httptest.NewRecorder()
	createStaffInvitation(db, w, adminPost(t, "/admin/team/invite", url.Values{"email": {email}}, 1))
	if w.Code != 200 {
		t.Fatalf("Invitation failed: %d %s", w.Code, w.Body.String())
	}
	match := regexp.MustCompile(`token=([a-f0-9]{64})`).FindStringSubmatch(w.Body.String())
	if len(match) != 2 {
		t.Fatal("Owner did not receive invitation link")
	}
	return match[1]
}
func signupFields(token string) url.Values {
	return url.Values{"token": {token}, "name": {"Invited Staff"}, "email": {"invited@example.test"}, "password": {"UniqueTestPassword!26"}, "confirm-password": {"UniqueTestPassword!26"}, "role": {"super_admin"}}
}
func TestStaffInvitationIsOwnerOnlyAndDoesNotStoreRawToken(t *testing.T) {
	db := adminFixture(t)
	for _, id := range []int64{0, 2, 3} {
		w := httptest.NewRecorder()
		createStaffInvitation(db, w, adminPost(t, "/admin/team/invite", url.Values{"email": {"invited@example.test"}}, id))
		if w.Code != 403 {
			t.Fatalf("Non-owner %d can invite: %d", id, w.Code)
		}
	}
	token := inviteForTest(t, db, "Invited@Example.Test")
	var hash, email string
	var expiry int64
	if err := db.QueryRow("SELECT token_hash,email,expires_at FROM staff_invitations").Scan(&hash, &email, &expiry); err != nil {
		t.Fatal(err)
	}
	want, _ := invitationHash(token)
	if hash != want || hash == token || email != "invited@example.test" {
		t.Fatal("Invitation is not securely bound to normalized email")
	}
	if expiry < time.Now().Add(47*time.Hour).Unix() || expiry > time.Now().Add(49*time.Hour).Unix() {
		t.Fatal("Incorrect expiry")
	}
	w := httptest.NewRecorder()
	createStaffInvitation(db, w, adminPost(t, "/admin/team/invite", url.Values{"email": {"OWNER@example.test"}}, 1))
	if w.Code != 409 {
		t.Fatal("Invitation can replace existing owner account")
	}
}
func TestInvitedSignupValidatesInputWithoutConsumingLink(t *testing.T) {
	db := adminFixture(t)
	token := inviteForTest(t, db, "invited@example.test")
	for _, tc := range []struct{ field, value string }{
		{"email", "other@example.test"}, {"confirm-password", "DifferentPassword!26"}, {"password", "short"}, {"name", " "},
	} {
		fields := signupFields(token)
		fields.Set(tc.field, tc.value)
		w := httptest.NewRecorder()
		serveStaffSignup(db, w, adminPost(t, "/admin-signup", fields, 0))
		if w.Code != 400 {
			t.Fatalf("Invalid %s accepted: %d", tc.field, w.Code)
		}
		var used sql.NullInt64
		if err := db.QueryRow("SELECT used_at FROM staff_invitations").Scan(&used); err != nil {
			t.Fatal(err)
		}
		if used.Valid {
			t.Fatal("Invalid submission consumes invitation")
		}
	}
}
func TestInvitedSignupCannotEscalateRoleAndCannotReplay(t *testing.T) {
	db := adminFixture(t)
	token := inviteForTest(t, db, "invited@example.test")
	w := httptest.NewRecorder()
	serveStaffSignup(db, w, adminPost(t, "/admin-signup", signupFields(token), 0))
	if w.Code != 303 || w.Header().Get("Location") != "/admin-login.html?notice=account-created" {
		t.Fatalf("Signup failed: %d %s", w.Code, w.Body.String())
	}
	var role, status, password string
	if err := db.QueryRow("SELECT role,status,password FROM admins WHERE email='invited@example.test'").Scan(&role, &status, &password); err != nil {
		t.Fatal(err)
	}
	if role != "admin" || status != "approved" {
		t.Fatal("Signup escalates role or bypasses invitation authorization")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(signupFields(token).Get("password"))); err != nil {
		t.Fatal("Password not hashed correctly")
	}
	w = httptest.NewRecorder()
	serveStaffSignup(db, w, adminPost(t, "/admin-signup", signupFields(token), 0))
	if w.Code != 400 {
		t.Fatal("Used invitation can be replayed")
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM admins WHERE email='invited@example.test'").Scan(&count)
	if count != 1 {
		t.Fatal("Replay created another account")
	}
}
func TestSignupRejectsExpiredRevokedReplacedAndUnauthorizedInvitations(t *testing.T) {
	for _, condition := range []string{"expired", "revoked", "replaced", "owner-disabled", "owner-demoted"} {
		t.Run(condition, func(t *testing.T) {
			db := adminFixture(t)
			token := inviteForTest(t, db, "invited@example.test")
			var err error
			switch condition {
			case "expired":
				_, err = db.Exec("UPDATE staff_invitations SET expires_at=?", time.Now().Add(-time.Minute).Unix())
			case "revoked":
				_, err = db.Exec("UPDATE staff_invitations SET revoked_at=?", time.Now().Unix())
			case "replaced":
				inviteForTest(t, db, "invited@example.test")
			case "owner-disabled":
				_, err = db.Exec("UPDATE admins SET status='disabled' WHERE id=1")
			case "owner-demoted":
				_, err = db.Exec("UPDATE admins SET role='admin' WHERE id=1")
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, method := range []string{"GET", "POST"} {
				var r *http.Request
				if method == "GET" {
					r = httptest.NewRequest("GET", "/admin-signup.html?token="+token, nil)
				} else {
					r = adminPost(t, "/admin-signup", signupFields(token), 0)
				}
				w := httptest.NewRecorder()
				serveStaffSignup(db, w, r)
				if w.Code != 400 || strings.Contains(w.Body.String(), "invited@example.test") {
					t.Fatal("Unavailable invitation exposes email or enables signup")
				}
			}
		})
	}
}
func TestInvitationRevocationIsOwnerOnlyAndImmediate(t *testing.T) {
	db := adminFixture(t)
	token := inviteForTest(t, db, "invited@example.test")
	for _, id := range []int64{2, 1} {
		w := httptest.NewRecorder()
		revokeStaffInvitation(db, w, adminPost(t, "/admin/team/revoke-invite", url.Values{"invitation-id": {"1"}}, id))
		want := 403
		if id == 1 {
			want = 303
		}
		if w.Code != want {
			t.Fatalf("Revocation for %d: %d", id, w.Code)
		}
	}
	w := httptest.NewRecorder()
	serveStaffSignup(db, w, adminPost(t, "/admin-signup", signupFields(token), 0))
	if w.Code != 400 {
		t.Fatal("Revoked invitation still accepts signup")
	}
}
func TestSignupConflictRollsBackInvitationConsumption(t *testing.T) {
	db := adminFixture(t)
	token := inviteForTest(t, db, "invited@example.test")
	if _, err := db.Exec("INSERT INTO admins(name,email,password,role,status) VALUES('Existing Staff','INVITED@example.test','unused','admin','disabled')"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	serveStaffSignup(db, w, adminPost(t, "/admin-signup", signupFields(token), 0))
	if w.Code != 409 {
		t.Fatal("Conflicting account was overwritten")
	}
	var used sql.NullInt64
	if err := db.QueryRow("SELECT used_at FROM staff_invitations").Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used.Valid {
		t.Fatal("Failed account creation consumes link")
	}
}
func TestSignupRequiresCSRFAndNoInvitationCannotCreateAccount(t *testing.T) {
	db := adminFixture(t)
	token := inviteForTest(t, db, "invited@example.test")
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveStaffSignup(db, w, r) }))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, adminPost(t, "/admin-signup", signupFields(token), 0))
	if w.Code != 403 {
		t.Fatal("Signup accepts missing CSRF")
	}
	w = httptest.NewRecorder()
	serveStaffSignup(db, w, adminPost(t, "/admin-signup", signupFields(""), 0))
	if w.Code != 400 {
		t.Fatal("Open signup creates administrator")
	}
	w = httptest.NewRecorder()
	serveStaffSignup(db, w, httptest.NewRequest("GET", "/admin-signup.html?token="+token, nil))
	if w.Code != 200 || w.Header().Get("Referrer-Policy") != "strict-origin" {
		t.Fatal("Valid invitation page leaks referrer or fails to render")
	}
}
func TestPaymentConfirmationUsesRealEvidenceAndCurrentOrderState(t *testing.T) {
	for _, tc := range []struct {
		status, payment, heading string
		evidence                 bool
	}{
		{"payment_pending", "", "Complete your payment details.", true},
		{"payment_pending", "pending", "Receipt received.", false},
		{"payment_pending", "rejected", "A quick payment update is needed.", true},
		{"processing", "verified", "Payment approved. Next stop: preparation.", false},
		{"delivered", "verified", "Your vehicle has arrived.", false},
	} {
		t.Run(tc.status+"/"+tc.payment, func(t *testing.T) {
			db := adminFixture(t)
			_, err := db.Exec("INSERT INTO orders(id,customer_id,name,email,phone,address,city,state,car_name,car_price,status) VALUES(1,1,'Test Customer','test@example.test','12345','Test Road','Lagos','Lagos','2025 Toyota Camry Hybrid XSE (New)',48500000,?)", tc.status)
			if err != nil {
				t.Fatal(err)
			}
			if tc.payment != "" {
				if _, err = db.Exec("INSERT INTO payments(id,order_id,amount_paid,payment_reference,receipt,payment_status) VALUES(1,1,48500000,'TEST-REF','test.pdf',?)", tc.payment); err != nil {
					t.Fatal(err)
				}
			}
			r := httptest.NewRequest("GET", "/payment-pending.html?order-id=1", nil)
			r.AddCookie(customerCookie(t, 1, ""))
			w := httptest.NewRecorder()
			serveCustomerDelivery(db, w, r, true)
			if w.Code != 200 || !strings.Contains(w.Body.String(), tc.heading) {
				t.Fatalf("Wrong confirmation: %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), `href="/order-payment?order-id=1"`) != tc.evidence {
				t.Fatal("Incorrect evidence resubmission action")
			}
			if tc.payment != "" && !strings.Contains(w.Body.String(), "TEST-REF") {
				t.Fatal("Payment reference missing")
			}
			r = httptest.NewRequest("GET", "/payment-pending.html?order-id=1", nil)
			r.AddCookie(customerCookie(t, 2, ""))
			w = httptest.NewRecorder()
			serveCustomerDelivery(db, w, r, true)
			if w.Code != 404 || strings.Contains(w.Body.String(), "TEST-REF") {
				t.Fatal("Payment summary exposed to another customer")
			}
		})
	}
}

func TestCustomerNavigationReflectsSessionAndActivePage(t *testing.T) {
	for _, signedIn := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/", nil)
		if signedIn {
			r.AddCookie(customerCookie(t, 1, ""))
		}
		w := httptest.NewRecorder()
		renderShowroom(w, r, "index.html", ShowroomData{Page: "home", Vehicles: vehicles[:6]})
		if w.Code != 200 {
			t.Fatal("Could not render showroom")
		}
		body := w.Body.String()
		if !strings.Contains(body, `class="active" href="/">Home`) {
			t.Fatal("Active navigation page is not shown")
		}
		account := `<a class="button button-small" href="/customer-dashboard.html">My account`
		login := `<a class="button button-small" href="/login.html">Sign in`
		if strings.Contains(body, account) != signedIn || strings.Contains(body, login) == signedIn {
			t.Fatal("Navigation does not reflect customer session")
		}
	}
}
