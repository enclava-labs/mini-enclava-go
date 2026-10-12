package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerAddsRequestIDSecureHeadersAndCORS(t *testing.T) {
	app := &App{
		mux:         http.NewServeMux(),
		corsOrigins: []string{"https://admin.example.com"},
		maxUploadMB: 1,
	}
	app.mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFromContext(r.Context()); got != "req-test" {
			t.Fatalf("expected request id in context, got %q", got)
		}
		WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	req.Header.Set("X-Request-ID", "req-test")
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Request-ID"); got != "req-test" {
		t.Fatalf("expected response request id header, got %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.example.com" {
		t.Fatalf("expected CORS origin, got %q", got)
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	app := &App{
		mux:         http.NewServeMux(),
		corsOrigins: []string{"https://admin.example.com"},
		maxUploadMB: 1,
	}
	app.mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("preflight should not reach route handler")
	})

	req := httptest.NewRequest(http.MethodOptions, "/ok", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Fatalf("expected CORS methods, got %q", got)
	}
}

func TestCORSPreflightReflectsProxyHeaders(t *testing.T) {
	app := &App{
		mux:         http.NewServeMux(),
		corsOrigins: []string{"https://admin.example.com"},
		maxUploadMB: 1,
	}
	app.mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("preflight should not reach route handler")
	})

	req := httptest.NewRequest(http.MethodOptions, "/v1/responses", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	req.Header.Set("Access-Control-Request-Headers", "Idempotency-Key, OpenAI-Beta, X-Client-Request-Id, X-Evil")
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PATCH") {
		t.Fatalf("expected PATCH in CORS methods, got %q", got)
	}
	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	for _, name := range []string{"Idempotency-Key", "OpenAI-Beta", "X-Client-Request-Id"} {
		if !strings.Contains(allowHeaders, name) {
			t.Fatalf("expected %s in allow headers, got %q", name, allowHeaders)
		}
	}
	if strings.Contains(allowHeaders, "X-Evil") {
		t.Fatalf("did not expect X-Evil in allow headers, got %q", allowHeaders)
	}
	if strings.Contains(allowHeaders, "Authorization") {
		t.Fatalf("unrequested allow-list header should not be reflected, got %q", allowHeaders)
	}
	vary := strings.Join(rec.Header().Values("Vary"), ", ")
	if !strings.Contains(vary, "Access-Control-Request-Headers") {
		t.Fatalf("expected Vary for request headers, got %q", vary)
	}
}
