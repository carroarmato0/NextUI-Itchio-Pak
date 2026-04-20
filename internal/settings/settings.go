package settings

import (
	"encoding/json"
	"os"
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
// MatureEnabled defaults to true; all other categories default to false.
type ContentFilter struct {
	MatureEnabled bool           `json:"mature_enabled"`
	LGBTQ         CategoryFilter `json:"lgbtq"`
	HeavyThemes   CategoryFilter `json:"heavy_themes"`
	SubstanceUse  CategoryFilter `json:"substance_use"`
	SexualContent CategoryFilter `json:"sexual_content"`
}

// Config is the top-level application configuration.
type Config struct {
	APIKey       string        `json:"api_key"`
	ROMSelection string        `json:"rom_selection"`
	Filter       ContentFilter `json:"content_filter"`
}

func defaults() *Config {
	return &Config{
		APIKey:       "",
		ROMSelection: "auto",
		Filter: ContentFilter{
			MatureEnabled: true,
			// All other categories default to disabled (zero value).
		},
	}
}

// Load reads the config from path. If the file is missing or corrupted,
// defaults are returned without an error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults(), nil
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaults(), nil
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
