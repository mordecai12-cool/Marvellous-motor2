package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type DeliveryRate struct {
	InsuranceBPS, DutyBPS, VATBPS                                     int64
	ID                                                                int64
	Body, Origin, Country, State, City, Label, Source, Notes, Expires string
	Delivery, Fees, Insurance, Taxes, PortFees                        int64
	Active, Demo, Complete, Estimated                                 bool
}
type PriceLine struct {
	Label  string `json:"label"`
	Amount int64  `json:"amount"`
}
type DeliveredQuote struct {
	RateVersion string      `json:"rate_version"`
	VehicleID   string      `json:"vehicle_id"`
	VehicleName string      `json:"vehicle_name"`
	Country     string      `json:"country"`
	State       string      `json:"state"`
	City        string      `json:"city"`
	RateID      int64       `json:"rate_id"`
	Lines       []PriceLine `json:"lines"`
	Total       int64       `json:"total"`
	Offer       string      `json:"offer"`
	Source      string      `json:"source"`
	Notes       string      `json:"notes"`
	Estimated   bool        `json:"estimated"`
	Complete    bool        `json:"complete"`
	Demo        bool        `json:"demo"`
	CanOrder    bool        `json:"can_order"`
	Token       string      `json:"token"`
}

func (q DeliveredQuote) TotalLabel() string { return formatAmount(float64(q.Total) / 100) }
func (p PriceLine) AmountLabel() string     { return formatAmount(float64(p.Amount) / 100) }

const rateSelect = "SELECT id,body,origin,country,state,city,delivery,fees,insurance,taxes,port_fees,label,source,notes,active,demo,complete,estimated,expires,insurance_bps,duty_bps,vat_bps FROM delivery_rates"

func scanDeliveryRate(row interface{ Scan(...any) error }) (v DeliveryRate, err error) {
	err = row.Scan(&v.ID, &v.Body, &v.Origin, &v.Country, &v.State, &v.City, &v.Delivery, &v.Fees, &v.Insurance, &v.Taxes, &v.PortFees, &v.Label, &v.Source, &v.Notes, &v.Active, &v.Demo, &v.Complete, &v.Estimated, &v.Expires, &v.InsuranceBPS, &v.DutyBPS, &v.VATBPS)
	return
}
func rateVersion(rate DeliveryRate) string {
	raw, _ := json.Marshal(rate)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}
