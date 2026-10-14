package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

type CustomerAccountPage struct {
	ShowroomData
	Name, Email, Phone, Joined, Error, Notice              string
	CustomerID                                             int64
	Orders                                                 []ManagedOrder
	OrderCount, ActiveCount, DeliveredCount, UnreadReplies int
}

func (p CustomerAccountPage) IDLabel() string     { return fmt.Sprintf("MM-C%06d", p.CustomerID) }
func (p CustomerAccountPage) Initials() string    { return (Staff{Name: p.Name}).Initials() }
func (p CustomerAccountPage) JoinedLabel() string { return displayTime(p.Joined) }
func renderCustomerAccount(w http.ResponseWriter, status int, file string, data CustomerAccountPage) {
	t, err := template.New(file).ParseFiles(file, "partials.html", "account-partials.html")
	if err != nil {
		http.Error(w, "Could not load your account", 500)
		return
	}
	var output bytes.Buffer
	if err = t.Execute(&output, data); err != nil {
		http.Error(w, "Could not render your account", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(output.Bytes())
}

var profilePhone = regexp.MustCompile(`^[+0-9][0-9 ()-]{6,24}$`)

func serveCustomerAccount(db *sql.DB, w http.ResponseWriter, r *http.Request, page string) {
	if r.Method != http.MethodGet && !(page == "edit-profile" && r.Method == http.MethodPost) {
		http.Error(w, "Method not allowed", 405)
		return
	}
	c, err := serviceCustomer(db, r)
	if err != nil {
		http.Redirect(w, r, "/login.html", 303)
		return
	}
	data := CustomerAccountPage{ShowroomData: ShowroomData{SignedIn: true, Page: page}, CustomerID: c.ID, Name: c.Name, Email: c.Email}
	if err = db.QueryRow("SELECT phone,CAST(created_at AS TEXT) FROM customers WHERE id=?", c.ID).Scan(&data.Phone, &data.Joined); err != nil {
		http.Error(w, "Could not load your profile", 500)
		return
	}
	files := map[string]string{"dashboard": "customer-dashboard.html", "profile": "customer-profile.html", "edit-profile": "edit-profile.html"}
	if r.Method == http.MethodPost {
		data.Name = strings.TrimSpace(r.FormValue("name"))
		data.Phone = strings.TrimSpace(r.FormValue("phone"))
		digits := 0
		for _, ch := range data.Phone {
			if ch >= '0' && ch <= '9' {
				digits++
			}
		}
		if !utf8.ValidString(data.Name) || utf8.RuneCountInString(data.Name) < 2 || utf8.RuneCountInString(data.Name) > 100 {
			data.Error = "Enter your full name using 2–100 characters."
		} else if !profilePhone.MatchString(data.Phone) || digits < 7 || digits > 15 {
			data.Error = "Enter a valid contact number with 7–15 digits."
		}
		if data.Error != "" {
			renderCustomerAccount(w, 400, files[page], data)
			return
		}
		if _, err = db.Exec("UPDATE customers SET name=?,phone=? WHERE id=?", data.Name, data.Phone, c.ID); err != nil {
			http.Error(w, "Could not save your profile", 500)
			return
		}
		session, _ := store.Get(r, "customer-session")
		session.Values["customer_name"] = data.Name
		if err = session.Save(r, w); err != nil {
			http.Error(w, "Profile saved, but please sign in again to refresh your session", 500)
			return
		}
		http.Redirect(w, r, "/customer-profile?notice=saved", 303)
		return
	}
	if page == "profile" && r.URL.Query().Get("notice") == "saved" {
		data.Notice = "Your profile changes have been saved."
	}
	if page == "dashboard" {
		data.Orders, err = loadOrders(db, "WHERE o.customer_id=?", c.ID)
		if err != nil {
			http.Error(w, "Could not load your orders", 500)
			return
		}
		data.OrderCount = len(data.Orders)
		for _, order := range data.Orders {
			if order.Status == "delivered" {
				data.DeliveredCount++
			} else {
				data.ActiveCount++
			}
		}
		if len(data.Orders) > 3 {
			data.Orders = data.Orders[:3]
		}
		if err = db.QueryRow("SELECT COUNT(*) FROM support_messages m JOIN support_conversations c ON c.id=m.conversation_id WHERE c.customer_id=? AND m.sender_kind='staff' AND m.id>c.customer_read_id", c.ID).Scan(&data.UnreadReplies); err != nil {
			http.Error(w, "Could not load customer care updates", 500)
			return
		}
	}
	renderCustomerAccount(w, 200, files[page], data)
}
