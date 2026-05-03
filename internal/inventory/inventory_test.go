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

// ── HasPendingUpdates ────────────────────────────────────────────────────────

func TestHasPendingUpdates_NoEntry(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false for missing entry")
	}
}

func TestHasPendingUpdates_NoUpstreamFiles(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false when no upstream files recorded")
	}
}

func TestHasPendingUpdates_AllDownloaded(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.SetUpstreamFiles("https://dev.itch.io/game",
		[]inventory.UpstreamFile{{Filename: "g.gb", UploadID: "1", SeenAt: time.Now()}})
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false when all upstream files are downloaded")
	}
}

func TestHasPendingUpdates_NewFile(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g.gb", UploadID: "1", SeenAt: time.Now()},
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: time.Now()},
	})
	if !inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want true when upstream has file not in downloaded set")
	}
}

func TestHasPendingUpdates_DismissedFile(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	seenAt := time.Now().Add(-time.Hour)
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g.gb", UploadID: "1", SeenAt: seenAt},
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: seenAt},
	})
	inv.DismissUpdate("https://dev.itch.io/game")
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false after dismiss")
	}
}

func TestHasPendingUpdates_NewFileAfterDismiss(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	oldSeenAt := time.Now().Add(-time.Hour)
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: oldSeenAt},
	})
	inv.DismissUpdate("https://dev.itch.io/game")

	newSeenAt := time.Now().Add(time.Hour)
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: oldSeenAt},
		{Filename: "g-v3.gb", UploadID: "3", SeenAt: newSeenAt},
	})
	if !inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want true when new file appears after dismiss")
	}
}

func TestHasPendingUpdates_FormatPickerExtensionMismatch(t *testing.T) {
	// Format-picker appends ".gbc" to an upload that has no extension.
	// Stored Filename = "Game Boy ROM.gbc", upstream Filename = "Game Boy ROM".
	// The downloaded file must not be treated as a new upstream entry.
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "Game Boy ROM.gbc", DestPath: "/roms/Game Boy ROM.gbc"})
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "Game Boy ROM", UploadID: "1", SeenAt: time.Now()},
	})
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false — 'Game Boy ROM' is the already-downloaded file, extension just appended by format-picker")
	}
}

func TestHasPendingUpdates_FormatPickerWithGenuineNewFile(t *testing.T) {
	// Same format-picker scenario, but there is also a genuinely new upload.
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "Game Boy ROM.gbc", DestPath: "/roms/Game Boy ROM.gbc"})
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "Game Boy ROM", UploadID: "1", SeenAt: time.Now()},
		{Filename: "Analogue Pocket ROM", UploadID: "2", SeenAt: time.Now()},
	})
	if !inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want true — 'Analogue Pocket ROM' is a genuinely new upload")
	}
}

// ── IsRemoved ────────────────────────────────────────────────────────────────

func TestIsRemoved_NoEntry(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false for missing entry")
	}
}

func TestIsRemoved_NotRemoved(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false when GameRemovedAt is zero")
	}
}

func TestIsRemoved_Removed(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	if !inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want true after MarkRemoved")
	}
}

func TestIsRemoved_DismissedRemoval(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	inv.DismissRemoval("https://dev.itch.io/game")
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false after DismissRemoval")
	}
}

func TestIsRemoved_MarkRemovedIdempotent(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	inv.DismissRemoval("https://dev.itch.io/game")
	inv.MarkRemoved("https://dev.itch.io/game")
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: MarkRemoved must be idempotent; badge must stay suppressed after dismiss")
	}
}

func TestMarkRemovedThenReachable_ClearsBoth(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	inv.MarkReachable("https://dev.itch.io/game")
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false after MarkReachable")
	}
	inv.MarkRemoved("https://dev.itch.io/game")
	if !inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want true after fresh MarkRemoved post-MarkReachable")
	}
}

// ── IsFree persistence ───────────────────────────────────────────────────────

func TestAdd_PreservesIsFree(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game",
		inventory.Entry{Title: "G", IsFree: true},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	e, ok := inv.Lookup("https://dev.itch.io/game")
	if !ok {
		t.Fatal("entry not found")
	}
	if !e.IsFree {
		t.Error("IsFree should be true after Add with IsFree:true")
	}
}

func TestDownloadedFile_UnifiedName_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"}, inventory.DownloadedFile{
		Filename:    "Game Boy ROM.gb",
		DestPath:    "/roms/Doomslinger Dungeon.gb",
		UnifiedName: true,
	})
	if err := inv.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, _ := inventory.Load(path)
	e, _ := loaded.Lookup("https://dev.itch.io/game")
	if !e.Files[0].UnifiedName {
		t.Error("UnifiedName should survive round-trip")
	}
}

func TestEntry_UnifiedNamingDisabled_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"}, inventory.DownloadedFile{
		Filename: "g.gb", DestPath: "/g.gb",
	})
	inv.SetUnifiedNamingDisabled("https://dev.itch.io/game", true)
	if err := inv.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, _ := inventory.Load(path)
	e, _ := loaded.Lookup("https://dev.itch.io/game")
	if !e.UnifiedNamingDisabled {
		t.Error("UnifiedNamingDisabled should survive round-trip")
	}
}

func TestUpdateFile_UpdatesDestPathAndUnifiedName(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: "/roms/Game Boy ROM.gb",
	})

	ok := inv.UpdateFile("https://dev.itch.io/game", "/roms/Game Boy ROM.gb", inventory.DownloadedFile{
		Filename:    "Game Boy ROM.gb",
		DestPath:    "/roms/Doomslinger Dungeon.gb",
		UnifiedName: true,
	})
	if !ok {
		t.Fatal("UpdateFile returned false")
	}
	e, _ := inv.Lookup("https://dev.itch.io/game")
	if e.Files[0].DestPath != "/roms/Doomslinger Dungeon.gb" {
		t.Errorf("DestPath = %q", e.Files[0].DestPath)
	}
	if !e.Files[0].UnifiedName {
		t.Error("UnifiedName should be true")
	}
}

func TestUpdateFile_UnknownDestPath_ReturnsFalse(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"}, inventory.DownloadedFile{
		Filename: "g.gb", DestPath: "/g.gb",
	})
	ok := inv.UpdateFile("https://dev.itch.io/game", "/nonexistent.gb", inventory.DownloadedFile{})
	if ok {
		t.Error("UpdateFile should return false for unknown dest path")
	}
}
