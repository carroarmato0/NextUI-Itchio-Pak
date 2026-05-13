package settings_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "" {
		t.Errorf("default APIKey = %q, want %q", cfg.APIKey, "")
	}
	if cfg.ROMLocation != "auto" {
		t.Errorf("default ROMLocation = %q, want %q", cfg.ROMLocation, "auto")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{APIKey: "abc123", ROMLocation: "ask"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.APIKey != "abc123" {
		t.Errorf("APIKey = %q, want %q", loaded.APIKey, "abc123")
	}
	if loaded.ROMLocation != "ask" {
		t.Errorf("ROMLocation = %q, want %q", loaded.ROMLocation, "ask")
	}
}

func TestLoadCorruptedFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ROMLocation != "auto" {
		t.Errorf("corrupted load should return defaults, got ROMLocation = %q", cfg.ROMLocation)
	}
}

func TestDefaultFilters(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Filter.AdultContent.Enabled {
		t.Error("expected AdultContent.Enabled=true by default")
	}
	if !cfg.Filter.HeavyThemes.Enabled {
		t.Error("expected HeavyThemes.Enabled=true by default")
	}
	if !cfg.Filter.SubstanceUse.Enabled {
		t.Error("expected SubstanceUse.Enabled=true by default")
	}
	if cfg.Filter.QueerContent.Enabled {
		t.Error("expected QueerContent.Enabled=false by default")
	}
}

func TestContentFilterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{
		APIKey:       "",
		ROMLocation: "auto",
		Filter: settings.ContentFilter{
			AdultContent: settings.CategoryFilter{Enabled: true, Disabled: []string{"ecchi", "suggestive"}},
			QueerContent: settings.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq"}},
			HeavyThemes:  settings.CategoryFilter{Enabled: true, Disabled: []string{"grief"}},
			SubstanceUse: settings.CategoryFilter{Enabled: false},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.Filter.AdultContent.Enabled {
		t.Error("AdultContent.Enabled not preserved")
	}
	if len(loaded.Filter.AdultContent.Disabled) != 2 {
		t.Errorf("AdultContent.Disabled: expected 2 entries, got %v", loaded.Filter.AdultContent.Disabled)
	}
	if !loaded.Filter.QueerContent.Enabled {
		t.Error("QueerContent.Enabled not preserved")
	}
	if loaded.Filter.SubstanceUse.Enabled {
		t.Error("SubstanceUse.Enabled not preserved (expected false)")
	}
}

func TestHasActiveTag(t *testing.T) {
	tags := []string{"grief", "suicide", "war"}

	// All enabled, none disabled → true
	cf := settings.CategoryFilter{Enabled: true}
	if !cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=true when all tags enabled")
	}

	// Master off → false
	cf = settings.CategoryFilter{Enabled: false}
	if cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=false when master disabled")
	}

	// All individually disabled → false
	cf = settings.CategoryFilter{Enabled: true, Disabled: []string{"grief", "suicide", "war"}}
	if cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=false when all tags individually disabled")
	}

	// One still active → true
	cf = settings.CategoryFilter{Enabled: true, Disabled: []string{"grief", "suicide"}}
	if !cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=true when one tag still active")
	}

	// Empty tagList → false
	cf = settings.CategoryFilter{Enabled: true}
	if cf.HasActiveTag([]string{}) {
		t.Error("expected HasActiveTag=false for empty tagList")
	}
}

func TestROMLocationDefault(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ROMLocation != "auto" {
		t.Errorf("default ROMLocation = %q, want %q", cfg.ROMLocation, "auto")
	}
	if cfg.LastROMDirs != nil {
		t.Errorf("default LastROMDirs should be nil, got %v", cfg.LastROMDirs)
	}
}

func TestLastROMDirsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{
		ROMLocation: "ask",
		LastROMDirs: map[string]string{
			".gbc": "/mnt/SDCARD/Roms/RPG/GBC/",
			".gb":  "/mnt/SDCARD/Roms/RPG/GB/",
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ROMLocation != "ask" {
		t.Errorf("ROMLocation = %q, want %q", loaded.ROMLocation, "ask")
	}
	if loaded.LastROMDirs[".gbc"] != "/mnt/SDCARD/Roms/RPG/GBC/" {
		t.Errorf(".gbc dir = %q, want %q", loaded.LastROMDirs[".gbc"], "/mnt/SDCARD/Roms/RPG/GBC/")
	}
	if loaded.LastROMDirs[".gb"] != "/mnt/SDCARD/Roms/RPG/GB/" {
		t.Errorf(".gb dir = %q, want %q", loaded.LastROMDirs[".gb"], "/mnt/SDCARD/Roms/RPG/GB/")
	}
}

func TestLastROMDirsOmittedWhenNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{ROMLocation: "ask"} // LastROMDirs is nil
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte("last_rom_dirs")) {
		t.Errorf("last_rom_dirs should be omitted when nil, found in JSON:\n%s", data)
	}
}

