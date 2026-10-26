package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"
	_ "time/tzdata"
)

// Assigned once at startup. Tests can use isolated database helpers directly.
var showroomDB *sql.DB

func initBusiness(db *sql.DB) error {
	_, err := db.Exec(`
 CREATE TABLE IF NOT EXISTS account_recovery_requests(customer_id INTEGER PRIMARY KEY REFERENCES customers(id),requested_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE IF NOT EXISTS account_recovery_links(token_hash TEXT PRIMARY KEY,customer_id INTEGER NOT NULL REFERENCES customers(id),expires_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS customer_auth_epochs(customer_id INTEGER PRIMARY KEY REFERENCES customers(id),epoch INTEGER NOT NULL DEFAULT 0);
 CREATE TABLE IF NOT EXISTS fleet_vehicles(id TEXT PRIMARY KEY,payload TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('Draft','Available','Reserved','Sold')),updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE IF NOT EXISTS vehicle_assets(id TEXT PRIMARY KEY,vehicle_id TEXT NOT NULL REFERENCES fleet_vehicles(id),name TEXT NOT NULL,media_type TEXT NOT NULL,kind TEXT NOT NULL,public INTEGER NOT NULL DEFAULT 0,storage_key TEXT NOT NULL,size INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS delivery_rates(id INTEGER PRIMARY KEY,body TEXT NOT NULL DEFAULT 'All',origin TEXT NOT NULL DEFAULT 'Lagos',country TEXT NOT NULL DEFAULT 'Nigeria',state TEXT NOT NULL,city TEXT NOT NULL DEFAULT '',delivery INTEGER NOT NULL CHECK(delivery>=0),fees INTEGER NOT NULL CHECK(fees>=0),insurance_bps INTEGER NOT NULL DEFAULT 0,duty_bps INTEGER NOT NULL DEFAULT 0,vat_bps INTEGER NOT NULL DEFAULT 0,insurance INTEGER NOT NULL DEFAULT 0,taxes INTEGER NOT NULL DEFAULT 0,port_fees INTEGER NOT NULL DEFAULT 0,label TEXT NOT NULL,source TEXT NOT NULL DEFAULT '',notes TEXT NOT NULL DEFAULT '',active INTEGER NOT NULL DEFAULT 1,demo INTEGER NOT NULL DEFAULT 0,complete INTEGER NOT NULL DEFAULT 0,estimated INTEGER NOT NULL DEFAULT 1,expires TEXT NOT NULL DEFAULT '',UNIQUE(body,origin,country,state,city));
 CREATE UNIQUE INDEX IF NOT EXISTS delivery_route_casefold ON delivery_rates(lower(body),lower(origin),lower(country),lower(state),lower(city));
 CREATE TABLE IF NOT EXISTS order_pricing(order_id INTEGER PRIMARY KEY REFERENCES orders(id),vehicle_id TEXT NOT NULL,quote TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS visit_slots(id INTEGER PRIMARY KEY,vehicle_id TEXT NOT NULL DEFAULT '',kind TEXT NOT NULL CHECK(kind IN ('test-drive','inspection')),starts_at TEXT NOT NULL,ends_at TEXT NOT NULL,location TEXT NOT NULL,active INTEGER NOT NULL DEFAULT 1);
 CREATE TABLE IF NOT EXISTS appointments(id INTEGER PRIMARY KEY,slot_id INTEGER NOT NULL REFERENCES visit_slots(id),customer_id INTEGER NOT NULL REFERENCES customers(id),vehicle_id TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'requested' CHECK(status IN ('requested','confirmed','completed','cancelled')),note TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE UNIQUE INDEX IF NOT EXISTS appointment_slot_active ON appointments(slot_id) WHERE status!='cancelled';
 CREATE TABLE IF NOT EXISTS appointment_notifications(id INTEGER PRIMARY KEY,customer_id INTEGER NOT NULL,appointment_id INTEGER NOT NULL,kind TEXT NOT NULL,message TEXT NOT NULL,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(appointment_id,kind));
 CREATE TABLE IF NOT EXISTS sales_followups(enquiry_id INTEGER PRIMARY KEY REFERENCES enquiries(id),stage TEXT NOT NULL DEFAULT 'New',assigned_to INTEGER,followup TEXT NOT NULL DEFAULT '',note TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE IF NOT EXISTS support_cases(conversation_id INTEGER PRIMARY KEY REFERENCES support_conversations(id),status TEXT NOT NULL DEFAULT 'open',assigned_to INTEGER,updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE IF NOT EXISTS dealership_settings(id INTEGER PRIMARY KEY CHECK(id=1),payload TEXT NOT NULL);
 INSERT OR IGNORE INTO dealership_settings(id,payload) VALUES(1,'{"Name":"Marvellous Motors"}');
 CREATE TABLE IF NOT EXISTS customer_reviews(id INTEGER PRIMARY KEY,customer_id INTEGER NOT NULL REFERENCES customers(id),order_id INTEGER NOT NULL UNIQUE REFERENCES orders(id),rating INTEGER NOT NULL CHECK(rating BETWEEN 1 AND 5),body TEXT NOT NULL,consent INTEGER NOT NULL CHECK(consent=1),status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','published','hidden')),created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 `)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, v := range vehicles {
		v.StockStatus = "Available"
		v.Sample = true
		v.PreviousOwners = -1
		raw, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if _, e = tx.Exec("INSERT OR IGNORE INTO fleet_vehicles(id,payload,status) VALUES(?,?,'Available')", v.ID, string(raw)); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func registerBusiness(db *sql.DB) {
	routes := map[string]http.HandlerFunc{
		"/admin/recovery":        func(w http.ResponseWriter, r *http.Request) { manageRecovery(db, w, r) },
		"/admin/inventory":       func(w http.ResponseWriter, r *http.Request) { manageInventory(db, w, r) },
		"/media/vehicle":         func(w http.ResponseWriter, r *http.Request) { serveVehicleAsset(db, w, r) },
		"/admin/pricing":         func(w http.ResponseWriter, r *http.Request) { managePricing(db, w, r) },
		"/api/quote":             func(w http.ResponseWriter, r *http.Request) { serveQuoteAPI(db, w, r) },
		"/quote":                 func(w http.ResponseWriter, r *http.Request) { serveQuotePage(db, w, r) },
		"/admin/appointments":    func(w http.ResponseWriter, r *http.Request) { manageAppointments(db, w, r) },
		"/appointments":          func(w http.ResponseWriter, r *http.Request) { customerAppointments(db, w, r) },
		"/appointments/calendar": func(w http.ResponseWriter, r *http.Request) { appointmentCalendar(db, w, r) },
		"/admin/sales":           func(w http.ResponseWriter, r *http.Request) { manageSales(db, w, r) },
		"/admin/dealership":      func(w http.ResponseWriter, r *http.Request) { manageDealership(db, w, r) },
		"/dealership":            func(w http.ResponseWriter, r *http.Request) { serveDealership(db, w, r) },
		"/account/reviews":       func(w http.ResponseWriter, r *http.Request) { customerReview(db, w, r) },
		"/admin/orders/cancel":   func(w http.ResponseWriter, r *http.Request) { cancelVehicleOrder(db, w, r) },
	}
	for path, handler := range routes {
		http.HandleFunc(path, handler)
	}
}
func operationsData(db *sql.DB, w http.ResponseWriter, r *http.Request, page, title, subtitle string) (AdminPage, bool) {
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return AdminPage{}, false
	}
	metrics, err := loadMetrics(db)
	if err != nil {
		http.Error(w, "Could not load workspace", 500)
		return AdminPage{}, false
	}
	d := AdminPage{CollectionOptions: []string{"Everyday", "Sports", "Luxury", "Classic", "Limited Edition"}, SalesStages: []string{"New", "Contacted", "Appointment", "Negotiating", "Sold"}, Staff: staff, Metrics: metrics, Page: page, Title: title, Subtitle: subtitle}
	if r.URL.Query().Get("saved") == "1" {
		d.Notice = "Your changes have been saved."
	}
	return d, true
}
func renderBusinessPage(w http.ResponseWriter, status int, file string, data any) {
	t, err := template.New(file).Funcs(template.FuncMap{"money": formatAmount, "statusLabel": statusLabel}).ParseFiles(file, "partials.html")
	if err != nil {
		log.Println(err)
		http.Error(w, "Could not load page", 500)
		return
	}
	var b bytes.Buffer
	if err = t.Execute(&b, data); err != nil {
		log.Println(err)
		http.Error(w, "Could not render page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(b.Bytes())
}
func dealershipTime() *time.Location {
	location, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		return time.FixedZone("WAT", 3600)
	}
	return location
}
