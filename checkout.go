package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func placeVehicleOrder(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	c, err := serviceCustomer(db, r)
	if err != nil {
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	v, err := fleetVehicle(db, r.FormValue("vehicle-id"), false)
	if err != nil {
		http.Error(w, "Please select an available vehicle", 400)
		return
	}
	address, city, state, country := strings.TrimSpace(r.FormValue("address")), strings.TrimSpace(r.FormValue("city")), strings.TrimSpace(r.FormValue("state")), strings.TrimSpace(r.FormValue("country"))
	if country == "" {
		country = "Nigeria"
	}
	var orderTx *sql.Tx
	fail := func(code int, message string) {
		if orderTx != nil {
			_ = orderTx.Rollback()
		}
		profile, _ := loadBusinessProfile(db)
		renderBusinessPage(w, code, "order.html", ShowroomData{SignedIn: true, Page: "cars", Vehicle: v, Business: profile, Error: message, Address: address, City: city, State: state, Country: country})
	}
	if len(address) < 2 || len(address) > 300 || len(city) > 100 || len(state) > 100 {
		fail(400, "Complete a valid delivery address, city and state or region.")
		return
	}
	q, err := quoteVehicle(db, v, country, state, city)
	if err != nil {
		fail(422, err.Error())
		return
	}
	if !q.CanOrder {
		fail(422, "This route is still an estimate with unconfirmed costs. An admin must complete the route pricing before checkout.")
		return
	}
	if token := r.FormValue("quote-token"); token != "" && token != q.Token {
		fail(409, "The price or delivery charges changed. Check the refreshed total before continuing.")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		fail(500, "Could not start your order. Please try again.")
		return
	}
	defer tx.Rollback()
	orderTx = tx
	result, err := tx.Exec("UPDATE vehicle_stock SET available=available-1 WHERE vehicle_id=? AND available>0 AND EXISTS(SELECT 1 FROM fleet_vehicles WHERE id=? AND status='Available')", v.ID, v.ID)
	if err != nil {
		fail(409, "Stock is being updated. Please try again.")
		return
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		fail(409, "The last available vehicle has been reserved. Choose another vehicle.")
		return
	}

	var raw, currentState string
	if err = tx.QueryRow("SELECT payload,status FROM fleet_vehicles WHERE id=?", v.ID).Scan(&raw, &currentState); err != nil || currentState != "Available" {
		fail(409, "This vehicle is no longer available. Choose another vehicle.")
		return
	}
	var current Vehicle
	json.Unmarshal([]byte(raw), &current)
	oldJSON, _ := json.Marshal(v)
	current.StockStatus = v.StockStatus
	current.Availability = v.Availability
	current.StockQuantity = v.StockQuantity
	newJSON, _ := json.Marshal(current)
	if string(oldJSON) != string(newJSON) {
		fail(409, "The vehicle details changed. Refresh this page to see the current price.")
		return
	}
	currentRate, e := scanDeliveryRate(tx.QueryRow(rateSelect+" WHERE id=?", q.RateID))
	if e != nil || !currentRate.Active || !currentRate.Complete || (currentRate.Estimated && !currentRate.Demo) || rateVersion(currentRate) != q.RateVersion {
		fail(409, "Delivery pricing changed. Refresh the total before continuing.")
		return
	}

	var phone string
	if err = tx.QueryRow("SELECT phone FROM customers WHERE id=?", c.ID).Scan(&phone); err != nil {
		fail(500, "Could not load your contact details.")
		return
	}
	carName := fmt.Sprintf("%d %s %s (%s)", v.Year, v.Name(), v.Trim, v.Condition)
	destinationState := state
	if country != "Nigeria" {
		destinationState = state + ", " + country
	}
	result, err = tx.Exec("INSERT INTO orders(customer_id,name,email,phone,address,city,state,status,car_name,car_price) VALUES(?,?,?,?,?,?,?,'payment_pending',?,?)", c.ID, c.Name, c.Email, phone, address, city, destinationState, carName, float64(q.Total)/100)
	if err != nil {
		fail(500, "Your order could not be saved. Please try again.")
		return
	}
	id, _ := result.LastInsertId()
	quoteJSON, _ := json.Marshal(q)
	if _, err = tx.Exec("INSERT INTO order_pricing(order_id,vehicle_id,quote) VALUES(?,?,?)", id, v.ID, string(quoteJSON)); err != nil {
		fail(500, "Your price breakdown could not be saved. Please try again.")
		return
	}
	if err = refreshStockStatus(tx, v.ID); err != nil {
		fail(500, "Could not reserve this vehicle. Please try again.")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(500, "Your order could not be confirmed. Please try again.")
		return
	}
	http.Redirect(w, r, "/order-payment?order-id="+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
