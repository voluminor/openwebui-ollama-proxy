package ollama

// // // // запросы // // // //

// Message — сообщение Ollama
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest — запрос POST /api/chat
type ChatRequest struct {
	Model     string         `json:"model"`
	Messages  []Message      `json:"messages"`
	System    string         `json:"system,omitempty"`
	Stream    *bool          `json:"stream,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Format    any            `json:"format,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
}

// GenerateRequest — запрос POST /api/generate
type GenerateRequest struct {
	Model     string         `json:"model"`
	Prompt    string         `json:"prompt"`
	System    string         `json:"system,omitempty"`
	Stream    *bool          `json:"stream,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Format    any            `json:"format,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Raw       bool           `json:"raw,omitempty"`
}

// ShowRequest — запрос POST /api/show
type ShowRequest struct {
	Model string `json:"model"`
}

// // // // ответы // // // //

// ChatResponse — ответ /api/chat (чанк стрима или полный)
type ChatResponse struct {
	Model      string  `json:"model"`
	CreatedAt  string  `json:"created_at"`
	Message    Message `json:"message"`
	Done       bool    `json:"done"`
	DoneReason string  `json:"done_reason,omitempty"`
	// метрики — только в финальном чанке
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// GenerateResponse — ответ /api/generate
type GenerateResponse struct {
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

// // // // модели // // // //

// ModelInfo — модель в ответе /api/tags
type ModelInfo struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt string       `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails — детали модели
type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// TagsResponse — ответ GET /api/tags
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ShowResponse — ответ POST /api/show
type ShowResponse struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt string       `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
	Modelfile  string       `json:"modelfile"`
	Parameters string       `json:"parameters"`
	Template   string       `json:"template"`
}

// PsResponse — ответ GET /api/ps
type PsResponse struct {
	Models []any `json:"models"`
}

// VersionResponse — ответ GET /api/version
type VersionResponse struct {
	Version string `json:"version"`
}
