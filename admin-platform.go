package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Staff struct {
	ID                        int64
	Name, Email, Role, Status string
}

func (s Staff) Initials() string {
	parts := strings.Fields(s.Name)
	result := ""
	for i, p := range parts {
		if i > 1 {
			break
		}
		r := []rune(p)
		if len(r) > 0 {
			result += strings.ToUpper(string(r[0]))
		}
	}
	return result
}
func (s Staff) RoleLabel() string {
	if s.Role == "super_admin" {
		return "Platform owner"
	}
	return "Administrator"
}

type staffContextKey struct{}

var errAccess = errors.New("approved staff account required")

func currentStaff(db *sql.DB, r *http.Request) (Staff, error) {
	if s, ok := r.Context().Value(staffContextKey{}).(Staff); ok {
		return s, nil
	}
	session, err := store.Get(r, "admin-session")
	if err != nil {
		return Staff{}, errAccess
	}
	id, ok := session.Values["admin_id"].(int64)
	if !ok {
		return Staff{}, errAccess
	}
	var s Staff
	err = db.QueryRow("SELECT id,name,email,role,status FROM admins WHERE id = ?", id).Scan(&s.ID, &s.Name, &s.Email, &s.Role, &s.Status)
	if err != nil || !authorizedStaff(s) {
		return Staff{}, errAccess
	}
	return s, nil
}
func requireStaff(db *sql.DB, w http.ResponseWriter, r *http.Request) (Staff, bool) {
	s, err := currentStaff(db, r)
	if err != nil {
		if r.Method == http.MethodGet {
			http.Redirect(w, r, "/admin-login.html", http.StatusSeeOther)
		} else {
			http.Error(w, "Approved administrator access required", http.StatusForbidden)
		}
		return Staff{}, false
	}
	return s, true
}
func adminAccessMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/admin-request" || p == "/admin-request.html" || p == "/admin-request-status" || p == "/admin-request-status.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			renderAdminLogin(w, r, "registration-closed")
			return
		}
		if p == "/admin.html" || strings.HasPrefix(p, "/admin/") || p == "/admin-requests" || strings.HasPrefix(p, "/admin-requests/") {
			staff, ok := requireStaff(db, w, r)
			if !ok {
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), staffContextKey{}, staff))
		}
		next.ServeHTTP(w, r)
	})
}

type DeliveryDetails struct{ ScheduledDate, Partner, Driver, DriverPhone, Reference, Note string }
type OrderEvent struct {
	ID                                               int64
	FromStatus, ToStatus, Note, ActorName, CreatedAt string
}

func (e OrderEvent) Label() string     { return statusLabel(e.ToStatus) }
func (e OrderEvent) TimeLabel() string { return displayTime(e.CreatedAt) }

type ManagedOrder struct {
	ID, CustomerID                                                       int64
	Name, Email, Phone, Address, City, State, CarName, Status, CreatedAt string
	CarPrice                                                             float64
	Vehicle                                                              Vehicle
	Delivery                                                             DeliveryDetails
	Events                                                               []OrderEvent
	PaymentCount                                                         int
}

