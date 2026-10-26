package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "modernc.org/sqlite"
)

type OrderData struct {
	Quote DeliveredQuote
	Bank  BankAccount
	ShowroomData
	Vehicle Vehicle
	OrderID int64
	Name    string
	Email   string
	Phone   string
	Address string
	City    string
	State   string
}

type AdminOrder struct {
	ID      int64
	Name    string
	Email   string
	Phone   string
	Address string
	City    string
	State   string
	Status  string
}

type AdminPayment struct {
	ID               int64
	OrderID          int64
	AmountPaid       float64
	PaymentReference string
	Receipt          string
	PaymentStatus    string
	OrderStatus      string
}

type OrderStatusData struct {
	OrderID  int64
	Name     string
	Email    string
	Phone    string
	Address  string
	City     string
	State    string
	CarName  string
	CarPrice float64
	Status   string
}

type AdminRequest struct {
	ID     int
	Name   string
	Email  string
	Status string
}

func formatAmount(amount float64) string {
	cents := int64(math.Round(amount * 100))
	label := "₦" + commaNumber(int(cents/100))
	if fraction := cents % 100; fraction != 0 {
		label += fmt.Sprintf(".%02d", fraction)
	}
	return label
}

var store = newSessionStore()

func main() {
	dbPath := databasePath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = os.MkdirAll(uploadDirectory(), 0700)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Database connected")

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS orders (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        email TEXT NOT NULL,
        phone TEXT NOT NULL,
        address TEXT NOT NULL,
        city TEXT NOT NULL,
        state TEXT NOT NULL,
        status TEXT NOT NULL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP 
        )
    `)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
	    ALTER TABLE orders
		ADD COLUMN car_name TEXT
	`)
	if err != nil {
		log.Println("Car name column:", err)
	}

	_, err = db.Exec(`
	    ALTER TABLE orders
		ADD COLUMN car_price REAL
	`)
	if err != nil {
		log.Println("Car price column:", err)
	}

	_, err = db.Exec(`
	    ALTER TABLE orders
        ADD COLUMN customer_id INTEGER
	`)
	if err != nil {
		log.Println("Customer ID column:", err)
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS customers (
		    id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            email TEXT NOT NULL UNIQUE,
            phone TEXT NOT NULL,
            password TEXT NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS admins (
		    id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            email TEXT NOT NULL UNIQUE,
            password TEXT NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
	    ALTER TABLE admins
        ADD COLUMN role TEXT NOT NULL DEFAULT 'admin'
	`)
	if err != nil {
		log.Println("Admin role column:", err)
	}

	_, err = db.Exec(`
	    ALTER TABLE admins
        ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'
	`)
	if err != nil {
		log.Println("Admin status column:", err)
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS payments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		order_id INTEGER NOT NULL,
		amount_paid REAL NOT NULL,
		payment_reference TEXT NOT NULL,
        receipt TEXT NOT NULL,
        payment_status TEXT NOT NULL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS password_reset_tokens (
		    id INTEGER PRIMARY KEY AUTOINCREMENT,
            customer_id INTEGER NOT NULL,
            token TEXT NOT NULL UNIQUE,
            expires_at DATETIME NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	if err := seedDeploymentOwner(db); err != nil {
		log.Fatal("Could not initialize platform owner")
	}
	seedLocalDemo(db)
	if !productionMode() && os.Getenv("MM_DEMO_MODE") == "true" {
		if err := seedConfiguredLocalOwner(db, ".local/owner-account.json"); err != nil {
			log.Fatal("Could not load local owner configuration")
		}
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { handleHealth(db, w, r) })

	registerShowroomRoutes()
	registerRequestRoutes(db)
	registerAdminPlatform(db)
	showroomDB = db
	seedLocalBusinessDemo(db)
	http.HandleFunc("/order-payment", func(w http.ResponseWriter, r *http.Request) { serveCustomerPayment(db, w, r) })

	http.HandleFunc("/start-order", func(w http.ResponseWriter, r *http.Request) {
		handleStartOrder(w, r)
	})
	http.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
		handleOrder(db, w, r)
	})
	http.HandleFunc("/order-status", func(w http.ResponseWriter, r *http.Request) {
		handleOrderStatus(db, w, r)
	})
	http.HandleFunc("/payment", func(w http.ResponseWriter, r *http.Request) {
		handlePayment(db, w, r)
	})
	http.HandleFunc("/admin.html", func(w http.ResponseWriter, r *http.Request) { handleAdminDashboard(db, w, r) })

	http.HandleFunc("/admin/orders", func(w http.ResponseWriter, r *http.Request) {
		handleAdminOrders(db, w, r)
	})
	http.HandleFunc("/admin/payments", func(w http.ResponseWriter, r *http.Request) {
		handleAdminPayments(db, w, r)
	})
	http.HandleFunc("/admin/payments/verify", func(w http.ResponseWriter, r *http.Request) {
		handleVerifyPayment(db, w, r)
	})
	http.HandleFunc("/admin/orders/delivery", func(w http.ResponseWriter, r *http.Request) {
		handleOrderDelivery(db, w, r)
	})
	http.HandleFunc("/admin/orders/delivered", func(w http.ResponseWriter, r *http.Request) {
		handleOrderDelivered(db, w, r)
	})
	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		handleRegister(db, w, r)
	})
	http.HandleFunc("/register.html", func(w http.ResponseWriter, r *http.Request) { serveRegistration(db, w, r) })
	http.HandleFunc("/login.html", serveCustomerSignIn)
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		handleLogin(db, w, r)
	})
	http.HandleFunc("/admin-login", func(w http.ResponseWriter, r *http.Request) {
		handleAdminLogin(db, w, r)
	})
	http.HandleFunc("/admin-logout", handleAdminLogout)

	http.HandleFunc("/customer-orders", func(w http.ResponseWriter, r *http.Request) {
		handleCustomerOrders(db, w, r)
	})
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/customer-dashboard.html", func(w http.ResponseWriter, r *http.Request) { handleCustomerDashboard(db, w, r) })
	http.HandleFunc("/customer-profile", func(w http.ResponseWriter, r *http.Request) {
		handleCustomerProfile(db, w, r)
	})
	http.HandleFunc("/edit-profile", func(w http.ResponseWriter, r *http.Request) {
		handleEditProfile(db, w, r)
	})
	http.HandleFunc("/forgot-password", func(w http.ResponseWriter, r *http.Request) {
		handleForgotPassword(db, w, r)
	})
	http.HandleFunc("/reset-password", func(w http.ResponseWriter, r *http.Request) {
		handleResetPassword(db, w, r)
	})
	http.HandleFunc("/admin-request", func(w http.ResponseWriter, r *http.Request) {
		handleAdminRequest(db, w, r)
	})
	http.HandleFunc("/admin-requests", func(w http.ResponseWriter, r *http.Request) {
		handleAdminRequests(db, w, r)
	})
	http.HandleFunc("/admin-requests/approve", func(w http.ResponseWriter, r *http.Request) {
		handleApproveAdminRequest(db, w, r)
	})
	http.HandleFunc("/admin-requests/reject", func(w http.ResponseWriter, r *http.Request) {
		handleRejectAdminRequest(db, w, r)
	})
	http.HandleFunc("/admin-request-status", func(w http.ResponseWriter, r *http.Request) {
		handleAdminRequestStatus(db, w, r)
	})
	http.HandleFunc("/payment-pending.html", func(w http.ResponseWriter, r *http.Request) { serveCustomerDelivery(db, w, r, true) })

	http.HandleFunc("/uploads/", func(w http.ResponseWriter, r *http.Request) { handlePrivateReceipt(db, w, r) })

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			handleHome(w, r)
			return
		}

		if r.URL.Path == "/admin.html" {
			handleAdminDashboard(db, w, r)
			return
		}

		if inventoryAsset(r.URL.Path) || r.URL.Path == "/security.js" || r.URL.Path == "/login.html" || r.URL.Path == "/register.html" {
			http.FileServer(http.Dir(".")).ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})

	err = runHTTPServer(securityMiddleware(recoverySessionMiddleware(db, adminAccessMiddleware(db, http.DefaultServeMux))))
	if err != nil {
		log.Fatal(err)
	}
}

func handleOrder(db *sql.DB, w http.ResponseWriter, r *http.Request) { placeVehicleOrder(db, w, r) }

func handlePayment(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("Payment form received")

	orderID := r.FormValue("order-id")
	session, _ := store.Get(r, "customer-session")
	if session == nil || session.Values["customer_id"] == nil {
		http.Error(w, "Please sign in", http.StatusUnauthorized)
		return
	}
	parsedID, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil || parsedID < 1 {
		http.Error(w, "Invalid order", http.StatusBadRequest)
		return
	}
	var owner int64
	var orderState string
	err = db.QueryRow("SELECT customer_id, status FROM orders WHERE id = ?", parsedID).Scan(&owner, &orderState)
	if err != nil || owner != session.Values["customer_id"] {
		http.NotFound(w, r)
		return
	}
	if orderState != "payment_pending" {
		http.Error(w, "This order is no longer awaiting payment", http.StatusConflict)
		return
	}
	log.Println("Receipt submitted for order:", parsedID)

	amount := r.FormValue("amount-paid")
	amountValue, err := strconv.ParseFloat(amount, 64)
	if err != nil || math.IsNaN(amountValue) || math.IsInf(amountValue, 0) || amountValue <= 0 || amountValue > 1000000000 {
		http.Error(w, "Enter a valid amount", http.StatusBadRequest)
		return
	}

	log.Println("Amount Paid:", amount)

	reference := r.FormValue("reference")
	if len(reference) < 3 || len(reference) > 120 {
		http.Error(w, "Enter a valid payment reference", http.StatusBadRequest)
		return
	}

	log.Println("Payment Reference:", reference)

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	file, header, err := r.FormFile("receipt")
	if err != nil {
		http.Error(w, "Receipt is required", http.StatusBadRequest)
		return
	}

	extension := strings.ToLower(filepath.Ext(header.Filename))

	if extension != ".png" && extension != ".jpg" && extension != ".jpeg" && extension != ".pdf" {
		http.Error(w, "Invalid receipt file type", http.StatusBadRequest)
		return
	}

	defer file.Close()

	buffer := make([]byte, 512)

	_, err = file.Read(buffer)
	if err != nil {
		http.Error(w, "Could not read receipt", http.StatusBadRequest)
		return
	}

	contentType := http.DetectContentType(buffer)
	log.Println("Receipt type:", contentType)

	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "application/pdf" {
		http.Error(w, "Invalid receipt file type", http.StatusBadRequest)
		return
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		http.Error(w, "Could not reset receipt", http.StatusInternalServerError)
		return
	}

	log.Println("Receipt:", header.Filename)

	filename := "receipt_" + orderID + "_" + time.Now().Format("20060102150405") + extension

	_, err = db.Exec(`
	    INSERT INTO payments (
		    order_id,
			amount_paid,
            payment_reference,
            receipt,
            payment_status
		)
		VALUES (?, ?, ?, ?, ?)
	`, orderID, amount, reference, filename, "pending")

	if err != nil {
		log.Println("Payment database error:", err)
		http.Error(w, "Could not save payment", http.StatusInternalServerError)
		return
	}

	log.Println("Payment saved to database")

	destination, err := os.OpenFile(filepath.Join(uploadDirectory(), filename), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		http.Error(w, "Could not save receipt", http.StatusInternalServerError)
		return
	}
	defer destination.Close()

	_, err = io.Copy(destination, file)
	if err != nil {
		http.Error(w, "Could not save receipt", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/payment-pending.html?order-id="+orderID, http.StatusSeeOther)
}

func handleAdminOrders(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveAdminPage(db, w, r, "orders")
}

func handleAdminPayments(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveAdminPage(db, w, r, "payments")
}

func handleVerifyPayment(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	reviewPayment(db, w, r, "verified")
}

func handleOrderDelivery(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	transitionDelivery(db, w, r, "out_for_delivery")
}

func handleOrderDelivered(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	transitionDelivery(db, w, r, "delivered")
}

func handleOrderStatus(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveCustomerDelivery(db, w, r, false)
}

func handleCustomerOrders(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveCustomerOrders(db, w, r)
}

func handleRegister(db *sql.DB, w http.ResponseWriter, r *http.Request) { serveRegistration(db, w, r) }

func handleLogin(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Could not read form", http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")

	// The same form recognizes approved admin credentials before customer login.
	if signInAdmin(db, w, r, email, password) {
		return
	}

	var storedPassword string

	err = db.QueryRow(`
        SELECT password
        FROM customers
        WHERE lower(email) = ?
    `, email).Scan(&storedPassword)

	if err != nil {
		log.Println("Could not find customer:", err)
		renderCustomerAuth(w, 401, "login.html", CustomerAuthPage{Error: "We could not sign you in. Check your email address and password."})
		return
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(storedPassword),
		[]byte(password),
	)

	if err != nil {
		renderCustomerAuth(w, 401, "login.html", CustomerAuthPage{Error: "We could not sign you in. Check your email address and password."})
		return
	}

	var customerID int64
	var customerName string

	err = db.QueryRow(`
	    SELECT id, name
        FROM customers
        WHERE lower(email) = ?
	`, email).Scan(&customerID, &customerName)

	if err != nil {
		log.Println("Could not get customer ID:", err)
		http.Error(w, "Could not log in", http.StatusInternalServerError)
		return
	}

	session, err := store.Get(r, "customer-session")
	if err != nil {
		log.Println("Could not get session:", err)
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}

	session.Values["auth_epoch"] = customerEpoch(db, customerID)
	session.Values["customer_id"] = customerID
	session.Values["customer_name"] = customerName

	err = session.Save(r, w)
	if err != nil {
		log.Println("Could not save session:", err)
		http.Error(w, "Could not save session", http.StatusInternalServerError)
		return
	}

	if selected, ok := session.Values["selected_vehicle"].(string); ok {
		if _, exists := findVehicle(selected); exists {
			http.Redirect(w, r, "/order.html?vehicle="+selected, http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/customer-dashboard.html", http.StatusSeeOther)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {

	session, err := store.Get(r, "customer-session")
	if err != nil {
		http.Error(w, "Could not get session", http.StatusInternalServerError)
		return
	}

	session.Values["customer_id"] = nil
	session.Options.MaxAge = -1

	err = session.Save(r, w)
	if err != nil {
		http.Error(w, "Could not log out", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login.html", http.StatusSeeOther)
}
func handleCustomerDashboard(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveCustomerAccount(db, w, r, "dashboard")
}
func handleCustomerProfile(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveCustomerAccount(db, w, r, "profile")
}
func handleEditProfile(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveCustomerAccount(db, w, r, "edit-profile")
}

func handleForgotPassword(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveRecoveryRequest(db, w, r)
}
func handleResetPassword(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	redeemRecoveryLink(db, w, r)
}

func statusReached(currentStatus, stepStatus string) bool {

	steps := map[string]int{
		"payment_pending":  1,
		"processing":       2,
		"out_for_delivery": 3,
		"delivered":        4,
	}

	current, currentOK := steps[currentStatus]
	step, stepOK := steps[stepStatus]

	if !currentOK || !stepOK {
		return false
	}

	return current >= step
}

func handleStartOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("vehicle")
	if id == "" {
		id = "toyota-camry"
	}
	if _, found := findVehicle(id); !found {
		http.NotFound(w, r)
		return
	}
	session, err := store.Get(r, "customer-session")
	if err != nil {
		http.Error(w, "Could not get session", http.StatusInternalServerError)
		return
	}
	session.Values["selected_vehicle"] = id
	for _, key := range []string{"country", "state", "city"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" && len(value) <= 100 {
			session.Values["quote_"+key] = value
		}
	}
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Could not save selection", http.StatusInternalServerError)
		return
	}
	if session.Values["customer_id"] == nil {
		http.Redirect(w, r, "/login.html", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/order.html?vehicle="+id, http.StatusSeeOther)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	values := publicVehicles()
	if len(values) > 6 {
		values = values[:6]
	}
	renderShowroom(w, r, "index.html", ShowroomData{Page: "home", Vehicles: values})
}

func createSuperAdmin(db *sql.DB) {
	name := "Marvellous Motors Main Admin"
	email := os.Getenv("MM_ADMIN_EMAIL")
	if email == "" {
		log.Println("Admin bootstrap skipped: MM_ADMIN_EMAIL is required")
		return
	}

	password := os.Getenv("MM_ADMIN_PASSWORD")
	if len(password) < 12 {
		log.Println("Admin bootstrap skipped: set MM_ADMIN_PASSWORD with at least 12 characters")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		log.Fatal("Could not hash admin password:", err)
	}

	_, err = db.Exec(`
        INSERT INTO admins (
            name,
            email,
            password,
            role,
            status
        )
        VALUES (?, ?, ?, ?, ?)
    `,
		name,
		email,
		hashedPassword,
		"super_admin",
		"approved",
	)

	if err != nil {
		log.Fatal("Could not create super admin:", err)
	}

	log.Println("Super admin created successfully")
}

// Keep existing bookmarks and form actions working through the shared login.
func handleAdminLogin(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	handleLogin(db, w, r)
}

func handleAdminDashboard(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveAdminPage(db, w, r, "overview")
}

func handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	session, err := store.Get(r, "admin-session")
	if err != nil {
		http.Error(w, "Could not get admin session", http.StatusInternalServerError)
		return
	}

	session.Options.MaxAge = -1

	err = session.Save(r, w)
	if err != nil {
		http.Error(w, "Could not log out", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	http.Redirect(w, r, "/index.html", http.StatusSeeOther)
}

func handleAdminRequest(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Public administrator registration is closed", http.StatusForbidden)
}

func handleAdminRequests(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	serveAdminPage(db, w, r, "team")
}

func handleApproveAdminRequest(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	r.Form.Set("status", "approved")
	manageTeam(db, w, r, "update")
}

func handleRejectAdminRequest(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	r.Form.Set("status", "rejected")
	manageTeam(db, w, r, "update")
}

func handleAdminRequestStatus(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Staff access is managed privately by the platform owner", http.StatusForbidden)
}
