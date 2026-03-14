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

// handleChat — POST /api/chat
// Проксирует Ollama → OpenAI и обратно, streaming и non-streaming.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	r.Body = http.MaxBytesReader(w, r.Body, s.maxBodySize)

	var req ollama.ChatRequest
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

	// конвертируем Ollama → OpenAI; top-level system всегда идёт первым
	messages := make([]openai.RequestMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, openai.RequestMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, openai.RequestMessage{Role: m.Role, Content: buildContentParts(m.Content, m.Images)})
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
			log.Printf("[chat] client disconnected")
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
		s.streamChatResponse(w, resp.Body, req.Model)
	} else {
		s.nonStreamChatResponse(w, resp.Body, req.Model)
	}
}

// // // //

// nonStreamChatResponse — обычный ответ от Open WebUI
func (s *Server) nonStreamChatResponse(w http.ResponseWriter, body io.Reader, model string) {
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

	writeJSON(w, http.StatusOK, ollama.ChatResponse{
		Model:     model,
		CreatedAt: now,
		Message: ollama.Message{
			Role:    "assistant",
			Content: choice.Message.Content,
		},
		Done:       true,
		DoneReason: "stop",
	})
}

// streamChatResponse — SSE → NDJSON в формате Ollama
func (s *Server) streamChatResponse(w http.ResponseWriter, body io.Reader, model string) {
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
			log.Printf("[chat/stream] SSE read error: %v", event.Err)
			break
		}

		if event.Done {
			break
		}

		chunk, err := parseStreamChunk(event.Data)
		if err != nil {
			log.Printf("[chat/stream] chunk parse error: %v", err)
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

		writeNDJSON(w, ollama.ChatResponse{
			Model:     model,
			CreatedAt: now,
			Message: ollama.Message{
				Role:    "assistant",
				Content: content,
			},
			Done: false,
		})
		flusher.Flush()
	}

	// финальный чанк
	duration := time.Since(startTime)
	doneReason := "stop"
	if finishReason != "" {
		doneReason = finishReason
	}

	writeNDJSON(w, ollama.ChatResponse{
		Model:     model,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Message: ollama.Message{
			Role:    "assistant",
			Content: "",
		},
		Done:          true,
		DoneReason:    doneReason,
		TotalDuration: duration.Nanoseconds(),
	})
	flusher.Flush()
}
