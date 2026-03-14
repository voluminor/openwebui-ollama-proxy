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

	"openwebui-ollama-proxy/ollama"
	"openwebui-ollama-proxy/openai"
)

// // // // // // // // // //

// handleTags — GET /api/tags
// Список моделей из Open WebUI → формат Ollama. Кеш 10 минут.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	// пробуем из кеша
	s.modelsMu.RLock()
	if s.modelsCache != nil && time.Since(s.modelsCacheAt) < modelsCacheTTL {
		cached := s.modelsCache
		s.modelsMu.RUnlock()
		log.Printf("[tags] from cache, %d models", len(cached))
		writeJSON(w, http.StatusOK, ollama.TagsResponse{Models: cached})
		return
	}
	s.modelsMu.RUnlock()

	models, err := s.fetchModels(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "%v", err)
		return
	}

	s.modelsMu.Lock()
	s.modelsCache = models
	s.modelsCacheAt = time.Now()
	s.modelsMu.Unlock()

	log.Printf("[tags] loaded %d models, cached for %v", len(models), modelsCacheTTL)
	writeJSON(w, http.StatusOK, ollama.TagsResponse{Models: models})
}

// handleShow — POST /api/show (заглушка)
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req ollama.ShowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, ollama.ShowResponse{
		Name:       req.Model,
		Model:      req.Model,
		ModifiedAt: now,
		Size:       0,
		Digest:     fmt.Sprintf("proxy-%s", req.Model),
		Details: ollama.ModelDetails{
			Format:            "proxy",
			Family:            "unknown",
			Families:          []string{},
			ParameterSize:     "unknown",
			QuantizationLevel: "unknown",
		},
		Modelfile:  fmt.Sprintf("FROM %s", req.Model),
		Parameters: "",
		Template:   "{{ .Prompt }}",
	})
}

// handlePs — GET /api/ps
func (s *Server) handlePs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ollama.PsResponse{Models: []any{}})
}

// // // //

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

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Open WebUI request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open WebUI returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

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
