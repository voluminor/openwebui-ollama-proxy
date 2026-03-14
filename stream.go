package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"

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

// readSSEStream — читает SSE-поток и отдаёт события через канал.
// idleTimeout > 0 закрывает body при отсутствии данных.
func readSSEStream(body io.ReadCloser, idleTimeout time.Duration) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)

	go func() {
		defer close(ch)

		// idle timeout: при срабатывании закрываем body → scanner.Scan() вернёт ошибку
		var timer *time.Timer
		if idleTimeout > 0 {
			timer = time.AfterFunc(idleTimeout, func() {
				body.Close()
			})
			defer timer.Stop()
		}

		scanner := bufio.NewScanner(body)
		// буфер до 1 МБ для длинных строк
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			if timer != nil {
				timer.Reset(idleTimeout)
			}

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
