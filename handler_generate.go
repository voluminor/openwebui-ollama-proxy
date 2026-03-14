package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"openwebui-ollama-proxy/ollama"
	"openwebui-ollama-proxy/openai"
)

// // // // // // // // // //

// handleGenerate — POST /api/generate
// Конвертирует prompt/system в messages и проксирует к Open WebUI.
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req ollama.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	// в Ollama stream по умолчанию true
	streaming := true
	if req.Stream != nil {
		streaming = *req.Stream
	}

	// prompt + system → messages
	var messages []openai.Message
	if req.System != "" {
		messages = append(messages, openai.Message{Role: "system", Content: req.System})
	}
	if req.Prompt != "" {
		messages = append(messages, openai.Message{Role: "user", Content: req.Prompt})
	}

	if len(messages) == 0 {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	oaiReq := openai.ChatRequest{
		Model:          req.Model,
		Messages:       messages,
		Stream:         streaming,
		ResponseFormat: ollamaFormatToResponseFormat(req.Format),
	}
	applyOllamaOptions(&oaiReq, req.Options)

	token, err := s.auth.EnsureToken(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "auth error: %v", err)
		return
	}

	payload, _ := json.Marshal(oaiReq)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.auth.BaseURL()+"/api/chat/completions", bytes.NewReader(payload))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "request creation: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("[generate] client disconnected")
			return
		}
		writeError(w, http.StatusBadGateway, "Open WebUI request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, "Open WebUI: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		return
	}

	if streaming {
		s.streamGenerateResponse(w, resp.Body, req.Model)
	} else {
		s.nonStreamGenerateResponse(w, resp.Body, req.Model)
	}
}

// // // //

// nonStreamGenerateResponse — обычный ответ → формат /api/generate
func (s *Server) nonStreamGenerateResponse(w http.ResponseWriter, body io.Reader, model string) {
	data, _ := io.ReadAll(body)

	var oaiResp openai.ChatResponse
	if err := json.Unmarshal(data, &oaiResp); err != nil {
		writeError(w, http.StatusBadGateway, "response decode: %v", err)
		return
	}

	if len(oaiResp.Choices) == 0 {
		writeError(w, http.StatusBadGateway, "empty response from Open WebUI")
		return
	}

	choice := oaiResp.Choices[0]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	writeJSON(w, http.StatusOK, ollama.GenerateResponse{
		Model:      model,
		CreatedAt:  now,
		Response:   choice.Message.Content,
		Done:       true,
		DoneReason: "stop",
	})
}

// streamGenerateResponse — SSE → NDJSON в формате /api/generate
func (s *Server) streamGenerateResponse(w http.ResponseWriter, body io.Reader, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	startTime := time.Now()
	var finishReason string

	events := readSSEStream(body)
	for event := range events {
		if event.Err != nil {
			log.Printf("[generate/stream] SSE read error: %v", event.Err)
			break
		}

		if event.Done {
			break
		}

		chunk, err := parseStreamChunk(event.Data)
		if err != nil {
			log.Printf("[generate/stream] chunk parse error: %v", err)
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

		now := time.Now().UTC().Format(time.RFC3339Nano)

		writeNDJSON(w, ollama.GenerateResponse{
			Model:     model,
			CreatedAt: now,
			Response:  content,
			Done:      false,
		})
		flusher.Flush()
	}

	// финальный чанк
	duration := time.Since(startTime)
	doneReason := "stop"
	if finishReason != "" {
		doneReason = finishReason
	}

	writeNDJSON(w, ollama.GenerateResponse{
		Model:         model,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Response:      "",
		Done:          true,
		DoneReason:    doneReason,
		TotalDuration: duration.Nanoseconds(),
	})
	flusher.Flush()
}
