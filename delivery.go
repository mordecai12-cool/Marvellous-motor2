package main

import (
	"bytes"
	"database/sql"
	"errors"
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
	Payment CustomerPayment
}

type CustomerPayment struct {
	ID                                    int64
	Amount                                float64
	Reference, Receipt, Status, CreatedAt string
}

func (p CustomerPayment) AmountLabel() string { return formatAmount(p.Amount) }
func (p CustomerPayment) DateLabel() string   { return displayTime(p.CreatedAt) }
func (p CustomerPayment) StatusLabel() string {
	if p.ID == 0 {
		return "Evidence not submitted"
	}
	if p.Status == "rejected" {
		return "Correction requested"
	}
	return statusLabel(p.Status)
}
func (p CustomerDeliveryPage) Heading() string {
	switch p.Order.Status {
	case "delivered":
		return "Your vehicle has arrived."
	case "out_for_delivery":
		return "Your next drive is on its way."
	case "processing":
		return "Payment approved. Next stop: preparation."
	}
	if p.Payment.ID == 0 {
		return "Your order is in. Let's complete the next step."
	}
	if p.Payment.Status == "rejected" {
		return "A quick payment update is needed."
	}
	return "Receipt received. We'll take it from here."
}
func (p CustomerDeliveryPage) Message() string {
	if p.Order.Status != "payment_pending" {
		return p.Order.NextStep()
	}
	if p.Payment.ID == 0 {
		return "Submit your payment reference and receipt so our team can review your order. Delivery begins after approval."
	}
	if p.Payment.Status == "rejected" {
		return "Our team requested a correction. Check the update below and upload corrected payment evidence for review."
	}
	return "Your receipt is saved and awaiting manual review. You can follow your order here; payment has not yet been approved."
}
func (p CustomerDeliveryPage) NeedsEvidence() bool {
	return p.Order.Status == "payment_pending" && (p.Payment.ID == 0 || p.Payment.Status == "rejected")
}

func renderCustomerDelivery(w http.ResponseWriter, file string, data CustomerDeliveryPage) {
	files := []string{file, "partials.html", "delivery-partials.html"}
	if file == "payment-pending.html" {
		files = append(files, "order-status.html")
	}
	t, err := template.New(file).Funcs(template.FuncMap{"statusReached": statusReached, "statusLabel": statusLabel}).ParseFiles(files...)
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
	var payment CustomerPayment
	err = db.QueryRow("SELECT id,amount_paid,payment_reference,receipt,payment_status,CAST(created_at AS TEXT) FROM payments WHERE order_id=? ORDER BY id DESC LIMIT 1", id).Scan(&payment.ID, &payment.Amount, &payment.Reference, &payment.Receipt, &payment.Status, &payment.CreatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Could not load payment summary", 500)
		return
	}
	file := "order-status.html"
	if pending {
		file = "payment-pending.html"
	}
	renderCustomerDelivery(w, file, CustomerDeliveryPage{ShowroomData: ShowroomData{SignedIn: true}, Order: order, Payment: payment, Pending: pending})
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
	data.Bank, err = loadBankAccount(db)
	if err != nil {
		http.Error(w, "Could not load payment details", 500)
		return
	}
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
