package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// // // // // // // // // //

func TestResponseRecorder_CapturesStatus(t *testing.T) {
	w := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	rr.WriteHeader(http.StatusNotFound)
	if rr.statusCode != http.StatusNotFound {
		t.Fatalf("statusCode = %d, want %d", rr.statusCode, http.StatusNotFound)
	}
}

func TestResponseRecorder_DefaultStatus(t *testing.T) {
	w := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	// without calling WriteHeader — default status 200
	if rr.statusCode != http.StatusOK {
		t.Fatalf("default status = %d, want %d", rr.statusCode, http.StatusOK)
	}
}

func TestResponseRecorder_FirstWriteHeaderWins(t *testing.T) {
	w := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	rr.WriteHeader(http.StatusCreated)
	rr.WriteHeader(http.StatusInternalServerError) // repeated call does not change

	if rr.statusCode != http.StatusCreated {
		t.Fatalf("statusCode = %d, want %d (first call)", rr.statusCode, http.StatusCreated)
	}
}

func TestResponseRecorder_Flush(t *testing.T) {
	w := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	// httptest.ResponseRecorder implements Flusher — should not panic
	rr.Flush()
}

func TestResponseRecorder_Unwrap(t *testing.T) {
	w := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	if rr.Unwrap() != w {
		t.Fatal("Unwrap should return underlying ResponseWriter")
	}
}

// // // //

func TestRateLimiter_AllowsBurst(t *testing.T) {
	rl := newRateLimiter(5) // 5 rps

	// first 5 requests — burst
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Fatalf("request %d rejected, expected allowed", i+1)
		}
	}

	// 6th should be rejected
	if rl.Allow() {
		t.Fatal("6th request allowed, expected rejected")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := newRateLimiter(10) // 10 rps

	// exhaust tokens
	for i := 0; i < 10; i++ {
		rl.Allow()
	}
	if rl.Allow() {
		t.Fatal("should be exhausted")
	}

	// wait 150ms → +1.5 tokens → 1 request should pass
	time.Sleep(150 * time.Millisecond)
	if !rl.Allow() {
		t.Fatal("should refill after wait")
	}
}

func TestRateLimiter_String(t *testing.T) {
	rl := newRateLimiter(42)
	if s := rl.String(); s != "42 rps" {
		t.Fatalf("String() = %q, want %q", s, "42 rps")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := newRateLimiter(100)
	var wg sync.WaitGroup

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow()
		}()
	}
	wg.Wait()
}

// // // //

func TestServeHTTP_CORS(t *testing.T) {
	srv := &Server{
		corsOrigins: "*",
		mux:         http.NewServeMux(),
	}
	srv.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// regular request
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("CORS origin = %q, want %q", got, "*")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("missing Allow-Methods header")
	}
}

func TestServeHTTP_CORS_Preflight(t *testing.T) {
	srv := &Server{
		corsOrigins: "https://example.com",
		mux:         http.NewServeMux(),
	}

	req := httptest.NewRequest(http.MethodOptions, "/api/chat", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("origin = %q", got)
	}
}

func TestServeHTTP_CORS_Disabled(t *testing.T) {
	srv := &Server{
		corsOrigins: "",
		mux:         http.NewServeMux(),
	}
	srv.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("CORS should be disabled, got %q", got)
	}
}

func TestServeHTTP_RateLimit(t *testing.T) {
	srv := &Server{
		mux:         http.NewServeMux(),
		rateLimiter: newRateLimiter(2), // 2 rps
	}
	srv.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// first 2 — OK
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d", i+1, w.Code)
		}
	}

	// 3rd — rate limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestServeHTTP_NoRateLimit(t *testing.T) {
	srv := &Server{
		mux: http.NewServeMux(),
		// rateLimiter = nil
	}
	srv.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// 100 requests — all OK
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d", i+1, w.Code)
		}
	}
}

// // // //

func TestHandleRoot_GET(t *testing.T) {
	srv := &Server{mux: http.NewServeMux()}
	srv.mux.HandleFunc("/", srv.handleRoot)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if body := w.Body.String(); body != "Ollama is running" {
		t.Fatalf("body = %q", body)
	}
}

func TestHandleRoot_HEAD(t *testing.T) {
	srv := &Server{mux: http.NewServeMux()}
	srv.mux.HandleFunc("/", srv.handleRoot)

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestHandleRoot_NotFound(t *testing.T) {
	srv := &Server{mux: http.NewServeMux()}
	srv.mux.HandleFunc("/", srv.handleRoot)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleVersion(t *testing.T) {
	srv := &Server{ollamaVersion: "0.5.4", mux: http.NewServeMux()}
	srv.mux.HandleFunc("GET /api/version", srv.handleVersion)

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if body := w.Body.String(); !contains(body, `"version":"0.5.4"`) {
		t.Fatalf("body = %q", body)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstr(s, substr)
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// // // // benchmarks // // // //

func BenchmarkRateLimiter_Allow(b *testing.B) {
	rl := newRateLimiter(1000000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow()
	}
}

func BenchmarkRateLimiter_Allow_Parallel(b *testing.B) {
	rl := newRateLimiter(1000000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rl.Allow()
		}
	})
}

func BenchmarkServeHTTP_WithCORS(b *testing.B) {
	srv := &Server{
		corsOrigins: "*",
		mux:         http.NewServeMux(),
	}
	srv.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}
}

func BenchmarkServeHTTP_WithRateLimit(b *testing.B) {
	srv := &Server{
		mux:         http.NewServeMux(),
		rateLimiter: newRateLimiter(1000000),
	}
	srv.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}
}
