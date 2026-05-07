package itchio

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// OwnedCache is the on-disk representation of the user's owned game URLs.
type OwnedCache struct {
	SavedAt time.Time `json:"saved_at"`
	URLs    []string  `json:"urls"`
}

// SaveOwnedCache writes urls to path atomically (write to .tmp then rename).
func SaveOwnedCache(path string, urls []string) error {
	cache := OwnedCache{SavedAt: time.Now(), URLs: urls}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("marshal owned cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write owned cache tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename owned cache: %w", err)
	}
	return nil
}

// LoadOwnedCache reads the cache at path and returns the URL list.
// Returns a nil slice (not an error) when the file does not exist.
func LoadOwnedCache(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read owned cache: %w", err)
	}
	var cache OwnedCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse owned cache: %w", err)
	}
	return cache.URLs, nil
}
