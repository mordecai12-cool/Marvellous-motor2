package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type VehicleDocument struct {
	Name, URL string
	Public    bool
}

func publicVehicles() []Vehicle {
	if showroomDB == nil {
		return vehicles
	}
	list, err := fleetList(showroomDB, false)
	if err != nil {
		logBusinessError(err)
		return []Vehicle{}
	}
	for i := range list {
		docs := []VehicleDocument{}
		for _, doc := range list[i].DocumentItems {
			if doc.Public {
				docs = append(docs, doc)
			}
		}
		list[i].DocumentItems = docs
	}
	return list
}
func logBusinessError(err error) { fmt.Println("Showroom data:", err) }
func fleetList(db *sql.DB, all bool) ([]Vehicle, error) {
	query := "SELECT payload,status FROM fleet_vehicles"
	if !all {
		query += " WHERE status!='Draft'"
	}
	query += " ORDER BY rowid"
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Vehicle{}
	for rows.Next() {
		var raw, state string
		if err = rows.Scan(&raw, &state); err != nil {
			return nil, err
		}
		var v Vehicle
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		v.StockStatus = state
		v.Availability = state
		list = append(list, v)
	}
	return list, rows.Err()
}
func fleetVehicle(db *sql.DB, id string, all bool) (Vehicle, error) {
	query := "SELECT payload,status FROM fleet_vehicles WHERE id=?"
	if !all {
		query += " AND status!='Draft'"
	}
	var raw, state string
	var v Vehicle
	err := db.QueryRow(query, id).Scan(&raw, &state)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal([]byte(raw), &v)
	v.StockStatus = state
	v.Availability = state
	return v, err
}
func (v Vehicle) Orderable() bool { return v.StockStatus == "" || v.StockStatus == "Available" }
func (v Vehicle) OwnerHistory() string {
	if v.PreviousOwners < 0 {
		return "Not supplied"
	}
	return strconv.Itoa(v.PreviousOwners)
}
func randomBusinessID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }
func integerField(r *http.Request, name string, min, max int64) (int64, error) {
	n, err := strconv.ParseInt(r.FormValue(name), 10, 64)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("Enter a valid %s between %d and %d", strings.ReplaceAll(name, "_", " "), min, max)
	}
	return n, nil
}
func optionalInteger(r *http.Request, name string, max int64) (int64, error) {
	if strings.TrimSpace(r.FormValue(name)) == "" {
		return 0, nil
	}
	return integerField(r, name, 0, max)
}
func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
func manageInventory(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	d, ok := operationsData(db, w, r, "inventory", "Your showroom stock.", "Publish vehicles, manage availability, upload actual photos and document the condition.")
	if !ok {
		return
	}
	d.Editing = Vehicle{StockStatus: "Draft", PreviousOwners: -1, Year: time.Now().Year(), Condition: "Used", Categories: []string{"Everyday"}}
	if id := r.URL.Query().Get("id"); id != "" {
		v, err := fleetVehicle(db, id, true)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		d.Editing = v
	}
	if r.Method == http.MethodPost {
		if r.FormValue("action") == "remove_asset" || r.FormValue("action") == "cover" {
			if err := changeVehicleAsset(db, r, d.Staff); err != nil {
				d.Error = err.Error()
			} else {
				http.Redirect(w, r, "/admin/inventory?id="+r.FormValue("id")+"&saved=1", 303)
				return
			}
			d.Fleet, _ = fleetList(db, true)
			renderAdmin(w, "admin-inventory.html", d)
			return
		}
		v, err := saveFleetVehicle(db, r, d.Staff)
		if err != nil {
			d.Error = err.Error()
			if r.FormValue("id") == "" {
				v.ID = ""
				d.Editing = v
			}
			if v.ID != "" {
				d.Editing = v
			}
		} else {
			http.Redirect(w, r, "/admin/inventory?id="+v.ID+"&saved=1", 303)
			return
		}
	} else if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var err error
	d.Fleet, err = fleetList(db, true)
	if err != nil {
		http.Error(w, "Could not load inventory", 500)
		return
	}
	renderAdmin(w, "admin-inventory.html", d)
}
func saveFleetVehicle(db *sql.DB, r *http.Request, staff Staff) (Vehicle, error) {
	id := strings.TrimSpace(r.FormValue("id"))
	v := Vehicle{ID: id, PreviousOwners: -1}
	creating := id == ""
	if !creating {
		var err error
		v, err = fleetVehicle(db, id, true)
		if err != nil {
			return v, errors.New("Vehicle not found.")
		}
	} else {
		v.ID = randomBusinessID()
	}
	v.VIN = strings.TrimSpace(r.FormValue("vin"))
	if len(v.VIN) > 40 {
		return v, errors.New("Keep VIN or chassis numbers under 40 characters.")
	}
	v.Brand = strings.TrimSpace(r.FormValue("brand"))
	v.Model = strings.TrimSpace(r.FormValue("model"))
	v.Trim = strings.TrimSpace(r.FormValue("trim"))
	v.Condition = r.FormValue("condition")
	v.Body = r.FormValue("body")
	v.Fuel = r.FormValue("fuel")
	v.Engine = strings.TrimSpace(r.FormValue("engine"))
	v.EngineLayout = r.FormValue("engine_layout")
	v.Transmission = r.FormValue("transmission")
	v.Drive = r.FormValue("drive")
	v.Location = strings.TrimSpace(r.FormValue("location"))
	v.Origin = r.FormValue("origin")
	v.Seller = "Marvellous Motors"
	v.Description = strings.TrimSpace(r.FormValue("description"))
	v.Tagline = strings.TrimSpace(r.FormValue("tagline"))
	v.ConditionReport = strings.TrimSpace(r.FormValue("condition_report"))
	v.ServiceHistory = strings.TrimSpace(r.FormValue("service_history"))
	v.Documents = strings.TrimSpace(r.FormValue("documents"))
	v.Features = splitLines(r.FormValue("features"))
	v.Categories = r.Form["categories"]
	v.ActualPhotos = r.FormValue("actual_photos") == "on"
	v.Sample = r.FormValue("sample") == "on"
	v.StockStatus = r.FormValue("stock_status")
	if len(v.Brand) < 2 || len(v.Brand) > 80 || len(v.Model) < 1 || len(v.Model) > 100 || len(v.Location) < 2 || len(v.Location) > 150 || len(v.Description) > 5000 || len(v.ConditionReport) > 5000 || len(v.ServiceHistory) > 5000 || len(v.Documents) > 3000 || len(v.Tagline) > 200 || len(v.Trim) > 100 || len(r.FormValue("features")) > 3000 {
		return v, errors.New("Enter the make, model and stock location; keep descriptions under 5,000 characters.")
	}
	for _, category := range v.Categories {
		if category != "Everyday" && category != "Sports" && category != "Luxury" && category != "Classic" && category != "Limited Edition" {
			return v, errors.New("Choose a valid collection.")
		}
	}
	if len(v.Categories) == 0 {
		v.Categories = []string{"Everyday"}
	}
	if !oneOf(v.StockStatus, "Draft", "Available", "Reserved", "Sold") || !oneOf(v.Condition, "New", "Used") || !oneOf(v.Body, "Sedan", "SUV", "Coupe", "Hatchback", "Pickup", "Van", "Convertible") || !oneOf(v.Fuel, "Petrol", "Diesel", "Hybrid", "Electric") || !oneOf(v.EngineLayout, "I4", "I6", "V6", "V8", "Flat6", "Electric", "Unconfirmed") || !oneOf(v.Transmission, "Automatic", "Manual", "CVT", "e-CVT", "PDK automatic", "Single-speed") {
		return v, errors.New("Choose valid stock and vehicle specification options.")
	}
	year, err := integerField(r, "year", 1950, int64(time.Now().Year()+2))
	if err != nil {
		return v, err
	}
	v.Year = int(year)
	price, err := integerField(r, "price", 1, 1_000_000_000_000)
	if err != nil {
		return v, err
	}
	v.Price = int(price)
	mileage, err := integerField(r, "mileage", 0, 10_000_000)
	if err != nil {
		return v, err
	}
	v.Mileage = int(mileage)
	owners, err := integerField(r, "previous_owners", -1, 100)
	if err != nil {
		return v, err
	}
	v.PreviousOwners = int(owners)
	discount, err := optionalInteger(r, "offer_discount", price)
	if err != nil {
		return v, err
	}
	v.OfferDiscount = int(discount)
	v.OfferName = strings.TrimSpace(r.FormValue("offer_name"))
	v.OfferStart = r.FormValue("offer_start")
	v.OfferEnd = r.FormValue("offer_end")
	if len(v.OfferName) > 100 || !validDate(v.OfferStart) || !validDate(v.OfferEnd) || (v.OfferStart != "" && v.OfferEnd != "" && v.OfferEnd < v.OfferStart) || (v.OfferDiscount > 0 && v.OfferName == "") {
		return v, errors.New("Give the offer a name, a discount no greater than the price, and valid start/end dates.")
	}
	if creating {
		v.Image = "/favicon.svg"
		v.Sample = false
	}
	tx, err := db.Begin()
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	if !creating {
		var active int
		err = tx.QueryRow("SELECT COUNT(*) FROM order_pricing p JOIN orders o ON o.id=p.order_id WHERE p.vehicle_id=? AND o.status IN ('payment_pending','processing','out_for_delivery')", v.ID).Scan(&active)
		if err != nil {
			return v, err
		}
		if active > 0 && v.StockStatus != "Reserved" {
			return v, errors.New("This car has an active order. Cancel that order or complete handover before changing its stock status.")
		}
	}
	raw, _ := json.Marshal(v)
	if _, err = tx.Exec("INSERT INTO fleet_vehicles(id,payload,status) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,status=excluded.status,updated_at=CURRENT_TIMESTAMP", v.ID, string(raw), v.StockStatus); err != nil {
		return v, err
	}
	paths := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, path := range paths {
				os.Remove(path)
			}
		}
	}()
	if r.MultipartForm != nil {
		photos := r.MultipartForm.File["photos"]
		documents := r.MultipartForm.File["files"]
		if len(photos) > 10 || len(documents) > 5 {
			return v, errors.New("Upload at most 10 photos and 5 documents at a time.")
		}
		for _, group := range []struct {
			field, kind string
			public      bool
		}{{"photos", "photo", true}, {"files", "document", r.FormValue("publish_documents") == "on"}} {
			for _, header := range r.MultipartForm.File[group.field] {
				file, e := header.Open()
				if e != nil {
					return v, e
				}
				data, e := io.ReadAll(io.LimitReader(file, supportAttachmentLimit+1))
				file.Close()
				if e != nil || len(data) == 0 || len(data) > supportAttachmentLimit {
					return v, errors.New("Each file must be non-empty and at most 5 MB.")
				}
				media := http.DetectContentType(data)
				if group.kind == "photo" && (media != "image/jpeg" && media != "image/png") {
					return v, errors.New("Vehicle photos must be JPEG or PNG.")
				}
				if group.kind == "document" && media != "application/pdf" && media != "image/jpeg" && media != "image/png" {
					return v, errors.New("Documents must be PDF, JPEG or PNG.")
				}
				if media == "image/jpeg" || media == "image/png" {
					config, _, e := image.DecodeConfig(bytes.NewReader(data))
					if e != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 50_000_000 {
						return v, errors.New("Upload a valid photograph under 50 megapixels.")
					}
				}
				name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
				if len(name) > 200 {
					return v, errors.New("File names must be at most 200 bytes.")
				}
				key := randomBusinessID()
				path := filepath.Join(uploadDirectory(), "vehicles", key)
				if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
					return v, e
				}
				if e = os.WriteFile(path, data, 0600); e != nil {
					return v, e
				}
				paths = append(paths, path)
				_, e = tx.Exec("INSERT INTO vehicle_assets(id,vehicle_id,name,media_type,kind,public,storage_key,size) VALUES(?,?,?,?,?,?,?,?)", key, v.ID, name, media, group.kind, group.public, key, len(data))
				if e != nil {
					return v, e
				}
				url := "/media/vehicle?id=" + key
				if group.kind == "photo" {
					photo := VehiclePhoto{Image: url, Label: name, Caption: "Stock vehicle photograph", Source: "/dealership", Artist: "Dealership", License: "Dealer supplied", PhotoSet: v.ID}
					v.PhotoItems = append(v.PhotoItems, photo)
					if len(v.PhotoItems) == 1 {
						v.Image = url
					}
				} else {
					v.DocumentItems = append(v.DocumentItems, VehicleDocument{Name: name, URL: url, Public: group.public})
				}
			}
		}
	}
	if v.ActualPhotos && len(v.PhotoItems) == 0 {
		return v, errors.New("Upload photographs of the stock vehicle before marking them as actual vehicle photos.")
	}
	if v.StockStatus != "Draft" && (v.Image == "/favicon.svg" || v.Engine == "" || v.Description == "") {
		return v, errors.New("Add a photo, engine details and a description before publishing this car.")
	}
	raw, _ = json.Marshal(v)
	if _, err = tx.Exec("UPDATE fleet_vehicles SET payload=? WHERE id=?", string(raw), v.ID); err != nil {
		return v, err
	}
	if err = addActivity(tx, staff, "Inventory "+v.StockStatus, v.Name()+" / "+v.ID); err != nil {
		return v, err
	}
	if err = tx.Commit(); err != nil {
		return v, err
	}
	committed = true
	return v, nil
}
func splitLines(value string) []string {
	out := []string{}
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
func oneOf(value string, values ...string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}
func serveVehicleAsset(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var vehicle, name, media, kind, key, state string
	var public bool
	err := db.QueryRow("SELECT a.vehicle_id,a.name,a.media_type,a.kind,a.public,a.storage_key,v.status FROM vehicle_assets a JOIN fleet_vehicles v ON v.id=a.vehicle_id WHERE a.id=?", r.URL.Query().Get("id")).Scan(&vehicle, &name, &media, &kind, &public, &key, &state)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, staffErr := currentStaff(db, r)
	if (state == "Draft" || !public) && staffErr != nil {
		c, e := serviceCustomer(db, r)
		var owns bool
		if e == nil {
			db.QueryRow("SELECT EXISTS(SELECT 1 FROM order_pricing p JOIN orders o ON o.id=p.order_id WHERE p.vehicle_id=? AND o.customer_id=? AND (o.status IN ('processing','out_for_delivery','delivered') OR (?='photo' AND o.status='payment_pending')))", vehicle, c.ID, kind).Scan(&owns)
		}
		if !owns {
			http.NotFound(w, r)
			return
		}
	}
	file, err := os.Open(filepath.Join(uploadDirectory(), "vehicles", key))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	disposition := "attachment"
	if kind == "photo" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, name, stat.ModTime(), file)
}

