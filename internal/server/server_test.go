package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Lcrro/devhub/internal/store"
)

func TestServicesEndpoint(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := New(st)
	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"projects"`) {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestCrossOriginMutationBlocked(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := New(st)
	body := strings.NewReader(`{"autoRemember":true,"autoCapture":true,"scanIntervalSeconds":5}`)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17890/api/settings", body)
	req.Host = "127.0.0.1:17890"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
