package main

import (
	"bytes"
	"html/template"
	"net/http"
)

type CustomerAuthPage struct{ Error, Notice string }

func renderCustomerAuth(w http.ResponseWriter, status int, file string, data CustomerAuthPage) {
	t, err := template.ParseFiles(file)
	if err != nil {
		http.Error(w, "Could not load account page", 500)
		return
	}
	var body bytes.Buffer
	if err = t.Execute(&body, data); err != nil {
		http.Error(w, "Could not render account page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(body.Bytes())
}
func serveCustomerSignIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	renderCustomerAuth(w, 200, "login.html", CustomerAuthPage{})
}
