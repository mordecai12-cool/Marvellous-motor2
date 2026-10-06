package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"github.com/gorilla/sessions"

	"golang.org/x/crypto/bcrypt"

	_ "modernc.org/sqlite"
)

type OrderData struct {
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

func formatAmount(amount float64) string {
	return fmt.Sprintf("₦%.0f", amount)
}

var store = sessions.NewCookieStore([]byte("marvellous-motors-secret-key"))

func main() {
	db, err := sql.Open("sqlite", "marvellous.db")
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
	http.HandleFunc("/admin.html", handleAdminDashboard)

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
	http.HandleFunc("/payment-pending.html", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.URL.Query().Get("order-id")

		if orderID == "" {
			http.Error(w, "Order ID missing", http.StatusBadRequest)
            return
		}

		data := struct {
			OrderID string
		}{
			OrderID: orderID,
		}

		tmpl, err := template.ParseFiles("payment-pending.html")
		if err != nil {
			log.Println("Could not load payment pending page:", err)
            http.Error(w, "Could not load page", http.StatusInternalServerError)
            return
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			log.Println("Could not display payment pending page:", err)
            http.Error(w, "Could not display page", http.StatusInternalServerError)
            return
		}
	})

	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			handleHome(w, r)
            return
		}

		http.FileServer(http.Dir(".")).ServeHTTP(w, r)
	})

	log.Println("Marvellous Motors is running on http://localhost:8080")

	err = http.ListenAndServe(":8080", nil)
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

	carName := "Toyota Camry"
	carPrice := 25000000

	log.Println("Customer:", name)
	log.Println("Email:", email)
	log.Println("Phone:", phone)
	log.Println("Address:", address)
	log.Println("City:", city)
	log.Println("State:", state)

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
		OrderID: orderID,
		Name:    name,
		Email:   email,
		Phone:   phone,
		Address: address,
		City:    city,
		State:   state,
	}

	log.Println("Order saved to database")

	tmpl, err := template.ParseFiles("payment.html")
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
	log.Println("Order ID:", orderID)

	amount := r.FormValue("amount-paid")

	log.Println("Amount Paid:", amount)

	reference := r.FormValue("reference")

	log.Println("Payment Reference:", reference)

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	file, header, err := r.FormFile("receipt")
	if err != nil {
		http.Error(w, "Receipt is required", http.StatusBadRequest)
		return
	}

	extension := filepath.Ext(header.Filename)

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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`
	    SELECT id, name, email, phone, address, city, state, status
		FROM orders
		ORDER BY id DESC
	`)
	if err != nil {
		log.Println("Could not get orders:", err)
		http.Error(w, "Could not load orders", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var orders []AdminOrder

	for rows.Next() {

		var order AdminOrder

		err := rows.Scan(
			&order.ID,
			&order.Name,
			&order.Email,
			&order.Phone,
			&order.Address,
			&order.City,
			&order.State,
			&order.Status,
		)

		if err != nil {
			log.Println("Could not read order:", err)
			http.Error(w, "Could not read order", http.StatusInternalServerError)
			return
		}

		orders = append(orders, order)
	}

	tmpl, err := template.ParseFiles("admin-orders.html")
	if err != nil {
		log.Println("Could not load admin orders page:", err)
		http.Error(w, "Could not load admin orders page", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, orders)
	if err != nil {
		log.Println("Could not display orders:", err)
		http.Error(w, "Could not display orders", http.StatusInternalServerError)
		return
	}
}

func handleAdminPayments(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`
	    SELECT
		    payments.id,
            payments.order_id,
            payments.amount_paid,
            payments.payment_reference,
            payments.receipt,
            payments.payment_status,
            orders.status
		FROM payments
        JOIN orders ON payments.order_id = orders.id
        ORDER BY payments.id DESC
	`)

	if err != nil {
		log.Println("Could not get payments:", err)
		http.Error(w, "Could not load payments", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var payments []AdminPayment

	for rows.Next() {

		var payment AdminPayment

		err := rows.Scan(
			&payment.ID,
			&payment.OrderID,
			&payment.AmountPaid,
			&payment.PaymentReference,
			&payment.Receipt,
			&payment.PaymentStatus,
			&payment.OrderStatus,
		)

		if err != nil {
			log.Println("Could not read payment:", err)
			http.Error(w, "Could not read payment", http.StatusInternalServerError)
			return
		}

		payments = append(payments, payment)

	}

	tmpl, err := template.New("admin-payments.html").Funcs(template.FuncMap{
		"formatAmount": formatAmount,
	}).ParseFiles("admin-payments.html")
	if err != nil {
		log.Println("Could not load admin payments page:", err)
		http.Error(w, "Could not load admin payments page", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, payments)
	if err != nil {
		log.Println("Could not display payments:", err)
		http.Error(w, "Could not display payments", http.StatusInternalServerError)
		return
	}

}

func handleVerifyPayment(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	paymentID := r.FormValue("payment-id")

	log.Println("Verifying payment:", paymentID)

	_, err := db.Exec(`
	    UPDATE payments
		SET payment_status = ?
		WHERE id = ?
	`, "verified", paymentID)

	if err != nil {
		log.Println("Could not verify payment:", err)
		http.Error(w, "Could not verify payment", http.StatusInternalServerError)
		return
	}

	result, err := db.Exec(`
	    UPDATE orders
		SET status = ? 
		WHERE id = (
		    SELECT order_id
			FROM payments
			WHERE id = ?
		)
	`, "processing", paymentID)

	if err != nil {
		log.Println("Could not update order status:", err)
		http.Error(w, "Could not update order status", http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Println("Could not check updated order:", err)
		return
	}

	log.Println("Orders updated:", rowsAffected)

	http.Redirect(w, r, "/admin/payments", http.StatusSeeOther)

}

func handleOrderDelivery(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orderID := r.FormValue("order-id")

	log.Println("Marking order out for delivery:", orderID)

	_, err := db.Exec(`
        UPDATE orders
        SET status = ?
        WHERE id = ?
    `, "out_for_delivery", orderID)

	if err != nil {
		log.Println("Could not update delivery status:", err)
		http.Error(w, "Could not update delivery status", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
}

func handleOrderDelivered(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orderID := r.FormValue("order-id")

	log.Println("Marking order as delivered:", orderID)

	_, err := db.Exec(`
        UPDATE orders
        SET status = ?
        WHERE id = ?
    `, "delivered", orderID)

	if err != nil {
		log.Println("Could not update delivered status:", err)
		http.Error(w, "Could not update delivered status", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
}

func handleOrderStatus(db *sql.DB, w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orderID := r.URL.Query().Get("order-id")

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

	var data OrderStatusData

	err = db.QueryRow(`
	    SELECT id, name, email, phone, address, city, state, car_name, car_price, status
		FROM orders
        WHERE id = ?
		AND customer_id = ?
	`, orderID, customerID).Scan(
		&data.OrderID,
		&data.Name,
		&data.Email,
		&data.Phone,
		&data.Address,
		&data.City,
		&data.State,
		&data.CarName,
		&data.CarPrice,
		&data.Status,
	)

	if err != nil {
		log.Println("Could not get order status:", err)
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	tmpl, err := template.New("order-status.html").Funcs(template.FuncMap{
		"statusReached": statusReached,
	}).ParseFiles("order-status.html")
	if err != nil {
		log.Println("Could not load order status page:", err)
		http.Error(w, "Could not load order status page", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Println("Could not display order status:", err)
		http.Error(w, "Could not display order status", http.StatusInternalServerError)
		return
	}
}

func handleCustomerOrders(db *sql.DB, w http.ResponseWriter, r *http.Request) {

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

    rows, err := db.Query(`
        SELECT id, car_name, car_price, status
        FROM orders
        WHERE customer_id = ?
        ORDER BY id DESC
    `, customerID)

    if err != nil {
        log.Println("Could not get customer orders:", err)
        http.Error(w, "Could not get orders", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    type CustomerOrder struct {
        OrderID  int64
        CarName  string
        CarPrice float64
        Status   string
    }

    var orders []CustomerOrder

    for rows.Next() {
        var order CustomerOrder

        err := rows.Scan(
            &order.OrderID,
            &order.CarName,
            &order.CarPrice,
            &order.Status,
        )

        if err != nil {
            log.Println("Could not read customer order:", err)
            http.Error(w, "Could not read orders", http.StatusInternalServerError)
            return
        }

        orders = append(orders, order)
    }

    tmpl, err := template.New("customer-orders.html").Funcs(template.FuncMap{
        "formatAmount": formatAmount,
    }).ParseFiles("customer-orders.html")
    if err != nil {
        log.Println("Could not load customer orders page:", err)
        http.Error(w, "Could not load customer orders page", http.StatusInternalServerError)
        return
    }

    err = tmpl.Execute(w, orders)
    if err != nil {
        log.Println("Could not display customer orders:", err)
        http.Error(w, "Could not display customer orders", http.StatusInternalServerError)
        return
    }
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
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm-password")

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

		log.Println("Password reset token created:", token)

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
        "payment_pending":    1,
        "processing":         2,
        "out_for_delivery":   3,
        "delivered":          4,
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

    session, err := store.Get(r, "customer-session")
    if err != nil {
        http.Error(w, "Could not get customer session", http.StatusInternalServerError)
        return
    }

    _, ok := session.Values["customer_id"]

    if !ok {
        http.Redirect(w, r, "/login.html", http.StatusSeeOther)
        return
    }

    http.Redirect(w, r, "/order.html", http.StatusSeeOther)
}

func handleHome(w http.ResponseWriter, r *http.Request) {

    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    session, err := store.Get(r, "customer-session")
    if err != nil {
        http.Error(w, "Could not get customer session", http.StatusInternalServerError)
        return
    }

    _, signedIn := session.Values["customer_id"]

    data := struct {
        SignedIn bool
    }{
        SignedIn: signedIn,
    }

    tmpl, err := template.ParseFiles("index.html")
    if err != nil {
        http.Error(w, "Could not load home page", http.StatusInternalServerError)
        return
    }

    err = tmpl.Execute(w, data)
    if err != nil {
        http.Error(w, "Could not display home page", http.StatusInternalServerError)
        return
    }
}

func createSuperAdmin(db *sql.DB) {
    name := "Marvellous Motors Main Admin"
    email := "marvellousmordecaiotiti@gmail.com"

    password := "strongpassword"

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

	email := r.FormValue("email")
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
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	if status != "approved" {
		http.Error(w, "Admin account is not approved", http.StatusUnauthorized)
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

func handleAdminDashboard(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := store.Get(r, "admin-session")
	if err != nil {
		http.Error(w, "Could not get admin session", http.StatusInternalServerError)
		return
	}

	adminID, ok := session.Values["admin_id"]
	if !ok || adminID == nil {
		http.Redirect(w, r, "/admin-login.html", http.StatusSeeOther)
		return
	}

	http.ServeFile(w, r, "admin.html")
}

func handleAdminLogout(w http.ResponseWriter, r *http.Request) {

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

	http.Redirect(w, r, "/admin-login.html", http.StatusSeeOther)
}