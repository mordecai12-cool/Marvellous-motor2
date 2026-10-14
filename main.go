package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "modernc.org/sqlite"
)

type OrderData struct {
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
	dbPath := os.Getenv("MM_DB_PATH")
	if dbPath == "" {
		dbPath = ".local/preview.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = os.MkdirAll("uploads", 0755)
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

	seedLocalDemo(db)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { handleHealth(db, w, r) })

	registerShowroomRoutes()
	registerRequestRoutes(db)
	registerAdminPlatform(db)
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
	http.HandleFunc("/customer-dashboard.html", handleCustomerDashboard)
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

	log.Println("Marvellous Motors is running on http://localhost:8080")

	err = http.ListenAndServe("127.0.0.1:8080", securityMiddleware(adminAccessMiddleware(db, http.DefaultServeMux)))
	if err != nil {
		log.Fatal(err)
	}
}

func handleOrder(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Could not read form", http.StatusBadRequest)
		return
	}

	session, err := store.Get(r, "customer-session")
	if err != nil {
		http.Error(w, "Could not get customer session", http.StatusInternalServerError)
		return
	}

	customerIDValue, ok := session.Values["customer_id"]
	if !ok {
		http.Error(w, "Please sign in before placing an order", http.StatusUnauthorized)
		return
	}

	customerID := customerIDValue.(int64)

	var name string
	var email string
	var phone string

	err = db.QueryRow(`
	    SELECT name, email, phone
        FROM customers
        WHERE id = ?
	`, customerID).Scan(&name, &email, &phone)

	if err != nil {
		log.Println("Could not get customer details:", err)
		http.Error(w, "Could not get customer details", http.StatusInternalServerError)
		return
	}

	address := r.FormValue("address")
	city := r.FormValue("city")
	state := r.FormValue("state")

	vehicle, found := findVehicle(r.FormValue("vehicle-id"))
	if !found {
		http.Error(w, "Please select an available vehicle", http.StatusBadRequest)
		return
	}
	if address == "" || city == "" || state == "" {
		http.Error(w, "Complete all delivery details", http.StatusBadRequest)
		return
	}
	carName := fmt.Sprintf("%d %s %s (%s)", vehicle.Year, vehicle.Name(), vehicle.Trim, vehicle.Condition)
	carPrice := vehicle.Price

	log.Println("Order received successfully")

	result, err := db.Exec(`
        INSERT INTO orders (
		    customer_id,
            name,
            email,
            phone,
            address,
            city,
            state,
            status,
			car_name,
			car_price
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, customerID, name, email, phone, address, city, state, "payment_pending", carName, carPrice)

	if err != nil {
		log.Println("Database error:", err)
		http.Error(w, "Could not save order", http.StatusInternalServerError)
		return
	}

	orderID, err := result.LastInsertId()
	if err != nil {
		log.Println("Could not get order ID:", err)
		http.Error(w, "Could not get order ID", http.StatusInternalServerError)
		return
	}

	log.Println("Order ID:", orderID)

	orderData := OrderData{
		ShowroomData: ShowroomData{SignedIn: true},
		Vehicle:      vehicle,
		OrderID:      orderID,
		Name:         name,
		Email:        email,
		Phone:        phone,
		Address:      address,
		City:         city,
		State:        state,
	}

	log.Println("Order saved to database")

	tmpl, err := template.ParseFiles("payment.html", "partials.html")
	if err != nil {
		http.Error(w, "Could not load payment page", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, orderData)
	if err != nil {
		http.Error(w, "Could not display payment page", http.StatusInternalServerError)
		return
	}
}

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

	destination, err := os.Create("uploads/" + filename)
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

func handleRegister(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Could not read form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	if _, err := mail.ParseAddress(email); err != nil || len(name) < 2 || len(phone) < 5 {
		http.Error(w, "Please enter a valid name, email and phone", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm-password")

	if len(password) < 8 || len(password) > 72 {
		http.Error(w, "Password must contain between 8 and 72 characters", http.StatusBadRequest)
		return
	}
	if password != confirmPassword {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		log.Println("Could not hash password:", err)
		http.Error(w, "Could not create account", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec(`
        INSERT INTO customers (
            name,
            email,
            phone,
            password
        )
        VALUES (?, ?, ?, ?)
    `, name, email, phone, hashedPassword)

	if err != nil {
		log.Println("Could not create customer:", err)
		http.Error(w, "Could not create account", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login.html", http.StatusSeeOther)
}

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

	email := r.FormValue("email")
	password := r.FormValue("password")

	var storedPassword string

	err = db.QueryRow(`
        SELECT password
        FROM customers
        WHERE email = ?
    `, email).Scan(&storedPassword)

	if err != nil {
		log.Println("Could not find customer:", err)
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(storedPassword),
		[]byte(password),
	)

	if err != nil {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	var customerID int64
	var customerName string

	err = db.QueryRow(`
	    SELECT id, name
        FROM customers
        WHERE email = ?
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
func handleCustomerDashboard(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := store.Get(r, "customer-session")
	if err != nil {
		http.Error(w, "Could not get customer session", http.StatusInternalServerError)
		return
	}

	customerName, ok := session.Values["customer_name"]
	if !ok {
		http.Error(w, "Please sign in first", http.StatusUnauthorized)
		return
	}

	data := struct {
		Name string
	}{
		Name: customerName.(string),
	}

	tmpl, err := template.ParseFiles("customer-dashboard.html")
	if err != nil {
		http.Error(w, "Could not load dashboard", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Could not display dashboard", http.StatusInternalServerError)
		return
	}
}

func handleCustomerProfile(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := store.Get(r, "customer-session")
	if err != nil {
		http.Error(w, "Could not get customer session", http.StatusInternalServerError)
		return
	}

	customerIDValue, ok := session.Values["customer_id"]
	if !ok {
		http.Error(w, "Please sign in first", http.StatusUnauthorized)
		return
	}

	customerID := customerIDValue.(int64)

	var name string
	var email string
	var phone string

	err = db.QueryRow(`
        SELECT name, email, phone
        FROM customers
        WHERE id = ?
    `, customerID).Scan(&name, &email, &phone)

	if err != nil {
		log.Println("Could not get customer profile:", err)
		http.Error(w, "Could not get profile", http.StatusInternalServerError)
		return
	}

	data := struct {
		Name  string
		Email string
		Phone string
	}{
		Name:  name,
		Email: email,
		Phone: phone,
	}

	tmpl, err := template.ParseFiles("customer-profile.html")
	if err != nil {
		log.Println("Could not load customer profile page:", err)
		http.Error(w, "Could not load profile page", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Println("Could not display customer profile:", err)
		http.Error(w, "Could not display profile", http.StatusInternalServerError)
		return
	}
}

func handleEditProfile(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	session, err := store.Get(r, "customer-session")
	if err != nil {
		http.Error(w, "Could not get customer session", http.StatusInternalServerError)
		return
	}

	customerIDValue, ok := session.Values["customer_id"]
	if !ok {
		http.Error(w, "Please sign in first", http.StatusUnauthorized)
		return
	}

	customerID := customerIDValue.(int64)

	if r.Method == http.MethodGet {

		var name string
		var email string
		var phone string

		err = db.QueryRow(`
            SELECT name, email, phone
            FROM customers
            WHERE id = ?
        `, customerID).Scan(&name, &email, &phone)

		if err != nil {
			log.Println("Could not get customer details:", err)
			http.Error(w, "Could not get profile", http.StatusInternalServerError)
			return
		}

		data := struct {
			Name  string
			Email string
			Phone string
		}{
			Name:  name,
			Email: email,
			Phone: phone,
		}

		tmpl, err := template.ParseFiles("edit-profile.html")
		if err != nil {
			http.Error(w, "Could not load edit profile page", http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Could not display edit profile page", http.StatusInternalServerError)
			return
		}

		return
	}

	if r.Method == http.MethodPost {

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Could not read form", http.StatusBadRequest)
			return
		}

		name := r.FormValue("name")
		phone := r.FormValue("phone")

		_, err = db.Exec(`
            UPDATE customers
            SET name = ?, phone = ?
            WHERE id = ?
        `, name, phone, customerID)

		if err != nil {
			log.Println("Could not update profile:", err)
			http.Error(w, "Could not update profile", http.StatusInternalServerError)
			return
		}

		session.Values["customer_name"] = name

		err = session.Save(r, w)
		if err != nil {
			http.Error(w, "Could not save session", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/customer-profile", http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleForgotPassword(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {
		tmpl, err := template.ParseFiles("forgot-password.html")
		if err != nil {
			http.Error(w, "Could not load forgot password page", http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, nil)
		if err != nil {
			http.Error(w, "Could not display forgot password page", http.StatusInternalServerError)
			return
		}

		return
	}

	if r.Method == http.MethodPost {

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Could not read form", http.StatusBadRequest)
			return
		}

		email := r.FormValue("email")

		var customerID int64

		err = db.QueryRow(`
            SELECT id
            FROM customers
            WHERE email = ?
        `, email).Scan(&customerID)

		if err != nil {
			http.Error(w, "Email not found", http.StatusNotFound)
			return
		}

		tokenBytes := make([]byte, 32)

		_, err = rand.Read(tokenBytes)
		if err != nil {
			log.Println("Could not generate reset token:", err)
			http.Error(w, "Could not create reset request", http.StatusInternalServerError)
			return
		}

		token := hex.EncodeToString(tokenBytes)

		_, err = db.Exec(`
		    INSERT INTO password_reset_tokens (
			    customer_id,
                token,
                expires_at
			)
			VALUES (?, ?, datetime('now', '+30 minutes'))
		`, customerID, token)

		if err != nil {
			log.Println("Could not save reset token:", err)
			http.Error(w, "Could not create reset request", http.StatusInternalServerError)
			return
		}

		log.Println("Password reset request created; email delivery must be configured")

		http.Error(w, "Password reset request received", http.StatusOK)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleResetPassword(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	token := r.URL.Query().Get("token")

	if r.Method == http.MethodGet {

		if token == "" {
			http.Error(w, "Invalid reset link", http.StatusBadRequest)
			return
		}

		var customerID int64
		var expiresAt time.Time

		err := db.QueryRow(`
            SELECT customer_id, expires_at
            FROM password_reset_tokens
            WHERE token = ?
        `, token).Scan(&customerID, &expiresAt)

		if err != nil {
			log.Println("Could not validate reset token:", err)
			http.Error(w, "Invalid or expired reset link", http.StatusBadRequest)
			return
		}

		if time.Now().After(expiresAt) {
			http.Error(w, "Reset link has expired", http.StatusBadRequest)
			return
		}

		data := struct {
			Token string
		}{
			Token: token,
		}

		tmpl, err := template.ParseFiles("reset-password.html")
		if err != nil {
			http.Error(w, "Could not load reset password page", http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Could not display reset password page", http.StatusInternalServerError)
			return
		}

		return
	}

	if r.Method == http.MethodPost {

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Could not read form", http.StatusBadRequest)
			return
		}

		token := r.FormValue("token")
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm-password")

		if len(password) < 8 || len(password) > 72 {
			http.Error(w, "Password must contain between 8 and 72 characters", http.StatusBadRequest)
			return
		}
		if password != confirmPassword {
			http.Error(w, "Passwords do not match", http.StatusBadRequest)
			return
		}

		var customerID int64
		var expiresAt time.Time

		err = db.QueryRow(`
		    SELECT customer_id, expires_at
            FROM password_reset_tokens
            WHERE token = ?
		`, token).Scan(&customerID, &expiresAt)

		if err != nil {
			log.Println("Could not validate reset token:", err)
			http.Error(w, "Invalid or expired reset link", http.StatusBadRequest)
			return
		}

		if time.Now().After(expiresAt) {
			http.Error(w, "Reset link has expired", http.StatusBadRequest)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword(
			[]byte(password),
			bcrypt.DefaultCost,
		)

		if err != nil {
			log.Println("Could not hash new password:", err)
			http.Error(w, "Could not reset password", http.StatusInternalServerError)
			return
		}

		_, err = db.Exec(`
		    UPDATE customers
            SET password = ?
            WHERE id = ? 
		`, hashedPassword, customerID)

		if err != nil {
			log.Println("Could not update password:", err)
			http.Error(w, "Could not reset password", http.StatusInternalServerError)
			return
		}

		_, err = db.Exec(`
		    DELETE FROM password_reset_tokens
            WHERE token = ?
		`, token)

		if err != nil {
			log.Println("Could not remove reset token:", err)
		}

		http.Redirect(w, r, "/login.html", http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
	renderShowroom(w, r, "index.html", ShowroomData{Page: "home", Vehicles: vehicles[:6]})
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

func handleAdminLogin(db *sql.DB, w http.ResponseWriter, r *http.Request) {

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

	var storedPassword string
	var adminID int64
	var adminName string
	var role string
	var status string

	err = db.QueryRow(`
		SELECT id, name, password, role, status
		FROM admins
		WHERE email = ?
	`, email).Scan(
		&adminID,
		&adminName,
		&storedPassword,
		&role,
		&status,
	)

	if err != nil {
		log.Println("Could not find admin:", err)
		http.Redirect(w, r, "/admin-login.html?error=invalid", http.StatusSeeOther)
		return
	}

	if status != "approved" || (role != "admin" && role != "super_admin") {
		http.Redirect(w, r, "/admin-login.html?error=invalid", http.StatusSeeOther)
		return
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(storedPassword),
		[]byte(password),
	)

	if err != nil {
		http.Redirect(w, r, "/admin-login.html?error=invalid", http.StatusSeeOther)
		return
	}

	session, err := store.Get(r, "admin-session")
	if err != nil {
		log.Println("Could not get admin session:", err)
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}

	session.Values["admin_id"] = adminID
	session.Values["admin_name"] = adminName
	session.Values["admin_role"] = role

	err = session.Save(r, w)
	if err != nil {
		log.Println("Could not save admin session:", err)
		http.Error(w, "Could not save session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
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
