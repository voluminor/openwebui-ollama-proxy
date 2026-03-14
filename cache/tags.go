package cache

import (
	"path/filepath"
	"time"

	"openwebui-ollama-proxy/ollama"
)

// // // // // // // // // //

var magicTags = [2]byte{0xCA, 0x02}

// TagsTTL — время жизни кеша списка моделей
const TagsTTL = 10 * time.Minute

// TagsObj — кешированный список моделей
type TagsObj struct {
	Models    []ollama.ModelInfo
	ExpiresAt time.Time
}

// // // //

// ReadTags — читает список моделей из кеша
func ReadTags(cacheDir string) *TagsObj {
	return Read[TagsObj](filepath.Join(cacheDir, "tags.bin"), magicTags)
}

// WriteTags — записывает список моделей в кеш
func WriteTags(cacheDir string, models []ollama.ModelInfo) error {
	return Write(filepath.Join(cacheDir, "tags.bin"), magicTags, TagsObj{
		Models:    models,
		ExpiresAt: time.Now().Add(TagsTTL),
	})
}
