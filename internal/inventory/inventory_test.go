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

func TestIsPresent_NoEntry(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	if inv.IsPresent("https://dev.itch.io/game") {
		t.Error("IsPresent should be false when no entry exists")
	}
}

func TestIsPresent_EntryWithFiles(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	if !inv.IsPresent("https://dev.itch.io/game") {
		t.Error("IsPresent should be true when entry has files")
	}
}

func TestVerifyAndClean_KeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	if err := os.WriteFile(romPath, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "inventory.json")

	inv := &inventory.Inventory{Entries: map[string]*inventory.Entry{
		"https://dev.itch.io/game": {
			GameURL: "https://dev.itch.io/game",
			Title:   "Game",
			Files:   []inventory.DownloadedFile{{Filename: "game.gb", DestPath: romPath}},
		},
	}}
	removed := inv.VerifyAndClean(path)
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if _, ok := inv.Lookup("https://dev.itch.io/game"); !ok {
		t.Error("entry should still exist")
	}
}

func TestVerifyAndClean_RemovesStaleFile(t *testing.T) {
	dir := t.TempDir()
	romPath := filepath.Join(dir, "real.gb")
	if err := os.WriteFile(romPath, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "inventory.json")

	inv := &inventory.Inventory{Entries: map[string]*inventory.Entry{
		"https://dev.itch.io/game": {
			GameURL: "https://dev.itch.io/game",
			Title:   "Game",
			Files: []inventory.DownloadedFile{
				{Filename: "real.gb", DestPath: romPath},
				{Filename: "gone.gb", DestPath: "/nonexistent/gone.gb"},
			},
		},
	}}
	removed := inv.VerifyAndClean(path)
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	e, ok := inv.Lookup("https://dev.itch.io/game")
	if !ok {
		t.Fatal("entry should still exist after partial file removal")
	}
	if len(e.Files) != 1 {
		t.Errorf("Files len = %d, want 1", len(e.Files))
	}
	if e.Files[0].Filename != "real.gb" {
		t.Errorf("kept wrong file: %q", e.Files[0].Filename)
	}
}

func TestVerifyAndClean_RemovesEmptyEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	inv := &inventory.Inventory{Entries: map[string]*inventory.Entry{
		"https://dev.itch.io/gone": {
			GameURL: "https://dev.itch.io/gone",
			Title:   "Gone",
			Files:   []inventory.DownloadedFile{{Filename: "gone.gb", DestPath: "/nonexistent/gone.gb"}},
		},
	}}
	removed := inv.VerifyAndClean(path)
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, ok := inv.Lookup("https://dev.itch.io/gone"); ok {
		t.Error("entry should have been removed")
	}
}

func TestRemoveFile_PartialRemoval(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "v1.gb", DestPath: "/v1.gb"})
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "v2.gbc", DestPath: "/v2.gbc"})

	allGone := inv.RemoveFile("https://dev.itch.io/game", "/v1.gb")
	if allGone {
		t.Error("allGone should be false when one file remains")
	}
	e, ok := inv.Lookup("https://dev.itch.io/game")
	if !ok {
		t.Fatal("entry should still exist")
	}
	if len(e.Files) != 1 {
		t.Fatalf("Files len = %d, want 1", len(e.Files))
	}
	if e.Files[0].Filename != "v2.gbc" {
		t.Errorf("wrong file kept: %q", e.Files[0].Filename)
	}
}

func TestRemoveFile_LastFileRemovesEntry(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{Filename: "game.gb", DestPath: "/game.gb"})

	allGone := inv.RemoveFile("https://dev.itch.io/game", "/game.gb")
	if !allGone {
		t.Error("allGone should be true when last file is removed")
	}
	if _, ok := inv.Lookup("https://dev.itch.io/game"); ok {
		t.Error("entry should have been removed from inventory")
	}
}

func TestRemoveFile_UnknownURL(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}

	allGone := inv.RemoveFile("https://nobody.itch.io/nothing", "/some.gb")
	if !allGone {
		t.Error("allGone should be true for unknown game URL")
	}
}

func TestCoverArtPath_WithPNGCover(t *testing.T) {
	got := inventory.CoverArtPath(
		"https://img.itch.zone/abc/cover.png",
		"/mnt/SDCARD/Roms/Game Boy (GB)/my-game.gb",
	)
	// Cover art is always stored as .png regardless of source format.
	want := "/mnt/SDCARD/Roms/Game Boy (GB)/.media/my-game.png"
	if got != want {
		t.Errorf("CoverArtPath = %q, want %q", got, want)
	}
}

func TestCoverArtPath_EmptyCoverURL(t *testing.T) {
	got := inventory.CoverArtPath("", "/roms/game.gb")
	if got != "" {
		t.Errorf("empty coverURL should return empty string, got %q", got)
	}
}

func TestCoverArtPath_NoExtensionInURL(t *testing.T) {
	got := inventory.CoverArtPath(
		"https://img.itch.zone/abc/coverimage",
		"/roms/game.gb",
	)
	// Always .png regardless of whether source URL has an extension.
	want := "/roms/.media/game.png"
	if got != want {
		t.Errorf("CoverArtPath = %q, want %q", got, want)
	}
}

func TestCoverArtPath_FullStemPreserved(t *testing.T) {
	// NextUI looks up cover art by the full ROM stem (including [v1.2]).
	got := inventory.CoverArtPath(
		"https://img.itch.zone/abc/cover.png",
		"/roms/Game Boy Color (GBC)/Kero Kero Cowboy [v1.2].gbc",
	)
	want := "/roms/Game Boy Color (GBC)/.media/Kero Kero Cowboy [v1.2].png"
	if got != want {
		t.Errorf("CoverArtPath = %q, want %q", got, want)
	}
}
