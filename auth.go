package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Auth — управление авторизацией в Open WebUI.
// Хранит JWT-токен, обновляет его при истечении, кеширует сессию на диск.
type Auth struct {
	baseURL  string
	email    string
	password string
	cacheDir string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	httpClient  *http.Client
}

// NewAuth — создаёт новый экземпляр Auth
func NewAuth(baseURL, email, password, cacheDir string) *Auth {
	return &Auth{
		baseURL:  strings.TrimRight(baseURL, "/"),
		email:    email,
		password: password,
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// EnsureToken — возвращает актуальный JWT-токен, при необходимости логинится заново
func (a *Auth) EnsureToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// пробуем загрузить сессию из файла если токена нет
	if a.token == "" {
		_ = a.loadSession()
	}

	// если токен ещё валиден — возвращаем
	if a.token != "" && time.Until(a.tokenExpiry) > 30*time.Second {
		return a.token, nil
	}

	// иначе логинимся заново
	if err := a.login(ctx); err != nil {
		return "", fmt.Errorf("авторизация: %w", err)
	}

	return a.token, nil
}

// login — выполняет авторизацию через POST /api/v1/auths/signin
func (a *Auth) login(ctx context.Context) error {
	payload, _ := json.Marshal(map[string]string{
		"email":    a.email,
		"password": a.password,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/v1/auths/signin", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("запрос к signin: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("signin вернул %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("декодирование ответа signin: %w", err)
	}
	if result.Token == "" {
		return errors.New("signin: пустой токен")
	}

	// парсим время истечения из JWT
	exp, err := parseJWTExpiry(result.Token)
	if err != nil || exp.IsZero() {
		// если не удалось распарсить — ставим 24 часа
		exp = time.Now().Add(24 * time.Hour)
	}

	a.token = result.Token
	a.tokenExpiry = exp
	_ = a.saveSession()

	return nil
}

// sessionPath — путь к файлу сессии
func (a *Auth) sessionPath() string {
	return filepath.Join(a.cacheDir, "session.json")
}

// loadSession — загружает токен из файла кеша
func (a *Auth) loadSession() error {
	f, err := os.Open(a.sessionPath())
	if err != nil {
		return err
	}
	defer f.Close()

	var s SessionData
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return err
	}

	// проверяем что сессия от того же аккаунта и сервера
	if s.BaseURL != a.baseURL || s.Email != a.email {
		return errors.New("сессия не совпадает")
	}

	a.token = s.Token
	a.tokenExpiry = s.Expiry
	return nil
}

// saveSession — сохраняет токен в файл кеша
func (a *Auth) saveSession() error {
	_ = os.MkdirAll(a.cacheDir, 0o755)

	f, err := os.Create(a.sessionPath())
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(SessionData{
		Token:   a.token,
		Expiry:  a.tokenExpiry,
		Email:   a.email,
		BaseURL: a.baseURL,
	})
}

// parseJWTExpiry — извлекает время истечения (exp) из JWT-токена без верификации подписи
func parseJWTExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, errors.New("невалидный JWT")
	}

	// декодируем payload (вторая часть JWT)
	payload := parts[1]
	// добавляем padding для base64
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return time.Time{}, err
	}

	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return time.Time{}, err
	}

	switch v := claims["exp"].(type) {
	case float64:
		return time.Unix(int64(v), 0), nil
	case json.Number:
		i, _ := v.Int64()
		return time.Unix(i, 0), nil
	default:
		return time.Time{}, errors.New("нет поля exp в JWT")
	}
}
