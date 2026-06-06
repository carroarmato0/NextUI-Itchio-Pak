package inventory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
)

func TestMigratePico8Files_movesROMAndCoverArt(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "Pico-8 (P8)") + "/"
	newDir := filepath.Join(base, "Pico-8 (PICO)") + "/"

	if err := os.MkdirAll(filepath.Join(base, "Pico-8 (P8)", ".media"), 0755); err != nil {
		t.Fatal(err)
	}
	romPath := filepath.Join(base, "Pico-8 (P8)", "game.p8.png")
	artPath := filepath.Join(base, "Pico-8 (P8)", ".media", "game.p8.png")
	if err := os.WriteFile(romPath, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artPath, []byte("art"), 0644); err != nil {
		t.Fatal(err)
	}

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game", CoverURL: "https://img.itch.zone/cover.jpg"}, inventory.DownloadedFile{
		Filename:     "game.p8.png",
		DestPath:     romPath,
		DownloadedAt: time.Now(),
		FileType:     inventory.FileTypeROM,
	})

	invPath := filepath.Join(base, "inventory.json")
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	if err := inventory.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files: %v", err)
	}

	newROMPath := filepath.Join(base, "Pico-8 (PICO)", "game.p8.png")
	if _, err := os.Stat(newROMPath); err != nil {
		t.Errorf("ROM not found at new path: %v", err)
	}
	if _, err := os.Stat(romPath); !os.IsNotExist(err) {
		t.Errorf("ROM still exists at old path")
	}

	newArtPath := filepath.Join(base, "Pico-8 (PICO)", ".media", "game.p8.png")
	if _, err := os.Stat(newArtPath); err != nil {
		t.Errorf("cover art not found at new path: %v", err)
	}

	entry, ok := inv.Lookup("https://dev.itch.io/game")
	if !ok || len(entry.Files) == 0 {
		t.Fatal("inventory entry missing after migration")
	}
	if entry.Files[0].DestPath != newROMPath {
		t.Errorf("inventory DestPath = %q, want %q", entry.Files[0].DestPath, newROMPath)
	}
}

