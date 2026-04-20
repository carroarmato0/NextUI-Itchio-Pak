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

func TestDefaultsHaveParentalAdvisoryEnabled(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Parental.MatureEnabled {
		t.Error("expected MatureEnabled=true by default")
	}
	if !cfg.Parental.SensitiveEnabled {
		t.Error("expected SensitiveEnabled=true by default")
	}
	if cfg.Parental.SensitiveDisabled != nil {
		t.Errorf("expected SensitiveDisabled=nil by default, got %v", cfg.Parental.SensitiveDisabled)
	}
}

func TestParentalAdvisoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	cfg := &settings.Config{
		APIKey:       "",
		ROMSelection: "auto",
		Parental: settings.ParentalAdvisory{
			MatureEnabled:     false,
			SensitiveEnabled:  true,
			SensitiveDisabled: []string{"lgbtq", "sexy"},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Parental.MatureEnabled != false {
		t.Error("MatureEnabled not preserved")
	}
	if loaded.Parental.SensitiveEnabled != true {
		t.Error("SensitiveEnabled not preserved")
	}
	if len(loaded.Parental.SensitiveDisabled) != 2 {
		t.Errorf("SensitiveDisabled: expected 2 entries, got %v", loaded.Parental.SensitiveDisabled)
	}
}
