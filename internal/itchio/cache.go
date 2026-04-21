package itchio

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// CacheMeta records when the cache was last populated.
type CacheMeta struct {
	FetchedAt  time.Time `json:"fetched_at"`
	TotalGames int       `json:"total_games"`
}

// GameCache is the on-disk representation of the full game list.
type GameCache struct {
	Meta  CacheMeta `json:"meta"`
	Games []Game    `json:"games"`
}

// SaveGamesCache writes games to path atomically (write to .tmp then rename).
func SaveGamesCache(path string, games []Game) error {
	cache := GameCache{
		Meta:  CacheMeta{FetchedAt: time.Now(), TotalGames: len(games)},
		Games: games,
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("marshal game cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write game cache tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename game cache: %w", err)
	}
	return nil
}

// LoadGamesCache reads and parses the cache file at path.
// Returns an error if the file is missing or unparseable.
func LoadGamesCache(path string) (*GameCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read game cache: %w", err)
	}
	var cache GameCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse game cache: %w", err)
	}
	return &cache, nil
}
