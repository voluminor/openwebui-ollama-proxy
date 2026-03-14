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
)

func main() {
	// флаги командной строки
	host := flag.String("host", "0.0.0.0", "хост для прослушивания")
	port := flag.Int("port", 11434, "порт сервера")
	openwebuiURL := flag.String("openwebui-url", "", "URL Open WebUI (обязательный)")
	email := flag.String("email", "", "email для авторизации в Open WebUI (обязательный)")
	password := flag.String("password", "", "пароль для авторизации в Open WebUI (обязательный)")
	cacheDir := flag.String("cache-dir", "./cache", "папка для кеш-файлов (сессия авторизации и тд)")

	flag.Parse()

	// проверка обязательных параметров
	if *openwebuiURL == "" || *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "Ошибка: обязательные параметры: --openwebui-url, --email, --password")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Использование:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// создаём авторизацию и сервер
	auth := NewAuth(*openwebuiURL, *email, *password, *cacheDir)
	srv := NewServer(auth)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// graceful shutdown по SIGINT / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Ollama-прокси запущен на %s", addr)
		log.Printf("Open WebUI: %s", *openwebuiURL)
		log.Printf("Кеш: %s", *cacheDir)

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка сервера: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Получен сигнал завершения, останавливаю сервер...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5e9) // 5 секунд
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка при остановке: %v", err)
	}

	log.Println("Сервер остановлен")
}
