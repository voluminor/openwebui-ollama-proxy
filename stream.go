package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// SSEEvent — одно событие из SSE-потока Open WebUI
type SSEEvent struct {
	Data string // содержимое после "data: "
	Done bool   // true если получили "data: [DONE]" или конец потока
	Err  error  // ошибка чтения
}

// readSSEStream — читает SSE-поток из body и отдаёт события через канал.
// Канал закрывается когда поток завершён или произошла ошибка.
// Формат SSE от Open WebUI:
//
//	data: {"choices":[...]}\n
//	\n
//	data: {"choices":[...]}\n
//	\n
//	data: [DONE]\n
func readSSEStream(body io.Reader) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)

	go func() {
		defer close(ch)

		scanner := bufio.NewScanner(body)
		// увеличиваем буфер для длинных строк (до 1 МБ)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// пропускаем пустые строки (разделители событий SSE)
			if line == "" {
				continue
			}

			// SSE-комментарии начинаются с ':'
			if strings.HasPrefix(line, ":") {
				continue
			}

			// нас интересуют только строки "data: ..."
			if !strings.HasPrefix(line, "data: ") && !strings.HasPrefix(line, "data:") {
				continue
			}

			// извлекаем данные после "data: " или "data:"
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimPrefix(data, "data:")
			data = strings.TrimSpace(data)

			if data == "" {
				continue
			}

			// маркер конца потока
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

// parseStreamChunk — парсит JSON-строку SSE-чанка в OpenAIStreamChunk
func parseStreamChunk(data string) (OpenAIStreamChunk, error) {
	var chunk OpenAIStreamChunk
	err := json.Unmarshal([]byte(data), &chunk)
	return chunk, err
}
