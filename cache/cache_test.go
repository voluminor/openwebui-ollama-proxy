package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"openwebui-ollama-proxy/ollama"
)

// // // // // // // // // //

func TestReadWrite_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	original := SessionObj{
		Token:     "eyJhbGciOiJIUzI1NiJ9.test.sig",
		ExpiresAt: time.Now().Add(time.Hour).Truncate(time.Millisecond),
		Email:     "user@example.com",
		BaseURL:   "https://example.com",
	}

	if err := Write(path, magicSession, original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := Read[SessionObj](path, magicSession)
	if got == nil {
		t.Fatal("Read returned nil")
	}

	if got.Token != original.Token {
		t.Fatalf("Token = %q, want %q", got.Token, original.Token)
	}
	if got.Email != original.Email {
		t.Fatalf("Email = %q, want %q", got.Email, original.Email)
	}
	if got.BaseURL != original.BaseURL {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, original.BaseURL)
	}
	if !got.ExpiresAt.Equal(original.ExpiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v", got.ExpiresAt, original.ExpiresAt)
	}
}

func TestRead_NonexistentFile(t *testing.T) {
	got := Read[SessionObj]("/nonexistent/path/file.bin", magicSession)
	if got != nil {
		t.Fatal("expected nil for nonexistent file")
	}
}

func TestRead_FileTooShort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "short.bin")

	os.WriteFile(path, []byte{0xCA, 0x01}, 0o600)

	got := Read[SessionObj](path, magicSession)
	if got != nil {
		t.Fatal("expected nil for too-short file")
	}

	// file should be removed
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid file should be removed")
	}
}

func TestRead_WrongMagic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	if err := Write(path, magicSession, SessionObj{Token: "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// read with different magic bytes
	wrongMagic := [2]byte{0xFF, 0xFF}
	got := Read[SessionObj](path, wrongMagic)
	if got != nil {
		t.Fatal("expected nil for wrong magic")
	}
}

func TestRead_CorruptedData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.bin")

	if err := Write(path, magicSession, SessionObj{Token: "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// corrupt the data
	data, _ := os.ReadFile(path)
	data[20] ^= 0xFF // flip a byte in ciphertext
	os.WriteFile(path, data, 0o600)

	got := Read[SessionObj](path, magicSession)
	if got != nil {
		t.Fatal("expected nil for corrupted data")
	}
}

func TestWrite_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.bin")

	err := Write(path, magicSession, SessionObj{Token: "test"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := Read[SessionObj](path, magicSession)
	if got == nil || got.Token != "test" {
		t.Fatal("roundtrip failed after mkdir")
	}
}

func TestWrite_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions are not supported on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "perms.bin")

	Write(path, magicSession, SessionObj{Token: "secret"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// 0o600 — owner read/write only
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 600", perm)
	}
}

// // // //

func TestSessionReadWrite(t *testing.T) {
	dir := t.TempDir()

	original := SessionObj{
		Token:     "jwt-token-here",
		ExpiresAt: time.Now().Add(24 * time.Hour).Truncate(time.Millisecond),
		Email:     "admin@example.com",
		BaseURL:   "https://openwebui.local",
	}

	if err := WriteSession(dir, original); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}

	got := ReadSession(dir)
	if got == nil {
		t.Fatal("ReadSession returned nil")
	}
	if got.Token != original.Token {
		t.Fatalf("Token mismatch")
	}
	if got.Email != original.Email {
		t.Fatalf("Email mismatch")
	}
}

func TestTagsReadWrite(t *testing.T) {
	dir := t.TempDir()

	models := []ollama.ModelInfo{
		{
			Name:  "llama3:8b",
			Model: "llama3:8b",
			Details: ollama.ModelDetails{
				Family: "llama",
			},
		},
		{
			Name:  "gpt-4o",
			Model: "gpt-4o",
			Details: ollama.ModelDetails{
				Family: "gpt",
			},
		},
	}

	if err := WriteTags(dir, models, 10*time.Minute); err != nil {
		t.Fatalf("WriteTags: %v", err)
	}

	got := ReadTags(dir)
	if got == nil {
		t.Fatal("ReadTags returned nil")
	}
	if len(got.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(got.Models))
	}
	if got.Models[0].Name != "llama3:8b" {
		t.Fatalf("model[0] = %q", got.Models[0].Name)
	}
	if time.Until(got.ExpiresAt) <= 0 {
		t.Fatal("cache should not be expired")
	}
}

func TestShowReadWrite(t *testing.T) {
	dir := t.TempDir()

	resp := ollama.ShowResponse{
		Name:  "test-model",
		Model: "test-model",
		Details: ollama.ModelDetails{
			Family:        "llama",
			ParameterSize: "8B",
		},
		Template: "{{ .Prompt }}",
	}

	if err := WriteShow(dir, "test-model", resp, 30*time.Minute); err != nil {
		t.Fatalf("WriteShow: %v", err)
	}

	got := ReadShow(dir, "test-model")
	if got == nil {
		t.Fatal("ReadShow returned nil")
	}
	if got.Response.Name != "test-model" {
		t.Fatalf("name = %q", got.Response.Name)
	}
	if got.Response.Details.Family != "llama" {
		t.Fatalf("family = %q", got.Response.Details.Family)
	}
}

func TestShowReadWrite_DifferentModels(t *testing.T) {
	dir := t.TempDir()

	resp1 := ollama.ShowResponse{Name: "model-a", Model: "model-a"}
	resp2 := ollama.ShowResponse{Name: "model-b", Model: "model-b"}

	WriteShow(dir, "model-a", resp1, time.Hour)
	WriteShow(dir, "model-b", resp2, time.Hour)

	got1 := ReadShow(dir, "model-a")
	got2 := ReadShow(dir, "model-b")

	if got1 == nil || got1.Response.Name != "model-a" {
		t.Fatal("model-a mismatch")
	}
	if got2 == nil || got2.Response.Name != "model-b" {
		t.Fatal("model-b mismatch")
	}
}

// // // // benchmarks // // // //

func BenchmarkCacheWrite(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.bin")
	data := SessionObj{
		Token:     "eyJhbGciOiJIUzI1NiJ9.benchmark-token-data.signature",
		ExpiresAt: time.Now().Add(time.Hour),
		Email:     "bench@example.com",
		BaseURL:   "https://example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Write(path, magicSession, data)
	}
}

func BenchmarkCacheRead(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.bin")
	data := SessionObj{
		Token:     "eyJhbGciOiJIUzI1NiJ9.benchmark-token-data.signature",
		ExpiresAt: time.Now().Add(time.Hour),
		Email:     "bench@example.com",
		BaseURL:   "https://example.com",
	}
	Write(path, magicSession, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Read[SessionObj](path, magicSession)
	}
}

func BenchmarkCacheRoundtrip(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.bin")
	data := SessionObj{
		Token:     "eyJhbGciOiJIUzI1NiJ9.benchmark-token-data.signature",
		ExpiresAt: time.Now().Add(time.Hour),
		Email:     "bench@example.com",
		BaseURL:   "https://example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Write(path, magicSession, data)
		Read[SessionObj](path, magicSession)
	}
}
