package main

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// // // // // // // // // //

// nopReadCloser — io.ReadCloser over io.Reader
type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error { return nil }

func newRC(s string) io.ReadCloser {
	return nopReadCloser{strings.NewReader(s)}
}

// // // //

func TestReadSSEStream_Basic(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"
	events := readSSEStream(newRC(input), 0)

	ev := <-events
	if ev.Err != nil || ev.Done {
		t.Fatalf("event 1: err=%v done=%v", ev.Err, ev.Done)
	}
	if ev.Data != `{"choices":[{"delta":{"content":"hi"}}]}` {
		t.Fatalf("data = %q", ev.Data)
	}

	ev = <-events
	if !ev.Done {
		t.Fatal("expected DONE event")
	}

	// channel should close
	_, ok := <-events
	if ok {
		t.Fatal("channel should be closed")
	}
}

func TestReadSSEStream_SkipsComments(t *testing.T) {
	input := ": comment\ndata: hello\ndata: [DONE]\n"
	events := readSSEStream(newRC(input), 0)

	ev := <-events
	if ev.Data != "hello" {
		t.Fatalf("data = %q, want %q", ev.Data, "hello")
	}

	ev = <-events
	if !ev.Done {
		t.Fatal("expected DONE")
	}
}

func TestReadSSEStream_SkipsNonData(t *testing.T) {
	input := "event: message\nid: 123\ndata: payload\ndata: [DONE]\n"
	events := readSSEStream(newRC(input), 0)

	ev := <-events
	if ev.Data != "payload" {
		t.Fatalf("data = %q", ev.Data)
	}

	ev = <-events
	if !ev.Done {
		t.Fatal("expected DONE")
	}
}

func TestReadSSEStream_EmptyData(t *testing.T) {
	input := "data: \ndata: actual\ndata: [DONE]\n"
	events := readSSEStream(newRC(input), 0)

	ev := <-events
	if ev.Data != "actual" {
		t.Fatalf("data = %q, want %q", ev.Data, "actual")
	}
}

func TestReadSSEStream_NoSpace(t *testing.T) {
	// "data:payload" without space
	input := "data:payload\ndata: [DONE]\n"
	events := readSSEStream(newRC(input), 0)

	ev := <-events
	if ev.Data != "payload" {
		t.Fatalf("data = %q, want %q", ev.Data, "payload")
	}
}

func TestReadSSEStream_EOF(t *testing.T) {
	// stream without [DONE] — simply closes
	input := "data: chunk1\ndata: chunk2\n"
	events := readSSEStream(newRC(input), 0)

	ev := <-events
	if ev.Data != "chunk1" {
		t.Fatalf("data = %q", ev.Data)
	}
	ev = <-events
	if ev.Data != "chunk2" {
		t.Fatalf("data = %q", ev.Data)
	}

	// channel closes
	_, ok := <-events
	if ok {
		t.Fatal("channel should be closed after EOF")
	}
}

func TestReadSSEStream_IdleTimeout(t *testing.T) {
	// use pipe: write one chunk and go silent
	pr, pw := io.Pipe()

	go func() {
		fmt.Fprintln(pw, "data: first")
		// stop writing — idle timeout should trigger
	}()

	events := readSSEStream(pr, 100*time.Millisecond)

	ev := <-events
	if ev.Data != "first" {
		t.Fatalf("data = %q, want %q", ev.Data, "first")
	}

	// wait for timeout — should receive error or close
	select {
	case ev, ok := <-events:
		if ok && ev.Err == nil {
			t.Fatal("expected error or channel close after idle timeout")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle timeout did not fire")
	}

	pw.Close()
}

// // // //

func TestParseStreamChunk(t *testing.T) {
	data := `{"choices":[{"delta":{"content":"hello"},"finish_reason":null}]}`
	chunk, err := parseStreamChunk(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("choices = %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Content != "hello" {
		t.Fatalf("content = %q", chunk.Choices[0].Delta.Content)
	}
	if chunk.Choices[0].FinishReason != nil {
		t.Fatal("finish_reason should be nil")
	}
}

func TestParseStreamChunk_FinishReason(t *testing.T) {
	data := `{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`
	chunk, err := parseStreamChunk(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if chunk.Choices[0].FinishReason == nil || *chunk.Choices[0].FinishReason != "stop" {
		t.Fatal("expected finish_reason=stop")
	}
}

func TestParseStreamChunk_Invalid(t *testing.T) {
	_, err := parseStreamChunk("not json")
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

// // // // benchmarks // // // //

func BenchmarkReadSSEStream(b *testing.B) {
	// generate SSE stream
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "data: {\"choices\":[{\"delta\":{\"content\":\"token%d\"}}]}\n\n", i)
	}
	sb.WriteString("data: [DONE]\n\n")
	payload := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		events := readSSEStream(newRC(payload), 0)
		for range events {
		}
	}
}

func BenchmarkParseStreamChunk(b *testing.B) {
	data := `{"choices":[{"delta":{"content":"hello world"},"finish_reason":null}]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseStreamChunk(data)
	}
}
