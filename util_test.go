package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"openwebui-ollama-proxy/openai"
)

// // // // // // // // // //

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"key": "value"})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["key"] != "value" {
		t.Fatalf("key = %q, want %q", got["key"], "value")
	}
}

func TestWriteNDJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeNDJSON(w, map[string]int{"n": 42})

	line := w.Body.String()
	if line[len(line)-1] != '\n' {
		t.Fatal("NDJSON line must end with newline")
	}

	var got map[string]int
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["n"] != 42 {
		t.Fatalf("n = %d, want 42", got["n"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "fail: %d", 1)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["error"] != "fail: 1" {
		t.Fatalf("error = %q, want %q", got["error"], "fail: 1")
	}
}

// // // //

func TestGetFloat64(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]any
		key  string
		want float64
		ok   bool
	}{
		{"float64", map[string]any{"t": 0.7}, "t", 0.7, true},
		{"int", map[string]any{"t": 42}, "t", 42, true},
		{"json.Number", map[string]any{"t": json.Number("1.5")}, "t", 1.5, true},
		{"missing", map[string]any{}, "t", 0, false},
		{"string", map[string]any{"t": "abc"}, "t", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := getFloat64(tc.m, tc.key)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("getFloat64 = (%v, %v), want (%v, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestGetInt(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]any
		key  string
		want int
		ok   bool
	}{
		{"float64", map[string]any{"n": float64(10)}, "n", 10, true},
		{"int", map[string]any{"n": 5}, "n", 5, true},
		{"json.Number", map[string]any{"n": json.Number("99")}, "n", 99, true},
		{"missing", map[string]any{}, "n", 0, false},
		{"string", map[string]any{"n": "abc"}, "n", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := getInt(tc.m, tc.key)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("getInt = (%v, %v), want (%v, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// // // //

func TestOllamaFormatToResponseFormat(t *testing.T) {
	// "json" → json_object
	rf := ollamaFormatToResponseFormat("json")
	if rf == nil || rf.Type != "json_object" {
		t.Fatalf("json → %v, want json_object", rf)
	}

	// schema → json_schema
	schema := map[string]any{"type": "object"}
	rf = ollamaFormatToResponseFormat(schema)
	if rf == nil || rf.Type != "json_schema" {
		t.Fatalf("schema → %v, want json_schema", rf)
	}

	// nil → nil
	if ollamaFormatToResponseFormat(nil) != nil {
		t.Fatal("nil → non-nil")
	}

	// unknown string → nil
	if ollamaFormatToResponseFormat("xml") != nil {
		t.Fatal("xml → non-nil")
	}
}

func TestDetectImageMIME(t *testing.T) {
	cases := []struct {
		prefix string
		want   string
	}{
		{"/9j/4AAQ", "image/jpeg"},
		{"iVBORw0K", "image/png"},
		{"R0lGODlh", "image/gif"},
		{"UklGRlYA", "image/webp"},
		{"AAAA", "image/jpeg"}, // fallback
	}
	for _, tc := range cases {
		got := detectImageMIME(tc.prefix)
		if got != tc.want {
			t.Errorf("detectImageMIME(%q) = %q, want %q", tc.prefix[:4], got, tc.want)
		}
	}
}

func TestBuildContentParts_TextOnly(t *testing.T) {
	result := buildContentParts("hello", nil)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "hello" {
		t.Fatalf("got %q, want %q", s, "hello")
	}
}

func TestBuildContentParts_WithImages(t *testing.T) {
	images := []string{"/9j/base64data", "iVBORbase64data"}
	result := buildContentParts("describe", images)

	parts, ok := result.([]openai.ContentPart)
	if !ok {
		t.Fatalf("expected []ContentPart, got %T", result)
	}
	if len(parts) != 3 {
		t.Fatalf("len = %d, want 3", len(parts))
	}

	// first part — text
	if parts[0].Type != "text" || parts[0].Text != "describe" {
		t.Fatalf("parts[0] = %+v, want text/describe", parts[0])
	}

	// second — jpeg
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil {
		t.Fatalf("parts[1] not image_url")
	}
	if got := parts[1].ImageURL.URL; got != "data:image/jpeg;base64,/9j/base64data" {
		t.Fatalf("parts[1].URL = %q", got)
	}

	// third — png
	if parts[2].ImageURL.URL != "data:image/png;base64,iVBORbase64data" {
		t.Fatalf("parts[2].URL = %q", parts[2].ImageURL.URL)
	}
}

// // // //

func TestApplyOllamaOptions(t *testing.T) {
	opts := map[string]any{
		"temperature":       0.8,
		"top_p":             0.9,
		"num_predict":       100,
		"frequency_penalty": 0.5,
		"presence_penalty":  0.3,
		"seed":              42,
		"stop":              []any{"<|end|>", "<|stop|>"},
	}

	var req openai.ChatRequest
	applyOllamaOptions(&req, opts)

	if req.Temperature == nil || *req.Temperature != 0.8 {
		t.Fatalf("temperature = %v", req.Temperature)
	}
	if req.TopP == nil || *req.TopP != 0.9 {
		t.Fatalf("top_p = %v", req.TopP)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 100 {
		t.Fatalf("max_tokens = %v", req.MaxTokens)
	}
	if req.FrequencyPenalty == nil || *req.FrequencyPenalty != 0.5 {
		t.Fatalf("frequency_penalty = %v", req.FrequencyPenalty)
	}
	if req.PresencePenalty == nil || *req.PresencePenalty != 0.3 {
		t.Fatalf("presence_penalty = %v", req.PresencePenalty)
	}
	if req.Seed == nil || *req.Seed != 42 {
		t.Fatalf("seed = %v", req.Seed)
	}
	if len(req.Stop) != 2 || req.Stop[0] != "<|end|>" || req.Stop[1] != "<|stop|>" {
		t.Fatalf("stop = %v", req.Stop)
	}
}

func TestApplyOllamaOptions_Nil(t *testing.T) {
	var req openai.ChatRequest
	applyOllamaOptions(&req, nil)
	if req.Temperature != nil {
		t.Fatal("nil options should leave fields nil")
	}
}

func TestApplyOllamaOptions_StopStrings(t *testing.T) {
	opts := map[string]any{
		"stop": []string{"A", "B"},
	}
	var req openai.ChatRequest
	applyOllamaOptions(&req, opts)
	if len(req.Stop) != 2 || req.Stop[0] != "A" {
		t.Fatalf("stop = %v", req.Stop)
	}
}

// // // // benchmarks // // // //

func BenchmarkDetectImageMIME(b *testing.B) {
	samples := []string{"/9j/4AAQ", "iVBORw0K", "R0lGODlh", "UklGRlYA", "AAAA"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detectImageMIME(samples[i%len(samples)])
	}
}

func BenchmarkBuildContentParts_TextOnly(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buildContentParts("hello world", nil)
	}
}

func BenchmarkBuildContentParts_WithImages(b *testing.B) {
	images := []string{"/9j/base64data", "iVBORbase64data"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildContentParts("describe this", images)
	}
}

func BenchmarkApplyOllamaOptions(b *testing.B) {
	opts := map[string]any{
		"temperature":       0.8,
		"top_p":             0.9,
		"num_predict":       100,
		"frequency_penalty": 0.5,
		"presence_penalty":  0.3,
		"seed":              42,
		"stop":              []any{"<|end|>"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req openai.ChatRequest
		applyOllamaOptions(&req, opts)
	}
}

func BenchmarkWriteJSON(b *testing.B) {
	data := map[string]string{"model": "llama3", "status": "ok"}
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		writeJSON(w, http.StatusOK, data)
	}
}

func BenchmarkWriteNDJSON(b *testing.B) {
	data := map[string]string{"model": "llama3", "content": "hello"}
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		writeNDJSON(w, data)
	}
}
