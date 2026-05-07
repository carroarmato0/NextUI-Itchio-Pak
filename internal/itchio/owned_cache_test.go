package itchio_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestOwnedCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned_cache.json")
	urls := []string{"https://a.itch.io/game1", "https://b.itch.io/game2"}

	if err := itchio.SaveOwnedCache(path, urls); err != nil {
		t.Fatalf("SaveOwnedCache: %v", err)
	}
	got, err := itchio.LoadOwnedCache(path)
	if err != nil {
		t.Fatalf("LoadOwnedCache: %v", err)
	}
	if len(got) != len(urls) {
		t.Fatalf("got %d URLs, want %d", len(got), len(urls))
	}
	for i, u := range urls {
		if got[i] != u {
			t.Errorf("url[%d]: got %q, want %q", i, got[i], u)
		}
	}

	// Verify SavedAt was persisted (read raw file to inspect)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw cache: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"saved_at"`)) {
		t.Error("saved_at field missing from serialised cache")
	}
	// SavedAt should not be the zero value
	if bytes.Contains(raw, []byte(`"saved_at":"0001-01-01`)) {
		t.Error("saved_at appears to be zero value")
	}
}

func TestLoadOwnedCache_MissingFile(t *testing.T) {
	got, err := itchio.LoadOwnedCache(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slice for missing file, got %v", got)
	}
}

func TestLoadOwnedCache_CorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := itchio.LoadOwnedCache(path)
	if err == nil {
		t.Fatal("expected error for corrupt file, got nil")
	}
}

func TestSaveOwnedCache_EmptySlice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned_cache.json")
	if err := itchio.SaveOwnedCache(path, []string{}); err != nil {
		t.Fatalf("SaveOwnedCache empty: %v", err)
	}
	got, err := itchio.LoadOwnedCache(path)
	if err != nil {
		t.Fatalf("LoadOwnedCache after empty save: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 URLs, got %v", got)
	}
}
