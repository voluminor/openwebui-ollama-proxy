package openai

// // // // запросы // // // //

// Message — сообщение OpenAI
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest — запрос к /api/chat/completions
type ChatRequest struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	Stream           bool      `json:"stream"`
	Temperature      *float64  `json:"temperature,omitempty"`
	TopP             *float64  `json:"top_p,omitempty"`
	MaxTokens        *int      `json:"max_tokens,omitempty"`
	Stop             []string  `json:"stop,omitempty"`
	FrequencyPenalty *float64  `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64  `json:"presence_penalty,omitempty"`
	Seed             *int      `json:"seed,omitempty"`
}

// // // // ответы // // // //

// ChatResponse — полный ответ (не streaming)
type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
}

// ChatChoice — вариант ответа
type ChatChoice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// StreamChunk — SSE-чанк при streaming
type StreamChunk struct {
	Choices []StreamChoice `json:"choices"`
}

// StreamChoice — вариант в стрим-чанке
type StreamChoice struct {
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// StreamDelta — дельта контента
type StreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// // // // модели // // // //

// ModelList — ответ GET /api/models
type ModelList struct {
	Data []Model `json:"data"`
}

// Model — модель в формате OpenAI
type Model struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Name   string `json:"name"`
}
