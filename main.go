package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"openwebui-ollama-proxy/auth"
)

// // // // // // // // // //

func main() {
	// флаги командной строки
	host := flag.String("host", "0.0.0.0", "listen host")
	port := flag.Int("port", 11434, "server port")
	openwebuiURL := flag.String("openwebui-url", "", "Open WebUI URL (required)")
	email := flag.String("email", "", "Open WebUI auth email (required)")
	password := flag.String("password", "", "Open WebUI auth password (required)")
	cacheDir := flag.String("cache-dir", "./cache", "cache directory for session files")

	flag.Parse()

	// проверка обязательных параметров
	if *openwebuiURL == "" || *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "Error: required flags: --openwebui-url, --email, --password")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	a := auth.New(*openwebuiURL, *email, *password, *cacheDir)
	srv := NewServer(a)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// graceful shutdown по SIGINT / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Ollama proxy started on %s", addr)
		log.Printf("Open WebUI: %s", *openwebuiURL)
		log.Printf("Cache: %s", *cacheDir)

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutdown signal received, stopping server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5e9) // 5 секунд
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
