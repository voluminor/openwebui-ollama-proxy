package cache

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"time"

	"openwebui-ollama-proxy/ollama"
)

// // // // // // // // // //

var magicShow = [2]byte{0xCA, 0x03}

// ShowObj — cached model metadata
type ShowObj struct {
	Response  ollama.ShowResponse
	ExpiresAt time.Time
}

// // // //

// showPath — cache file path for a specific model
func showPath(cacheDir, model string) string {
	h := sha256.Sum256([]byte(model))
	return filepath.Join(cacheDir, fmt.Sprintf("show_%x.bin", h[:8]))
}

// ReadShow — reads model metadata from cache
func ReadShow(cacheDir, model string) *ShowObj {
	return Read[ShowObj](showPath(cacheDir, model), magicShow)
}

// WriteShow — writes model metadata to cache
func WriteShow(cacheDir, model string, resp ollama.ShowResponse, ttl time.Duration) error {
	return Write(showPath(cacheDir, model), magicShow, ShowObj{
		Response:  resp,
		ExpiresAt: time.Now().Add(ttl),
	})
}
