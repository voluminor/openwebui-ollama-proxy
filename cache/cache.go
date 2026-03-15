package cache

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"openwebui-ollama-proxy/target"
)

// // // // // // // // // //

// nonceSize — standard nonce size for AES-GCM
const nonceSize = 12

// minFileSize — magic (2) + nonce (12) + integrity SHA-256 (32)
const minFileSize = 2 + nonceSize + 32

// // // //

// deriveKey — AES key: SHA-256(integrity + build salt)
func deriveKey(integrity []byte) []byte {
	h := sha256.New()
	h.Write(integrity)
	h.Write([]byte(target.GlobalHash))
	return h.Sum(nil)
}

// removeInvalid — removes a corrupted cache file
func removeInvalid(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("[cache] remove %s: %v", path, err)
	}
}

// // // //

// Read — reads, decrypts, and decodes an object from file.
// On any error: logs, removes the file, returns nil.
func Read[T any](path string, magic [2]byte) *T {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[cache] read %s: %v", path, err)
		}
		return nil
	}

	if len(data) < minFileSize {
		log.Printf("[cache] %s: file too short (%d bytes)", path, len(data))
		removeInvalid(path)
		return nil
	}

	if data[0] != magic[0] || data[1] != magic[1] {
		log.Printf("[cache] %s: magic mismatch (got %02x%02x, want %02x%02x)", path, data[0], data[1], magic[0], magic[1])
		removeInvalid(path)
		return nil
	}

	integrity := data[len(data)-32:]
	aesKey := deriveKey(integrity)
	nonce := data[2 : 2+nonceSize]
	ciphertext := data[2+nonceSize : len(data)-32]

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		log.Printf("[cache] %s: cipher init: %v", path, err)
		removeInvalid(path)
		return nil
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Printf("[cache] %s: GCM init: %v", path, err)
		removeInvalid(path)
		return nil
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		log.Printf("[cache] %s: decrypt failed: %v", path, err)
		removeInvalid(path)
		return nil
	}

	check := sha256.Sum256(plaintext)
	if !bytes.Equal(check[:], integrity) {
		log.Printf("[cache] %s: integrity check failed", path)
		removeInvalid(path)
		return nil
	}

	var v T
	if err := gob.NewDecoder(bytes.NewReader(plaintext)).Decode(&v); err != nil {
		log.Printf("[cache] %s: decode failed: %v", path, err)
		removeInvalid(path)
		return nil
	}

	return &v
}

// Write — encodes, encrypts, and writes an object to file.
// Format: [magic(2)][nonce(12)][ciphertext][integrity SHA-256(32)]
func Write[T any](path string, magic [2]byte, v T) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	plaintext := buf.Bytes()

	intArr := sha256.Sum256(plaintext)
	integrity := intArr[:]
	aesKey := deriveKey(integrity)

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return fmt.Errorf("cipher init: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("GCM init: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("nonce gen: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, 2+nonceSize+len(ciphertext)+32)
	out = append(out, magic[:]...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	out = append(out, integrity...)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	return os.WriteFile(path, out, 0o600)
}
