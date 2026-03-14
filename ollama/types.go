package ollama

// // // // requests // // // //

// Message — Ollama message
type Message struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

// ChatRequest — POST /api/chat request
type ChatRequest struct {
	Model     string         `json:"model"`
	Messages  []Message      `json:"messages"`
	System    string         `json:"system,omitempty"`
	Stream    *bool          `json:"stream,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Format    any            `json:"format,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
}

// GenerateRequest — POST /api/generate request
type GenerateRequest struct {
	Model     string         `json:"model"`
	Prompt    string         `json:"prompt"`
	System    string         `json:"system,omitempty"`
	Images    []string       `json:"images,omitempty"`
	Stream    *bool          `json:"stream,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Format    any            `json:"format,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Raw       bool           `json:"raw,omitempty"`
}

// ShowRequest — POST /api/show request
type ShowRequest struct {
	Model string `json:"model"`
}

// // // // responses // // // //

// ChatResponse — /api/chat response (stream chunk or full)
type ChatResponse struct {
	Model      string  `json:"model"`
	CreatedAt  string  `json:"created_at"`
	Message    Message `json:"message"`
	Done       bool    `json:"done"`
	DoneReason string  `json:"done_reason,omitempty"`
	// metrics — only in the final chunk
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// GenerateResponse — /api/generate response
type GenerateResponse struct {
	Model      string `json:"model"`
	CreatedAt  string `json:"created_at"`
	Response   string `json:"response"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason,omitempty"`
	// metrics
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// // // // models // // // //

// ModelInfo — model entry in /api/tags response
type ModelInfo struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt string       `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails — model details
type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// TagsResponse — GET /api/tags response
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ShowResponse — POST /api/show response
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

// PsResponse — GET /api/ps response
type PsResponse struct {
	Models []any `json:"models"`
}

// VersionResponse — GET /api/version response
type VersionResponse struct {
	Version string `json:"version"`
}
