package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"openwebui-ollama-proxy/auth"
	"openwebui-ollama-proxy/cache"
	"openwebui-ollama-proxy/ollama"
)

// // // // // // // // // //

// Server — HTTP-сервер, имитирующий Ollama API
type Server struct {
	auth       *auth.Obj
	cacheDir   string
	httpClient *http.Client
	mux        *http.ServeMux

	// in-memory кеш моделей (L1, поверх дискового)
	modelsMu      sync.RWMutex
	modelsCache   []ollama.ModelInfo
	modelsCacheAt time.Time
}

// NewServer — создаёт сервер с роутингом
func NewServer(a *auth.Obj, cacheDir string) *Server {
	s := &Server{
		auth:     a,
		cacheDir: cacheDir,
		httpClient: &http.Client{
			// без таймаута — streaming-запросы могут длиться долго
			Timeout: 0,
		},
		mux: http.NewServeMux(),
	}

	// предзагрузка кеша моделей с диска при старте
	if cached := cache.ReadTags(cacheDir); cached != nil && time.Now().Before(cached.ExpiresAt) {
		s.modelsCache = cached.Models
		s.modelsCacheAt = cached.ExpiresAt.Add(-cache.TagsTTL)
	}

	s.setupRoutes()
	return s
}

// // // //

// setupRoutes — регистрирует маршруты
func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/", s.handleRoot)

	s.mux.HandleFunc("GET /api/version", s.handleVersion)

	// модели
	s.mux.HandleFunc("GET /api/tags", s.handleTags)
	s.mux.HandleFunc("POST /api/show", s.handleShow)
	s.mux.HandleFunc("GET /api/ps", s.handlePs)

	// чат и генерация
	s.mux.HandleFunc("POST /api/chat", s.handleChat)
	s.mux.HandleFunc("POST /api/generate", s.handleGenerate)

	// запрещённые операции
	s.mux.HandleFunc("POST /api/pull", s.handleForbidden)
	s.mux.HandleFunc("POST /api/push", s.handleForbidden)
	s.mux.HandleFunc("POST /api/create", s.handleForbidden)
	s.mux.HandleFunc("DELETE /api/delete", s.handleForbidden)
	s.mux.HandleFunc("POST /api/copy", s.handleForbidden)

	// embeddings — не поддерживаются
	s.mux.HandleFunc("POST /api/embed", s.handleEmbedNotSupported)
	s.mux.HandleFunc("POST /api/embeddings", s.handleEmbedNotSupported)
}

// // // //

// ServeHTTP — реализует http.Handler, логирует запросы
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Printf("[%s] %s %s", r.RemoteAddr, r.Method, r.URL.Path)

	s.mux.ServeHTTP(w, r)

	log.Printf("[%s] %s %s — %v", r.RemoteAddr, r.Method, r.URL.Path, time.Since(start))
}

// handleRoot — health check (GET / и HEAD /)
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
	writeJSON(w, http.StatusOK, ollama.VersionResponse{Version: "0.5.4"})
}
