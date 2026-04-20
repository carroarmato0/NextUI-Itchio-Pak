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

// Mature content is the only filter that defaults to ON.
func TestDefaultsMatureEnabled(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Filter.MatureEnabled {
		t.Error("expected MatureEnabled=true by default")
	}
	if cfg.Filter.LGBTQ.Enabled {
		t.Error("expected LGBTQ.Enabled=false by default")
	}
	if cfg.Filter.HeavyThemes.Enabled {
		t.Error("expected HeavyThemes.Enabled=false by default")
	}
	if cfg.Filter.SubstanceUse.Enabled {
		t.Error("expected SubstanceUse.Enabled=false by default")
	}
	if cfg.Filter.SexualContent.Enabled {
		t.Error("expected SexualContent.Enabled=false by default")
	}
}

func TestContentFilterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{
		APIKey:       "",
		ROMSelection: "auto",
		Filter: settings.ContentFilter{
			MatureEnabled: false,
			LGBTQ:         settings.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq", "gay"}},
			HeavyThemes:   settings.CategoryFilter{Enabled: true, Disabled: []string{"grief"}},
			SubstanceUse:  settings.CategoryFilter{Enabled: true},
			SexualContent: settings.CategoryFilter{},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Filter.MatureEnabled {
		t.Error("MatureEnabled not preserved")
	}
	if !loaded.Filter.LGBTQ.Enabled {
		t.Error("LGBTQ.Enabled not preserved")
	}
	if len(loaded.Filter.LGBTQ.Disabled) != 2 {
		t.Errorf("LGBTQ.Disabled: expected 2 entries, got %v", loaded.Filter.LGBTQ.Disabled)
	}
	if len(loaded.Filter.HeavyThemes.Disabled) != 1 {
		t.Errorf("HeavyThemes.Disabled: expected 1 entry, got %v", loaded.Filter.HeavyThemes.Disabled)
	}
	if !loaded.Filter.SubstanceUse.Enabled {
		t.Error("SubstanceUse.Enabled not preserved")
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
}
