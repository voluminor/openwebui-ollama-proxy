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
)

// handleTags — GET /api/tags
// Получает список моделей из Open WebUI и конвертирует в формат Ollama.
// Результат кешируется на 10 минут.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	// пробуем отдать из кеша
	s.modelsMu.RLock()
	if s.modelsCache != nil && time.Since(s.modelsCacheAt) < modelsCacheTTL {
		cached := s.modelsCache
		s.modelsMu.RUnlock()
		log.Printf("[tags] из кеша, %d моделей", len(cached))
		writeJSON(w, http.StatusOK, OllamaTagsResponse{Models: cached})
		return
	}
	s.modelsMu.RUnlock()

	// кеш пуст или устарел — запрашиваем Open WebUI
	models, err := s.fetchModels(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "%v", err)
		return
	}

	// сохраняем в кеш
	s.modelsMu.Lock()
	s.modelsCache = models
	s.modelsCacheAt = time.Now()
	s.modelsMu.Unlock()

	log.Printf("[tags] загружено %d моделей, закешировано на %v", len(models), modelsCacheTTL)
	writeJSON(w, http.StatusOK, OllamaTagsResponse{Models: models})
}

// fetchModels — запрашивает список моделей из Open WebUI и конвертирует в формат Ollama
func (s *Server) fetchModels(ctx context.Context) ([]OllamaModelInfo, error) {
	token, err := s.auth.EnsureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("ошибка авторизации: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.auth.baseURL+"/api/models", nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("запрос к Open WebUI: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open WebUI вернул %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	// парсим ответ — может быть {data: [...]} или просто [...]
	var models []OpenAIModel

	var wrapper OpenAIModelList
	if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Data) > 0 {
		models = wrapper.Data
	} else {
		if err := json.Unmarshal(body, &models); err != nil {
			return nil, fmt.Errorf("неожиданный ответ /api/models: %s", string(body))
		}
	}

	// конвертируем в формат Ollama
	now := time.Now().UTC().Format(time.RFC3339)
	result := make([]OllamaModelInfo, 0, len(models))
	for _, m := range models {
		name := m.ID
		if name == "" {
			name = m.Name
		}
		result = append(result, OllamaModelInfo{
			Name:       name,
			Model:      name,
			ModifiedAt: now,
			Size:       0,
			Digest:     fmt.Sprintf("proxy-%s", name),
			Details: OllamaModelDetails{
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

// handleShow — POST /api/show
// Возвращает минимальную информацию о модели (заглушка)
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	var req OllamaShowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "невалидный JSON: %v", err)
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model обязателен")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, OllamaShowResponse{
		Name:       req.Model,
		Model:      req.Model,
		ModifiedAt: now,
		Size:       0,
		Digest:     fmt.Sprintf("proxy-%s", req.Model),
		Details: OllamaModelDetails{
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
// Возвращает пустой список запущенных моделей
func (s *Server) handlePs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, OllamaPsResponse{Models: []any{}})
}
