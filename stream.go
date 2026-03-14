package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"openwebui-ollama-proxy/openai"
)

// // // // // // // // // //

// SSEEvent — событие из SSE-потока Open WebUI
type SSEEvent struct {
	Data string
	Done bool
	Err  error
}

// // // //

// readSSEStream — читает SSE-поток и отдаёт события через канал
func readSSEStream(body io.Reader) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)

	go func() {
		defer close(ch)

		scanner := bufio.NewScanner(body)
		// буфер до 1 МБ для длинных строк
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				continue
			}

			// SSE-комментарии
			if strings.HasPrefix(line, ":") {
				continue
			}

			if !strings.HasPrefix(line, "data: ") && !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimPrefix(data, "data:")
			data = strings.TrimSpace(data)

			if data == "" {
				continue
			}

			if data == "[DONE]" {
				ch <- SSEEvent{Done: true}
				return
			}

			ch <- SSEEvent{Data: data}
		}

		if err := scanner.Err(); err != nil {
			ch <- SSEEvent{Err: err}
		}
	}()

	return ch
}

// parseStreamChunk — парсит SSE-чанк в openai.StreamChunk
func parseStreamChunk(data string) (openai.StreamChunk, error) {
	var chunk openai.StreamChunk
	err := json.Unmarshal([]byte(data), &chunk)
	return chunk, err
}
