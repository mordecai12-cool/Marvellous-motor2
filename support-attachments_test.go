package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func attachmentRequest(t *testing.T, customer int64, nonce, name string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("client-id", nonce)
	file, err := form.CreateFormFile("attachment", name)
	if err != nil {
		t.Fatal(err)
	}
	file.Write(data)
	form.Close()
	r := httptest.NewRequest("POST", "/api/support/send", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	if customer > 0 {
		r.AddCookie(customerCookie(t, customer, ""))
	}
	return r
}
func complaintPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestSupportAttachmentPrivacyPersistenceAndRetries(t *testing.T) {
	db := serviceFixture(t)
	dir := t.TempDir()
	t.Setenv("MM_UPLOADS_DIR", dir)
	data := complaintPNG(t)
	for _, status := range []int{201, 200} {
		w := httptest.NewRecorder()
		customerSupportAPI(db, w, attachmentRequest(t, 1, "attachment-retry-0001", "complaint.png", data), "send")
		if w.Code != status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	state := supportState(t, db, 1)
	if len(state.Messages) != 1 || state.Messages[0].Attachment == nil {
		t.Fatal("Attachment did not persist", state)
	}
	file := state.Messages[0].Attachment
	if file.Name != "complaint.png" || file.Type != "image/png" || file.Size != int64(len(data)) {
		t.Fatal(file)
	}
	files, err := os.ReadDir(filepath.Join(dir, "support"))
	if err != nil || len(files) != 1 {
		t.Fatal("Retry wrote extra files", err)
	}
	for _, tc := range []struct {
		customer, staff int64
		status          int
	}{{0, 0, 401}, {2, 0, 404}, {1, 0, 200}, {0, 2, 200}, {0, 3, 401}} {
		r := httptest.NewRequest("GET", file.URL, nil)
		if tc.customer > 0 {
			r.AddCookie(customerCookie(t, tc.customer, ""))
		}
		if tc.staff > 0 {
			r.AddCookie(staffCookie(t, tc.staff))
		}
		w := httptest.NewRecorder()
		serveSupportAttachment(db, w, r)
		if w.Code != tc.status {
			t.Fatalf("Attachment access %+v: %d", tc, w.Code)
		}
		if w.Code == 200 && (!bytes.Equal(w.Body.Bytes(), data) || w.Header().Get("Content-Type") != "image/png") {
			t.Fatal("Image bytes or type changed")
		}
	}
	w := httptest.NewRecorder()
	customerSupportAPI(db, w, attachmentRequest(t, 1, "attachment-retry-0001", "complaint.txt", []byte("different evidence")), "send")
	if w.Code != 409 {
		t.Fatal("Retry accepted different attachment", w.Code)
	}
	// Staff can also send files back to the same customer.
	r := attachmentRequest(t, 0, "staff-attachment-0001", "review.txt", []byte("We reviewed your complaint."))
	if err = r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	defer r.MultipartForm.RemoveAll()
	r.Form.Set("conversation-id", "1")
	r.AddCookie(staffCookie(t, 2))
	w = httptest.NewRecorder()
	staffSupportAPI(db, w, r, "send")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	state = supportState(t, db, 1)
	if len(state.Messages) != 2 || state.Messages[1].SenderKind != "staff" || state.Messages[1].Attachment == nil {
		t.Fatal("Staff file missing in customer chat")
	}
	r = httptest.NewRequest("GET", state.Messages[1].Attachment.URL, nil)
	r.AddCookie(customerCookie(t, 1, ""))
	w = httptest.NewRecorder()
	serveSupportAttachment(db, w, r)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("Document not served as private download")
	}
}
func TestSupportRejectsInvalidAttachmentsWithoutMessagesOrFiles(t *testing.T) {
	db := serviceFixture(t)
	dir := t.TempDir()
	t.Setenv("MM_UPLOADS_DIR", dir)
	for _, tc := range []struct {
		name string
		data []byte
	}{{"script.html", []byte("<script>alert(1)</script>")}, {"fake.png", []byte("not a picture")}, {"wrong.pdf", complaintPNG(t)}, {"empty.txt", nil}, {"large.txt", bytes.Repeat([]byte("x"), supportAttachmentLimit+1)}} {
		w := httptest.NewRecorder()
		customerSupportAPI(db, w, attachmentRequest(t, 1, "invalid-attachment-0001", tc.name, tc.data), "send")
		if w.Code != 400 {
			t.Fatal(tc.name, w.Code, w.Body.String())
		}
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM support_messages").Scan(&count)
	if count != 0 {
		t.Fatal("Invalid upload created a message")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatal("Invalid upload left files", err)
	}
	// Authentication happens before any attachment is persisted.
	w := httptest.NewRecorder()
	customerSupportAPI(db, w, attachmentRequest(t, 0, "anonymous-upload-0001", "complaint.png", complaintPNG(t)), "send")
	if w.Code != 401 {
		t.Fatal("Anonymous attachment accepted")
	}
}
func TestSupportAttachmentsRespectCSRFAndMultipartLimits(t *testing.T) {
	db := serviceFixture(t)
	t.Setenv("MM_UPLOADS_DIR", t.TempDir())
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { customerSupportAPI(db, w, r, "send") }))
	r := attachmentRequest(t, 1, "csrf-attachment-0001", "complaint.png", complaintPNG(t))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("Attachment bypassed CSRF", w.Code)
	}
	r = attachmentRequest(t, 1, "csrf-attachment-0001", "complaint.png", complaintPNG(t))
	token := strings.Repeat("a", 64)
	r.AddCookie(&http.Cookie{Name: "mm-csrf", Value: token})
	r.Header.Set("X-CSRF-Token", token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal("Valid multipart attachment failed", w.Code, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
}

func TestSupportAttachmentRollbackRemovesFileAndMessage(t *testing.T) {
	db := serviceFixture(t)
	dir := t.TempDir()
	t.Setenv("MM_UPLOADS_DIR", dir)
	if _, err := db.Exec(`CREATE TRIGGER reject_support_update BEFORE UPDATE ON support_conversations BEGIN SELECT RAISE(ABORT,'test update failure'); END;`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	customerSupportAPI(db, w, attachmentRequest(t, 1, "rollback-attachment-0001", "complaint.png", complaintPNG(t)), "send")
	if w.Code != 500 {
		t.Fatal("Expected simulated save failure", w.Code)
	}
	for _, table := range []string{"support_messages", "support_attachments"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("Failed upload left database records", table, count, err)
		}
	}
	files, err := os.ReadDir(filepath.Join(dir, "support"))
	if err != nil || len(files) != 0 {
		t.Fatal("Failed transaction left private file behind", err)
	}
}
