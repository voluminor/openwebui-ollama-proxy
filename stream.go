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

// SSEEvent — event from Open WebUI SSE stream
type SSEEvent struct {
	Data string
	Done bool
	Err  error
}

// // // //

// readSSEStream — reads SSE stream and emits events via channel.
// idleTimeout > 0 closes body on data absence.
func readSSEStream(body io.ReadCloser, idleTimeout time.Duration) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)

	go func() {
		defer close(ch)

		// idle timeout: on trigger, close body → scanner.Scan() returns error
		var timer *time.Timer
		if idleTimeout > 0 {
			timer = time.AfterFunc(idleTimeout, func() {
				body.Close()
			})
			defer timer.Stop()
		}

		scanner := bufio.NewScanner(body)
		// buffer up to 1 MB for long lines
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			if timer != nil {
				timer.Reset(idleTimeout)
			}

			line := scanner.Text()

			if line == "" {
				continue
			}

			// SSE comments
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

// parseStreamChunk — parses SSE chunk into openai.StreamChunk
func parseStreamChunk(data string) (openai.StreamChunk, error) {
	var chunk openai.StreamChunk
	err := json.Unmarshal([]byte(data), &chunk)
	return chunk, err
}
