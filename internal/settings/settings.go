package settings

import (
	"encoding/json"
	"os"
	"time"

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
	// AuthToken is the itch.io API key from QR sign-in. It does not expire;
	// it stays valid until the user signs out or revokes it on itch.io.
	AuthToken      string    `json:"auth_token,omitempty"`
	AuthUser       string    `json:"auth_user,omitempty"`
	AuthObtainedAt time.Time `json:"auth_obtained_at,omitempty"`
	// OnboardingSeen is set once the first-run account prompt was answered.
	OnboardingSeen bool `json:"onboarding_seen,omitempty"`
	// LegacyKeyRemoved records that a 1.0.x API key was removed on upgrade,
	// so the app can explain once that paid games need a sign-in.
	LegacyKeyRemoved bool              `json:"legacy_key_removed,omitempty"`
	ROMLocation      string            `json:"rom_location"`
	LastROMDirs      map[string]string `json:"last_rom_dirs,omitempty"`
	Filter           ContentFilter     `json:"content_filter"`
	LogLevel         string            `json:"log_level,omitempty"`       // "debug" | "" (resolves to "info")
	SortMode         string            `json:"sort_mode,omitempty"`       // "az" | "za" | "new" | "dl" | "free" | "paid" | "" (empty = [RSS])
	PlatformFilter   string            `json:"platform_filter,omitempty"` // "" = All; persisted to config.json
	NextUITheme      bool              `json:"nextui_theme"`
	UnifiedNaming    bool              `json:"unified_naming"`           // default true — no omitempty so false survives save/load
	MusicDownload    string            `json:"music_download,omitempty"` // "auto" | "ask" | "off"
	MusicLocation    string            `json:"music_location,omitempty"` // "auto" | "ask"
	Pico8Core        string            `json:"pico8_core,omitempty"`     // "fakeo8" | "pico8"
	// ShareDeviceInfo adds firmware, device and platform to the User-Agent.
	// Default true — no omitempty so an opt-out survives save/load.
	ShareDeviceInfo bool `json:"share_device_info"`
}

func defaults() *Config {
	return &Config{
		ROMLocation:     "auto",
		UnifiedNaming:   true,
		MusicDownload:   "off",
		MusicLocation:   "auto",
		Pico8Core:       "fakeo8",
		ShareDeviceInfo: true,
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
	migrateLegacyAPIKey(data, cfg, path)
	return cfg, nil
}

// migrateLegacyAPIKey removes the api_key 1.0.x stored. QR sign-in replaced
// it; keeping it on disk would leave a credential nothing uses. Every other
// setting, and the inventory, is left as it was.
func migrateLegacyAPIKey(data []byte, cfg *Config, path string) {
	var legacy struct {
		APIKey *string `json:"api_key"`
	}
	if json.Unmarshal(data, &legacy) != nil || legacy.APIKey == nil {
		return
	}
	if key := *legacy.APIKey; key != "" {
		// Masked before anything could echo the raw file into the log.
		logger.RegisterSecret(key, "[API-KEY]")
		cfg.LegacyKeyRemoved = true
		logger.Info("settings: removing legacy api_key (sign in with a QR code instead)")
	} else {
		logger.Debug("settings: dropping empty legacy api_key")
	}
	if err := cfg.Save(path); err != nil {
		logger.Warn("settings: could not rewrite config without api_key: %v", err)
	}
}

// SignedIn reports whether the user is signed in to itch.io.
func (c *Config) SignedIn() bool { return c.AuthToken != "" }

// SetSignedIn records a successful QR sign-in.
func (c *Config) SetSignedIn(token, user string, at time.Time) {
	c.AuthToken, c.AuthUser, c.AuthObtainedAt = token, user, at
	c.LegacyKeyRemoved = false
}

// SignOut forgets the token. Installed games are not touched.
func (c *Config) SignOut() {
	c.AuthToken, c.AuthUser, c.AuthObtainedAt = "", "", time.Time{}
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
