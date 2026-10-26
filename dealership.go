package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
)

type BusinessProfile struct{ Name, Email, Phone, Address, Hours, Story, Team string }

func (p BusinessProfile) PhoneLink() string {
	return regexp.MustCompile(`[^+0-9]`).ReplaceAllString(p.Phone, "")
}

type Review struct {
	ID, OrderID                            int64
	Name, Vehicle, Body, Status, CreatedAt string
	Rating                                 int
}

func loadBusinessProfile(db *sql.DB) (BusinessProfile, error) {
	var p BusinessProfile
	var raw string
	err := db.QueryRow("SELECT payload FROM dealership_settings WHERE id=1").Scan(&raw)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(raw), &p)
	return p, err
}
func loadReviews(db *sql.DB, public bool) ([]Review, error) {
	query := `SELECT r.id,r.order_id,c.name,COALESCE(o.car_name,''),r.body,r.status,r.rating,CAST(r.created_at AS TEXT) FROM customer_reviews r JOIN customers c ON c.id=r.customer_id JOIN orders o ON o.id=r.order_id`
	if public {
		query += " WHERE r.status='published' AND r.consent=1 AND o.status='delivered'"
	}
	query += " ORDER BY r.id DESC LIMIT 100"
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Review{}
	for rows.Next() {
		var v Review
		if err = rows.Scan(&v.ID, &v.OrderID, &v.Name, &v.Vehicle, &v.Body, &v.Status, &v.Rating, &v.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
func manageDealership(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	d, ok := operationsData(db, w, r, "dealership", "The people behind the showroom.", "Publish your real location, contact details, opening hours and permission-based buyer reviews.")
	if !ok {
		return
	}
	if r.Method == "POST" {
		var err error
		if r.FormValue("action") == "review" {
			id, e := requestID(r, "id")
			state := r.FormValue("status")
			if e != nil || !oneOf(state, "published", "hidden") {
				err = errors.New("Choose a valid review update.")
			} else {
				tx, e := db.Begin()
				if e != nil {
					err = e
				} else {
					defer tx.Rollback()
					result, e := tx.Exec("UPDATE customer_reviews SET status=? WHERE id=? AND consent=1 AND EXISTS(SELECT 1 FROM orders o WHERE o.id=customer_reviews.order_id AND o.status='delivered')", state, id)
					err = e
					if e == nil {
						n, _ := result.RowsAffected()
						if n != 1 {
							err = errors.New("Only a consenting buyer with a delivered order can have a published review.")
						}
					}
					if err == nil {
						err = addActivity(tx, d.Staff, "Customer review "+state, r.FormValue("id"))
					}
					if err == nil {
						err = tx.Commit()
					}
				}
			}
		} else {
			err = saveBusinessProfile(db, r, d.Staff)
		}
		if err != nil {
			d.Error = err.Error()
		} else {
			http.Redirect(w, r, "/admin/dealership?saved=1", 303)
			return
		}
	} else if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d.Business, _ = loadBusinessProfile(db)
	d.Reviews, _ = loadReviews(db, false)
	renderAdmin(w, "admin-dealership.html", d)
}
func saveBusinessProfile(db *sql.DB, r *http.Request, staff Staff) error {
	p := BusinessProfile{Name: strings.TrimSpace(r.FormValue("name")), Email: strings.TrimSpace(r.FormValue("email")), Phone: strings.TrimSpace(r.FormValue("phone")), Address: strings.TrimSpace(r.FormValue("address")), Hours: strings.TrimSpace(r.FormValue("hours")), Story: strings.TrimSpace(r.FormValue("story")), Team: strings.TrimSpace(r.FormValue("team"))}
	if len(p.Name) < 2 || len(p.Name) > 100 || len(p.Address) > 500 || len(p.Hours) > 1000 || len(p.Story) > 5000 || len(p.Team) > 3000 {
		return errors.New("Enter a business name and keep the profile within the displayed limits.")
	}
	if p.Email != "" {
		parsed, e := mail.ParseAddress(p.Email)
		if e != nil || parsed.Address != p.Email {
			return errors.New("Enter a valid business email.")
		}
	}
	if p.Phone != "" && !profilePhone.MatchString(p.Phone) {
		return errors.New("Enter a valid business phone number.")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	raw, _ := json.Marshal(p)
	if _, err = tx.Exec("UPDATE dealership_settings SET payload=? WHERE id=1", string(raw)); err != nil {
		return err
	}
	if err = addActivity(tx, staff, "Public dealership profile updated", p.Name); err != nil {
		return err
	}
	return tx.Commit()
}
func serveDealership(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d := struct {
		ShowroomData
		Reviews []Review
	}{ShowroomData: publicCommerceData(db, r, "dealership").ShowroomData}
	d.Reviews, _ = loadReviews(db, true)
	renderBusinessPage(w, 200, "dealership.html", d)
}
func customerReview(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	c, err := serviceCustomer(db, r)
	if err != nil {
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	d := publicCommerceData(db, r, "reviews")
	if r.Method == "POST" {
		rating, e := integerField(r, "rating", 1, 5)
		order, e2 := requestID(r, "order")
		body := strings.TrimSpace(r.FormValue("body"))
		consent := r.FormValue("consent") == "on"
		if e != nil || e2 != nil || len(body) < 10 || len(body) > 2000 || !consent {
			d.Error = "Choose a delivered order, rate it 1–5, write 10–2,000 characters and confirm permission to publish your name and review."
		} else {
			result, e := db.Exec("INSERT INTO customer_reviews(customer_id,order_id,rating,body,consent) SELECT ?,?,?,?,1 WHERE EXISTS(SELECT 1 FROM orders WHERE id=? AND customer_id=? AND status='delivered') ON CONFLICT(order_id) DO NOTHING", c.ID, order, rating, body, order, c.ID)
			if e != nil {
				d.Error = "Your review could not be saved. Please try again."
			} else {
				n, _ := result.RowsAffected()
				if n != 1 {
					d.Error = "This order is not eligible, or you have already reviewed it."
				} else {
					http.Redirect(w, r, "/account/reviews?saved=1", 303)
					return
				}
			}
		}
	} else if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	d.ReviewOrders, _ = loadOrders(db, "WHERE o.customer_id=? AND o.status='delivered'", c.ID)
	if r.URL.Query().Get("saved") == "1" {
		d.Notice = "Thank you. Your review is awaiting moderation."
	}
	renderBusinessPage(w, 200, "customer-reviews.html", d)
}
