package main

import (
    "html/template"
	"log"
	"net/http"
)

type OrderData struct {
    Name    string
    Email   string
    Phone   string
    Address string
    City    string
    State   string
}

func main() {
	http.HandleFunc("/order", handleOrder)
    http.HandleFunc("/payment", handlePayment)
	http.Handle("/", http.FileServer(http.Dir(".")))

	log.Println("Marvellous Motors is running on http://localhost:8080")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func handleOrder(w http.ResponseWriter, r *http.Request) {
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

    orderData := OrderData{
    Name:    name,
    Email:   email,
    Phone:   phone,
    Address: address,
    City:    city,
    State:   state,
    }
    log.Println("Customer:", name)
    log.Println("Email:", email)
    log.Println("Phone:", phone)
	log.Println("Address:", address)
    log.Println("City:", city)
    log.Println("State:", state)

	log.Println("Order received successfully")

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

func handlePayment(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    log.Println("Payment form received")

    amount := r.FormValue("amount-paid")

    log.Println("Amount Paid:", amount)

    reference := r.FormValue("reference")

    log.Println("Payment Reference:", reference)

    http.Redirect(w, r, "/payment-pending.html", http.StatusSeeOther)
}