package httpx

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type requestIDContextKey struct{}

var sensitiveLogPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(authorization["'=:\s]+)[^,\s"]+`),
	regexp.MustCompile(`(?i)(api[_-]?key["'=:\s]+)[^,\s"]+`),
	regexp.MustCompile(`(?i)(password["'=:\s]+)[^,\s"]+`),
	regexp.MustCompile(`\b(enc_)[A-Za-z0-9_-]{8,}\b`),
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := sanitizeRequestID(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = NewID("req")
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

func sanitizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return ""
	}
	return value
}

func (a *App) accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf(
			"request id=%s method=%s path=%s status=%d bytes=%d duration=%s remote=%s",
			requestIDFromContext(r.Context()),
			r.Method,
			redactForLog(r.URL.EscapedPath()),
			rec.status,
			rec.bytes,
			time.Since(start).Round(time.Millisecond),
			clientIP(r, a.trustProxy),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func secureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' data: https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) corsMiddleware(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	allowAll := false
	for _, origin := range a.corsOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			allowAll = true
			continue
		}
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && (allowAll || originAllowed(allowed, origin)) {
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			allowHeaders, varyRequestHeaders := corsAllowHeaderValue(r.Header.Get("Access-Control-Request-Headers"))
			w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			if varyRequestHeaders {
				w.Header().Add("Vary", "Access-Control-Request-Headers")
			}
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func originAllowed(allowed map[string]struct{}, origin string) bool {
	_, ok := allowed[origin]
	return ok
}

var corsAllowedRequestHeaders = []string{
	"Authorization",
	"Content-Type",
	"X-API-Key",
	"X-Request-ID",
	"Idempotency-Key",
	"X-Client-Request-Id",
}

func corsAllowHeaderValue(requested string) (string, bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return strings.Join(corsAllowedRequestHeaders, ", "), false
	}
	parts := strings.Split(requested, ",")
	reflected := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if !corsHeaderNameOK(name) {
			continue
		}
		canonical, ok := canonicalCORSHeader(name)
		if !ok {
			continue
		}
		lower := strings.ToLower(canonical)
		if _, dup := seen[lower]; dup {
			continue
		}
		seen[lower] = struct{}{}
		reflected = append(reflected, canonical)
	}
	return strings.Join(reflected, ", "), true
}

func canonicalCORSHeader(name string) (string, bool) {
	lower := strings.ToLower(name)
	for _, allowed := range corsAllowedRequestHeaders {
		if strings.ToLower(allowed) == lower {
			return allowed, true
		}
	}
	if strings.HasPrefix(lower, "openai-") {
		return name, true
	}
	return "", false
}

func corsHeaderNameOK(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r >= 127 {
			return false
		}
		switch r {
		case '(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}':
			return false
		}
	}
	return true
}

func (a *App) maxRequestBodyBytes() int64 {
	maxUploadBytes := a.maxUploadMB * 1024 * 1024
	if maxUploadBytes <= 0 {
		maxUploadBytes = 16 * 1024 * 1024
	}
	chatLimit := int64(16 * 1024 * 1024)
	if maxUploadBytes < chatLimit {
		return chatLimit
	}
	return maxUploadBytes
}

func redactForLog(value string) string {
	out := value
	for _, pattern := range sensitiveLogPatterns {
		out = pattern.ReplaceAllString(out, "${1}<redacted>")
	}
	return out
}
