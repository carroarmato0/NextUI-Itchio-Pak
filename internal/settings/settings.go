package settings

import (
	"encoding/json"
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// CategoryFilter holds the enabled state and individually-disabled tags for
// one content filter category.
type CategoryFilter struct {
	Enabled  bool     `json:"enabled"`
	Disabled []string `json:"disabled,omitempty"`
}

// HasActiveTag reports whether at least one tag from tagList would be filtered
// (Enabled is true and the tag is not in Disabled).
func (cf CategoryFilter) HasActiveTag(tagList []string) bool {
	if !cf.Enabled {
		return false
	}
	for _, tag := range tagList {
		inDisabled := false
		for _, d := range cf.Disabled {
			if d == tag {
				inDisabled = true
				break
			}
		}
		if !inDisabled {
			return true
		}
	}
	return false
}

// ContentFilter holds the complete content filter configuration.
// AdultContent, HeavyThemes, and SubstanceUse default to enabled.
// QueerContent defaults to disabled.
type ContentFilter struct {
	AdultContent CategoryFilter `json:"adult_content"`
	QueerContent CategoryFilter `json:"queer_content"`
	HeavyThemes  CategoryFilter `json:"heavy_themes"`
	SubstanceUse CategoryFilter `json:"substance_use"`
}

// Config is the top-level application configuration.
type Config struct {
	APIKey       string            `json:"api_key"`
	ROMLocation  string            `json:"rom_location"`
	LastROMDirs   map[string]string `json:"last_rom_dirs,omitempty"`
	Filter        ContentFilter     `json:"content_filter"`
	LogLevel      string            `json:"log_level,omitempty"`  // "debug" | "" (resolves to "info")
	SortMode      string            `json:"sort_mode,omitempty"`  // "az" | "za" | "new" | "dl" | "free" | "paid" | "" (empty = [RSS])
	PlatformFilter string            `json:"platform_filter,omitempty"` // "" = All; persisted to config.json
	NextUITheme   bool              `json:"nextui_theme"`
	UnifiedNaming bool              `json:"unified_naming"` // default true — no omitempty so false survives save/load
	MusicDownload string            `json:"music_download,omitempty"` // "auto" | "ask" | "off"
	MusicLocation string            `json:"music_location,omitempty"` // "auto" | "ask"
	Pico8Core     string            `json:"pico8_core,omitempty"`     // "fakeo8" | "pico8"
}

func defaults() *Config {
	return &Config{
		APIKey:        "",
		ROMLocation:   "auto",
		UnifiedNaming: true,
		MusicDownload: "off",
		MusicLocation: "auto",
		Pico8Core:     "fakeo8",
		Filter: ContentFilter{
			AdultContent: CategoryFilter{Enabled: true},
			HeavyThemes:  CategoryFilter{Enabled: true},
			SubstanceUse: CategoryFilter{Enabled: true},
			// QueerContent defaults to disabled (zero value).
		},
	}
}

// Load reads the config from path. If the file is missing or corrupted,
// defaults are returned without an error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Debug("settings: config not found at %s, using defaults", path)
		return defaults(), nil
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		logger.Warn("settings: config at %s is invalid, using defaults: %v", path, err)
		return defaults(), nil
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Error("settings: failed to write tmp config %s: %v", tmp, err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		logger.Error("settings: failed to rename config %s → %s: %v", tmp, path, err)
		return err
	}
	return nil
}
