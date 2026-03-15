package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"openwebui-ollama-proxy/openai"
)

// // // // // // // // // //

// writeJSON — JSON response with given status
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeNDJSON — single JSON line (NDJSON)
func writeNDJSON(w http.ResponseWriter, v any) {
	data, _ := json.Marshal(v)
	w.Write(data)
	w.Write([]byte("\n"))
}

// writeError — JSON error with logging
func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[error] %s", msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// // // //

// getFloat64 — extracts float64 from map
func getFloat64(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// getInt — extracts int from map
func getInt(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

// // // //

// ollamaFormatToResponseFormat — converts Ollama format to OpenAI response_format.
// "json" → {type: "json_object"}, schema object → {type: "json_schema", json_schema: ...}
func ollamaFormatToResponseFormat(format any) *openai.ResponseFormat {
	switch f := format.(type) {
	case string:
		if f == "json" {
			return &openai.ResponseFormat{Type: "json_object"}
		}
	case map[string]any:
		return &openai.ResponseFormat{Type: "json_schema", JSONSchema: f}
	}
	return nil
}

// detectImageMIME — detects MIME type by magic bytes at the start of base64 string
func detectImageMIME(b64 string) string {
	switch {
	case strings.HasPrefix(b64, "/9j/"):
		return "image/jpeg"
	case strings.HasPrefix(b64, "iVBOR"):
		return "image/png"
	case strings.HasPrefix(b64, "R0lGO"):
		return "image/gif"
	case strings.HasPrefix(b64, "UklGR"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// buildContentParts — builds content for OpenAI request.
// Without images returns a string; with images — []ContentPart (text first, then images).
func buildContentParts(text string, images []string) any {
	if len(images) == 0 {
		return text
	}
	parts := make([]openai.ContentPart, 0, 1+len(images))
	parts = append(parts, openai.ContentPart{Type: "text", Text: text})
	for _, img := range images {
		mime := detectImageMIME(img)
		parts = append(parts, openai.ContentPart{
			Type:     "image_url",
			ImageURL: &openai.ImageURL{URL: "data:" + mime + ";base64," + img},
		})
	}
	return parts
}

// applyOllamaOptions — converts Ollama options to OpenAI request fields
func applyOllamaOptions(req *openai.ChatRequest, options map[string]any) {
	if options == nil {
		return
	}

	if v, ok := getFloat64(options, "temperature"); ok {
		req.Temperature = &v
	}
	if v, ok := getFloat64(options, "top_p"); ok {
		req.TopP = &v
	}
	if v, ok := getInt(options, "num_predict"); ok {
		req.MaxTokens = &v
	}
	if v, ok := getFloat64(options, "frequency_penalty"); ok {
		req.FrequencyPenalty = &v
	}
	if v, ok := getFloat64(options, "presence_penalty"); ok {
		req.PresencePenalty = &v
	}
	if v, ok := getInt(options, "seed"); ok {
		req.Seed = &v
	}
	if v, ok := options["stop"]; ok {
		switch s := v.(type) {
		case []any:
			stops := make([]string, 0, len(s))
			for _, item := range s {
				if str, ok := item.(string); ok {
					stops = append(stops, str)
				}
			}
			if len(stops) > 0 {
				req.Stop = stops
			}
		case []string:
			req.Stop = s
		}
	}
}
