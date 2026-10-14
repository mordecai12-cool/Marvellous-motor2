package main

import (
	"crypto/hmac"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/sessions"
)

func newSessionStore() *sessions.CookieStore {
	key := []byte(os.Getenv("MM_SESSION_SECRET"))
	if len(key) == 0 {
		if err := os.MkdirAll(".local", 0700); err != nil {
			log.Fatal(err)
		}
		var err error
		key, err = os.ReadFile(".local/session-key")
		if os.IsNotExist(err) {
			key = make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				log.Fatal(err)
			}
			if err = os.WriteFile(".local/session-key", key, 0600); err != nil {
				log.Fatal(err)
			}
		} else if err != nil {
			log.Fatal(err)
		}
	}
	if len(key) < 32 {
		log.Fatal("MM_SESSION_SECRET must contain at least 32 bytes")
	}
	s := sessions.NewCookieStore(key)
	s.Options = &sessions.Options{Path: "/", MaxAge: 12 * 60 * 60, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("MM_SECURE_COOKIES") == "true"}
	return s
}

type rateEntry struct {
	count int
	since time.Time
}

var authRates = struct {
	sync.Mutex
	entries map[string]rateEntry
}{entries: make(map[string]rateEntry)}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path.Clean(r.URL.Path) || strings.Contains(r.URL.Path, "\\") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; font-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		cookie, err := r.Cookie("mm-csrf")
		if err != nil || len(cookie.Value) != 64 {
			bytes := make([]byte, 32)
			if _, err := rand.Read(bytes); err != nil {
				http.Error(w, "Could not initialise session", 500)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "mm-csrf", Value: hex.EncodeToString(bytes), Path: "/", MaxAge: 43200, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("MM_SECURE_COOKIES") == "true"})
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					http.Error(w, "Request origin rejected", http.StatusForbidden)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					http.Error(w, "Invalid or oversized form", http.StatusBadRequest)
					return
				}
				if r.MultipartForm != nil {
					defer r.MultipartForm.RemoveAll()
				}
			} else if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form", http.StatusBadRequest)
				return
			}
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("csrf-token")
			}
			if cookie == nil || len(token) != 64 || !hmac.Equal([]byte(token), []byte(cookie.Value)) {
				http.Error(w, "Please reload the page before submitting this form", http.StatusForbidden)
				return
			}
			if r.URL.Path == "/login" || r.URL.Path == "/register" || r.URL.Path == "/admin-login" || r.URL.Path == "/forgot-password" {
				ip, _, _ := net.SplitHostPort(r.RemoteAddr)
				authRates.Lock()
				for key, value := range authRates.entries {
					if time.Since(value.since) > time.Minute {
						delete(authRates.entries, key)
					}
				}
				entry := authRates.entries[ip]
				if entry.since.IsZero() {
					entry.since = time.Now()
				}
				entry.count++
				authRates.entries[ip] = entry
				authRates.Unlock()
				if entry.count > 20 {
					w.Header().Set("Retry-After", "60")
					http.Error(w, "Please wait a minute before trying again", http.StatusTooManyRequests)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func handlePrivateReceipt(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	filename := strings.TrimPrefix(r.URL.Path, "/uploads/")
	if filename == "" || filepath.Base(filename) != filename {
		http.NotFound(w, r)
		return
	}
	admin, _ := store.Get(r, "admin-session")
	if admin == nil || admin.Values["admin_id"] == nil {
		customer, _ := store.Get(r, "customer-session")
		if customer == nil || customer.Values["customer_id"] == nil {
			http.Error(w, "Please sign in", http.StatusUnauthorized)
			return
		}
		var owned int
		err := db.QueryRow(`SELECT COUNT(*) FROM payments JOIN orders ON payments.order_id = orders.id WHERE receipt = ? AND orders.customer_id = ?`, filename, customer.Values["customer_id"]).Scan(&owned)
		if err != nil || owned == 0 {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Disposition", "attachment")
	http.ServeFile(w, r, filepath.Join("uploads", filename))
}
