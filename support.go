package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type SupportConversation struct {
	CaseReference  string `json:"case_reference"`
	CaseStatus     string `json:"case_status"`
	Assignee       string `json:"assignee"`
	AssignedID     int64  `json:"assigned_id"`
	ID             int64  `json:"id"`
	CustomerID     int64  `json:"customer_id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	CustomerReadID int64  `json:"customer_read_id"`
	StaffReadID    int64  `json:"staff_read_id"`
	UnreadCustomer int    `json:"unread_customer"`
	UnreadStaff    int    `json:"unread_staff"`
	LastMessage    string `json:"last_message"`
	UpdatedAt      int64  `json:"updated_at"`
}
type SupportMessage struct {
	Attachment *SupportAttachment `json:"attachment,omitempty"`
	ID         int64              `json:"id"`
	SenderKind string             `json:"sender_kind"`
	SenderName string             `json:"sender_name"`
	Body       string             `json:"body"`
	OrderID    int64              `json:"order_id"`
	CreatedAt  int64              `json:"created_at"`
	ClientID   string             `json:"client_id"`
}
type SupportSnapshot struct {
	Staff         []CaseStaff           `json:"available_staff,omitempty"`
	Conversation  *SupportConversation  `json:"conversation"`
	Conversations []SupportConversation `json:"conversations,omitempty"`
	Messages      []SupportMessage      `json:"messages"`
	Online        bool                  `json:"online"`
	HasOlder      bool                  `json:"has_older"`
}

var supportConversationSelect = `SELECT c.id,c.customer_id,u.name,u.email,c.status,c.customer_read_id,c.staff_read_id,
 (SELECT COUNT(*) FROM support_messages m WHERE m.conversation_id=c.id AND m.sender_kind='staff' AND m.id>c.customer_read_id),
 (SELECT COUNT(*) FROM support_messages m WHERE m.conversation_id=c.id AND m.sender_kind='customer' AND m.id>c.staff_read_id),
 COALESCE((SELECT m.body FROM support_messages m WHERE m.conversation_id=c.id ORDER BY m.id DESC LIMIT 1),''),c.updated_at,COALESCE(k.status,c.status),COALESCE(k.assigned_to,0),` + staffDisplayNameSQL("a", "COALESCE(a.name,'')") + `
 FROM support_conversations c JOIN customers u ON u.id=c.customer_id LEFT JOIN support_cases k ON k.conversation_id=c.id LEFT JOIN admins a ON a.id=k.assigned_to `

func loadSupportConversations(db *sql.DB, where string, args ...any) ([]SupportConversation, error) {
	rows, err := db.Query(supportConversationSelect+where+" ORDER BY c.updated_at DESC,c.id DESC LIMIT 100", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []SupportConversation{}
	for rows.Next() {
		var c SupportConversation
		if err = rows.Scan(&c.ID, &c.CustomerID, &c.Name, &c.Email, &c.Status, &c.CustomerReadID, &c.StaffReadID, &c.UnreadCustomer, &c.UnreadStaff, &c.LastMessage, &c.UpdatedAt, &c.CaseStatus, &c.AssignedID, &c.Assignee); err != nil {
			return nil, err
		}
		c.CaseReference = fmt.Sprintf("MM-S%06d", c.ID)
		list = append(list, c)
	}
	return list, rows.Err()
}
func supportOnline(db *sql.DB) (bool, error) {
	var online bool
	query := "SELECT EXISTS(SELECT 1 FROM support_presence p JOIN admins a ON a.id=p.staff_id WHERE p.last_seen>? AND a.status='approved' AND a.role IN ('admin','super_admin')"
	args := []any{time.Now().Add(-60 * time.Second).Unix()}
	if ownerOnlyAccess() {
		query += " AND (a.role='admin' OR lower(a.email) IN (?,?))"
		args = append(args, deploymentOwnerEmail, deploymentSecondOwnerEmail)
	}
	err := db.QueryRow(query+")", args...).Scan(&online)
	return online, err
}
func supportMessages(db *sql.DB, id, before int64) ([]SupportMessage, bool, error) {
	if before == 0 {
		before = 1 << 62
	}
	rows, err := db.Query("SELECT m.id,m.sender_kind,"+staffDisplayNameSQL("sender", "m.sender_name")+",m.body,COALESCE(m.order_id,0),m.client_id,m.created_at,COALESCE(a.name,''),COALESCE(a.media_type,''),COALESCE(a.size,0) FROM support_messages m LEFT JOIN admins sender ON m.sender_kind='staff' AND sender.id=m.sender_id LEFT JOIN support_attachments a ON a.message_id=m.id WHERE m.conversation_id=? AND m.id<? ORDER BY m.id DESC LIMIT 100", id, before)
	if err != nil {
		return nil, false, err
	}
	messages := []SupportMessage{}
	for rows.Next() {
		var m SupportMessage
		var attachment SupportAttachment
		if err = rows.Scan(&m.ID, &m.SenderKind, &m.SenderName, &m.Body, &m.OrderID, &m.ClientID, &m.CreatedAt, &attachment.Name, &attachment.Type, &attachment.Size); err != nil {
			rows.Close()
			return nil, false, err
		}
		if attachment.Name != "" {
			attachment.URL = supportAttachmentURL(m.ID)
			m.Attachment = &attachment
		}
		messages = append(messages, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	for a, b := 0, len(messages)-1; a < b; a, b = a+1, b-1 {
		messages[a], messages[b] = messages[b], messages[a]
	}
	var older bool
	if len(messages) > 0 {
		err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM support_messages WHERE conversation_id=? AND id<?)", id, messages[0].ID).Scan(&older)
	}
	return messages, older, err
}
func supportBefore(r *http.Request) (int64, error) {
	if r.URL.Query().Get("before") == "" {
		return 0, nil
	}
	id, err := serviceQueryID(r, "before")
	if err == nil && id < 1 {
		err = errAccess
	}
	return id, err
}
func customerSupportAPI(db *sql.DB, w http.ResponseWriter, r *http.Request, action string) {
	want := http.MethodPost
	if action == "get" {
		want = http.MethodGet
	}
	if r.Method != want {
		serviceError(w, 405, "Method not allowed.")
		return
	}
	c, err := serviceCustomer(db, r)
	if err != nil {
		serviceError(w, 401, "Sign in to chat privately with our team.")
		return
	}
	if action == "send" {
		sendSupportMessage(db, w, r, c, Staff{})
		return
	}
	conversations, err := loadSupportConversations(db, "WHERE c.customer_id=?", c.ID)
	if err != nil {
		serviceError(w, 500, "Could not load your conversation.")
		return
	}
	if action == "read" {
		if len(conversations) == 0 {
			serviceJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		markSupportRead(db, w, r, conversations[0], false)
		return
	}
	before, err := supportBefore(r)
	if err != nil {
		serviceError(w, 400, "Invalid message history request.")
		return
	}
	s := SupportSnapshot{Messages: []SupportMessage{}}
	if len(conversations) > 0 {
		s.Conversation = &conversations[0]
		s.Messages, s.HasOlder, err = supportMessages(db, s.Conversation.ID, before)
	}
	if err == nil {
		s.Online, err = supportOnline(db)
	}
	if err != nil {
		serviceError(w, 500, "Could not load support messages.")
		return
	}
	serviceJSON(w, 200, s)
}
func serveSupportInbox(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	staff, ok := requireStaff(db, w, r)
	if !ok {
		return
	}
	metrics, err := loadMetrics(db)
	if err != nil {
		http.Error(w, "Could not load workspace", 500)
		return
	}
	renderServicePage(w, "admin-support.html", AdminPage{Staff: staff, Metrics: metrics, Page: "support", Title: "A helpful human, one message away.", Subtitle: "Private customer conversations. Real replies from your operations team."})
}
func staffSupportAPI(db *sql.DB, w http.ResponseWriter, r *http.Request, action string) {
	want := http.MethodPost
	if action == "get" {
		want = http.MethodGet
	}
	if r.Method != want {
		serviceError(w, 405, "Method not allowed.")
		return
	}
	staff, err := currentStaff(db, r)
	if err != nil {
		serviceError(w, 401, "Your staff session has ended. Sign in again.")
		return
	}
	if action == "case" {
		updateSupportCase(db, w, r, staff)
		return
	}
	if action == "presence" {
		available := r.FormValue("available")
		if available != "true" && available != "false" {
			serviceError(w, 400, "Invalid availability.")
			return
		}
		seen := int64(0)
		if available == "true" {
			seen = time.Now().Unix()
		}
		_, err = db.Exec("INSERT INTO support_presence(staff_id,last_seen) VALUES(?,?) ON CONFLICT(staff_id) DO UPDATE SET last_seen=excluded.last_seen", staff.ID, seen)
		if err != nil {
			serviceError(w, 500, "Could not update availability.")
			return
		}
		serviceJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if action == "send" {
		sendSupportMessage(db, w, r, ServiceCustomer{}, staff)
		return
	}
	if action == "get" {
		where := ""
		args := []any{}
		if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
			where = "WHERE (u.name LIKE ? OR u.email LIKE ?)"
			args = []any{"%" + q + "%", "%" + q + "%"}
		}
		list, err := loadSupportConversations(db, where, args...)
		if err != nil {
			serviceError(w, 500, "Could not load inbox.")
			return
		}
		s := SupportSnapshot{Conversations: list, Messages: []SupportMessage{}}
		s.Staff, _ = availableCaseStaff(db)
		if raw := r.URL.Query().Get("id"); raw != "" {
			id, err := serviceQueryID(r, "id")
			if err != nil || id < 1 {
				serviceError(w, 400, "Invalid conversation.")
				return
			}
			selected, err := loadSupportConversations(db, "WHERE c.id=?", id)
			if err != nil {
				serviceError(w, 500, "Could not load conversation.")
				return
			}
			if len(selected) == 0 {
				serviceError(w, 404, "Conversation not found.")
				return
			}
			before, err := supportBefore(r)
			if err != nil {
				serviceError(w, 400, "Invalid message history request.")
				return
			}
			s.Conversation = &selected[0]
			s.Messages, s.HasOlder, err = supportMessages(db, id, before)
			if err != nil {
				serviceError(w, 500, "Could not load messages.")
				return
			}
		}
		s.Online, err = supportOnline(db)
		if err != nil {
			serviceError(w, 500, "Could not load availability.")
			return
		}
		serviceJSON(w, 200, s)
		return
	}
	id, err := requestID(r, "conversation-id")
	if err != nil {
		serviceError(w, 400, "Invalid conversation.")
		return
	}
	conversations, err := loadSupportConversations(db, "WHERE c.id=?", id)
	if err != nil {
		serviceError(w, 500, "Could not load conversation.")
		return
	}
	if len(conversations) == 0 {
		serviceError(w, 404, "Conversation not found.")
		return
	}
	if action == "read" {
		markSupportRead(db, w, r, conversations[0], true)
		return
	}
	if action == "status" {
		status := r.FormValue("status")
		if status != "open" && status != "resolved" {
			serviceError(w, 400, "Invalid conversation status.")
			return
		}
		tx, err := db.Begin()
		if err != nil {
			serviceError(w, 500, "Could not update conversation.")
			return
		}
		defer tx.Rollback()
		if _, err = tx.Exec("UPDATE support_conversations SET status=?,updated_at=? WHERE id=?", status, time.Now().Unix(), id); err == nil {
			err = addActivity(tx, staff, "Support conversation "+status, fmt.Sprintf("Conversation #%d", id))
		}
		if err != nil {
			serviceError(w, 500, "Could not record conversation update.")
			return
		}
		if _, err = tx.Exec("INSERT INTO support_cases(conversation_id,status) VALUES(?,?) ON CONFLICT(conversation_id) DO UPDATE SET status=excluded.status,updated_at=CURRENT_TIMESTAMP", id, status); err != nil {
			serviceError(w, 500, "Could not save complaint status.")
			return
		}
		if err = tx.Commit(); err != nil {
			serviceError(w, 500, "Could not save conversation.")
			return
		}
		serviceJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	serviceError(w, 404, "Action not found.")
}
func markSupportRead(db *sql.DB, w http.ResponseWriter, r *http.Request, c SupportConversation, staff bool) {
	id, err := requestID(r, "message-id")
	if err != nil {
		serviceError(w, 400, "Invalid read position.")
		return
	}
	var exists bool
	if err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM support_messages WHERE conversation_id=? AND id=?)", c.ID, id).Scan(&exists); err != nil {
		serviceError(w, 500, "Could not check message.")
		return
	}
	if !exists {
		serviceError(w, 400, "Message does not belong to this conversation.")
		return
	}
	field := "customer_read_id"
	if staff {
		field = "staff_read_id"
	}
	if _, err = db.Exec("UPDATE support_conversations SET "+field+"=MAX("+field+",?) WHERE id=?", id, c.ID); err != nil {
		serviceError(w, 500, "Could not update read position.")
		return
	}
	serviceJSON(w, 200, map[string]bool{"ok": true})
}

var supportClientID = regexp.MustCompile(`^[a-zA-Z0-9-]{16,80}$`)

func sendSupportMessage(db *sql.DB, w http.ResponseWriter, r *http.Request, customer ServiceCustomer, staff Staff) {
	body := strings.TrimSpace(r.FormValue("message"))
	clientID := r.FormValue("client-id")
	file, fileErr := readSupportFile(r)
	if fileErr != nil {
		serviceError(w, 400, fileErr.Error())
		return
	}
	if body == "" && file != nil {
		body = "Attachment: " + file.Name
	}

	if !utf8.ValidString(body) || utf8.RuneCountInString(body) < 1 || utf8.RuneCountInString(body) > 2000 || !supportClientID.MatchString(clientID) {
		serviceError(w, 400, "Enter a message of 1–2,000 characters and retry the send.")
		return
	}
	orderID := int64(0)
	var err error
	if raw := r.FormValue("order-id"); raw != "" && raw != "0" {
		orderID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || orderID < 1 {
			serviceError(w, 400, "Invalid order reference.")
			return
		}
	}
	tx, err := db.Begin()
	if err != nil {
		serviceError(w, 500, "Could not start your message.")
		return
	}
	defer tx.Rollback()
	kind := "customer"
	senderID := customer.ID
	senderName := customer.Name
	customerID := customer.ID
	var conversationID int64
	staffState := ""
	now := time.Now().Unix()
	if staff.ID > 0 {
		kind = "staff"
		senderID = staff.ID
		senderName = staffDisplayName(staff)
		conversationID, err = requestID(r, "conversation-id")
		if err != nil {
			serviceError(w, 400, "Choose a conversation.")
			return
		}
		if err = tx.QueryRow("SELECT customer_id,status FROM support_conversations WHERE id=?", conversationID).Scan(&customerID, &staffState); err != nil {
			serviceError(w, 404, "Conversation not found.")
			return
		}
	}
	if orderID > 0 {
		var owns bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM orders WHERE id=? AND customer_id=?)", orderID, customerID).Scan(&owns); err != nil {
			serviceError(w, 500, "Could not check order reference.")
			return
		}
		if !owns {
			serviceError(w, 404, "Order not found for this customer.")
			return
		}
	}
	if staff.ID == 0 {
		if _, err = tx.Exec("INSERT INTO support_conversations(customer_id,created_at,updated_at) VALUES(?,?,?) ON CONFLICT(customer_id) DO NOTHING", customerID, now, now); err != nil {
			serviceError(w, 500, "Could not open your conversation.")
			return
		}
		if err = tx.QueryRow("SELECT id FROM support_conversations WHERE customer_id=?", customerID).Scan(&conversationID); err != nil {
			serviceError(w, 500, "Could not find your conversation.")
			return
		}
	}
	var existingID, existingOrder int64
	var existingBody string
	err = tx.QueryRow("SELECT id,body,COALESCE(order_id,0) FROM support_messages WHERE conversation_id=? AND sender_kind=? AND sender_id=? AND client_id=?", conversationID, kind, senderID, clientID).Scan(&existingID, &existingBody, &existingOrder)
	if err == nil {
		var digest, name string
		attachmentErr := tx.QueryRow("SELECT digest,name FROM support_attachments WHERE message_id=?", existingID).Scan(&digest, &name)
		if attachmentErr != nil && !errors.Is(attachmentErr, sql.ErrNoRows) {
			serviceError(w, 500, "Could not check attachment.")
			return
		}
		if (file == nil && attachmentErr == nil) || (file != nil && (attachmentErr != nil || digest != file.Digest || name != file.Name)) {
			serviceError(w, 409, "This send reference was already used for a different attachment.")
			return
		}
		if existingBody != body || existingOrder != orderID {
			serviceError(w, 409, "This send reference was already used for a different message.")
			return
		}
		serviceJSON(w, 200, map[string]any{"id": existingID, "replayed": true})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		serviceError(w, 500, "Could not check your message.")
		return
	}
	if staffState == "resolved" {
		serviceError(w, 409, "Reopen this conversation before replying.")
		return
	}
	var recent int
	if err = tx.QueryRow("SELECT COUNT(*) FROM support_messages WHERE sender_kind=? AND sender_id=? AND created_at>?", kind, senderID, now-60).Scan(&recent); err != nil {
		serviceError(w, 500, "Could not check message delivery.")
		return
	}
	if recent >= 30 {
		w.Header().Set("Retry-After", "60")
		serviceError(w, 429, "Please wait a minute before sending more messages.")
		return
	}
	var order any
	if orderID > 0 {
		order = orderID
	}
	result, err := tx.Exec("INSERT INTO support_messages(conversation_id,sender_kind,sender_id,sender_name,body,order_id,client_id,created_at) VALUES(?,?,?,?,?,?,?,?)", conversationID, kind, senderID, senderName, body, order, clientID, now)
	if err != nil {
		serviceError(w, 500, "Your message was not saved. Please retry.")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		serviceError(w, 500, "Could not confirm message delivery.")
		return
	}
	var storedFile string
	committed := false
	defer func() {
		if storedFile != "" && !committed {
			os.Remove(storedFile)
		}
	}()
	if file != nil {
		storedFile, err = saveSupportFile(tx, id, file)
		if err != nil {
			serviceError(w, 500, "Your attachment was not saved. Please retry.")
			return
		}
	}
	if _, err = tx.Exec("UPDATE support_conversations SET status='open',updated_at=? WHERE id=?", now, conversationID); err != nil {
		serviceError(w, 500, "Could not update conversation.")
		return
	}
	caseStatus := "open"
	if staff.ID > 0 {
		caseStatus = "awaiting_customer"
	}
	_, err = tx.Exec("INSERT INTO support_cases(conversation_id,status,assigned_to) VALUES(?,?,?) ON CONFLICT(conversation_id) DO UPDATE SET status=CASE WHEN excluded.status='awaiting_customer' THEN excluded.status ELSE CASE WHEN support_cases.status IN ('resolved','awaiting_customer') THEN 'open' ELSE support_cases.status END END,assigned_to=COALESCE(support_cases.assigned_to,excluded.assigned_to),updated_at=CURRENT_TIMESTAMP", conversationID, caseStatus, nullableID(staff.ID))
	if err != nil {
		serviceError(w, 500, "Could not update complaint.")
		return
	}
	if staff.ID > 0 {
		if err = addActivity(tx, staff, "Customer support reply sent", fmt.Sprintf("Conversation #%d", conversationID)); err != nil {
			serviceError(w, 500, "Could not record support reply.")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		serviceError(w, 500, "Your message was not saved. Please retry.")
		return
	}
	committed = true
	serviceJSON(w, 201, map[string]any{"id": id, "replayed": false})
}