func (v Vehicle) HasCategory(category string) bool {
	for _, c := range v.Categories {
		if c == category {
			return true
		}
	}
	return false
}
func (v Vehicle) HasActiveOffer() bool {
	today := time.Now().In(dealershipTime()).Format("2006-01-02")
	return v.OfferDiscount > 0 && v.OfferName != "" && (v.OfferStart == "" || v.OfferStart <= today) && (v.OfferEnd == "" || v.OfferEnd >= today)
}

func changeVehicleAsset(db *sql.DB, r *http.Request, staff Staff) error {
	id := r.FormValue("id")
	asset := strings.TrimPrefix(r.FormValue("asset"), "/media/vehicle?id=")
	v, err := fleetVehicle(db, id, true)
	if err != nil {
		return errors.New("Vehicle not found.")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var key, kind string
	if err = tx.QueryRow("SELECT storage_key,kind FROM vehicle_assets WHERE id=? AND vehicle_id=?", asset, id).Scan(&key, &kind); err != nil {
		return errors.New("Asset not found for this vehicle.")
	}
	url := "/media/vehicle?id=" + asset
	if r.FormValue("action") == "cover" {
		if kind != "photo" {
			return errors.New("Choose a photograph for the cover.")
		}
		v.Image = url
		photos := []VehiclePhoto{}
		for _, p := range v.PhotoItems {
			if p.Image == url {
				photos = append(photos, p)
			}
		}
		for _, p := range v.PhotoItems {
			if p.Image != url {
				photos = append(photos, p)
			}
		}
		v.PhotoItems = photos
	} else {
		if kind == "photo" {
			photos := []VehiclePhoto{}
			for _, p := range v.PhotoItems {
				if p.Image != url {
					photos = append(photos, p)
				}
			}
			if len(photos) == 0 && v.StockStatus != "Draft" {
				return errors.New("Set the vehicle to Draft before removing its last photograph.")
			}
			v.PhotoItems = photos
			if v.Image == url {
				v.Image = "/favicon.svg"
				if len(photos) > 0 {
					v.Image = photos[0].Image
				}
			}
			if len(photos) == 0 {
				v.ActualPhotos = false
			}
		} else {
			docs := []VehicleDocument{}
			for _, doc := range v.DocumentItems {
				if doc.URL != url {
					docs = append(docs, doc)
				}
			}
			v.DocumentItems = docs
		}
		if _, err = tx.Exec("DELETE FROM vehicle_assets WHERE id=? AND vehicle_id=?", asset, id); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(v)
	if _, err = tx.Exec("UPDATE fleet_vehicles SET payload=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", string(raw), id); err != nil {
		return err
	}
	if err = addActivity(tx, staff, "Vehicle asset "+r.FormValue("action"), id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if r.FormValue("action") == "remove_asset" {
		os.Remove(filepath.Join(uploadDirectory(), "vehicles", key))
	}
	return nil
}

func (v Vehicle) OfferDiscountLabel() string { return "₦" + commaNumber(v.OfferDiscount) }
