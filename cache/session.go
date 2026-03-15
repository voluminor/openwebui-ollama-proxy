package cache

import (
	"path/filepath"
	"time"
)

// // // // // // // // // //

var magicSession = [2]byte{0xCA, 0x01}

// SessionObj — cached authorization session
type SessionObj struct {
	Token     string
	ExpiresAt time.Time
	Email     string
	BaseURL   string
}

// // // //

// ReadSession — reads session from cache
func ReadSession(cacheDir string) *SessionObj {
	return Read[SessionObj](filepath.Join(cacheDir, "session.bin"), magicSession)
}

// WriteSession — writes session to cache
func WriteSession(cacheDir string, s SessionObj) error {
	return Write(filepath.Join(cacheDir, "session.bin"), magicSession, s)
}