func (o ManagedOrder) PriceLabel() string  { return formatAmount(o.CarPrice) }
func (o ManagedOrder) StatusLabel() string { return statusLabel(o.Status) }
func (o ManagedOrder) DateLabel() string   { return displayTime(o.CreatedAt) }
func (o ManagedOrder) ScheduledLabel() string {
	if o.Delivery.ScheduledDate == "" {
		return "Not scheduled yet"
	}
	t, e := time.Parse("2006-01-02", o.Delivery.ScheduledDate)
	if e != nil {
		return o.Delivery.ScheduledDate
	}
	return t.Format("02 Jan 2006")
}
func (o ManagedOrder) Destination() string { return o.City + ", " + o.State }
func (o ManagedOrder) NextStep() string {
	switch o.Status {
	case "payment_pending":
		return "Your order is received. Our team will review your payment evidence before preparing delivery."
	case "processing":
		return "Your payment has been reviewed. Our operations team is preparing your vehicle and delivery arrangements."
	case "out_for_delivery":
		return "Your vehicle has been dispatched. Follow the delivery details and updates from our team below."
	case "delivered":
		return "Your delivery has been marked complete by our operations team. Thank you for choosing Marvellous Motors."
	}
	return "Our operations team will update this order."
}
func statusLabel(status string) string {
	switch status {
	case "payment_pending":
		return "Awaiting review"
	case "processing":
		return "Preparing vehicle"
	case "out_for_delivery":
		return "Out for delivery"
	case "delivered":
		return "Delivered"
	case "pending":
		return "Pending review"
	case "verified":
		return "Approved"
	case "rejected":
		return "Rejected"
	case "approved":
		return "Active"
	case "disabled":
		return "Disabled"
	}
	return strings.Title(strings.ReplaceAll(status, "_", " "))
}
func displayTime(raw string) string {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z"} {
		if t, e := time.Parse(layout, raw); e == nil {
			loc, _ := time.LoadLocation("Africa/Casablanca")
			if loc != nil {
				t = t.In(loc)
			}
			return t.Format("02 Jan 2006 · 15:04")
		}
	}
	return raw
}

type PaymentReview struct {
	ID, OrderID                                                               int64
	Amount                                                                    float64
	Reference, Receipt, Status, OrderStatus, CustomerName, CarName, CreatedAt string
	Price                                                                     float64
}

func (p PaymentReview) AmountLabel() string { return formatAmount(p.Amount) }
func (p PaymentReview) PriceLabel() string  { return formatAmount(p.Price) }
func (p PaymentReview) StatusLabel() string { return statusLabel(p.Status) }
func (p PaymentReview) CoversTotal() bool   { return math.Round(p.Amount*100) >= math.Round(p.Price*100) }

type CustomerOverview struct {
	ID                            int64
	Name, Email, Phone, CreatedAt string
	Orders                        int
	Total                         float64
}

func (c CustomerOverview) IDLabel() string    { return fmt.Sprintf("MM-C%06d", c.ID) }
func (c CustomerOverview) TotalLabel() string { return formatAmount(c.Total) }

type Activity struct {
	ID                               int64
	Actor, Action, Entity, CreatedAt string
}

func (a Activity) TimeLabel() string { return displayTime(a.CreatedAt) }

type AdminMetrics struct {
	Orders, PendingPayments, Preparing, OnRoad, Delivered, Enquiries, Customers int
	ReviewedTotal                                                               float64
}

func (m AdminMetrics) TotalLabel() string { return formatAmount(m.ReviewedTotal) }

type AdminPage struct {
	Staff                                               Staff
	Page, Title, Subtitle, Notice, Error, Query, Filter string
	Metrics                                             AdminMetrics
	Orders                                              []ManagedOrder
	Order                                               ManagedOrder
	Payments                                            []PaymentReview
	Customers                                           []CustomerOverview
	Requests                                            []CustomerRequest
	Team                                                []Staff
	Invitations                                         []StaffInvitation
	InviteURL, InviteEmail                              string
	Shipment                                            ShipmentSnapshot
	OwnerOnly                                           bool
	SecondOwnerEmail                                    string
	Activity                                            []Activity
}