func TestMigratePico8Files_handlesSubdirectory(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "Pico-8 (P8)") + "/"
	newDir := filepath.Join(base, "Pico-8 (PICO)") + "/"

	// Set up: game subdirectory with two .p8 files, an untracked .m3u launcher,
	// and directory-level cover art in the parent's .media/.
	gameSubDir := filepath.Join(base, "Pico-8 (P8)", "Poom")
	if err := os.MkdirAll(gameSubDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "Pico-8 (P8)", ".media"), 0755); err != nil {
		t.Fatal(err)
	}
	file1 := filepath.Join(gameSubDir, "poom_0.p8")
	file2 := filepath.Join(gameSubDir, "poom_1.p8")
	m3uFile := filepath.Join(gameSubDir, "Poom.m3u")
	dirArt := filepath.Join(base, "Pico-8 (P8)", ".media", "Poom.png")
	for _, f := range []string{file1, file2, m3uFile, dirArt} {
		if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	for i, path := range []string{file1, file2} {
		inv.Add("https://dev.itch.io/poom", inventory.Entry{Title: "Poom"}, inventory.DownloadedFile{
			Filename:     filepath.Base(path),
			DestPath:     path,
			DownloadedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	invPath := filepath.Join(base, "inventory.json")
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	if err := inventory.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files: %v", err)
	}

	// All tracked .p8 files must be at new paths.
	for _, name := range []string{"poom_0.p8", "poom_1.p8"} {
		newPath := filepath.Join(base, "Pico-8 (PICO)", "Poom", name)
		if _, err := os.Stat(newPath); err != nil {
			t.Errorf("file %s not at new path: %v", name, err)
		}
	}
	// Untracked .m3u must also have moved (whole-directory rename).
	newM3u := filepath.Join(base, "Pico-8 (PICO)", "Poom", "Poom.m3u")
	if _, err := os.Stat(newM3u); err != nil {
		t.Errorf("untracked .m3u not moved to new location: %v", err)
	}
	// Old subdirectory must be gone.
	if _, err := os.Stat(gameSubDir); !os.IsNotExist(err) {
		t.Errorf("old subdir still exists after migration")
	}
	// Directory-level cover art must be at new parent .media/.
	newArt := filepath.Join(base, "Pico-8 (PICO)", ".media", "Poom.png")
	if _, err := os.Stat(newArt); err != nil {
		t.Errorf("directory cover art not moved: %v", err)
	}
	// Old cover art must be gone.
	if _, err := os.Stat(dirArt); !os.IsNotExist(err) {
		t.Errorf("old directory cover art still exists")
	}
	// Inventory DestPaths must point to the new locations.
	entry, ok := inv.Lookup("https://dev.itch.io/poom")
	if !ok || len(entry.Files) == 0 {
		t.Fatal("inventory entry missing after migration")
	}
	for _, f := range entry.Files {
		if !strings.HasPrefix(f.DestPath, newDir) {
			t.Errorf("inventory DestPath %q still points to old dir", f.DestPath)
		}
	}
}

func TestMigratePico8Files_skipsNonPico8Files(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "Pico-8 (P8)") + "/"
	newDir := filepath.Join(base, "Pico-8 (PICO)") + "/"

	gbcDir := filepath.Join(base, "GBC")
	if err := os.MkdirAll(gbcDir, 0755); err != nil {
		t.Fatal(err)
	}
	gbcROM := filepath.Join(gbcDir, "game.gbc")
	if err := os.WriteFile(gbcROM, []byte("gbc"), 0644); err != nil {
		t.Fatal(err)
	}

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/gbgame", inventory.Entry{Title: "GBC Game"}, inventory.DownloadedFile{
		Filename: "game.gbc",
		DestPath: gbcROM,
	})

	invPath := filepath.Join(base, "inventory.json")
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	if err := inventory.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files: %v", err)
	}

	if _, err := os.Stat(gbcROM); err != nil {
		t.Errorf("GBC ROM unexpectedly gone: %v", err)
	}
	entry, _ := inv.Lookup("https://dev.itch.io/gbgame")
	if entry.Files[0].DestPath != gbcROM {
		t.Errorf("GBC inventory DestPath changed to %q", entry.Files[0].DestPath)
	}
}

func TestMigratePico8Files_partialFailureContinues(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "Pico-8 (P8)") + "/"
	newDir := filepath.Join(base, "Pico-8 (PICO)") + "/"

	if err := os.MkdirAll(filepath.Join(base, "Pico-8 (P8)"), 0755); err != nil {
		t.Fatal(err)
	}

	goodROM := filepath.Join(base, "Pico-8 (P8)", "good.p8.png")
	missingROM := filepath.Join(base, "Pico-8 (P8)", "missing.p8.png")

	if err := os.WriteFile(goodROM, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/good", inventory.Entry{Title: "Good"}, inventory.DownloadedFile{
		Filename: "good.p8.png",
		DestPath: goodROM,
	})
	inv.Add("https://dev.itch.io/missing", inventory.Entry{Title: "Missing"}, inventory.DownloadedFile{
		Filename: "missing.p8.png",
		DestPath: missingROM,
	})

	invPath := filepath.Join(base, "inventory.json")
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	if err := inventory.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files returned error on partial failure: %v", err)
	}

	newGoodPath := filepath.Join(base, "Pico-8 (PICO)", "good.p8.png")
	if _, err := os.Stat(newGoodPath); err != nil {
		t.Errorf("good ROM not moved: %v", err)
	}
	goodEntry, _ := inv.Lookup("https://dev.itch.io/good")
	if goodEntry.Files[0].DestPath != newGoodPath {
		t.Errorf("good inventory DestPath = %q, want %q", goodEntry.Files[0].DestPath, newGoodPath)
	}

	missingEntry, _ := inv.Lookup("https://dev.itch.io/missing")
	if missingEntry.Files[0].DestPath != missingROM {
		t.Errorf("missing inventory DestPath changed to %q", missingEntry.Files[0].DestPath)
	}
}
