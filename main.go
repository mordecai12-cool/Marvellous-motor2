package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
    ID              int64
    OrderID         int64
    AmountPaid      float64
    PaymentReference string
    Receipt         string
    PaymentStatus   string
}

func formatAmount(amount float64) string {
    return fmt.Sprintf("₦%.0f", amount)
}

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

	http.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
		handleOrder(db, w, r)
	})
	http.HandleFunc("/payment", func(w http.ResponseWriter, r *http.Request) {
		handlePayment(db, w, r)
	})
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

	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	http.Handle("/", http.FileServer(http.Dir(".")))

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

	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	address := r.FormValue("address")
	city := r.FormValue("city")
	state := r.FormValue("state")

	log.Println("Customer:", name)
	log.Println("Email:", email)
	log.Println("Phone:", phone)
	log.Println("Address:", address)
	log.Println("City:", city)
	log.Println("State:", state)

	log.Println("Order received successfully")

	result, err := db.Exec(`
        INSERT INTO orders (
            name,
            email,
            phone,
            address,
            city,
            state,
            status
        )
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, name, email, phone, address, city, state, "payment_pending")

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

	if contentType != "image/png" && contentType != "image/jpeg" &&  contentType != "application/pdf" {
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

	http.Redirect(w, r, "/payment-pending.html", http.StatusSeeOther)
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
	    SELECT id, order_id, amount_paid, payment_reference, receipt, payment_status
		FROM payments
		ORDER BY id DESC
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