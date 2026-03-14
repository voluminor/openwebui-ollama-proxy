package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleChat — POST /api/chat
// Проксирует чат-запросы к Open WebUI, конвертируя формат Ollama → OpenAI и обратно.
// Поддерживает как streaming (NDJSON), так и non-streaming режимы.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// декодируем запрос в формате Ollama
	var ollamaReq OllamaChatRequest
	if err := json.NewDecoder(r.Body).Decode(&ollamaReq); err != nil {
		writeError(w, http.StatusBadRequest, "невалидный JSON: %v", err)
		return
	}

	if ollamaReq.Model == "" {
		writeError(w, http.StatusBadRequest, "model обязателен")
		return
	}

	// в Ollama stream по умолчанию true
	streaming := true
	if ollamaReq.Stream != nil {
		streaming = *ollamaReq.Stream
	}

	// конвертируем сообщения Ollama → OpenAI
	messages := make([]OpenAIMessage, len(ollamaReq.Messages))
	for i, m := range ollamaReq.Messages {
		messages[i] = OpenAIMessage{Role: m.Role, Content: m.Content}
	}

	// собираем запрос к Open WebUI
	openaiReq := OpenAIChatRequest{
		Model:    ollamaReq.Model,
		Messages: messages,
		Stream:   streaming,
	}
	applyOllamaOptions(&openaiReq, ollamaReq.Options)

	token, err := s.auth.EnsureToken(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка авторизации: %v", err)
		return
	}

	// отправляем запрос к Open WebUI
	payload, _ := json.Marshal(openaiReq)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.auth.baseURL+"/api/chat/completions", bytes.NewReader(payload))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "создание запроса: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "запрос к Open WebUI: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, "Open WebUI: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		return
	}

	if streaming {
		s.streamChatResponse(w, resp.Body, ollamaReq.Model)
	} else {
		s.nonStreamChatResponse(w, resp.Body, ollamaReq.Model)
	}
}

// nonStreamChatResponse — обрабатывает обычный (не streaming) ответ от Open WebUI
func (s *Server) nonStreamChatResponse(w http.ResponseWriter, body io.Reader, model string) {
	data, _ := io.ReadAll(body)

	var openaiResp OpenAIChatResponse
	if err := json.Unmarshal(data, &openaiResp); err != nil {
		writeError(w, http.StatusBadGateway, "декодирование ответа: %v", err)
		return
	}

	if len(openaiResp.Choices) == 0 {
		writeError(w, http.StatusBadGateway, "пустой ответ от Open WebUI")
		return
	}

	choice := openaiResp.Choices[0]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	writeJSON(w, http.StatusOK, OllamaChatResponse{
		Model:     model,
		CreatedAt: now,
		Message: OllamaMessage{
			Role:    "assistant",
			Content: choice.Message.Content,
		},
		Done:            true,
		DoneReason:      "stop",
		TotalDuration:   0,
		PromptEvalCount: 0,
		EvalCount:       len(choice.Message.Content) / 4, // примерная оценка токенов
	})
}

// streamChatResponse — читает SSE-поток от Open WebUI и отдаёт NDJSON в формате Ollama
func (s *Server) streamChatResponse(w http.ResponseWriter, body io.Reader, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming не поддерживается")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	startTime := time.Now()
	evalCount := 0
	var finishReason string

	events := readSSEStream(body)
	for event := range events {
		if event.Err != nil {
			log.Printf("[chat/stream] ошибка чтения SSE: %v", event.Err)
			break
		}

		if event.Done {
			break
		}

		// парсим SSE-чанк
		chunk, err := parseStreamChunk(event.Data)
		if err != nil {
			log.Printf("[chat/stream] ошибка парсинга чанка: %v", err)
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// проверяем finish_reason
		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}

		content := choice.Delta.Content
		if content == "" && choice.FinishReason == nil {
			continue
		}

		evalCount++
		now := time.Now().UTC().Format(time.RFC3339Nano)

		ollamaChunk := OllamaChatResponse{
			Model:     model,
			CreatedAt: now,
			Message: OllamaMessage{
				Role:    "assistant",
				Content: content,
			},
			Done: false,
		}

		writeNDJSON(w, ollamaChunk)
		flusher.Flush()
	}

	// финальный чанк с done: true
	duration := time.Since(startTime)
	doneReason := "stop"
	if finishReason != "" {
		doneReason = finishReason
	}

	finalChunk := OllamaChatResponse{
		Model:     model,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Message: OllamaMessage{
			Role:    "assistant",
			Content: "",
		},
		Done:          true,
		DoneReason:    doneReason,
		TotalDuration: duration.Nanoseconds(),
		EvalCount:     evalCount,
	}

	writeNDJSON(w, finalChunk)
	flusher.Flush()
}

// applyOllamaOptions — конвертирует Ollama options в поля OpenAI запроса
func applyOllamaOptions(req *OpenAIChatRequest, options map[string]any) {
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

// getFloat64 — извлекает float64 из map[string]any
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

// getInt — извлекает int из map[string]any
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

// writeJSON — записывает JSON-ответ с заданным статусом
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeNDJSON — записывает одну JSON-строку (NDJSON формат)
func writeNDJSON(w http.ResponseWriter, v any) {
	data, _ := json.Marshal(v)
	w.Write(data)
	w.Write([]byte("\n"))
}

// writeError — записывает JSON-ошибку
func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[ошибка] %s", msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
