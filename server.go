package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"openwebui-ollama-proxy/auth"
	"openwebui-ollama-proxy/cache"
	"openwebui-ollama-proxy/ollama"
)

// // // // // // // // // //

// Server — HTTP server emulating the Ollama API
type Server struct {
	auth              *auth.Obj
	cacheDir          string
	maxBodySize       int64
	maxErrorBody      int64
	tagsTTL           time.Duration
	showTTL           time.Duration
	streamIdleTimeout time.Duration
	corsOrigins       string
	ollamaVersion     string
	httpClient        *http.Client // streaming: no timeout
	httpClientShort   *http.Client // non-streaming: with timeout
	mux               *http.ServeMux
	rateLimiter       *rateLimiterObj

	// in-memory model cache (L1, on top of disk cache)
	modelsMu      sync.RWMutex
	modelsCache   []ollama.ModelInfo
	modelsCacheAt time.Time

	// thundering herd protection for model fetching
	tagsFetchMu sync.Mutex
}

// NewServer — creates a server with routing
func NewServer(a *auth.Obj, cacheDir string, maxBodySize, maxErrorBody int64, tagsTTL, showTTL, timeout, streamIdleTimeout time.Duration, ollamaVersion, corsOrigins string, rateLimit int) *Server {
	s := &Server{
		auth:              a,
		cacheDir:          cacheDir,
		maxBodySize:       maxBodySize,
		maxErrorBody:      maxErrorBody,
		tagsTTL:           tagsTTL,
		showTTL:           showTTL,
		streamIdleTimeout: streamIdleTimeout,
		corsOrigins:       corsOrigins,
		ollamaVersion:     ollamaVersion,
		httpClient: &http.Client{
			Timeout: 0, // streaming can last indefinitely
		},
		httpClientShort: &http.Client{
			Timeout: timeout,
		},
		mux: http.NewServeMux(),
	}

	if rateLimit > 0 {
		s.rateLimiter = newRateLimiter(float64(rateLimit))
	}

	// preload model cache from disk on startup
	if cached := cache.ReadTags(cacheDir); cached != nil && time.Now().Before(cached.ExpiresAt) {
		s.modelsCache = cached.Models
		s.modelsCacheAt = cached.ExpiresAt.Add(-tagsTTL)
	}

	s.setupRoutes()
	return s
}

// // // //

// setupRoutes — registers routes
func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/", s.handleRoot)

	s.mux.HandleFunc("GET /api/version", s.handleVersion)

	// models
	s.mux.HandleFunc("GET /api/tags", s.handleTags)
	s.mux.HandleFunc("POST /api/show", s.handleShow)
	s.mux.HandleFunc("GET /api/ps", s.handlePs)

	// chat and generation
	s.mux.HandleFunc("POST /api/chat", s.handleChat)
	s.mux.HandleFunc("POST /api/generate", s.handleGenerate)

	// forbidden operations
	s.mux.HandleFunc("POST /api/pull", s.handleForbidden)
	s.mux.HandleFunc("POST /api/push", s.handleForbidden)
	s.mux.HandleFunc("POST /api/create", s.handleForbidden)
	s.mux.HandleFunc("DELETE /api/delete", s.handleForbidden)
	s.mux.HandleFunc("POST /api/copy", s.handleForbidden)

	// embeddings — not supported
	s.mux.HandleFunc("POST /api/embed", s.handleEmbedNotSupported)
	s.mux.HandleFunc("POST /api/embeddings", s.handleEmbedNotSupported)
}

// // // //

// ServeHTTP — implements http.Handler; CORS, rate limit, logging
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS
	if s.corsOrigins != "" {
		w.Header().Set("Access-Control-Allow-Origin", s.corsOrigins)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	// rate limit
	if s.rateLimiter != nil && !s.rateLimiter.Allow() {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	start := time.Now()
	rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	s.mux.ServeHTTP(rr, r)
	log.Printf("[%s] %s %s %d %v", r.RemoteAddr, r.Method, r.URL.Path, rr.statusCode, time.Since(start))
}

// // // //

// responseRecorder — wrapper for capturing status code
type responseRecorder struct {
	http.ResponseWriter
	statusCode  int
	headersSent bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if !rr.headersSent {
		rr.statusCode = code
		rr.headersSent = true
	}
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Flush() {
	if f, ok := rr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rr *responseRecorder) Unwrap() http.ResponseWriter {
	return rr.ResponseWriter
}

// // // //

// rateLimiterObj — token bucket without external dependencies
type rateLimiterObj struct {
	mu       sync.Mutex
	tokens   float64
	maxBurst float64
	rate     float64 // tokens/sec
	lastTime time.Time
}

func newRateLimiter(rps float64) *rateLimiterObj {
	return &rateLimiterObj{
		tokens:   rps,
		maxBurst: rps,
		rate:     rps,
		lastTime: time.Now(),
	}
}

func (rl *rateLimiterObj) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rl.tokens += now.Sub(rl.lastTime).Seconds() * rl.rate
	rl.lastTime = now

	if rl.tokens > rl.maxBurst {
		rl.tokens = rl.maxBurst
	}
	if rl.tokens < 1 {
		return false
	}
	rl.tokens--
	return true
}

func (rl *rateLimiterObj) String() string {
	return fmt.Sprintf("%.0f rps", rl.rate)
}

// handleRoot — health check (GET / and HEAD /)
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ollama is running"))
}

// handleVersion — GET /api/version
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ollama.VersionResponse{Version: s.ollamaVersion})
}
