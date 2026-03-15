package cache

import (
	"path/filepath"
	"time"

	"openwebui-ollama-proxy/ollama"
)

// // // // // // // // // //

var magicTags = [2]byte{0xCA, 0x02}

// TagsObj — cached model list
type TagsObj struct {
	Models    []ollama.ModelInfo
	ExpiresAt time.Time
}

// // // //

// ReadTags — reads model list from cache
func ReadTags(cacheDir string) *TagsObj {
	return Read[TagsObj](filepath.Join(cacheDir, "tags.bin"), magicTags)
}

// WriteTags — writes model list to cache
func WriteTags(cacheDir string, models []ollama.ModelInfo, ttl time.Duration) error {
	return Write(filepath.Join(cacheDir, "tags.bin"), magicTags, TagsObj{
		Models:    models,
		ExpiresAt: time.Now().Add(ttl),
	})
}
