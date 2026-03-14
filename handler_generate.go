package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleGenerate — POST /api/generate
// Конвертирует prompt/system в формат messages и проксирует к Open WebUI.
// Поддерживает streaming (NDJSON) и non-streaming режимы.
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var ollamaReq OllamaGenerateRequest
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

	// конвертируем prompt + system в массив messages
	var messages []OpenAIMessage
	if ollamaReq.System != "" {
		messages = append(messages, OpenAIMessage{Role: "system", Content: ollamaReq.System})
	}
	if ollamaReq.Prompt != "" {
		messages = append(messages, OpenAIMessage{Role: "user", Content: ollamaReq.Prompt})
	}

	if len(messages) == 0 {
		writeError(w, http.StatusBadRequest, "prompt обязателен")
		return
	}

	// собираем OpenAI запрос
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
		s.streamGenerateResponse(w, resp.Body, ollamaReq.Model)
	} else {
		s.nonStreamGenerateResponse(w, resp.Body, ollamaReq.Model)
	}
}

// nonStreamGenerateResponse — обрабатывает обычный ответ и конвертирует в формат /api/generate
func (s *Server) nonStreamGenerateResponse(w http.ResponseWriter, body io.Reader, model string) {
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

	writeJSON(w, http.StatusOK, OllamaGenerateResponse{
		Model:      model,
		CreatedAt:  now,
		Response:   choice.Message.Content,
		Done:       true,
		DoneReason: "stop",
		EvalCount:  len(choice.Message.Content) / 4,
	})
}

// streamGenerateResponse — читает SSE-поток и отдаёт NDJSON в формате /api/generate
func (s *Server) streamGenerateResponse(w http.ResponseWriter, body io.Reader, model string) {
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
			log.Printf("[generate/stream] ошибка чтения SSE: %v", event.Err)
			break
		}

		if event.Done {
			break
		}

		chunk, err := parseStreamChunk(event.Data)
		if err != nil {
			log.Printf("[generate/stream] ошибка парсинга чанка: %v", err)
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}

		content := choice.Delta.Content
		if content == "" && choice.FinishReason == nil {
			continue
		}

		evalCount++
		now := time.Now().UTC().Format(time.RFC3339Nano)

		ollamaChunk := OllamaGenerateResponse{
			Model:     model,
			CreatedAt: now,
			Response:  content,
			Done:      false,
		}

		writeNDJSON(w, ollamaChunk)
		flusher.Flush()
	}

	// финальный чанк
	duration := time.Since(startTime)
	doneReason := "stop"
	if finishReason != "" {
		doneReason = finishReason
	}

	finalChunk := OllamaGenerateResponse{
		Model:         model,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Response:      "",
		Done:          true,
		DoneReason:    doneReason,
		TotalDuration: duration.Nanoseconds(),
		EvalCount:     evalCount,
	}

	writeNDJSON(w, finalChunk)
	flusher.Flush()
}
