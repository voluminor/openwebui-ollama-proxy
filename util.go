package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"openwebui-ollama-proxy/openai"
)

// // // // // // // // // //

// лимит тела запроса — защита от OOM
const maxBodySize = 10 << 20 // 10 MB

// // // //

// writeJSON — JSON-ответ с заданным статусом
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeNDJSON — одна JSON-строка (NDJSON)
func writeNDJSON(w http.ResponseWriter, v any) {
	data, _ := json.Marshal(v)
	w.Write(data)
	w.Write([]byte("\n"))
}

// writeError — JSON-ошибка с логированием
func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[error] %s", msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// // // //

// getFloat64 — извлекает float64 из map
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

// getInt — извлекает int из map
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

// applyOllamaOptions — конвертирует Ollama options в поля OpenAI-запроса
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
