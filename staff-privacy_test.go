package main

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func privateStaffFixture(t *testing.T) (*sql.DB, Staff) {
	t.Helper()
	db := serviceFixture(t)
	s := Staff{ID: 4, Name: "Emmanuel Michel", Email: strings.ToUpper(deploymentOwnerEmail), Role: "super_admin", Status: "approved"}
	if _, err := db.Exec("INSERT INTO admins(id,name,email,password,role,status) VALUES(?,?,?,?,?,?)", s.ID, s.Name, s.Email, "unused", s.Role, s.Status); err != nil {
		t.Fatal(err)
	}
	return db, s
}

func TestPrivateOwnerRosterVisibilityAndPermissions(t *testing.T) {
	db, private := privateStaffFixture(t)
	if _, err := db.Exec("INSERT INTO staff_invitations(email,token_hash,created_by,expires_at) VALUES(?,'private-owner-old-invite',1,9999999999)", private.Email); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []int64{1, 2, private.ID} {
		r := httptest.NewRequest("GET", "/admin/team", nil)
		r.AddCookie(staffCookie(t, viewer))
		w := httptest.NewRecorder()
		serveAdminPage(db, w, r, "team")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		body := w.Body.String()
		if viewer != private.ID && (strings.Contains(body, private.Name) || strings.Contains(strings.ToLower(body), deploymentOwnerEmail)) {
			t.Fatal("Private profile leaked to another administrator")
		}
		if viewer == private.ID && (!strings.Contains(body, "Test Owner") || !strings.Contains(body, "Test Staff")) {
			t.Fatal("Private owner lost visibility of other admins")
		}
	}
	options, err := availableCaseStaff(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range options {
		if s.ID == private.ID {
			t.Fatal("Private owner exposed in assignment options")
		}
	}
	r := httptest.NewRequest("GET", "/admin.html", nil)
	r.AddCookie(staffCookie(t, private.ID))
	current, err := currentStaff(db, r)
	if err != nil || current.Role != "super_admin" || !authorizedStaff(current) {
		t.Fatal("Privacy changed owner access", err)
	}
	// Owner-only access management still works for this account.
	w := httptest.NewRecorder()
	manageTeam(db, w, adminPost(t, "/admin/team", url.Values{"admin-id": {"2"}, "status": {"disabled"}}, private.ID), "update")
	if w.Code != 303 {
		t.Fatal("Private owner cannot manage another admin", w.Code, w.Body.String())
	}
}

func TestPrivateOwnerHistoricalIdentityIsMaskedWithoutDeletingAudit(t *testing.T) {
	db, private := privateStaffFixture(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = addActivity(tx, private, "Staff account created", deploymentOwnerEmail); err != nil {
		t.Fatal(err)
	}
	if err = addOrderEvent(tx, private, 1, "processing", "processing", "Reviewed order"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	activity, err := loadActivity(db)
	if err != nil || len(activity) != 1 {
		t.Fatal(activity, err)
	}
	if activity[0].Actor != privateStaffLabel || strings.Contains(activity[0].Entity, deploymentOwnerEmail) {
		t.Fatal("Historical activity exposes private identity", activity)
	}
	events, err := loadOrderEvents(db, 1)
	if err != nil || len(events) != 1 || events[0].ActorName != privateStaffLabel {
		t.Fatal("Order history exposes identity", events, err)
	}
	var raw string
	if err = db.QueryRow("SELECT actor_name FROM admin_activity WHERE actor_id=?", private.ID).Scan(&raw); err != nil || raw != private.Name {
		t.Fatal("Internal audit identity was erased", raw, err)
	}
	if _, err = db.Exec("INSERT INTO enquiries(id,kind,name,email,vehicle,message) VALUES(1,'test-drive','Buyer','buyer@example.test','Car','Please call'); INSERT INTO sales_followups(enquiry_id,assigned_to) VALUES(1,4)"); err != nil {
		t.Fatal(err)
	}
	leads, err := loadSales(db)
	if err != nil || len(leads) != 1 || leads[0].Assignee != privateStaffLabel {
		t.Fatal("Sales assignment exposes identity", leads, err)
	}
	r := httptest.NewRequest("GET", "/admin/sales", nil)
	r.AddCookie(staffCookie(t, 1))
	w := httptest.NewRecorder()
	manageSales(db, w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), private.Name) || !strings.Contains(w.Body.String(), `<option value="4" selected>Marvellous Motors team</option>`) {
		t.Fatal("Sales form leaks profile or loses existing assignment", w.Code, w.Body.String())
	}
}

func TestPrivateOwnerChatIdentityMaskedForCustomersAndAdmins(t *testing.T) {
	db, private := privateStaffFixture(t)
	if w := customerMessage(t, db, 1, "Please help", "privacy-customer-0001"); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := db.Exec("UPDATE support_cases SET status='investigating',assigned_to=? WHERE conversation_id=1; INSERT INTO support_messages(conversation_id,sender_kind,sender_id,sender_name,body,client_id,created_at) VALUES(1,'staff',?,?,'Earlier reply','privacy-old-0001',1)", private.ID, private.ID, private.Name); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	staffSupportAPI(db, w, adminPost(t, "/admin/support/send", url.Values{"conversation-id": {"1"}, "message": {"New reply"}, "client-id": {"privacy-staff-0001"}}, private.ID), "send")
	if w.Code != 201 {
		t.Fatal("Private owner cannot reply", w.Code, w.Body.String())
	}
	state := supportState(t, db, 1)
	if state.Conversation == nil || state.Conversation.Assignee != privateStaffLabel {
		t.Fatal("Case assignment exposes identity", state.Conversation)
	}
	for _, m := range state.Messages {
		if m.SenderKind == "staff" && m.SenderName != privateStaffLabel {
			t.Fatal("Chat exposes private name", m)
		}
	}
	r := httptest.NewRequest("GET", "/admin/support?id=1", nil)
	r.AddCookie(staffCookie(t, 1))
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, r, "get")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var snapshot SupportSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), private.Name) || strings.Contains(strings.ToLower(w.Body.String()), deploymentOwnerEmail) {
		t.Fatal("Staff support JSON exposes private profile", w.Body.String())
	}
}
