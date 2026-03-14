package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openwebui-ollama-proxy/auth"
	"openwebui-ollama-proxy/target"
)

// // // // // // // // // //

const infoDescription = `openwebui-ollama-proxy bridges Ollama-compatible API clients to Open WebUI.
It translates Ollama API calls (/api/chat, /api/generate, /api/tags, /api/show)
into OpenAI-compatible requests forwarded to Open WebUI, enabling native Ollama
clients (Ollie, Enchanted, etc.) to work with models hosted on Open WebUI.

Features:
  - Streaming and non-streaming chat and text generation
  - Multimodal support: images forwarded as OpenAI content parts
  - Three-level model list cache: memory -> disk -> upstream
  - Per-model show cache with configurable TTL
  - AES-256-GCM encrypted binary cache with SHA-256 integrity check
  - Automatic session token management with encrypted disk persistence
  - Graceful shutdown with configurable timeout`

// // // //

// printUsage — prints usage (called on -h / --help)
func printUsage() {
	fmt.Fprintf(os.Stderr, "%s %s  (build: %s)\n\n", target.GlobalName, target.GlobalVersion, buildHash())
	fmt.Fprintf(os.Stderr, "Usage:\n  %s --openwebui-url <url> --email <email> --password <pass> [flags]\n\n", target.GlobalName)
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

// printInfo — prints build info in the specified format
func printInfo(format string) {
	build := buildHash()
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]string{
			"name":        target.GlobalName,
			"version":     target.GlobalVersion,
			"updated":     target.GlobalDateUpdate,
			"build":       build,
			"description": infoDescription,
		})
	default: // text
		fmt.Printf("%-8s %s %s\n", "Name", target.GlobalName, target.GlobalVersion)
		fmt.Printf("%-8s %s\n", "Updated", target.GlobalDateUpdate)
		fmt.Printf("%-8s %s\n", "Build", build)
		fmt.Printf("\n%s\n", infoDescription)
	}
}

// buildHash — last 8 characters of GlobalHash
func buildHash() string {
	h := target.GlobalHash
	if len(h) > 8 {
		return h[len(h)-8:]
	}
	return h
}

// // // //

func main() {
	// command-line flags
	host := flag.String("host", "0.0.0.0", "listen host")
	port := flag.Int("port", 11434, "server port")
	openwebuiURL := flag.String("openwebui-url", "", "Open WebUI URL (required)")
	email := flag.String("email", "", "Open WebUI auth email (required)")
	password := flag.String("password", "", "Open WebUI auth password (required)")
	cacheDir := flag.String("cache-dir", "./cache", "cache directory for session files")
	maxBody := flag.Int64("max-body", 100<<20, "max request body size in bytes (default 100 MB)")
	maxErrorBody := flag.Int64("max-error-body", 1<<20, "max upstream error body size in bytes (default 1 MB)")
	tagsTTL := flag.Duration("tags-ttl", 10*time.Minute, "TTL for /api/tags disk+memory cache")
	showTTL := flag.Duration("show-ttl", 30*time.Minute, "TTL for /api/show disk cache")
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP timeout for non-streaming requests")
	streamIdleTimeout := flag.Duration("stream-idle-timeout", 5*time.Minute, "idle timeout for streaming responses (0 = disabled)")
	shutdownTimeout := flag.Duration("shutdown-timeout", 5*time.Second, "graceful shutdown timeout")
	corsOrigins := flag.String("cors-origins", "*", "CORS Access-Control-Allow-Origin (empty = disabled)")
	rateLimit := flag.Int("rate-limit", 0, "global rate limit in requests per second (0 = disabled)")
	ollamaVersion := flag.String("ollama-version", "0.5.4", "Ollama API version string reported to clients")
	infoShort := flag.Bool("i", false, "print info in text format and exit")
	infoFmt := flag.String("info", "", "print info and exit (text|json)")

	flag.Usage = printUsage
	flag.Parse()

	// info mode: does not require mandatory flags
	if *infoShort || *infoFmt != "" {
		format := *infoFmt
		if format == "" {
			format = "text"
		}
		printInfo(format)
		return
	}

	// required parameters check
	if *openwebuiURL == "" || *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "Error: required flags: --openwebui-url, --email, --password")
		fmt.Fprintln(os.Stderr, "")
		flag.Usage()
		os.Exit(1)
	}

	a := auth.New(*openwebuiURL, *email, *password, *cacheDir)

	// verify upstream availability and credentials
	if _, err := a.EnsureToken(context.Background()); err != nil {
		log.Fatalf("Upstream not available: %v", err)
	}
	log.Printf("Upstream OK: %s", *openwebuiURL)

	srv := NewServer(a, *cacheDir, *maxBody, *maxErrorBody, *tagsTTL, *showTTL, *timeout, *streamIdleTimeout, *ollamaVersion, *corsOrigins, *rateLimit)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// graceful shutdown on SIGINT / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Ollama proxy started on %s", addr)
		log.Printf("Open WebUI: %s", *openwebuiURL)
		log.Printf("Cache: %s (tags TTL: %v, show TTL: %v)", *cacheDir, *tagsTTL, *showTTL)
		log.Printf("Max body: %d MB, timeout: %v, stream idle: %v", *maxBody>>20, *timeout, *streamIdleTimeout)
		if *rateLimit > 0 {
			log.Printf("Rate limit: %d rps", *rateLimit)
		}

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutdown signal received, stopping server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), *shutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
