package inventory_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
)

func TestLoad_MissingFile_ReturnsEmpty(t *testing.T) {
	inv, err := inventory.Load("/nonexistent/path/inventory.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(inv.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(inv.Entries))
	}
}

func TestLoad_CorruptFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}
	inv, err := inventory.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(inv.Entries) != 0 {
		t.Errorf("expected empty entries on corrupt file, got %d", len(inv.Entries))
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{
		GameURL:  "https://dev.itch.io/game",
		Title:    "My Game",
		Author:   "dev",
		CoverURL: "https://img.itch.zone/cover.png",
	}, inventory.DownloadedFile{
		Filename:     "my-game.gb",
		DestPath:     "/mnt/SDCARD/Roms/Game Boy (GB)/my-game.gb",
		DownloadedAt: time.Now().Truncate(time.Second),
	})

	if err := inv.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := inventory.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	e, ok := loaded.Lookup("https://dev.itch.io/game")
	if !ok {
		t.Fatal("expected entry, not found")
	}
	if e.Title != "My Game" {
		t.Errorf("Title = %q, want %q", e.Title, "My Game")
	}
	if len(e.Files) != 1 {
		t.Fatalf("Files len = %d, want 1", len(e.Files))
	}
	if e.Files[0].Filename != "my-game.gb" {
		t.Errorf("Filename = %q, want %q", e.Files[0].Filename, "my-game.gb")
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/a", inventory.Entry{Title: "Old"}, inventory.DownloadedFile{Filename: "a.gb", DestPath: "/a.gb"})
	_ = inv.Save(path)

	inv.Add("https://dev.itch.io/b", inventory.Entry{Title: "New"}, inventory.DownloadedFile{Filename: "b.gb", DestPath: "/b.gb"})
	if err := inv.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file should not linger after successful save")
	}
}

func TestAdd_NewEntry(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{
		Title: "Game", Author: "dev",
	}, inventory.DownloadedFile{Filename: "game.gb", DestPath: "/roms/game.gb"})

	e, ok := inv.Lookup("https://dev.itch.io/game")
	if !ok {
		t.Fatal("entry not found")
	}
	if e.Title != "Game" {
		t.Errorf("Title = %q, want %q", e.Title, "Game")
	}
	if len(e.Files) != 1 {
		t.Errorf("Files len = %d, want 1", len(e.Files))
	}
}

func TestAdd_ExistingEntry_AppendsFile(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "v1.gb", DestPath: "/v1.gb"})
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "v2.gbc", DestPath: "/v2.gbc"})

	e, _ := inv.Lookup("https://dev.itch.io/game")
	if len(e.Files) != 2 {
		t.Errorf("Files len = %d, want 2", len(e.Files))
	}
}

func TestAdd_DeduplicatesByDestPath(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "game.gb", DestPath: "/roms/game.gb"})
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "game.gb", DestPath: "/roms/game.gb"})

	e, _ := inv.Lookup("https://dev.itch.io/game")
	if len(e.Files) != 1 {
		t.Errorf("Files len = %d, want 1 (dedup)", len(e.Files))
	}
}

func TestRemove(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "game.gb", DestPath: "/roms/game.gb"})
	inv.Remove("https://dev.itch.io/game")

	if _, ok := inv.Lookup("https://dev.itch.io/game"); ok {
		t.Error("entry should have been removed")
	}
}

func TestLookup_Missing(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	_, ok := inv.Lookup("https://nobody.itch.io/nothing")
	if ok {
		t.Error("Lookup of missing key should return false")
	}
}
