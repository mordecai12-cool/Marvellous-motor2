package main

import (
    "database/sql"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"

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

func main() {
    db, err := sql.Open("sqlite", "marvellous.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

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

	file, header, err := r.FormFile("receipt")
	if err != nil {
		http.Error(w, "Receipt is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	log.Println("Receipt:", header.Filename)

	filename := "receipt_" + reference

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
