package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/order", handleOrder)
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

    log.Println("Customer:", name)
    log.Println("Email:", email)
    log.Println("Phone:", phone)
	log.Println("Address:", address)
    log.Println("City:", city)
    log.Println("State:", state)

	log.Println("Order received successfully")

	http.Redirect(w, r, "/payment.html?name="+name, http.StatusSeeOther)
}