package main

import "time"

// ─── Ollama API: входящие запросы от клиентов ───

// OllamaMessage — сообщение в формате Ollama
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaChatRequest — запрос к POST /api/chat
type OllamaChatRequest struct {
	Model     string          `json:"model"`
	Messages  []OllamaMessage `json:"messages"`
	Stream    *bool           `json:"stream,omitempty"`  // по умолчанию true в Ollama
	Options   map[string]any  `json:"options,omitempty"` // temperature, top_p, num_predict и тд
	Format    any             `json:"format,omitempty"`  // "json" или json-schema
	KeepAlive string          `json:"keep_alive,omitempty"`
}

// OllamaGenerateRequest — запрос к POST /api/generate
type OllamaGenerateRequest struct {
	Model     string         `json:"model"`
	Prompt    string         `json:"prompt"`
	System    string         `json:"system,omitempty"`
	Stream    *bool          `json:"stream,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Format    any            `json:"format,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Raw       bool           `json:"raw,omitempty"`
}

// OllamaShowRequest — запрос к POST /api/show
type OllamaShowRequest struct {
	Model string `json:"model"`
}

// ─── Ollama API: исходящие ответы клиентам ───

// OllamaChatResponse — ответ на /api/chat (один чанк в стриме или полный ответ)
type OllamaChatResponse struct {
	Model      string        `json:"model"`
	CreatedAt  string        `json:"created_at"`
	Message    OllamaMessage `json:"message"`
	Done       bool          `json:"done"`
	DoneReason string        `json:"done_reason,omitempty"`
	// метрики — заполняются только в финальном чанке
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// OllamaGenerateResponse — ответ на /api/generate
type OllamaGenerateResponse struct {
	Model      string `json:"model"`
	CreatedAt  string `json:"created_at"`
	Response   string `json:"response"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason,omitempty"`
	// метрики
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// OllamaModelInfo — модель в ответе /api/tags
type OllamaModelInfo struct {
	Name       string             `json:"name"`
	Model      string             `json:"model"`
	ModifiedAt string             `json:"modified_at"`
	Size       int64              `json:"size"`
	Digest     string             `json:"digest"`
	Details    OllamaModelDetails `json:"details"`
}

// OllamaModelDetails — детали модели
type OllamaModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// OllamaTagsResponse — ответ на GET /api/tags
type OllamaTagsResponse struct {
	Models []OllamaModelInfo `json:"models"`
}

// OllamaShowResponse — ответ на POST /api/show
type OllamaShowResponse struct {
	Name       string             `json:"name"`
	Model      string             `json:"model"`
	ModifiedAt string             `json:"modified_at"`
	Size       int64              `json:"size"`
	Digest     string             `json:"digest"`
	Details    OllamaModelDetails `json:"details"`
	Modelfile  string             `json:"modelfile"`
	Parameters string             `json:"parameters"`
	Template   string             `json:"template"`
}

// OllamaPsResponse — ответ на GET /api/ps
type OllamaPsResponse struct {
	Models []any `json:"models"`
}

// OllamaVersionResponse — ответ на GET /api/version
type OllamaVersionResponse struct {
	Version string `json:"version"`
}

// ─── OpenAI API: запросы к Open WebUI ───

// OpenAIMessage — сообщение в формате OpenAI
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIChatRequest — запрос к Open WebUI /api/chat/completions
type OpenAIChatRequest struct {
	Model            string          `json:"model"`
	Messages         []OpenAIMessage `json:"messages"`
	Stream           bool            `json:"stream"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
}

// ─── OpenAI API: ответы от Open WebUI ───

// OpenAIChatResponse — полный (не streaming) ответ от Open WebUI
type OpenAIChatResponse struct {
	Choices []OpenAIChatChoice `json:"choices"`
}

// OpenAIChatChoice — один вариант ответа
type OpenAIChatChoice struct {
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIStreamChunk — один SSE-чанк при streaming
type OpenAIStreamChunk struct {
	Choices []OpenAIStreamChoice `json:"choices"`
}

// OpenAIStreamChoice — один вариант в стрим-чанке
type OpenAIStreamChoice struct {
	Delta        OpenAIStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

// OpenAIStreamDelta — дельта контента в стрим-чанке
type OpenAIStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// OpenAIModelList — ответ Open WebUI на GET /api/models
type OpenAIModelList struct {
	Data []OpenAIModel `json:"data"`
}

// OpenAIModel — модель в формате OpenAI
type OpenAIModel struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Name   string `json:"name"`
}

// ─── Сессия авторизации ───

// SessionData — данные сессии для кеширования на диск
type SessionData struct {
	Token   string    `json:"token"`
	Expiry  time.Time `json:"expiry"`
	Email   string    `json:"email"`
	BaseURL string    `json:"base_url"`
}
