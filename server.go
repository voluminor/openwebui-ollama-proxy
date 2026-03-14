package main

import (
	"log"
	"net/http"
	"sync"
	"time"
)

const modelsCacheTTL = 10 * time.Minute

// Server — HTTP-сервер, имитирующий Ollama API.
// Проксирует запросы к Open WebUI через Auth.
type Server struct {
	auth       *Auth
	httpClient *http.Client
	mux        *http.ServeMux

	// кеш моделей
	modelsMu      sync.RWMutex
	modelsCache   []OllamaModelInfo
	modelsCacheAt time.Time
}

// NewServer — создаёт сервер с настроенным роутингом
func NewServer(auth *Auth) *Server {
	s := &Server{
		auth: auth,
		httpClient: &http.Client{
			// без таймаута для streaming-запросов — они могут длиться долго
			Timeout: 0,
		},
		mux: http.NewServeMux(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes — регистрирует все маршруты
func (s *Server) setupRoutes() {
	// health check (GET и HEAD на /)
	s.mux.HandleFunc("/", s.handleRoot)

	// версия
	s.mux.HandleFunc("GET /api/version", s.handleVersion)

	// модели
	s.mux.HandleFunc("GET /api/tags", s.handleTags)
	s.mux.HandleFunc("POST /api/show", s.handleShow)
	s.mux.HandleFunc("GET /api/ps", s.handlePs)

	// чат и генерация — основные рабочие эндпоинты
	s.mux.HandleFunc("POST /api/chat", s.handleChat)
	s.mux.HandleFunc("POST /api/generate", s.handleGenerate)

	// запрещённые операции (управление моделями)
	s.mux.HandleFunc("POST /api/pull", s.handleForbidden)
	s.mux.HandleFunc("POST /api/push", s.handleForbidden)
	s.mux.HandleFunc("POST /api/create", s.handleForbidden)
	s.mux.HandleFunc("DELETE /api/delete", s.handleForbidden)
	s.mux.HandleFunc("POST /api/copy", s.handleForbidden)

	// embeddings — не поддерживаются
	s.mux.HandleFunc("POST /api/embed", s.handleEmbedNotSupported)
	s.mux.HandleFunc("POST /api/embeddings", s.handleEmbedNotSupported)
}

// ServeHTTP — реализует http.Handler, логирует запросы
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Printf("[%s] %s %s", r.RemoteAddr, r.Method, r.URL.Path)

	s.mux.ServeHTTP(w, r)

	log.Printf("[%s] %s %s — %v", r.RemoteAddr, r.Method, r.URL.Path, time.Since(start))
}

// handleRoot — GET / и HEAD /
// Стандартный ответ Ollama на health check
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// отвечаем только на корневой путь, остальное — 404
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Ollama отвечает plain text
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ollama is running"))
}

// handleVersion — GET /api/version
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, OllamaVersionResponse{Version: "0.5.4"})
}