func loadRates(db *sql.DB) ([]DeliveryRate, error) {
	rows, err := db.Query(rateSelect + " ORDER BY country,state,city")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []DeliveryRate{}
	for rows.Next() {
		v, e := scanDeliveryRate(rows)
		if e != nil {
			return nil, e
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
func percentOf(value, bps int64) int64          { return (value*bps + 5000) / 10000 }
func percentLabel(bps int64) string             { return fmt.Sprintf("%d.%02d", bps/100, bps%100) }
func (r DeliveryRate) InsurancePercent() string { return percentLabel(r.InsuranceBPS) }
func (r DeliveryRate) DutyPercent() string      { return percentLabel(r.DutyBPS) }
func (r DeliveryRate) VATPercent() string       { return percentLabel(r.VATBPS) }
func percentField(r *http.Request, name string) (int64, error) {
	raw := strings.TrimSpace(r.FormValue(name))
	if raw == "" {
		return 0, nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || len(parts[0]) < 1 {
		return 0, errors.New("Enter a percentage from 0 to 100 with at most two decimal places.")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > 100 {
		return 0, errors.New("Enter a percentage from 0 to 100.")
	}
	fraction := int64(0)
	if len(parts) == 2 {
		if len(parts[1]) > 2 || len(parts[1]) < 1 {
			return 0, errors.New("Use at most two decimal places for percentages.")
		}
		part := parts[1]
		if len(part) == 1 {
			part += "0"
		}
		fraction, err = strconv.ParseInt(part, 10, 64)
		if err != nil || fraction < 0 {
			return 0, errors.New("Enter a valid percentage.")
		}
	}
	value := whole*100 + fraction
	if value > 10000 {
		return 0, errors.New("Percentages cannot exceed 100%.")
	}
	return value, nil
}

func quoteVehicle(db *sql.DB, v Vehicle, country, state, city string) (DeliveredQuote, error) {
	q := DeliveredQuote{VehicleID: v.ID, VehicleName: v.Name(), Country: strings.TrimSpace(country), State: strings.TrimSpace(state), City: strings.TrimSpace(city)}
	if q.Country == "" {
		q.Country = "Nigeria"
	}
	if q.State == "" || q.City == "" || len(q.Country) > 80 || len(q.State) > 100 || len(q.City) > 100 {
		return q, errors.New("Enter your destination country, state or region, and city.")
	}
	if !v.Orderable() {
		return q, errors.New("This vehicle is " + strings.ToLower(v.StockStatus) + ". Choose an available car.")
	}
	origin := strings.TrimSpace(strings.Split(v.Location, ",")[0])
	rates, err := loadRates(db)
	if err != nil {
		return q, err
	}
	var chosen *DeliveryRate
	best := -1
	today := time.Now().In(dealershipTime()).Format("2006-01-02")
	for i := range rates {
		rate := &rates[i]
		if !rate.Active || !strings.EqualFold(rate.Origin, origin) || !strings.EqualFold(rate.Country, q.Country) || (rate.State != "" && !strings.EqualFold(rate.State, q.State)) || (rate.City != "" && !strings.EqualFold(rate.City, q.City)) || (rate.Body != "All" && rate.Body != v.Body) || (rate.Expires != "" && rate.Expires < today) {
			continue
		}
		score := 0
		if rate.State != "" {
			score++
		}
		if rate.City != "" {
			score += 2
		}
		if rate.Body != "All" {
			score += 4
		}
		if score > best {
			chosen = rate
			best = score
		}
	}
	if chosen == nil {
		return q, errors.New("A delivery rate has not been configured for this route. Your request can still be sent from Customer care; the site will calculate the total automatically once an admin adds the route.")
	}
	rate := *chosen
	q.RateID = rate.ID
	q.Estimated = rate.Estimated
	q.Complete = rate.Complete
	q.Demo = rate.Demo || v.Sample
	q.Source = rate.Source
	q.Notes = rate.Notes
	q.CanOrder = rate.Complete && (!rate.Estimated || (rate.Demo && !productionMode() && os.Getenv("MM_DEMO_MODE") == "true"))
	q.RateVersion = rateVersion(rate)
	discount := int64(0)
	if v.HasActiveOffer() {
		q.Offer = v.OfferName
		discount = int64(v.OfferDiscount) * 100
	}
	net := int64(v.Price)*100 - discount
	insurance := rate.Insurance*100 + percentOf(net, rate.InsuranceBPS)
	taxable := net + rate.Delivery*100 + insurance
	duty := percentOf(taxable, rate.DutyBPS)
	vat := percentOf(taxable+duty, rate.VATBPS)
	q.Lines = []PriceLine{{"Vehicle price", int64(v.Price) * 100}, {"Delivery / freight", rate.Delivery * 100}, {"Paperwork & service fees", rate.Fees * 100}, {"Insurance", insurance}, {"Other taxes & duties (fixed)", rate.Taxes * 100}, {"Import duty (configured rate)", duty}, {"VAT (configured rate)", vat}, {"Port & destination handling", rate.PortFees * 100}}
	if discount > 0 {
		q.Lines = append(q.Lines, PriceLine{q.Offer, -discount})
	}
	for _, line := range q.Lines {
		q.Total += line.Amount
	}
	if q.Total < 0 {
		return q, errors.New("The configured discount exceeds the total.")
	}
	raw, _ := json.Marshal(q)
	digest := sha256.Sum256(raw)
	q.Token = hex.EncodeToString(digest[:])
	return q, nil
}
func serveQuoteAPI(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		serviceError(w, 405, "Method not allowed")
		return
	}
	v, err := fleetVehicle(db, r.URL.Query().Get("vehicle"), false)
	if err != nil {
		serviceError(w, 404, "Vehicle not found")
		return
	}
	q, err := quoteVehicle(db, v, r.URL.Query().Get("country"), r.URL.Query().Get("state"), r.URL.Query().Get("city"))
	if err != nil {
		serviceError(w, 422, err.Error())
		return
	}
	serviceJSON(w, 200, q)
}

type CommercePage struct {
	ShowroomData
	Error, Notice string
	Rates         []DeliveryRate
	Slots         []VisitSlot
	Appointments  []Appointment
	Notifications []string
	ReviewOrders  []ManagedOrder
}

func publicCommerceData(db *sql.DB, r *http.Request, page string) CommercePage {
	_, err := serviceCustomer(db, r)
	profile, _ := loadBusinessProfile(db)
	values, _ := fleetList(db, false)
	return CommercePage{ShowroomData: ShowroomData{SignedIn: err == nil, Page: page, Business: profile, Vehicles: values}}
}
func serveQuotePage(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d := publicCommerceData(db, r, "quote")
	d.Rates, _ = loadRates(db)
	renderBusinessPage(w, 200, "quote.html", d)
}
func managePricing(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	d, ok := operationsData(db, w, r, "pricing", "Automatic delivered prices.", "Set route charges, mandatory fees and the information customers see before ordering.")
	if !ok {
		return
	}
	if r.Method == "POST" {
		if err := saveRate(db, r, d.Staff); err != nil {
			d.Error = err.Error()
		} else {
			http.Redirect(w, r, "/admin/pricing?saved=1", 303)
			return
		}
	} else if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d.Rates, _ = loadRates(db)
	renderAdmin(w, "admin-pricing.html", d)
}
func saveRate(db *sql.DB, r *http.Request, staff Staff) error {
	id, _ := optionalInteger(r, "id", 1<<50)
	state := strings.TrimSpace(r.FormValue("state"))
	city := strings.TrimSpace(r.FormValue("city"))
	country := strings.TrimSpace(r.FormValue("country"))
	origin := strings.TrimSpace(r.FormValue("origin"))
	body := r.FormValue("body")
	if len(origin) < 2 || len(origin) > 100 || len(country) < 2 || len(country) > 80 || len(state) > 100 || len(city) > 100 || !oneOf(body, "All", "Sedan", "SUV", "Coupe", "Hatchback", "Pickup", "Van", "Convertible") {
		return errors.New("Enter a collection city, destination country and valid vehicle type.")
	}
	values := []int64{}
	for _, key := range []string{"delivery", "fees", "insurance", "taxes", "port_fees"} {
		value, err := optionalInteger(r, key, 1_000_000_000_000)
		if err != nil {
			return err
		}
		values = append(values, value)
	}
	bps := []int64{}
	for _, key := range []string{"insurance_percent", "duty_percent", "vat_percent"} {
		value, e := percentField(r, key)
		if e != nil {
			return e
		}
		bps = append(bps, value)
	}
	label := strings.TrimSpace(r.FormValue("label"))
	source := strings.TrimSpace(r.FormValue("source"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	expires := r.FormValue("expires")
	if label == "" || len(label) > 150 || len(source) > 1000 || len(notes) > 2000 || !validDate(expires) || (source != "" && !strings.HasPrefix(source, "https://")) {
		return errors.New("Add a route label, a valid expiry date and an optional HTTPS source link.")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := []any{body, origin, country, state, city, values[0], values[1], values[2], values[3], values[4], label, source, notes, r.FormValue("active") == "on", r.FormValue("complete") == "on", r.FormValue("estimated") == "on", expires, bps[0], bps[1], bps[2]}
	if id > 0 {
		args = append(args, id)
		_, err = tx.Exec("UPDATE delivery_rates SET body=?,origin=?,country=?,state=?,city=?,delivery=?,fees=?,insurance=?,taxes=?,port_fees=?,label=?,source=?,notes=?,active=?,complete=?,estimated=?,expires=?,insurance_bps=?,duty_bps=?,vat_bps=?,demo=0 WHERE id=?", args...)
	} else {
		_, err = tx.Exec("INSERT INTO delivery_rates(body,origin,country,state,city,delivery,fees,insurance,taxes,port_fees,label,source,notes,active,complete,estimated,expires,insurance_bps,duty_bps,vat_bps) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", args...)
	}
	if err != nil {
		return errors.New("Could not save this route. Check whether the same route and vehicle type already exist.")
	}
	if err = addActivity(tx, staff, "Delivery pricing updated", origin+" → "+country+" / "+state+" / "+city); err != nil {
		return err
	}
	return tx.Commit()
}
func seedLocalBusinessDemo(db *sql.DB) {
	// Public reference estimates are not carrier quotes. Production cannot order
	// against them until an admin confirms all applicable costs.
	demo := !productionMode() && os.Getenv("MM_DEMO_MODE") == "true"
	source := "https://services.travo.ng/nigeria/logistics/car-delivery-service-in-nigeria-nationwide-vehicle-transport-by-travo-ng/"
	for _, rate := range []struct {
		state string
		cost  int64
	}{{"Lagos", 50000}, {"Oyo", 90000}, {"Edo", 170000}, {"Abuja", 265000}, {"Rivers", 300000}, {"Enugu", 320000}} {
		label := "Published-guide planning estimate"
		link := source
		if rate.state == "Lagos" {
			label = "Within-Lagos planning allowance"
			link = ""
		}
		notes := "Provisional planning allowance, not a booked carrier quote. Confirm transport method, insurance and all other fees before payment. Researched 26 October 2026."
		db.Exec("INSERT OR IGNORE INTO delivery_rates(origin,country,state,city,delivery,fees,label,source,notes,demo,complete,estimated) VALUES('Lagos','Nigeria',?,'',?,0,?,?,?,?,?,1)", rate.state, rate.cost, label, link, notes, demo, demo)
	}
	db.Exec(`INSERT OR IGNORE INTO delivery_rates(origin,country,state,city,delivery,fees,label,source,notes,demo,complete,estimated) VALUES('Lagos','United Kingdom','','',3960000,0,'UK container freight starting-price illustration','https://shipcars.co.uk/africa-to-uk/','Freight only: GBP 2,200 starting reference at a planning conversion of NGN 1,800/GBP (not a live exchange rate). Excludes unconfirmed Nigerian export fees, insurance, UK taxes, port handling, compliance and onward delivery. All-in total requires admin configuration. Researched 26 October 2026.',?,0,1)`, demo)
	for _, country := range []string{"United States", "Canada", "Ghana"} {
		db.Exec(`INSERT OR IGNORE INTO delivery_rates(origin,country,state,city,delivery,label,notes,demo,complete,estimated,fees) VALUES('Lagos',?,'','',0,'Carrier quote required','No reliable route-specific vehicle freight tariff was verified. Admin must configure freight and every applicable fee before an all-in total can be offered.',?,0,1,0)`, country, demo)
	}
}
func storedOrderQuote(db *sql.DB, id int64) DeliveredQuote {
	var raw string
	var q DeliveredQuote
	if db.QueryRow("SELECT quote FROM order_pricing WHERE order_id=?", id).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &q)
	}
	return q
}
func cancelVehicleOrder(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
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
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Could not cancel order", 500)
		return
	}
	defer tx.Rollback()
	var status, vehicle string
	if err = tx.QueryRow("SELECT o.status,p.vehicle_id FROM orders o JOIN order_pricing p ON p.order_id=o.id WHERE o.id=?", id).Scan(&status, &vehicle); err != nil {
		http.NotFound(w, r)
		return
	}
	if status != "payment_pending" {
		http.Error(w, "Only an order awaiting payment review can be cancelled here. Contact the owner for later-stage orders.", 409)
		return
	}
	var verified int
	tx.QueryRow("SELECT COUNT(*) FROM payments WHERE order_id=? AND payment_status='verified'", id).Scan(&verified)
	if verified > 0 {
		http.Error(w, "Reviewed payments require an owner-led cancellation", 409)
		return
	}
	if _, err = tx.Exec("UPDATE orders SET status='cancelled' WHERE id=?", id); err == nil {
		_, err = tx.Exec("UPDATE fleet_vehicles SET status='Available' WHERE id=? AND status='Reserved'", vehicle)
	}
	if err == nil {
		err = addOrderEvent(tx, staff, id, status, "cancelled", "Order cancelled; stock released.")
	}
	if err == nil {
		err = addActivity(tx, staff, "Order cancelled", fmt.Sprint(id))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		http.Error(w, "Could not cancel order", 500)
		return
	}
	http.Redirect(w, r, "/admin/order?id="+fmt.Sprint(id), 303)
}

func (q DeliveredQuote) TotalAmount() float64 { return float64(q.Total) / 100 }
