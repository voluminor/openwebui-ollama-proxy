package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"openwebui-ollama-proxy/cache"
)

// // // // // // // // // //

// Obj — управление авторизацией в Open WebUI.
// Хранит JWT, обновляет при истечении, кеширует сессию на диск.
type Obj struct {
	baseURL  string
	email    string
	password string
	cacheDir string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	httpClient  *http.Client
}

// // // //

// New — создаёт экземпляр Obj
func New(baseURL, email, password, cacheDir string) *Obj {
	return &Obj{
		baseURL:  strings.TrimRight(baseURL, "/"),
		email:    email,
		password: password,
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BaseURL — базовый URL Open WebUI
func (a *Obj) BaseURL() string {
	return a.baseURL
}

// // // //

// loadSession — загружает токен из бинарного кеша
func (a *Obj) loadSession() error {
	s := cache.ReadSession(a.cacheDir)
	if s == nil {
		return errors.New("no cached session")
	}

	if s.BaseURL != a.baseURL || s.Email != a.email {
		return errors.New("session mismatch")
	}

	a.token = s.Token
	a.tokenExpiry = s.ExpiresAt
	return nil
}

// saveSession — сохраняет токен в бинарный кеш
func (a *Obj) saveSession() error {
	return cache.WriteSession(a.cacheDir, cache.SessionObj{
		Token:     a.token,
		ExpiresAt: a.tokenExpiry,
		Email:     a.email,
		BaseURL:   a.baseURL,
	})
}

// parseJWTExpiry — извлекает exp из JWT без верификации подписи
func parseJWTExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, errors.New("invalid JWT")
	}

	payload := parts[1]
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
		return time.Time{}, errors.New("no exp field in JWT")
	}
}

// // // //

// EnsureToken — возвращает актуальный JWT, при необходимости логинится
func (a *Obj) EnsureToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token == "" {
		_ = a.loadSession()
	}

	// токен ещё валиден
	if a.token != "" && time.Until(a.tokenExpiry) > 30*time.Second {
		return a.token, nil
	}

	if err := a.login(ctx); err != nil {
		return "", fmt.Errorf("authorization: %w", err)
	}

	return a.token, nil
}

// login — авторизация через POST /api/v1/auths/signin
func (a *Obj) login(ctx context.Context) error {
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
		return fmt.Errorf("signin request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("signin returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("signin response decode: %w", err)
	}
	if result.Token == "" {
		return errors.New("signin: empty token")
	}

	exp, err := parseJWTExpiry(result.Token)
	if err != nil || exp.IsZero() {
		// не удалось распарсить — fallback 24 часа
		exp = time.Now().Add(24 * time.Hour)
	}

	a.token = result.Token
	a.tokenExpiry = exp
	_ = a.saveSession()

	return nil
}
