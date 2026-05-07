package itchio_test

import (
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
