package main

import "net/http"

// handleForbidden — заглушка для запрещённых операций (pull, push, create, delete, copy).
// Возвращает 403 Forbidden.
func (s *Server) handleForbidden(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "операция запрещена через прокси",
	})
}

// handleEmbedNotSupported — заглушка для /api/embed и /api/embeddings.
// Возвращает 501 Not Implemented.
func (s *Server) handleEmbedNotSupported(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "embeddings не поддерживаются через прокси",
	})
}
