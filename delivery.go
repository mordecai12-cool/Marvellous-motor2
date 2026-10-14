package main

import (
	"bytes"
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"strconv"
)

type CustomerDeliveryPage struct {
	ShowroomData
	Order   ManagedOrder
	Orders  []ManagedOrder
	Pending bool
}

func renderCustomerDelivery(w http.ResponseWriter, file string, data CustomerDeliveryPage) {
	t, err := template.New(file).Funcs(template.FuncMap{"statusReached": statusReached, "statusLabel": statusLabel}).ParseFiles(file, "partials.html", "delivery-partials.html")
	if err != nil {
		log.Println(err)
		http.Error(w, "Could not load delivery page", 500)
		return
	}
	var output bytes.Buffer
	if err = t.Execute(&output, data); err != nil {
		log.Println(err)
		http.Error(w, "Could not render delivery page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(output.Bytes())
}
func serveCustomerDelivery(db *sql.DB, w http.ResponseWriter, r *http.Request, pending bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	session, _ := store.Get(r, "customer-session")
	if session == nil || session.Values["customer_id"] == nil {
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("order-id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	orders, err := loadOrders(db, "WHERE o.id=? AND o.customer_id=?", id, session.Values["customer_id"])
	if err != nil {
		log.Println(err)
		http.Error(w, "Could not load delivery", 500)
		return
	}
	if len(orders) == 0 {
		http.NotFound(w, r)
		return
	}
	order := orders[0]
	order.Events, err = loadOrderEvents(db, id)
	if err != nil {
		http.Error(w, "Could not load updates", 500)
		return
	}
	renderCustomerDelivery(w, "order-status.html", CustomerDeliveryPage{ShowroomData: ShowroomData{SignedIn: true}, Order: order, Pending: pending})
}
func serveCustomerOrders(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	session, _ := store.Get(r, "customer-session")
	if session == nil || session.Values["customer_id"] == nil {
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	orders, err := loadOrders(db, "WHERE o.customer_id=?", session.Values["customer_id"])
	if err != nil {
		http.Error(w, "Could not load orders", 500)
		return
	}
	renderCustomerDelivery(w, "customer-orders.html", CustomerDeliveryPage{ShowroomData: ShowroomData{SignedIn: true}, Orders: orders})
}

// Let a customer submit corrected evidence without creating another order.
func serveCustomerPayment(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	session, _ := store.Get(r, "customer-session")
	if session == nil || session.Values["customer_id"] == nil {
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("order-id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	orders, err := loadOrders(db, "WHERE o.id=? AND o.customer_id=?", id, session.Values["customer_id"])
	if err != nil {
		http.Error(w, "Could not load order", 500)
		return
	}
	if len(orders) == 0 {
		http.NotFound(w, r)
		return
	}
	o := orders[0]
	if o.Status != "payment_pending" {
		http.Redirect(w, r, "/order-status?order-id="+strconv.FormatInt(id, 10), 303)
		return
	}
	v := o.Vehicle
	if v.ID == "" {
		v = Vehicle{Brand: o.CarName, Price: int(o.CarPrice)}
	}
	data := OrderData{ShowroomData: ShowroomData{SignedIn: true}, Vehicle: v, OrderID: o.ID, Name: o.Name, Email: o.Email, Phone: o.Phone, Address: o.Address, City: o.City, State: o.State}
	t, err := template.ParseFiles("payment.html", "partials.html")
	if err != nil {
		http.Error(w, "Could not load payment page", 500)
		return
	}
	var output bytes.Buffer
	if err = t.Execute(&output, data); err != nil {
		http.Error(w, "Could not render payment page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(output.Bytes())
}
