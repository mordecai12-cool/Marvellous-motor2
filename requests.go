package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
)

var requestKinds = map[string]string{
	"import": "Vehicle sourcing", "inspection": "Vehicle inspection", "finance": "Finance enquiry", "test-drive": "Test drive", "trade-in": "Trade-in enquiry", "dealer": "Dealer application", "support": "Customer support",
}

type CustomerRequest struct {
	ID                                                                            int64
	Kind, Name, Email, Phone, Vehicle, Market, Budget, Message, Status, CreatedAt string
}

type RequestPage struct {
	ShowroomData
	Kind, Title string
	ID          int64
	Requests    []CustomerRequest
}

func renderRequests(w http.ResponseWriter, r *http.Request, file string, data RequestPage) {
	session, _ := store.Get(r, "customer-session")
	data.SignedIn = session != nil && session.Values["customer_id"] != nil
	t, err := template.ParseFiles(file, "partials.html")
	if err != nil {
		http.Error(w, "Could not load page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		fmt.Println("Request template:", err)
	}
}

func initRequestsDB(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS enquiries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		customer_id INTEGER,
		kind TEXT NOT NULL, name TEXT NOT NULL, email TEXT NOT NULL, phone TEXT NOT NULL DEFAULT '',
		vehicle TEXT NOT NULL DEFAULT '', market TEXT NOT NULL DEFAULT '', budget TEXT NOT NULL DEFAULT '',
		message TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new','reviewing','responded','closed')),
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS enquiries_customer_idx ON enquiries(customer_id);
	CREATE TABLE IF NOT EXISTS enquiry_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT, enquiry_id INTEGER NOT NULL REFERENCES enquiries(id),
		admin_id INTEGER NOT NULL, previous_status TEXT NOT NULL, next_status TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`)
	return err
}

func registerRequestRoutes(db *sql.DB) {
	if err := initRequestsDB(db); err != nil {
		panic(err)
	}
	http.HandleFunc("/request", func(w http.ResponseWriter, r *http.Request) { handleRequest(db, w, r) })
	http.HandleFunc("/my-requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", 405)
			return
		}
		session, _ := store.Get(r, "customer-session")
		if session == nil || session.Values["customer_id"] == nil {
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		}
		requests, err := readRequests(db, "WHERE customer_id = ?", session.Values["customer_id"])
		if err != nil {
			http.Error(w, "Could not load requests", 500)
			return
		}
		renderRequests(w, r, "requests-list.html", RequestPage{Title: "Your requests", Requests: requests})
	})
	http.HandleFunc("/admin/enquiries", func(w http.ResponseWriter, r *http.Request) {
		staff, ok := requireStaff(db, w, r)
		if !ok {
			return
		}

		if r.Method == http.MethodPost {
			id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
			status := r.FormValue("status")
			if err != nil || id < 1 || (status != "new" && status != "reviewing" && status != "responded" && status != "closed") {
				http.Error(w, "Invalid request update", 400)
				return
			}
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Could not update request", 500)
				return
			}
			defer tx.Rollback()
			var previous string
			if err = tx.QueryRow("SELECT status FROM enquiries WHERE id = ?", id).Scan(&previous); err != nil {
				http.NotFound(w, r)
				return
			}
			if _, err = tx.Exec("UPDATE enquiries SET status = ? WHERE id = ?", status, id); err != nil {
				http.Error(w, "Could not update request", 500)
				return
			}
			if _, err = tx.Exec("INSERT INTO enquiry_events(enquiry_id,admin_id,previous_status,next_status) VALUES(?,?,?,?)", id, staff.ID, previous, status); err != nil {
				http.Error(w, "Could not record update", 500)
				return
			}
			if err = addActivity(tx, staff, "Enquiry status updated", fmt.Sprintf("Request #%d · %s", id, status)); err != nil {
				http.Error(w, "Could not record update", 500)
				return
			}
			if err = tx.Commit(); err != nil {
				http.Error(w, "Could not save update", 500)
				return
			}
			http.Redirect(w, r, "/admin/enquiries", http.StatusSeeOther)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", 405)
			return
		}
		requests, err := readRequests(db, "")
		if err != nil {
			http.Error(w, "Could not load enquiries", 500)
			return
		}
		metrics, err := loadMetrics(db)
		if err != nil {
			http.Error(w, "Could not load operations", 500)
			return
		}
		renderAdmin(w, "admin-enquiries.html", AdminPage{Staff: staff, Page: "enquiries", Title: "Every conversation matters.", Subtitle: "Review sourcing, dealer applications, and customer enquiries.", Metrics: metrics, Requests: requests})
	})
}

func readRequests(db *sql.DB, where string, args ...any) ([]CustomerRequest, error) {
	rows, err := db.Query("SELECT id, kind, name, email, phone, vehicle, market, budget, message, status, created_at FROM enquiries "+where+" ORDER BY id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []CustomerRequest{}
	for rows.Next() {
		var v CustomerRequest
		if err := rows.Scan(&v.ID, &v.Kind, &v.Name, &v.Email, &v.Phone, &v.Vehicle, &v.Market, &v.Budget, &v.Message, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Kind = requestKinds[v.Kind]
		requests = append(requests, v)
	}
	return requests, rows.Err()
}

func handleRequest(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	kind := r.URL.Query().Get("kind")
	if r.Method == http.MethodPost {
		kind = r.FormValue("kind")
	}
	if kind == "" {
		kind = "support"
	}
	title, ok := requestKinds[kind]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data := RequestPage{Kind: kind, Title: title}
	if id := r.URL.Query().Get("vehicle"); id != "" {
		data.Vehicle, _ = findVehicle(id)
	}
	if r.Method == http.MethodGet {
		renderRequests(w, r, "request.html", data)
		return
	}
	name, email, phone := strings.TrimSpace(r.FormValue("name")), strings.TrimSpace(r.FormValue("email")), strings.TrimSpace(r.FormValue("phone"))
	message, vehicle, market, budget := strings.TrimSpace(r.FormValue("message")), strings.TrimSpace(r.FormValue("vehicle")), strings.TrimSpace(r.FormValue("market")), strings.TrimSpace(r.FormValue("budget"))
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email || len(name) < 2 || len(name) > 100 || len(email) > 254 || len(phone) > 30 || len(message) < 10 || len(message) > 3000 || len(vehicle) > 200 || len(market) > 80 || len(budget) > 100 {
		http.Error(w, "Please provide a valid name, email and message (10–3,000 characters)", 400)
		return
	}
	var customerID any
	session, _ := store.Get(r, "customer-session")
	if session != nil {
		customerID = session.Values["customer_id"]
	}
	result, err := db.Exec("INSERT INTO enquiries(customer_id,kind,name,email,phone,vehicle,market,budget,message) VALUES(?,?,?,?,?,?,?,?,?)", customerID, kind, name, email, phone, vehicle, market, budget, message)
	if err != nil {
		http.Error(w, "Could not save request", 500)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		http.Error(w, "Could not confirm request", 500)
		return
	}
	data.ID = id
	renderRequests(w, r, "request-success.html", data)
}
