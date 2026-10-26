package main

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestBuiltInAdminsAndPasswordChangesSurviveStartup(t *testing.T) {
	db := adminFixture(t)
	t.Setenv("MM_ENV", "production")
	if err := seedDeploymentOwner(db); err != nil {
		t.Fatal(err)
	}
	for _, a := range []struct{ email, password string }{{deploymentOwnerEmail, "Ikese2020"}, {deploymentSecondOwnerEmail, "password"}} {
		w := httptest.NewRecorder()
		handleAdminLogin(db, w, adminPost(t, "/admin-login", url.Values{"email": {a.email}, "password": {a.password}}, 0))
		if w.Header().Get("Location") != "/admin.html" {
			t.Fatalf("Built-in admin login failed: %s", a.email)
		}
	}
	var id int64
	db.QueryRow("SELECT id FROM admins WHERE email=?", deploymentSecondOwnerEmail).Scan(&id)
	fields := url.Values{"current-password": {"password"}, "new-password": {"ChangedPassword123"}, "confirm-password": {"ChangedPassword123"}}
	if msg, status := changeAccountPassword(db, adminPost(t, "/admin/settings", fields, id), "admins", id); status != 303 {
		t.Fatal(status, msg)
	}
	if err := seedDeploymentOwner(db); err != nil {
		t.Fatal(err)
	}
	var hash string
	db.QueryRow("SELECT password FROM admins WHERE id=?", id).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("ChangedPassword123")) != nil {
		t.Fatal("Restart reset changed password")
	}
	fields.Set("current-password", "password")
	fields.Set("new-password", "AnotherPassword123")
	fields.Set("confirm-password", "AnotherPassword123")
	if _, status := changeAccountPassword(db, adminPost(t, "/admin/settings", fields, id), "admins", id); status != 400 {
		t.Fatal("Old password can still change password")
	}
}

func TestPaymentAccountOnlyAdminsCanSaveAndCheckoutUsesLatest(t *testing.T) {
	db := adminFixture(t)
	fields := url.Values{"action": {"bank"}, "bank-name": {"Test Bank"}, "account-name": {"Test Motors"}, "account-number": {"0123456789"}}
	w := httptest.NewRecorder()
	serveSettings(db, w, adminPost(t, "/admin/settings", fields, 0), true)
	if w.Code != 403 {
		t.Fatal("Anonymous user can save payment account")
	}
	fields.Set("account-number", "bad")
	w = httptest.NewRecorder()
	serveSettings(db, w, adminPost(t, "/admin/settings", fields, 2), true)
	b, err := loadBankAccount(db)
	if err != nil || b.Configured() {
		t.Fatal("Invalid account persisted", err)
	}
	fields.Set("account-number", "0123456789")
	for _, number := range []string{"0123456789", "0987654321"} {
		fields.Set("account-number", number)
		w = httptest.NewRecorder()
		serveSettings(db, w, adminPost(t, "/admin/settings", fields, 2), true)
		if w.Code != 303 {
			t.Fatal(w.Code, w.Body.String())
		}
		b, err = loadBankAccount(db)
		if err != nil || b.AccountNumber != number {
			t.Fatal("Bank details did not persist", b, err)
		}
	}
	db.Exec("INSERT INTO customers(id,name,email,password) VALUES(1,'Test Customer','test@example.test','unused')")
	db.Exec("INSERT INTO orders(id,customer_id,name,email,phone,address,city,state,status,car_name,car_price) VALUES(1,1,'Test Customer','test@example.test','08000000000','Test Road','Lagos','Lagos','payment_pending','Test Vehicle',1000)")
	r := httptest.NewRequest("GET", "/order-payment?order-id=1", nil)
	session, _ := store.Get(r, "customer-session")
	session.Values["customer_id"] = int64(1)
	cookieWriter := httptest.NewRecorder()
	session.Save(r, cookieWriter)
	r.AddCookie(cookieWriter.Result().Cookies()[0])
	w = httptest.NewRecorder()
	serveCustomerPayment(db, w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "0987654321") || !strings.Contains(w.Body.String(), "Test Bank") {
		t.Fatal("Checkout missing updated bank details", w.Code, w.Body.String())
	}
	r = adminPost(t, "/account/settings", fields, 0)
	r.AddCookie(cookieWriter.Result().Cookies()[0])
	w = httptest.NewRecorder()
	serveSettings(db, w, r, false)
	if w.Code != 403 {
		t.Fatal("Customer can change bank details")
	}
}

func TestCustomerPasswordRequiresCurrentPasswordAndClearsResetTokens(t *testing.T) {
	db := adminFixture(t)
	db.Exec("CREATE TABLE password_reset_tokens(customer_id INTEGER, token TEXT)")
	hash, _ := bcrypt.GenerateFromPassword([]byte("OriginalPassword"), bcrypt.DefaultCost)
	db.Exec("INSERT INTO customers(id,name,email,password) VALUES(1,'Customer','customer@example.test',?)", string(hash))
	db.Exec("INSERT INTO password_reset_tokens(customer_id,token) VALUES(1,'old-token')")
	db.Exec("INSERT INTO account_recovery_links(token_hash,customer_id,expires_at) VALUES('old-hash',1,'2099-01-01T00:00:00Z')")
	fields := url.Values{"current-password": {"wrong"}, "new-password": {"NextPassword123"}, "confirm-password": {"NextPassword123"}}
	if _, status := changeAccountPassword(db, adminPost(t, "/account/settings", fields, 0), "customers", 1); status != 400 {
		t.Fatal("Incorrect current password accepted")
	}
	fields.Set("current-password", "OriginalPassword")
	if msg, status := changeAccountPassword(db, adminPost(t, "/account/settings", fields, 0), "customers", 1); status != 303 {
		t.Fatal(status, msg)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM password_reset_tokens WHERE customer_id=1").Scan(&count)
	var recoveryCount int
	db.QueryRow("SELECT COUNT(*) FROM account_recovery_links WHERE customer_id=1").Scan(&recoveryCount)
	if recoveryCount != 0 {
		t.Fatal("Password change left an assisted recovery link valid")
	}
	if count != 0 {
		t.Fatal("Old recovery tokens still valid")
	}
	db.QueryRow("SELECT password FROM customers WHERE id=1").Scan(&hash)
	if bcrypt.CompareHashAndPassword(hash, []byte("NextPassword123")) != nil {
		t.Fatal("New customer password does not verify")
	}
}
