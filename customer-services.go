package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
)

func initCustomerServices(db *sql.DB) error {
	_, err := db.Exec(`
 CREATE TABLE IF NOT EXISTS shipment_events(id INTEGER PRIMARY KEY AUTOINCREMENT,order_id INTEGER NOT NULL REFERENCES orders(id),stage TEXT NOT NULL,location TEXT NOT NULL,note TEXT NOT NULL,actor_id INTEGER NOT NULL,actor_name TEXT NOT NULL,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE INDEX IF NOT EXISTS shipment_order_idx ON shipment_events(order_id,id);
 CREATE TABLE IF NOT EXISTS support_conversations(id INTEGER PRIMARY KEY AUTOINCREMENT,customer_id INTEGER NOT NULL UNIQUE REFERENCES customers(id),status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','resolved')),customer_read_id INTEGER NOT NULL DEFAULT 0,staff_read_id INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS support_messages(id INTEGER PRIMARY KEY AUTOINCREMENT,conversation_id INTEGER NOT NULL REFERENCES support_conversations(id),sender_kind TEXT NOT NULL CHECK(sender_kind IN ('customer','staff')),sender_id INTEGER NOT NULL,sender_name TEXT NOT NULL,body TEXT NOT NULL CHECK(length(body)>0 AND length(body)<=2000),order_id INTEGER REFERENCES orders(id),client_id TEXT NOT NULL,created_at INTEGER NOT NULL,UNIQUE(conversation_id,sender_kind,sender_id,client_id));
 CREATE INDEX IF NOT EXISTS support_message_idx ON support_messages(conversation_id,id);
 CREATE INDEX IF NOT EXISTS support_sender_idx ON support_messages(sender_kind,sender_id,created_at);
 CREATE TABLE IF NOT EXISTS support_presence(staff_id INTEGER PRIMARY KEY REFERENCES admins(id),last_seen INTEGER NOT NULL);`)
	return err
}
func registerCustomerServices(db *sql.DB) {
	http.HandleFunc("/shipment-tracking", func(w http.ResponseWriter, r *http.Request) { serveShipmentPage(db, w, r) })
	http.HandleFunc("/api/shipment", func(w http.ResponseWriter, r *http.Request) { shipmentAPI(db, w, r) })
	http.HandleFunc("/admin/shipment/update", func(w http.ResponseWriter, r *http.Request) { recordShipmentCheckpoint(db, w, r) })
	http.HandleFunc("/support.html", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", 405)
			return
		}
		_, err := serviceCustomer(db, r)
		renderServicePage(w, "support.html", struct{ ShowroomData }{ShowroomData{SignedIn: err == nil, Page: "support"}})
	})
	http.HandleFunc("/admin/support", func(w http.ResponseWriter, r *http.Request) { serveSupportInbox(db, w, r) })
	http.HandleFunc("/api/support", func(w http.ResponseWriter, r *http.Request) { customerSupportAPI(db, w, r, "get") })
	http.HandleFunc("/api/support/send", func(w http.ResponseWriter, r *http.Request) { customerSupportAPI(db, w, r, "send") })
	http.HandleFunc("/api/support/read", func(w http.ResponseWriter, r *http.Request) { customerSupportAPI(db, w, r, "read") })
	for _, action := range []string{"get", "send", "read", "status", "presence"} {
		path := "/api/admin/support/" + action
		if action == "get" {
			path = "/api/admin/support"
		}
		http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) { staffSupportAPI(db, w, r, action) })
	}
}
func serviceJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
func serviceError(w http.ResponseWriter, status int, message string) {
	serviceJSON(w, status, map[string]string{"error": message})
}
func renderServicePage(w http.ResponseWriter, file string, data any) {
	t, err := template.New(file).Funcs(template.FuncMap{"statusLabel": statusLabel}).ParseFiles(file, "partials.html", "admin-partials.html")
	if err != nil {
		http.Error(w, "Could not load page", 500)
		return
	}
	var output bytes.Buffer
	if err = t.Execute(&output, data); err != nil {
		http.Error(w, "Could not render page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(output.Bytes())
}

type ServiceCustomer struct {
	ID          int64
	Name, Email string
}

func serviceCustomer(db *sql.DB, r *http.Request) (ServiceCustomer, error) {
	session, err := store.Get(r, "customer-session")
	if err != nil {
		return ServiceCustomer{}, errAccess
	}
	id, ok := session.Values["customer_id"].(int64)
	if !ok || id < 1 {
		return ServiceCustomer{}, errAccess
	}
	var c ServiceCustomer
	err = db.QueryRow("SELECT id,name,email FROM customers WHERE id=?", id).Scan(&c.ID, &c.Name, &c.Email)
	return c, err
}
func serviceQueryID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
}
