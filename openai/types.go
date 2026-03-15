package openai

// // // // requests // // // //

// Message — OpenAI message (used in responses, Content is always string)
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ContentPart — multimodal content element
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL — image data URL
type ImageURL struct {
	URL string `json:"url"`
}

// RequestMessage — request message (Content can be string or []ContentPart)
type RequestMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// ResponseFormat — response format (json_object / json_schema)
type ResponseFormat struct {
	Type       string `json:"type"`
	JSONSchema any    `json:"json_schema,omitempty"`
}

// ChatRequest — request to /api/chat/completions
type ChatRequest struct {
	Model            string           `json:"model"`
	Messages         []RequestMessage `json:"messages"`
	Stream           bool             `json:"stream"`
	Temperature      *float64         `json:"temperature,omitempty"`
	TopP             *float64         `json:"top_p,omitempty"`
	MaxTokens        *int             `json:"max_tokens,omitempty"`
	Stop             []string         `json:"stop,omitempty"`
	FrequencyPenalty *float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64         `json:"presence_penalty,omitempty"`
	Seed             *int             `json:"seed,omitempty"`
	ResponseFormat   *ResponseFormat  `json:"response_format,omitempty"`
}

// // // // responses // // // //

// ChatResponse — full response (non-streaming)
type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
}

// ChatChoice — response choice
type ChatChoice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// StreamChunk — SSE chunk during streaming
type StreamChunk struct {
	Choices []StreamChoice `json:"choices"`
}

// StreamChoice — choice in a stream chunk
type StreamChoice struct {
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// StreamDelta — content delta
type StreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// // // // models // // // //

// ModelList — GET /api/models response
type ModelList struct {
	Data []Model `json:"data"`
}

// Model — model in OpenAI format
type Model struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Name   string `json:"name"`
}