func TestLogLevelDefault(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Default is "" which LevelFromString maps to INFO at runtime.
	if cfg.LogLevel != "" {
		t.Errorf("default LogLevel = %q, want %q", cfg.LogLevel, "")
	}
}

func TestLogLevelRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{LogLevel: "debug"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", loaded.LogLevel, "debug")
	}
}

func TestLogLevelOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{} // LogLevel is ""
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte("log_level")) {
		t.Errorf("log_level should be omitted when empty, found in JSON:\n%s", data)
	}
}

func TestSortModeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{SortMode: "az"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.SortMode != "az" {
		t.Errorf("SortMode = %q, want %q", loaded.SortMode, "az")
	}
}

func TestSortModeDefaultsToEmpty(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SortMode != "" {
		t.Errorf("default SortMode = %q, want %q", cfg.SortMode, "")
	}
}

func TestSortModeOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{} // SortMode is ""
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte("sort_mode")) {
		t.Errorf("sort_mode should be omitted when empty, found in JSON:\n%s", data)
	}
}

func TestSortModeBackwardsCompatible(t *testing.T) {
	// Old config without sort_mode key must unmarshal to ""
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	oldJSON := `{"api_key":"","rom_selection":"auto","content_filter":{}}`
	if err := os.WriteFile(path, []byte(oldJSON), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.SortMode != "" {
		t.Errorf("old config SortMode = %q, want empty string", loaded.SortMode)
	}
}

func TestNextUIThemeDefault(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Default must be false.
	if cfg.NextUITheme {
		t.Errorf("default NextUITheme = %v, want %v", cfg.NextUITheme, false)
	}
}

func TestNextUIThemeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{NextUITheme: true}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.NextUITheme {
		t.Errorf("NextUITheme = %v, want %v", loaded.NextUITheme, true)
	}
}

func TestUnifiedNaming_DefaultTrue(t *testing.T) {
	dir := t.TempDir()
	cfg, err := settings.Load(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UnifiedNaming {
		t.Error("UnifiedNaming default should be true")
	}
}

func TestUnifiedNaming_OldConfigGetsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Old config with no unified_naming field
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UnifiedNaming {
		t.Error("UnifiedNaming should default to true when absent from config")
	}
}

func TestUnifiedNaming_ExplicitFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"unified_naming": false}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnifiedNaming {
		t.Error("UnifiedNaming should be false when explicitly set to false")
	}
}

func TestUnifiedNaming_RoundTrip_False(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg, _ := settings.Load(filepath.Join(dir, "missing.json"))
	cfg.UnifiedNaming = false
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.UnifiedNaming {
		t.Error("UnifiedNaming=false should survive save/load round-trip")
	}
}

func TestSave_IsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{APIKey: "test-key", ROMLocation: "ask"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to be absent after successful save, stat: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", loaded.APIKey, "test-key")
	}
}

func TestMusicDefaults(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MusicDownload != "off" {
		t.Errorf("MusicDownload default = %q, want \"off\"", cfg.MusicDownload)
	}
	if cfg.MusicLocation != "auto" {
		t.Errorf("MusicLocation default = %q, want \"auto\"", cfg.MusicLocation)
	}
}

func TestMusicBackwardCompat(t *testing.T) {
	// Old config JSON without music fields
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{"api_key":"","rom_selection":"auto","rom_location":"auto","unified_naming":true}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// After Load() the defaults are applied.
	if cfg.MusicDownload != "off" {
		t.Errorf("backward-compat default MusicDownload = %q, want \"off\"", cfg.MusicDownload)
	}
	if cfg.MusicLocation != "auto" {
		t.Errorf("backward-compat default MusicLocation = %q, want \"auto\"", cfg.MusicLocation)
	}
}
