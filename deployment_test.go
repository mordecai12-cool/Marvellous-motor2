package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostingPortAndBinding(t *testing.T) {
	for _, tc := range []struct {
		name, mode, port, host, want string
		invalid                      bool
	}{
		{"local", "", "", "", "127.0.0.1:8080", false},
		{"host_port", "", "10000", "", "0.0.0.0:10000", false},
		{"production", "production", "", "", "0.0.0.0:8080", false},
		{"explicit_loopback", "production", "9090", "127.0.0.1", "127.0.0.1:9090", false},
		{"ipv6", "production", "9090", "::1", "[::1]:9090", false},
		{"invalid_port", "production", "abc", "", "", true},
		{"zero_port", "production", "0", "", "", true},
		{"large_port", "production", "65536", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MM_ENV", tc.mode)
			t.Setenv("PORT", tc.port)
			t.Setenv("MM_HOST", tc.host)
			got, err := listenAddress()
			if tc.invalid {
				if err == nil {
					t.Fatal("Invalid hosting port accepted")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatal(got, err)
			}
		})
	}
}
func TestPersistentStorageAndLocalDefaults(t *testing.T) {
	t.Setenv("MM_ENV", "")
	t.Setenv("MM_DATA_DIR", "")
	t.Setenv("MM_DB_PATH", "")
	t.Setenv("MM_UPLOADS_DIR", "")
	if databasePath() != ".local/preview.db" || uploadDirectory() != "uploads" {
		t.Fatal("Local database/receipt paths changed")
	}
	directory := t.TempDir()
	t.Setenv("MM_ENV", "production")
	t.Setenv("MM_DATA_DIR", directory)
	if databasePath() != filepath.Join(directory, "marvellous.db") || uploadDirectory() != filepath.Join(directory, "uploads") {
		t.Fatal("Production data is outside the persistent directory")
	}
	t.Setenv("MM_DB_PATH", filepath.Join(directory, "custom.db"))
	t.Setenv("MM_UPLOADS_DIR", filepath.Join(directory, "receipts"))
	if databasePath() != filepath.Join(directory, "custom.db") || uploadDirectory() != filepath.Join(directory, "receipts") {
		t.Fatal("Explicit persistent paths ignored")
	}
}
func TestPersistentSessionKeySurvivesStoreRestart(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("MM_ENV", "production")
	t.Setenv("MM_DATA_DIR", directory)
	t.Setenv("MM_SESSION_SECRET", "")
	t.Setenv("MM_SECURE_COOKIES", "true")
	first := newSessionStore()
	r := httptest.NewRequest("GET", "https://example.test/", nil)
	session, _ := first.Get(r, "customer-session")
	session.Values["customer_id"] = int64(23)
	w := httptest.NewRecorder()
	if err := session.Save(r, w); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly {
		t.Fatal("Production session cookie is not secure")
	}
	keyPath := filepath.Join(directory, "session-key")
	info, err := os.Stat(keyPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("Persistent signing key is not private", err)
	}
	second := newSessionStore()
	r = httptest.NewRequest("GET", "https://example.test/", nil)
	r.AddCookie(cookie)
	restored, err := second.Get(r, "customer-session")
	if err != nil || restored.Values["customer_id"] != int64(23) {
		t.Fatal("Session could not be verified after a restart", err)
	}
}
func TestProductionCookieDefaults(t *testing.T) {
	t.Setenv("MM_ENV", "production")
	t.Setenv("MM_SECURE_COOKIES", "")
	os.Unsetenv("MM_SECURE_COOKIES")
	if !secureCookiesEnabled() {
		t.Fatal("Production HTTPS cookies disabled")
	}
	t.Setenv("MM_ENV", "")
	t.Setenv("MM_SECURE_COOKIES", "false")
	if secureCookiesEnabled() {
		t.Fatal("Local HTTP cookies forced secure")
	}
}
func TestConfiguredUploadStorageKeepsReceiptsPrivate(t *testing.T) {
	db := adminFixture(t)
	directory := t.TempDir()
	t.Setenv("MM_UPLOADS_DIR", directory)
	body := []byte("Fictional storage verification receipt")
	if err := os.WriteFile(filepath.Join(directory, "storage-test.pdf"), body, 0600); err != nil {
		t.Fatal(err)
	}
	db.Exec("INSERT INTO customers(id,name,email,phone) VALUES(1,'First','first@example.test','08012345678'),(2,'Second','second@example.test','08098765432')")
	db.Exec("INSERT INTO orders(id,customer_id,status) VALUES(1,1,'payment_pending');INSERT INTO payments(id,order_id,receipt) VALUES(1,1,'storage-test.pdf')")
	for _, tc := range []struct {
		customer int64
		want     int
	}{{1, 200}, {2, 404}, {0, 401}} {
		r := httptest.NewRequest("GET", "/uploads/storage-test.pdf", nil)
		if tc.customer > 0 {
			r.AddCookie(customerCookie(t, tc.customer, ""))
		}
		w := httptest.NewRecorder()
		handlePrivateReceipt(db, w, r)
		if w.Code != tc.want {
			t.Fatal(tc.customer, w.Code, w.Body.String())
		}
		if tc.want == 200 && w.Body.String() != string(body) {
			t.Fatal("Receipt was read from the wrong storage path")
		}
	}
}
func TestProductionHealthAndDemoIsolation(t *testing.T) {
	db := testDatabase(t)
	t.Setenv("MM_ENV", "production")
	t.Setenv("MM_DEMO_MODE", "true")
	seedLocalDemo(db)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM customers").Scan(&count); err != nil || count != 0 {
		t.Fatal("Fictional customer was seeded in production", err)
	}
	w := httptest.NewRecorder()
	handleHealth(db, w, httptest.NewRequest("GET", "/health", nil))
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response["mode"] != "production" || response["status"] != "ok" {
		t.Fatal("Production health check failed", w.Body.String(), err)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("Health check is not JSON")
	}
}
