package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openwebui-ollama-proxy/ollama"
)

// // // // // // // // // //

func TestBuildShowResponse(t *testing.T) {
	resp := buildShowResponse("llama3:8b")

	if resp.Name != "llama3:8b" {
		t.Fatalf("Name = %q", resp.Name)
	}
	if resp.Model != "llama3:8b" {
		t.Fatalf("Model = %q", resp.Model)
	}
	if resp.Details.Format != "proxy" {
		t.Fatalf("Format = %q", resp.Details.Format)
	}
	if resp.Details.Family != "unknown" {
		t.Fatalf("Family = %q", resp.Details.Family)
	}
	if resp.Template != "{{ .Prompt }}" {
		t.Fatalf("Template = %q", resp.Template)
	}
	if resp.Modelfile != "FROM llama3:8b" {
		t.Fatalf("Modelfile = %q", resp.Modelfile)
	}
	if resp.Digest != "proxy-llama3:8b" {
		t.Fatalf("Digest = %q", resp.Digest)
	}
	if resp.ModifiedAt == "" {
		t.Fatal("ModifiedAt is empty")
	}
}

// // // //

func TestHandlePs(t *testing.T) {
	srv := &Server{mux: http.NewServeMux()}
	srv.mux.HandleFunc("GET /api/ps", srv.handlePs)

	req := httptest.NewRequest(http.MethodGet, "/api/ps", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp ollama.PsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Models) != 0 {
		t.Fatalf("models = %d, want 0", len(resp.Models))
	}
}

func TestHandleShow(t *testing.T) {
	dir := t.TempDir()
	srv := &Server{
		cacheDir:    dir,
		maxBodySize: 1 << 20,
		showTTL:     0, // no caching
		mux:         http.NewServeMux(),
	}
	srv.mux.HandleFunc("POST /api/show", srv.handleShow)

	body := `{"model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp ollama.ShowResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "gpt-4o" {
		t.Fatalf("name = %q", resp.Name)
	}
	if resp.Details.Format != "proxy" {
		t.Fatalf("format = %q", resp.Details.Format)
	}
}

func TestHandleShow_EmptyModel(t *testing.T) {
	srv := &Server{
		maxBodySize: 1 << 20,
		mux:         http.NewServeMux(),
	}
	srv.mux.HandleFunc("POST /api/show", srv.handleShow)

	body := `{"model":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleShow_InvalidJSON(t *testing.T) {
	srv := &Server{
		maxBodySize: 1 << 20,
		mux:         http.NewServeMux(),
	}
	srv.mux.HandleFunc("POST /api/show", srv.handleShow)

	req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// // // //

func TestHandleForbidden(t *testing.T) {
	srv := &Server{mux: http.NewServeMux()}
	srv.mux.HandleFunc("POST /api/pull", srv.handleForbidden)

	req := httptest.NewRequest(http.MethodPost, "/api/pull", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleEmbedNotSupported(t *testing.T) {
	srv := &Server{mux: http.NewServeMux()}
	srv.mux.HandleFunc("POST /api/embed", srv.handleEmbedNotSupported)

	req := httptest.NewRequest(http.MethodPost, "/api/embed", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

// // // // benchmarks // // // //

func BenchmarkBuildShowResponse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buildShowResponse("llama3:70b-instruct-q4_0")
	}
}

func BenchmarkHandlePs(b *testing.B) {
	srv := &Server{mux: http.NewServeMux()}
	srv.mux.HandleFunc("GET /api/ps", srv.handlePs)
	req := httptest.NewRequest(http.MethodGet, "/api/ps", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
	}
}