func initAdminPlatform(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS delivery_details (
 order_id INTEGER PRIMARY KEY REFERENCES orders(id),scheduled_date TEXT NOT NULL DEFAULT '',partner TEXT NOT NULL DEFAULT '',driver TEXT NOT NULL DEFAULT '',driver_phone TEXT NOT NULL DEFAULT '',reference TEXT NOT NULL DEFAULT '',note TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE IF NOT EXISTS order_events(id INTEGER PRIMARY KEY AUTOINCREMENT,order_id INTEGER NOT NULL REFERENCES orders(id),from_status TEXT NOT NULL,to_status TEXT NOT NULL,note TEXT NOT NULL,actor_id INTEGER NOT NULL,actor_name TEXT NOT NULL,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE INDEX IF NOT EXISTS order_events_order_idx ON order_events(order_id,id);
 CREATE TABLE IF NOT EXISTS admin_activity(id INTEGER PRIMARY KEY AUTOINCREMENT,actor_id INTEGER NOT NULL,actor_name TEXT NOT NULL,action TEXT NOT NULL,entity TEXT NOT NULL,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE IF NOT EXISTS staff_invitations(id INTEGER PRIMARY KEY AUTOINCREMENT,email TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,created_by INTEGER NOT NULL REFERENCES admins(id),expires_at INTEGER NOT NULL,used_at INTEGER,revoked_at INTEGER,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);`)
	if err != nil {
		return err
	}
	if err = initOwnerSignup(db); err != nil {
		return err
	}
	return initCustomerServices(db)
}
func registerAdminPlatform(db *sql.DB) {
	if err := initAdminPlatform(db); err != nil {
		log.Fatal(err)
	}
	registerCustomerServices(db)
	http.HandleFunc("/admin-login.html", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", 405)
			return
		}
		renderAdminLogin(w, r, r.URL.Query().Get("error"))
	})
	for _, page := range []string{"deliveries", "order", "customers", "team", "activity"} {
		page := page
		http.HandleFunc("/admin/"+page, func(w http.ResponseWriter, r *http.Request) { serveAdminPage(db, w, r, page) })
	}
	http.HandleFunc("/admin/delivery/save", func(w http.ResponseWriter, r *http.Request) { saveDelivery(db, w, r) })
	http.HandleFunc("/admin/team/create", func(w http.ResponseWriter, r *http.Request) { manageTeam(db, w, r, "create") })
	http.HandleFunc("/admin/team/update", func(w http.ResponseWriter, r *http.Request) { manageTeam(db, w, r, "update") })
	http.HandleFunc("/admin/team/invite", func(w http.ResponseWriter, r *http.Request) { createStaffInvitation(db, w, r) })
	http.HandleFunc("/admin/team/revoke-invite", func(w http.ResponseWriter, r *http.Request) { revokeStaffInvitation(db, w, r) })
	http.HandleFunc("/admin-signup.html", func(w http.ResponseWriter, r *http.Request) { serveStaffSignup(db, w, r) })
	http.HandleFunc("/admin-signup", func(w http.ResponseWriter, r *http.Request) { serveStaffSignup(db, w, r) })
	http.HandleFunc("/admin/payments/reject", func(w http.ResponseWriter, r *http.Request) { reviewPayment(db, w, r, "rejected") })
}
func renderAdminLogin(w http.ResponseWriter, r *http.Request, code string) {
	message := ""
	if code == "invalid" {
		message = "We couldn't sign you in. Check your credentials and confirm your account is active."
	}
	if code == "registration-closed" {
		message = "Staff registration is closed. Only the platform owner can grant administrator access."
	}
	t, err := template.ParseFiles("admin-login.html")
	if err != nil {
		http.Error(w, "Could not load sign in", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, struct {
		Error     string
		Closed    bool
		Created   bool
		OwnerOnly bool
	}{message, code == "registration-closed", r.URL.Query().Get("notice") == "account-created", ownerOnlyAccess()})
}
func renderAdmin(w http.ResponseWriter, file string, data AdminPage) {
	funcs := template.FuncMap{"statusLabel": statusLabel, "statusReached": statusReached, "amount": formatAmount}
	t, err := template.New(file).Funcs(funcs).ParseFiles(file, "admin-partials.html")
	if err != nil {
		log.Println(err)
		http.Error(w, "Could not load workspace", 500)
		return
	}
	var output bytes.Buffer
	if err = t.Execute(&output, data); err != nil {
		log.Println(err)
		http.Error(w, "Could not render workspace", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(output.Bytes())
}
func loadMetrics(db *sql.DB) (m AdminMetrics, err error) {
	err = db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(status='processing'),0),COALESCE(SUM(status='out_for_delivery'),0),COALESCE(SUM(status='delivered'),0) FROM orders`).Scan(&m.Orders, &m.Preparing, &m.OnRoad, &m.Delivered)
	if err != nil {
		return
	}
	err = db.QueryRow("SELECT COUNT(*),COALESCE(SUM(CASE WHEN payment_status='verified' THEN amount_paid ELSE 0 END),0) FROM payments WHERE payment_status IN ('pending','verified')").Scan(&m.PendingPayments, &m.ReviewedTotal)
	if err != nil {
		return
	}
	err = db.QueryRow("SELECT COUNT(*) FROM payments WHERE payment_status='pending'").Scan(&m.PendingPayments)
	if err != nil {
		return
	}
	err = db.QueryRow("SELECT COUNT(*) FROM enquiries WHERE status IN ('new','reviewing')").Scan(&m.Enquiries)
	if err != nil {
		return
	}
	err = db.QueryRow("SELECT COUNT(*) FROM customers").Scan(&m.Customers)
	return
}

const managedOrderSelect = `SELECT o.id,COALESCE(o.customer_id,0),o.name,o.email,o.phone,o.address,o.city,o.state,COALESCE(o.car_name,'Vehicle order'),COALESCE(o.car_price,0),o.status,CAST(o.created_at AS TEXT),COALESCE(d.scheduled_date,''),COALESCE(d.partner,''),COALESCE(d.driver,''),COALESCE(d.driver_phone,''),COALESCE(d.reference,''),COALESCE(d.note,''),(SELECT COUNT(*) FROM payments p WHERE p.order_id=o.id) FROM orders o LEFT JOIN delivery_details d ON d.order_id=o.id `

func loadOrders(db *sql.DB, where string, args ...any) ([]ManagedOrder, error) {
	rows, err := db.Query(managedOrderSelect+where+" ORDER BY o.id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []ManagedOrder{}
	for rows.Next() {
		var o ManagedOrder
		if err = rows.Scan(&o.ID, &o.CustomerID, &o.Name, &o.Email, &o.Phone, &o.Address, &o.City, &o.State, &o.CarName, &o.CarPrice, &o.Status, &o.CreatedAt, &o.Delivery.ScheduledDate, &o.Delivery.Partner, &o.Delivery.Driver, &o.Delivery.DriverPhone, &o.Delivery.Reference, &o.Delivery.Note, &o.PaymentCount); err != nil {
			return nil, err
		}
		for _, v := range vehicles {
			if strings.Contains(o.CarName, v.Name()) && strings.Contains(o.CarName, strconv.Itoa(v.Year)) {
				o.Vehicle = v
				break
			}
		}
		list = append(list, o)
	}
	return list, rows.Err()
}
func loadOrderEvents(db *sql.DB, id int64) ([]OrderEvent, error) {
	rows, err := db.Query("SELECT id,from_status,to_status,note,actor_name,CAST(created_at AS TEXT) FROM order_events WHERE order_id=? ORDER BY id DESC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []OrderEvent{}
	for rows.Next() {
		var e OrderEvent
		if err = rows.Scan(&e.ID, &e.FromStatus, &e.ToStatus, &e.Note, &e.ActorName, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
func loadPayments(db *sql.DB) ([]PaymentReview, error) {
	rows, err := db.Query(`SELECT p.id,p.order_id,p.amount_paid,p.payment_reference,p.receipt,p.payment_status,o.status,o.name,COALESCE(o.car_name,'Vehicle order'),CAST(p.created_at AS TEXT),COALESCE(o.car_price,0) FROM payments p JOIN orders o ON o.id=p.order_id ORDER BY p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []PaymentReview{}
	for rows.Next() {
		var p PaymentReview
		if err = rows.Scan(&p.ID, &p.OrderID, &p.Amount, &p.Reference, &p.Receipt, &p.Status, &p.OrderStatus, &p.CustomerName, &p.CarName, &p.CreatedAt, &p.Price); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
func loadActivity(db *sql.DB) ([]Activity, error) {
	rows, err := db.Query("SELECT id,actor_name,action,entity,CAST(created_at AS TEXT) FROM admin_activity ORDER BY id DESC LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Activity{}
	for rows.Next() {
		var a Activity
		if err = rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Entity, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}
func serveAdminPage(db *sql.DB, w http.ResponseWriter, r *http.Request, page string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return
	}
	if page == "team" && staff.Role != "super_admin" {
		http.Error(w, "Only the platform owner can manage staff", 403)
		return
	}
	data := AdminPage{Staff: staff, Page: page, Query: r.URL.Query().Get("q"), Filter: r.URL.Query().Get("status")}
	data.OwnerOnly = ownerOnlyAccess()
	data.SecondOwnerEmail = deploymentSecondOwnerEmail
	var err error
	data.Metrics, err = loadMetrics(db)
	if err != nil {
		log.Println(err)
		http.Error(w, "Could not load operations", 500)
		return
	}
	notices := map[string]string{"saved": "Delivery details saved. The customer can see the latest update.", "dispatched": "Vehicle dispatched. The delivery timeline has been updated.", "delivered": "Delivery confirmed and recorded.", "approved": "Payment evidence approved. The vehicle is ready for preparation.", "rejected": "Payment evidence rejected. The customer can submit a corrected receipt.", "created": "Staff account created. Share the credentials privately.", "updated": "Staff access updated.", "invite-revoked": "Invitation revoked. The signup link can no longer be used.", "checkpoint": "Shipment checkpoint recorded. The customer tracker will refresh automatically."}
	data.Notice = notices[r.URL.Query().Get("notice")]
	files := map[string]string{"overview": "admin.html", "orders": "admin-orders.html", "deliveries": "admin-deliveries.html", "order": "admin-order-detail.html", "payments": "admin-payments.html", "customers": "admin-customers.html", "team": "admin-team.html", "activity": "admin-activity.html"}
	file := files[page]
	switch page {
	case "overview":
		data.Title = "Good things are moving."
		data.Subtitle = "Your showroom, your customers, and every next step. All in one place."
		data.Orders, err = loadOrders(db, "")
		if len(data.Orders) > 5 {
			data.Orders = data.Orders[:5]
		}
		if err == nil {
			data.Activity, err = loadActivity(db)
		}
	case "orders", "deliveries":
		data.Title = "Every order. One clear view."
		data.Subtitle = "Review customer orders and keep the next step moving."
		if page == "deliveries" {
			data.Title = "Deliver with confidence."
			data.Subtitle = "Plan, dispatch, and confirm every handover from one workspace."
		}
		where := "WHERE 1=1"
		args := []any{}
		if data.Filter != "" {
			where += " AND o.status=?"
			args = append(args, data.Filter)
		} else if page == "deliveries" {
			where += " AND o.status IN ('processing','out_for_delivery','delivered')"
		}
		if data.Query != "" {
			where += " AND (o.name LIKE ? OR o.email LIKE ? OR o.car_name LIKE ? OR o.city LIKE ? OR CAST(o.id AS TEXT)=?)"
			for i := 0; i < 4; i++ {
				args = append(args, "%"+data.Query+"%")
			}
			args = append(args, data.Query)
		}
		data.Orders, err = loadOrders(db, where, args...)
	case "order":
		id, e := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		var orders []ManagedOrder
		orders, err = loadOrders(db, "WHERE o.id=?", id)
		if err == nil && len(orders) == 0 {
			http.NotFound(w, r)
			return
		}
		if err == nil {
			data.Order = orders[0]
			data.Order.Events, err = loadOrderEvents(db, id)
		}
		if err == nil {
			data.Shipment, err = loadShipment(db, data.Order)
		}
		data.Title = fmt.Sprintf("Order #%04d", id)
		data.Subtitle = "A clear picture of the customer, vehicle, and delivery."
	case "payments":
		data.Title = "Review with confidence."
		data.Subtitle = "Check receipt evidence before approving an order for preparation."
		data.Payments, err = loadPayments(db)
	case "customers":
		data.Title = "The people behind the drive."
		data.Subtitle = "Customer profiles and order activity, together."
		var rows *sql.Rows
		rows, err = db.Query(`SELECT c.id,c.name,c.email,c.phone,CAST(c.created_at AS TEXT),COUNT(o.id),COALESCE(SUM(o.car_price),0) FROM customers c LEFT JOIN orders o ON o.customer_id=c.id GROUP BY c.id ORDER BY c.id DESC`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var c CustomerOverview
				if err = rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.CreatedAt, &c.Orders, &c.Total); err != nil {
					break
				}
				data.Customers = append(data.Customers, c)
			}
			if err == nil {
				err = rows.Err()
			}
		}
	case "team":
		data.Title = "The right people. The right access."
		data.Subtitle = "Only the platform owner can create and manage administrator accounts."
		var rows *sql.Rows
		rows, err = db.Query("SELECT id,name,email,role,status FROM admins WHERE status != 'setup_pending' ORDER BY id")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var s Staff
				if err = rows.Scan(&s.ID, &s.Name, &s.Email, &s.Role, &s.Status); err != nil {
					break
				}
				data.Team = append(data.Team, s)
			}
			if err == nil {
				err = rows.Err()
			}
		}
		if err == nil {
			err = rows.Close()
		}
		if err == nil {
			data.Invitations, err = loadStaffInvitations(db)
		}
	case "activity":
		data.Title = "Every action, accounted for."
		data.Subtitle = "A record of staff reviews, delivery changes, and access management."
		data.Activity, err = loadActivity(db)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Println(err)
		http.Error(w, "Could not load workspace", 500)
		return
	}
	renderAdmin(w, file, data)
}
func addActivity(tx *sql.Tx, staff Staff, action, entity string) error {
	_, err := tx.Exec("INSERT INTO admin_activity(actor_id,actor_name,action,entity) VALUES(?,?,?,?)", staff.ID, staff.Name, action, entity)
	return err
}
func addOrderEvent(tx *sql.Tx, staff Staff, id int64, from, to, note string) error {
	_, err := tx.Exec("INSERT INTO order_events(order_id,from_status,to_status,note,actor_id,actor_name) VALUES(?,?,?,?,?,?)", id, from, to, note, staff.ID, staff.Name)
	return err
}
func requestID(r *http.Request, key string) (int64, error) {
	id, err := strconv.ParseInt(r.FormValue(key), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid ID")
	}
	return id, nil
}
func reviewPayment(db *sql.DB, w http.ResponseWriter, r *http.Request, decision string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return
	}
	id, err := requestID(r, "payment-id")
	if err != nil {
		http.Error(w, "Invalid payment", 400)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if len(note) > 500 {
		http.Error(w, "Review note is too long", 400)
		return
	}
	if decision == "rejected" && len(note) < 5 {
		http.Error(w, "Add a reason for rejecting this receipt", 400)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not begin review", 500)
		return
	}
	defer tx.Rollback()
	var orderID int64
	var amount, price float64
	var state, paymentState string
	err = tx.QueryRow(`SELECT p.order_id,p.amount_paid,p.payment_status,o.status,o.car_price FROM payments p JOIN orders o ON o.id=p.order_id WHERE p.id=?`, id).Scan(&orderID, &amount, &paymentState, &state, &price)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if paymentState != "pending" || state != "payment_pending" {
		http.Error(w, "This payment has already been reviewed or the order has moved forward", 409)
		return
	}
	if decision == "verified" && (math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || math.Round(amount*100) < math.Round(price*100)) {
		http.Error(w, "The submitted amount does not cover this order. Request a corrected receipt.", 409)
		return
	}
	result, err := tx.Exec("UPDATE payments SET payment_status=? WHERE id=? AND payment_status='pending'", decision, id)
	if err != nil {
		http.Error(w, "Could not save review", 500)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		http.Error(w, "This payment was already reviewed", 409)
		return
	}
	next := state
	eventNote := "Payment evidence rejected: " + note
	notice := "rejected"
	if decision == "verified" {
		next = "processing"
		notice = "approved"
		eventNote = "Payment evidence reviewed and approved by our operations team."
		if note != "" {
			eventNote += " " + note
		}
		result, err = tx.Exec("UPDATE orders SET status='processing' WHERE id=? AND status='payment_pending'", orderID)
		if err != nil {
			http.Error(w, "Could not update order", 500)
			return
		}
		n, _ = result.RowsAffected()
		if n != 1 {
			http.Error(w, "Order state changed during review", 409)
			return
		}
	}
	if err = addOrderEvent(tx, staff, orderID, state, next, eventNote); err != nil {
		http.Error(w, "Could not record review", 500)
		return
	}
	if err = addActivity(tx, staff, "Payment "+statusLabel(decision), fmt.Sprintf("Payment #%d · Order #%d", id, orderID)); err != nil {
		http.Error(w, "Could not record review", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save review", 500)
		return
	}
	http.Redirect(w, r, "/admin/payments?notice="+notice, 303)
}
func transitionDelivery(db *sql.DB, w http.ResponseWriter, r *http.Request, next string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return
	}
	id, err := requestID(r, "order-id")
	if err != nil {
		http.Error(w, "Invalid order", 400)
		return
	}
	from := "processing"
	note := "Our operations team dispatched your vehicle."
	notice := "dispatched"
	if next == "delivered" {
		from = "out_for_delivery"
		note = "Vehicle handover confirmed by our operations team."
		notice = "delivered"
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not update delivery", 500)
		return
	}
	defer tx.Rollback()
	if next == "out_for_delivery" {
		var details int
		err = tx.QueryRow("SELECT COUNT(*) FROM delivery_details WHERE order_id=? AND partner!='' AND reference!=''", id).Scan(&details)
		if err != nil || details == 0 {
			http.Error(w, "Save a delivery partner and dispatch reference before dispatching this vehicle", 409)
			return
		}
	}
	result, err := tx.Exec("UPDATE orders SET status=? WHERE id=? AND status=?", next, id, from)
	if err != nil {
		http.Error(w, "Could not update delivery", 500)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		http.Error(w, "This delivery cannot move to the requested stage", 409)
		return
	}
	if err = addOrderEvent(tx, staff, id, from, next, note); err != nil {
		http.Error(w, "Could not record delivery update", 500)
		return
	}
	if err = addActivity(tx, staff, statusLabel(next), fmt.Sprintf("Order #%d", id)); err != nil {
		http.Error(w, "Could not record delivery update", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save delivery", 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/order?id=%d&notice=%s", id, notice), 303)
}
func saveDelivery(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return
	}
	id, err := requestID(r, "order-id")
	if err != nil {
		http.Error(w, "Invalid order", 400)
		return
	}
	d := DeliveryDetails{strings.TrimSpace(r.FormValue("scheduled-date")), strings.TrimSpace(r.FormValue("partner")), strings.TrimSpace(r.FormValue("driver")), strings.TrimSpace(r.FormValue("driver-phone")), strings.TrimSpace(r.FormValue("reference")), strings.TrimSpace(r.FormValue("note"))}
	if d.ScheduledDate != "" {
		if _, err = time.Parse("2006-01-02", d.ScheduledDate); err != nil {
			http.Error(w, "Enter a valid delivery date", 400)
			return
		}
	}
	if len(d.Partner) > 120 || len(d.Driver) > 100 || len(d.DriverPhone) > 30 || len(d.Reference) > 120 || len(d.Note) > 1000 {
		http.Error(w, "Please shorten the delivery details", 400)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not save delivery", 500)
		return
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow("SELECT status FROM orders WHERE id=?", id).Scan(&status); err != nil {
		http.NotFound(w, r)
		return
	}
	if status != "processing" && status != "out_for_delivery" {
		http.Error(w, "Delivery details can be edited only after payment review and before handover", 409)
		return
	}
	_, err = tx.Exec(`INSERT INTO delivery_details(order_id,scheduled_date,partner,driver,driver_phone,reference,note) VALUES(?,?,?,?,?,?,?) ON CONFLICT(order_id) DO UPDATE SET scheduled_date=excluded.scheduled_date,partner=excluded.partner,driver=excluded.driver,driver_phone=excluded.driver_phone,reference=excluded.reference,note=excluded.note,updated_at=CURRENT_TIMESTAMP`, id, d.ScheduledDate, d.Partner, d.Driver, d.DriverPhone, d.Reference, d.Note)
	if err != nil {
		http.Error(w, "Could not save delivery", 500)
		return
	}
	note := "Delivery arrangements updated by our operations team."
	if d.Note != "" {
		note = d.Note
	}
	if err = addOrderEvent(tx, staff, id, status, status, note); err != nil {
		http.Error(w, "Could not record delivery details", 500)
		return
	}
	if err = addActivity(tx, staff, "Delivery details updated", fmt.Sprintf("Order #%d", id)); err != nil {
		http.Error(w, "Could not record delivery details", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save delivery", 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/order?id=%d&notice=saved", id), 303)
}
func manageTeam(db *sql.DB, w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return
	}
	if staff.Role != "super_admin" {
		http.Error(w, "Only the platform owner can manage staff", 403)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not update staff", 500)
		return
	}
	defer tx.Rollback()
	notice := "updated"
	if action == "create" {
		name := strings.TrimSpace(r.FormValue("name"))
		email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
		password := r.FormValue("password")
		parsed, e := mail.ParseAddress(email)

		if e != nil || parsed.Address != email || len(name) < 2 || len(name) > 100 || len(email) > 254 || len(password) < 12 || len(password) > 72 {
			http.Error(w, "Enter a name, valid email and a password with 12–72 characters", 400)
			return
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if e != nil {
			http.Error(w, "Could not secure staff account", 500)
			return
		}
		role := "admin"
		if ownerOnlyAccess() && allowedOwnerEmail(email) {
			role = "super_admin"
		}
		if _, err = tx.Exec("INSERT INTO admins(name,email,password,role,status) VALUES(?,?,?,?,'approved')", name, email, string(hash), role); err != nil {
			http.Error(w, "Could not create staff account. Check whether this email already exists.", 409)
			return
		}
		err = addActivity(tx, staff, "Staff account created", email)
		notice = "created"
	} else {
		id, e := requestID(r, "admin-id")
		status := r.FormValue("status")
		if e != nil || (status != "approved" && status != "disabled" && status != "rejected") {
			http.Error(w, "Invalid staff update", 400)
			return
		}
		var role string
		if e = tx.QueryRow("SELECT role FROM admins WHERE id=?", id).Scan(&role); e != nil {
			http.NotFound(w, r)
			return
		}
		if id == staff.ID || role == "super_admin" {
			http.Error(w, "Owner accounts cannot be disabled from this staff screen", 403)
			return
		}
		if _, err = tx.Exec("UPDATE admins SET status=? WHERE id=?", status, id); err != nil {
			http.Error(w, "Could not update staff", 500)
			return
		}
		err = addActivity(tx, staff, "Staff access "+statusLabel(status), fmt.Sprintf("Staff #%d", id))
	}
	if err != nil {
		http.Error(w, "Could not record staff update", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "Could not save staff access", 500)
		return
	}
	http.Redirect(w, r, "/admin/team?notice="+notice, 303)
}
