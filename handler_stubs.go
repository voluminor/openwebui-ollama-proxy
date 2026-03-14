package main

import "net/http"

// // // // // // // // // //

// handleForbidden — заглушка для запрещённых операций (pull, push, create, delete, copy)
func (s *Server) handleForbidden(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "operation forbidden via proxy",
	})
}

// handleEmbedNotSupported — заглушка для /api/embed и /api/embeddings
func (s *Server) handleEmbedNotSupported(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "embeddings not supported via proxy",
	})
}
