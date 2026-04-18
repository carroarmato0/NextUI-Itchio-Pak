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
	os.WriteFile(path, []byte("not json"), 0644)

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ROMSelection != "auto" {
		t.Errorf("corrupted load should return defaults, got ROMSelection = %q", cfg.ROMSelection)
	}
}
