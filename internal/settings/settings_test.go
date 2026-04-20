package settings_test

import (
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
	if cfg.ROMSelection != "auto" {
		t.Errorf("default ROMSelection = %q, want %q", cfg.ROMSelection, "auto")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{APIKey: "abc123", ROMSelection: "ask"}
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
	if loaded.ROMSelection != "ask" {
		t.Errorf("ROMSelection = %q, want %q", loaded.ROMSelection, "ask")
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
	if cfg.ROMSelection != "auto" {
		t.Errorf("corrupted load should return defaults, got ROMSelection = %q", cfg.ROMSelection)
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
		ROMSelection: "auto",
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
