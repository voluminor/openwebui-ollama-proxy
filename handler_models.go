package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"openwebui-ollama-proxy/cache"
	"openwebui-ollama-proxy/ollama"
	"openwebui-ollama-proxy/openai"
)

// // // // // // // // // //

// handleTags — GET /api/tags
// Список моделей из Open WebUI → формат Ollama.
// L1: in-memory (cache.TagsTTL), L2: диск, L3: upstream.
// tagsFetchMu защищает от thundering herd при cache miss.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	// L1: in-memory
	s.modelsMu.RLock()
	if s.modelsCache != nil && time.Since(s.modelsCacheAt) < s.tagsTTL {
		cached := s.modelsCache
		s.modelsMu.RUnlock()
		log.Printf("[tags] from memory cache, %d models", len(cached))
		writeJSON(w, http.StatusOK, ollama.TagsResponse{Models: cached})
		return
	}
	s.modelsMu.RUnlock()

	// один fetch за раз — остальные горутины ждут на локе
	s.tagsFetchMu.Lock()
	defer s.tagsFetchMu.Unlock()

	// повторная проверка L1: другая горутина могла уже загрузить
	s.modelsMu.RLock()
	if s.modelsCache != nil && time.Since(s.modelsCacheAt) < s.tagsTTL {
		cached := s.modelsCache
		s.modelsMu.RUnlock()
		log.Printf("[tags] from memory cache (after wait), %d models", len(cached))
		writeJSON(w, http.StatusOK, ollama.TagsResponse{Models: cached})
		return
	}
	s.modelsMu.RUnlock()

	// L2: диск
	if disk := cache.ReadTags(s.cacheDir); disk != nil && time.Now().Before(disk.ExpiresAt) {
		s.modelsMu.Lock()
		s.modelsCache = disk.Models
		s.modelsCacheAt = disk.ExpiresAt.Add(-s.tagsTTL)
		s.modelsMu.Unlock()
		log.Printf("[tags] from disk cache, %d models", len(disk.Models))
		writeJSON(w, http.StatusOK, ollama.TagsResponse{Models: disk.Models})
		return
	}

	// L3: upstream
	models, err := s.fetchModels(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "%v", err)
		return
	}

	s.modelsMu.Lock()
	s.modelsCache = models
	s.modelsCacheAt = time.Now()
	s.modelsMu.Unlock()

	if err := cache.WriteTags(s.cacheDir, models, s.tagsTTL); err != nil {
		log.Printf("[tags] disk cache write: %v", err)
	}

	log.Printf("[tags] fetched %d models from upstream, cached for %v", len(models), s.tagsTTL)
	writeJSON(w, http.StatusOK, ollama.TagsResponse{Models: models})
}

// handleShow — POST /api/show
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBodySize)

	var req ollama.ShowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	// диск
	if disk := cache.ReadShow(s.cacheDir, req.Model); disk != nil && time.Now().Before(disk.ExpiresAt) {
		log.Printf("[show] from disk cache: %s", req.Model)
		writeJSON(w, http.StatusOK, disk.Response)
		return
	}

	resp := buildShowResponse(req.Model)

	if err := cache.WriteShow(s.cacheDir, req.Model, resp, s.showTTL); err != nil {
		log.Printf("[show] disk cache write: %v", err)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePs — GET /api/ps
func (s *Server) handlePs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ollama.PsResponse{Models: []any{}})
}

// // // //

// buildShowResponse — формирует stub-ответ для модели
func buildShowResponse(model string) ollama.ShowResponse {
	now := time.Now().UTC().Format(time.RFC3339)
	return ollama.ShowResponse{
		Name:       model,
		Model:      model,
		ModifiedAt: now,
		Size:       0,
		Digest:     fmt.Sprintf("proxy-%s", model),
		Details: ollama.ModelDetails{
			Format:            "proxy",
			Family:            "unknown",
			Families:          []string{},
			ParameterSize:     "unknown",
			QuantizationLevel: "unknown",
		},
		Modelfile:  fmt.Sprintf("FROM %s", model),
		Parameters: "",
		Template:   "{{ .Prompt }}",
	}
}

// fetchModels — запрос моделей из Open WebUI → формат Ollama
func (s *Server) fetchModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	token, err := s.auth.EnsureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.auth.BaseURL()+"/api/models", nil)
	if err != nil {
		return nil, fmt.Errorf("request creation: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClientShort.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Open WebUI request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, s.maxErrorBody))
		return nil, fmt.Errorf("Open WebUI returned %s: %s", resp.Status, strings.TrimSpace(string(errBody)))
	}

	body, _ := io.ReadAll(resp.Body)

	// ответ может быть {data: [...]} или [...]
	var models []openai.Model

	var wrapper openai.ModelList
	if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Data) > 0 {
		models = wrapper.Data
	} else {
		if err := json.Unmarshal(body, &models); err != nil {
			return nil, fmt.Errorf("unexpected /api/models response: %s", string(body))
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result := make([]ollama.ModelInfo, 0, len(models))
	for _, m := range models {
		name := m.ID
		if name == "" {
			name = m.Name
		}
		result = append(result, ollama.ModelInfo{
			Name:       name,
			Model:      name,
			ModifiedAt: now,
			Size:       0,
			Digest:     fmt.Sprintf("proxy-%s", name),
			Details: ollama.ModelDetails{
				Format:            "proxy",
				Family:            "unknown",
				Families:          []string{},
				ParameterSize:     "unknown",
				QuantizationLevel: "unknown",
			},
		})
	}

	return result, nil
}
