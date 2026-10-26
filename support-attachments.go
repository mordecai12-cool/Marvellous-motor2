package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const supportAttachmentLimit = 5 << 20

type SupportAttachment struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}
type incomingSupportFile struct {
	Name, Type, Digest string
	Data               []byte
}

func readSupportFile(r *http.Request) (*incomingSupportFile, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	files := r.MultipartForm.File["attachment"]
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) != 1 {
		return nil, errors.New("Attach one file per message.")
	}
	header := files[0]
	if header.Size > supportAttachmentLimit {
		return nil, errors.New("Attachments must be 5 MB or smaller.")
	}
	file, err := header.Open()
	if err != nil {
		return nil, errors.New("Could not read your attachment.")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, supportAttachmentLimit+1))
	if err != nil || len(data) == 0 || len(data) > supportAttachmentLimit {
		return nil, errors.New("Choose a non-empty file of 5 MB or smaller.")
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if !utf8.ValidString(name) || len(name) > 200 || strings.ContainsAny(name, "\r\n\x00") {
		return nil, errors.New("Use a file name of at most 200 bytes.")
	}
	mediaType := http.DetectContentType(data)
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case (mediaType == "image/jpeg" && (ext == ".jpg" || ext == ".jpeg")) || (mediaType == "image/png" && ext == ".png"):
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 50_000_000 {
			return nil, errors.New("Choose a valid JPEG or PNG image under 50 megapixels.")
		}
	case mediaType == "application/pdf" && ext == ".pdf":
	case strings.HasPrefix(mediaType, "text/plain") && ext == ".txt" && utf8.Valid(data):
		mediaType = "text/plain"
	default:
		return nil, errors.New("Choose a JPEG, PNG, PDF or plain text (.txt) file.")
	}
	digest := sha256.Sum256(data)
	return &incomingSupportFile{Name: name, Type: mediaType, Digest: hex.EncodeToString(digest[:]), Data: data}, nil
}

// Store below the configured uploads directory, which deployments can persist.
// The caller removes this file if its database transaction does not commit.
func saveSupportFile(tx *sql.Tx, id int64, file *incomingSupportFile) (string, error) {
	dir := filepath.Join(uploadDirectory(), "support")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	key := hex.EncodeToString(random)
	path := filepath.Join(dir, key)
	if err := os.WriteFile(path, file.Data, 0600); err != nil {
		return "", err
	}
	_, err := tx.Exec("INSERT INTO support_attachments(message_id,name,media_type,size,storage_key,digest) VALUES(?,?,?,?,?,?)", id, file.Name, file.Type, len(file.Data), key, file.Digest)
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func serveSupportAttachment(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", 405)
		return
	}
	customer, customerErr := serviceCustomer(db, r)
	_, staffErr := currentStaff(db, r)
	if customerErr != nil && staffErr != nil {
		http.Error(w, "Sign in to view this attachment", 401)
		return
	}
	id, err := serviceQueryID(r, "id")
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	var name, mediaType, key string
	var owner int64
	err = db.QueryRow(`SELECT a.name,a.media_type,a.storage_key,c.customer_id FROM support_attachments a JOIN support_messages m ON m.id=a.message_id JOIN support_conversations c ON c.id=m.conversation_id WHERE a.message_id=?`, id).Scan(&name, &mediaType, &key, &owner)
	if err != nil || (staffErr != nil && customer.ID != owner) {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(filepath.Join(uploadDirectory(), "support", key))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Could not load attachment", 500)
		return
	}
	disposition := "attachment"
	if strings.HasPrefix(mediaType, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, stat.ModTime(), file)
}
func supportAttachmentURL(id int64) string { return fmt.Sprintf("/api/support/attachments?id=%d", id) }
