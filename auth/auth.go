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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

// Session — данные сессии для кеша на диск
type Session struct {
	Token   string    `json:"token"`
	Expiry  time.Time `json:"expiry"`
	Email   string    `json:"email"`
	BaseURL string    `json:"base_url"`
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

// sessionPath — путь к файлу сессии
func (a *Obj) sessionPath() string {
	return filepath.Join(a.cacheDir, "session.json")
}

// loadSession — загружает токен из файла кеша
func (a *Obj) loadSession() error {
	f, err := os.Open(a.sessionPath())
	if err != nil {
		return err
	}
	defer f.Close()

	var s Session
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return err
	}

	if s.BaseURL != a.baseURL || s.Email != a.email {
		return errors.New("session mismatch")
	}

	a.token = s.Token
	a.tokenExpiry = s.Expiry
	return nil
}

// saveSession — сохраняет токен в файл кеша
func (a *Obj) saveSession() error {
	_ = os.MkdirAll(a.cacheDir, 0o755)

	f, err := os.Create(a.sessionPath())
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(Session{
		Token:   a.token,
		Expiry:  a.tokenExpiry,
		Email:   a.email,
		BaseURL: a.baseURL,
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
