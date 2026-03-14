package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// // // // // // // // // //

// makeJWT — creates a test JWT with given exp
func makeJWT(exp int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := fmt.Sprintf(`{"sub":"user","exp":%d}`, exp)
	payload := base64.RawURLEncoding.EncodeToString([]byte(claims))
	return header + "." + payload + ".signature"
}

// // // //

func TestParseJWTExpiry_Valid(t *testing.T) {
	exp := time.Now().Add(24 * time.Hour).Unix()
	token := makeJWT(exp)

	got, err := parseJWTExpiry(token)
	if err != nil {
		t.Fatalf("parseJWTExpiry: %v", err)
	}
	if got.Unix() != exp {
		t.Fatalf("exp = %d, want %d", got.Unix(), exp)
	}
}

func TestParseJWTExpiry_FloatExp(t *testing.T) {
	// JSON numbers are decoded as float64
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1700000000}`))
	token := header + "." + payload + ".sig"

	got, err := parseJWTExpiry(token)
	if err != nil {
		t.Fatalf("parseJWTExpiry: %v", err)
	}
	if got.Unix() != 1700000000 {
		t.Fatalf("exp = %d, want 1700000000", got.Unix())
	}
}

func TestParseJWTExpiry_JsonNumber(t *testing.T) {
	// simulate json.Number — manually encode
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	claims := map[string]any{"exp": json.Number("1700000000")}
	claimsBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsBytes)
	token := header + "." + payload + ".sig"

	// parseJWTExpiry uses json.Unmarshal without UseNumber,
	// so exp will be float64, not json.Number
	got, err := parseJWTExpiry(token)
	if err != nil {
		t.Fatalf("parseJWTExpiry: %v", err)
	}
	if got.Unix() != 1700000000 {
		t.Fatalf("exp = %d", got.Unix())
	}
}

func TestParseJWTExpiry_InvalidJWT(t *testing.T) {
	_, err := parseJWTExpiry("not-a-jwt")
	if err == nil {
		t.Fatal("expected error for invalid JWT")
	}
}

func TestParseJWTExpiry_InvalidBase64(t *testing.T) {
	_, err := parseJWTExpiry("header.!!!invalid!!!.sig")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestParseJWTExpiry_InvalidJSON(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`not json`))
	token := header + "." + payload + ".sig"

	_, err := parseJWTExpiry(token)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseJWTExpiry_NoExp(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user"}`))
	token := header + "." + payload + ".sig"

	_, err := parseJWTExpiry(token)
	if err == nil {
		t.Fatal("expected error for missing exp")
	}
}

func TestParseJWTExpiry_PaddingVariants(t *testing.T) {
	// payload lengths requiring different padding
	cases := []string{
		`{"exp":1}`,       // 9 bytes → base64 12 chars (len%4 == 0)
		`{"exp":12}`,      // 10 bytes → base64 14 chars (len%4 == 2)
		`{"exp":123}`,     // 11 bytes → base64 16 chars (len%4 == 0)
		`{"exp":1234567}`, // 15 bytes → base64 20 chars (len%4 == 0)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))

	for _, c := range cases {
		payload := base64.RawURLEncoding.EncodeToString([]byte(c))
		token := header + "." + payload + ".sig"
		_, err := parseJWTExpiry(token)
		if err != nil {
			t.Fatalf("padding case %q: %v", c, err)
		}
	}
}

// // // //

func TestNew(t *testing.T) {
	a := New("https://example.com/", "user@test.com", "pass", "/tmp/cache")
	if a.BaseURL() != "https://example.com" {
		t.Fatalf("BaseURL = %q, trailing slash should be trimmed", a.BaseURL())
	}
}

// // // // benchmarks // // // //

func BenchmarkParseJWTExpiry(b *testing.B) {
	token := makeJWT(time.Now().Add(24 * time.Hour).Unix())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseJWTExpiry(token)
	}
}
