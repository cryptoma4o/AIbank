package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aibank/platform/services/document-service/internal/domain"
	"github.com/aibank/platform/services/document-service/internal/storage"
)

// fakeRepo — потокобезопасный in-memory репозиторий для unit-тестов.
type fakeRepo struct {
	mu   sync.Mutex
	docs map[string]*domain.Document
}

func newFakeRepo() *fakeRepo { return &fakeRepo{docs: map[string]*domain.Document{}} }

func (r *fakeRepo) Create(_ context.Context, doc *domain.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs[doc.ID] = doc
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, _ /*tenantID*/, id string) (*domain.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.docs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return d, nil
}

func (r *fakeRepo) ListByApplication(_ context.Context, _, applicationID string) ([]*domain.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*domain.Document{}
	for _, d := range r.docs {
		if d.ApplicationID == applicationID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (r *fakeRepo) MarkParsed(context.Context, string, string, json.RawMessage) error { return nil }
func (r *fakeRepo) MarkRejected(context.Context, string, string, string) error        { return nil }

// newTestHandler собирает handler с in-memory зависимостями.
func newTestHandler() (*DocumentHandler, *fakeRepo, *storage.InMemoryStorage) {
	repo := newFakeRepo()
	st := storage.NewInMemoryStorage()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewDocumentHandler(repo, st, nil, log), repo, st
}

// buildMultipart собирает multipart-тело с заданным набором полей и опциональным файлом.
func buildMultipart(t *testing.T, fields map[string]string, fileField, filename, mimeType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if fileField != "" {
		hdr := make(map[string][]string)
		hdr["Content-Disposition"] = []string{`form-data; name="` + fileField + `"; filename="` + filename + `"`}
		hdr["Content-Type"] = []string{mimeType}
		fw, err := mw.CreatePart(hdr)
		if err != nil {
			t.Fatalf("CreatePart: %v", err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

func TestUploadDocumentHappyPath(t *testing.T) {
	t.Parallel()
	h, repo, st := newTestHandler()

	payload := []byte("%PDF-1.4 minimal pdf body")
	body, contentType := buildMultipart(t,
		map[string]string{
			"tenant_id":      "alfa",
			"application_id": "app_123",
			"type":           "passport",
		},
		"file", "passport.pdf", "application/pdf", payload,
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/documents", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	h.UploadDocument(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var doc domain.Document
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.TenantID != "alfa" || doc.ApplicationID != "app_123" {
		t.Errorf("unexpected ids: %+v", doc)
	}
	if doc.SizeBytes != int64(len(payload)) {
		t.Errorf("size %d, want %d", doc.SizeBytes, len(payload))
	}
	if doc.SHA256 == "" {
		t.Error("sha256 must be computed")
	}
	if doc.State != domain.DocumentStateUploaded {
		t.Errorf("state %q, want uploaded", doc.State)
	}
	if doc.Type != domain.DocumentTypePassport {
		t.Errorf("type %q, want passport", doc.Type)
	}

	// Файл должен быть в storage.
	repo.mu.Lock()
	count := len(repo.docs)
	repo.mu.Unlock()
	if count != 1 {
		t.Errorf("repo should hold 1 document, has %d", count)
	}
	key := storageKeyFromPath(doc.StoragePath)
	exists, err := st.Exists(context.Background(), key)
	if err != nil || !exists {
		t.Errorf("storage missing key %q: exists=%v err=%v", key, exists, err)
	}
}

func TestUploadDocumentMissingTenant(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandler()

	body, contentType := buildMultipart(t,
		map[string]string{
			"application_id": "app_123",
			"type":           "passport",
		},
		"file", "passport.pdf", "application/pdf", []byte("body"),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/documents", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	h.UploadDocument(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "missing_tenant") {
		t.Errorf("expected missing_tenant code, got: %s", rr.Body.String())
	}
}

func TestUploadDocumentTooLarge(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandler()

	// Размер > MaxFileSize (25MB) — должны получить 413.
	big := bytes.Repeat([]byte{0x25}, int(MaxFileSize)+1024)
	body, contentType := buildMultipart(t,
		map[string]string{
			"tenant_id":      "alfa",
			"application_id": "app_big",
			"type":           "passport",
		},
		"file", "huge.pdf", "application/pdf", big,
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/documents", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	h.UploadDocument(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "file_too_large") {
		t.Errorf("expected file_too_large code, got: %s", rr.Body.String())
	}
}
