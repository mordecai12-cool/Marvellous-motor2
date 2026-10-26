package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
)

//go:embed inventory.json
var inventoryJSON []byte

type Vehicle struct {
	VIN             string            `json:"vin"`
	StockStatus     string            `json:"stock_status"`
	Sample          bool              `json:"sample"`
	ActualPhotos    bool              `json:"actual_photos"`
	ServiceHistory  string            `json:"service_history"`
	ConditionReport string            `json:"condition_report"`
	PreviousOwners  int               `json:"previous_owners"`
	Documents       string            `json:"documents"`
	PhotoItems      []VehiclePhoto    `json:"photos,omitempty"`
	DocumentItems   []VehicleDocument `json:"documents_files,omitempty"`
	OfferName       string            `json:"offer_name"`
	OfferDiscount   int               `json:"offer_discount"`
	OfferStart      string            `json:"offer_start"`
	OfferEnd        string            `json:"offer_end"`

	EngineLayout string   `json:"engine_layout"`
	Categories   []string `json:"categories"`
	Location     string   `json:"location"`
	Origin       string   `json:"origin"`
	Availability string   `json:"availability"`
	Seller       string   `json:"seller"`
	ID           string   `json:"id"`
	Brand        string   `json:"brand"`
	Model        string   `json:"model"`
	Trim         string   `json:"trim"`
	Year         int      `json:"year"`
	Condition    string   `json:"condition"`
	Price        int      `json:"price"`
	Mileage      int      `json:"mileage"`
	Body         string   `json:"body"`
	Fuel         string   `json:"fuel"`
	Transmission string   `json:"transmission"`
	Drive        string   `json:"drive"`
	Engine       string   `json:"engine"`
	Image        string   `json:"image"`
	Tagline      string   `json:"tagline"`
	Description  string   `json:"description"`
	Features     []string `json:"features"`
}

func (v Vehicle) CategoryLabel() string { return strings.Join(v.Categories, " · ") }
func (v Vehicle) EngineLayoutLabel() string {
	switch v.EngineLayout {
	case "I4":
		return "I4 · inline four-cylinder"
	case "I6":
		return "I6 · inline six-cylinder"
	case "V10":
		return "V10 · ten-cylinder V engine"
	case "Rotary":
		return "Rotary engine · twin rotor"
	case "V8":
		return "V8 · eight-cylinder V engine"
	case "Flat4":
		return "Flat-four boxer"
	case "V6":
		return "V6 · six-cylinder V engine"
	case "Flat6":
		return "Flat-six boxer"
	case "Electric":
		return "Electric motor · no combustion cylinders"
	default:
		return "Confirm engine configuration with dealer"
	}
}

func (v Vehicle) Name() string         { return v.Brand + " " + v.Model }
func (v Vehicle) PriceLabel() string   { return "₦" + commaNumber(v.Price) }
func (v Vehicle) MileageLabel() string { return commaNumber(v.Mileage) + " km" }
func commaNumber(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

var vehicles = func() []Vehicle {
	var values []Vehicle
	if err := json.Unmarshal(inventoryJSON, &values); err != nil {
		panic(err)
	}
	return values
}()

func findVehicle(id string) (Vehicle, bool) {
	if showroomDB != nil {
		v, err := fleetVehicle(showroomDB, id, false)
		return v, err == nil
	}
	for _, v := range vehicles {
		if v.ID == id {
			return v, true
		}
	}
	return Vehicle{}, false
}

type ShowroomData struct {
	Error, Address, City, State, Country string
	Business                             BusinessProfile
	SignedIn                             bool
	Page                                 string
	Vehicles                             []Vehicle
	Vehicle                              Vehicle
	Related                              []Vehicle
}

func renderShowroom(w http.ResponseWriter, r *http.Request, page string, data ShowroomData) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if showroomDB != nil {
		data.Business, _ = loadBusinessProfile(showroomDB)
	}
	session, _ := store.Get(r, "customer-session")
	data.SignedIn = session != nil && session.Values["customer_id"] != nil
	t, err := template.ParseFiles(page, "partials.html")
	if err != nil {
		log.Println(err)
		http.Error(w, "Could not load page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		log.Println(err)
	}
}

func handleCatalogue(w http.ResponseWriter, r *http.Request) {
	renderShowroom(w, r, "cars.html", ShowroomData{Page: "cars", Vehicles: publicVehicles()})
}

func handleVehicle(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = "toyota-camry"
	}
	v, ok := findVehicle(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var related []Vehicle
	for _, other := range publicVehicles() {
		if other.ID != id && other.Body == v.Body && len(related) < 3 {
			related = append(related, other)
		}
	}
	renderShowroom(w, r, "car-details.html", ShowroomData{Page: "cars", Vehicle: v, Related: related})
}

func handleVehicleAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(publicVehicles())
}

func handleOrderPage(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "customer-session")
	if session == nil || session.Values["customer_id"] == nil {
		http.Redirect(w, r, "/start-order?vehicle="+r.URL.Query().Get("vehicle"), http.StatusSeeOther)
		return
	}
	id := r.URL.Query().Get("vehicle")
	if id == "" {
		if saved, ok := session.Values["selected_vehicle"].(string); ok {
			id = saved
		}
	}
	v, ok := findVehicle(id)
	if !ok {
		http.Redirect(w, r, "/cars.html", http.StatusSeeOther)
		return
	}
	d := ShowroomData{Page: "cars", Vehicle: v}
	d.Country, _ = session.Values["quote_country"].(string)
	d.State, _ = session.Values["quote_state"].(string)
	d.City, _ = session.Values["quote_city"].(string)
	renderShowroom(w, r, "order.html", d)
}

func registerShowroomRoutes() {
	http.HandleFunc("/cars.html", handleCatalogue)
	http.HandleFunc("/car-details.html", handleVehicle)
	http.HandleFunc("/api/vehicles", handleVehicleAPI)
	http.HandleFunc("/order.html", handleOrderPage)
	for _, page := range []string{"about", "contact"} {
		page := page
		http.HandleFunc("/"+page+".html", func(w http.ResponseWriter, r *http.Request) {
			renderShowroom(w, r, page+".html", ShowroomData{Page: page})
		})
	}
}

func inventoryAsset(path string) bool {
	if path == "/commerce.js" || path == "/business.css" || path == "/business.js" {
		return true
	}
	if path == "/account.css" || path == "/gallery.js" || path == "/gallery.css" || path == "/customer-care.js" || path == "/customer-care.css" || path == "/shipment.js" {
		return true
	}
	return strings.HasPrefix(path, "/images/") || path == "/admin.css" || path == "/admin.js" || path == "/delivery.css" || path == "/style.css" || path == "/legacy.css" || path == "/favicon.svg" || path == "/showroom.js" || strings.HasPrefix(path, "/fonts/") || path == "/image-credits.html"
}
