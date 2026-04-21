package itchio_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestSaveAndLoadGamesCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "games_cache.json")

	games := []itchio.Game{
		{Title: "Alpha", Author: "dev1", URL: "https://dev1.itch.io/alpha", IsFree: true},
		{Title: "Beta", Author: "dev2", URL: "https://dev2.itch.io/beta", Price: 4.99},
	}

	if err := itchio.SaveGamesCache(path, games); err != nil {
		t.Fatalf("SaveGamesCache: %v", err)
	}

	cache, err := itchio.LoadGamesCache(path)
	if err != nil {
		t.Fatalf("LoadGamesCache: %v", err)
	}
	if len(cache.Games) != 2 {
		t.Fatalf("got %d games, want 2", len(cache.Games))
	}
	if cache.Games[0].Title != "Alpha" {
		t.Errorf("Games[0].Title = %q, want %q", cache.Games[0].Title, "Alpha")
	}
	if cache.Games[1].Price != 4.99 {
		t.Errorf("Games[1].Price = %v, want 4.99", cache.Games[1].Price)
	}
	if cache.Meta.TotalGames != 2 {
		t.Errorf("Meta.TotalGames = %d, want 2", cache.Meta.TotalGames)
	}
	if cache.Meta.FetchedAt.IsZero() {
		t.Error("Meta.FetchedAt should not be zero")
	}
	if time.Since(cache.Meta.FetchedAt) > 5*time.Second {
		t.Error("Meta.FetchedAt should be recent")
	}
}

func TestLoadGamesCache_MissingFile(t *testing.T) {
	_, err := itchio.LoadGamesCache("/tmp/does-not-exist-xyz.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadGamesCache_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_cache.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := itchio.LoadGamesCache(path)
	if err == nil {
		t.Fatal("expected error for corrupt file, got nil")
	}
}

func TestSaveGamesCache_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "games_cache.json")

	// Save once, then overwrite — previous data should not leak.
	_ = itchio.SaveGamesCache(path, []itchio.Game{{Title: "Old"}})
	if err := itchio.SaveGamesCache(path, []itchio.Game{{Title: "New"}}); err != nil {
		t.Fatalf("second SaveGamesCache: %v", err)
	}
	cache, err := itchio.LoadGamesCache(path)
	if err != nil {
		t.Fatalf("LoadGamesCache after overwrite: %v", err)
	}
	if cache.Games[0].Title != "New" {
		t.Errorf("got %q, want %q", cache.Games[0].Title, "New")
	}
	// Temp file must not linger.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file .tmp should not exist after successful save")
	}
}
