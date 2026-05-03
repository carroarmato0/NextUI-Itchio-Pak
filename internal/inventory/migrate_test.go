package inventory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
)

func TestReadMigrateFormats_Defaults(t *testing.T) {
	f := inventory.ReadMigrateFormats("/nonexistent/path/minuisettings.txt")
	if f.SaveFormat != 0 || f.StateFormat != 0 || f.UseExtractedFileName {
		t.Errorf("defaults: got %+v", f)
	}
}

func TestReadMigrateFormats_AllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minuisettings.txt")
	content := "saveFormat=2\nstateFormat=3\nuseExtractedFileName=1\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	f := inventory.ReadMigrateFormats(path)
	if f.SaveFormat != 2 {
		t.Errorf("SaveFormat = %d, want 2", f.SaveFormat)
	}
	if f.StateFormat != 3 {
		t.Errorf("StateFormat = %d, want 3", f.StateFormat)
	}
	if !f.UseExtractedFileName {
		t.Error("UseExtractedFileName should be true")
	}
}

func TestReadMigrateFormats_PartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(path, []byte("saveFormat=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f := inventory.ReadMigrateFormats(path)
	if f.SaveFormat != 1 {
		t.Errorf("SaveFormat = %d, want 1", f.SaveFormat)
	}
	if f.StateFormat != 0 {
		t.Errorf("StateFormat = %d, want 0 (default)", f.StateFormat)
	}
}

// stubCallback is a test double for SaveDataCallback.
type stubCallback struct {
	askRename    bool
	askOverwrite bool
	askStates    bool
}

func (s stubCallback) AskRenameExistingSave(_ string) bool     { return s.askRename }
func (s stubCallback) AskOverwriteExistingSave(_ string) bool  { return s.askOverwrite }
func (s stubCallback) AskRenameExistingStates(_ []string) bool { return s.askStates }

func setupMigrateTest(t *testing.T) (dir string, inv *inventory.Inventory, invPath string) {
	t.Helper()
	dir = t.TempDir()
	invPath = filepath.Join(dir, "inventory.json")
	inv = &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	return
}

func addTestFile(t *testing.T, inv *inventory.Inventory, dir, gameURL, gameTitle, upstream, dest string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, dest), []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	inv.Add(gameURL, inventory.Entry{Title: gameTitle}, inventory.DownloadedFile{
		Filename: upstream, DestPath: filepath.Join(dir, dest),
	})
}

func TestMigrateFile_EnableUnifiedName_NoSave(t *testing.T) {
	dir, inv, invPath := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	addTestFile(t, inv, dir, gameURL, "Doomslinger Dungeon", "Game Boy ROM.gb", "Game Boy ROM.gb")

	e, _ := inv.Lookup(gameURL)
	f := e.Files[0]
	res, err := inventory.MigrateFile(inv, invPath, gameURL, f, "Doomslinger Dungeon", true,
		inventory.MigrateFormats{}, stubCallback{})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	if !res.ROMRenamed {
		t.Error("ROMRenamed should be true")
	}
	want := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if res.NewDestPath != want {
		t.Errorf("NewDestPath = %q, want %q", res.NewDestPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("renamed file not on disk: %v", err)
	}
	e2, _ := inv.Lookup(gameURL)
	if e2.Files[0].DestPath != want {
		t.Errorf("inventory DestPath = %q, want %q", e2.Files[0].DestPath, want)
	}
	if !e2.Files[0].UnifiedName {
		t.Error("inventory UnifiedName should be true")
	}
}

func TestMigrateFile_EnableUnifiedName_WithCoverArt(t *testing.T) {
	dir, inv, invPath := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	addTestFile(t, inv, dir, gameURL, "Doomslinger Dungeon", "Game Boy ROM.gb", "Game Boy ROM.gb")
	// Create cover art
	mediaDir := filepath.Join(dir, ".media")
	os.MkdirAll(mediaDir, 0755)
	coverPath := filepath.Join(mediaDir, "Game Boy ROM.png")
	os.WriteFile(coverPath, []byte("png"), 0644)
	// Update entry with cover URL (re-add with CoverURL set)
	inv.Add(gameURL, inventory.Entry{Title: "Doomslinger Dungeon", CoverURL: "https://img.itch.zone/c.png"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: filepath.Join(dir, "Game Boy ROM.gb"),
	})

	e, _ := inv.Lookup(gameURL)
	res, err := inventory.MigrateFile(inv, invPath, gameURL, e.Files[0], "Doomslinger Dungeon", true,
		inventory.MigrateFormats{}, stubCallback{})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	if !res.CoverArtRenamed {
		t.Error("CoverArtRenamed should be true")
	}
	newCover := filepath.Join(mediaDir, "Doomslinger Dungeon.png")
	if _, err := os.Stat(newCover); err != nil {
		t.Errorf("cover art not renamed: %v", err)
	}
}

func TestMigrateFile_EnableUnifiedName_NameAlreadyCorrect(t *testing.T) {
	dir, inv, invPath := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	addTestFile(t, inv, dir, gameURL, "Doomslinger Dungeon", "Game Boy ROM.gb", "Doomslinger Dungeon.gb")
	inv.UpdateFile(gameURL, filepath.Join(dir, "Doomslinger Dungeon.gb"), inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: filepath.Join(dir, "Doomslinger Dungeon.gb"), UnifiedName: false,
	})

	e, _ := inv.Lookup(gameURL)
	res, err := inventory.MigrateFile(inv, invPath, gameURL, e.Files[0], "Doomslinger Dungeon", true,
		inventory.MigrateFormats{}, stubCallback{})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	if res.ROMRenamed {
		t.Error("ROMRenamed should be false when name is already correct")
	}
	e2, _ := inv.Lookup(gameURL)
	if !e2.Files[0].UnifiedName {
		t.Error("UnifiedName should be set to true even when no rename needed")
	}
}

func TestMigrateFile_Enable_WithSave_UserRenames(t *testing.T) {
	dir, _, _ := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"

	gbDir := filepath.Join(dir, "Roms", "Game Boy (GB)")
	os.MkdirAll(gbDir, 0755)
	romPath := filepath.Join(gbDir, "Game Boy ROM.gb")
	os.WriteFile(romPath, []byte("rom"), 0644)
	savesGB := filepath.Join(dir, "Saves", "GB")
	os.MkdirAll(savesGB, 0755)
	saveFile := filepath.Join(savesGB, "Game Boy ROM.gb.sav")
	os.WriteFile(saveFile, []byte("save"), 0644)

	inv2 := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	invPath2 := filepath.Join(dir, "inv2.json")
	inv2.Add(gameURL, inventory.Entry{Title: "Doomslinger Dungeon"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: romPath,
	})

	e, _ := inv2.Lookup(gameURL)
	res, err := inventory.MigrateFile(inv2, invPath2, gameURL, e.Files[0], "Doomslinger Dungeon", true,
		inventory.MigrateFormats{SaveFormat: 0}, stubCallback{askRename: true})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	if !res.SaveRenamed {
		t.Error("SaveRenamed should be true")
	}
	newSave := filepath.Join(savesGB, "Doomslinger Dungeon.gb.sav")
	if _, err := os.Stat(newSave); err != nil {
		t.Errorf("save not renamed: %v", err)
	}
}

func TestMigrateFile_Enable_WithSave_UserSkips(t *testing.T) {
	dir, _, invPath := setupMigrateTest(t)
	gbDir := filepath.Join(dir, "Roms", "Game Boy (GB)")
	os.MkdirAll(gbDir, 0755)
	romPath := filepath.Join(gbDir, "Game Boy ROM.gb")
	os.WriteFile(romPath, []byte("rom"), 0644)
	savesGB := filepath.Join(dir, "Saves", "GB")
	os.MkdirAll(savesGB, 0755)
	saveFile := filepath.Join(savesGB, "Game Boy ROM.gb.sav")
	os.WriteFile(saveFile, []byte("save"), 0644)

	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	inv2 := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv2.Add(gameURL, inventory.Entry{Title: "D"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: romPath,
	})
	e, _ := inv2.Lookup(gameURL)
	res, err := inventory.MigrateFile(inv2, invPath, gameURL, e.Files[0], "Doomslinger Dungeon", true,
		inventory.MigrateFormats{}, stubCallback{askRename: false})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	if res.SaveRenamed {
		t.Error("SaveRenamed should be false when user skips")
	}
	if !res.SaveSkipped {
		t.Error("SaveSkipped should be true")
	}
	if _, err := os.Stat(saveFile); err != nil {
		t.Errorf("old save should remain: %v", err)
	}
	if !res.ROMRenamed {
		t.Error("ROM should still be renamed even when save skipped")
	}
}

func TestMigrateFile_Enable_OverwriteGuard_UserCancels(t *testing.T) {
	dir, _, invPath := setupMigrateTest(t)
	gbDir := filepath.Join(dir, "Roms", "Game Boy (GB)")
	os.MkdirAll(gbDir, 0755)
	romPath := filepath.Join(gbDir, "Game Boy ROM.gb")
	os.WriteFile(romPath, []byte("rom"), 0644)
	savesGB := filepath.Join(dir, "Saves", "GB")
	os.MkdirAll(savesGB, 0755)
	// Save exists at BOTH old and new paths
	os.WriteFile(filepath.Join(savesGB, "Game Boy ROM.gb.sav"), []byte("save"), 0644)
	os.WriteFile(filepath.Join(savesGB, "Doomslinger Dungeon.gb.sav"), []byte("other"), 0644)

	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	inv2 := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv2.Add(gameURL, inventory.Entry{Title: "D"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: romPath,
	})
	e, _ := inv2.Lookup(gameURL)
	_, err := inventory.MigrateFile(inv2, invPath, gameURL, e.Files[0], "Doomslinger Dungeon", true,
		inventory.MigrateFormats{}, stubCallback{askRename: true, askOverwrite: false})
	if err == nil {
		t.Error("expected error when overwrite cancelled")
	}
	// ROM should NOT have been renamed
	if _, statErr := os.Stat(romPath); statErr != nil {
		t.Error("ROM should remain at original path when overwrite cancelled")
	}
}

func TestMigrateFile_Disable_NoSave(t *testing.T) {
	dir, inv, invPath := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	renamedPath := filepath.Join(dir, "Doomslinger Dungeon.gb")
	os.WriteFile(renamedPath, []byte("rom"), 0644)
	inv.Add(gameURL, inventory.Entry{Title: "D"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: renamedPath, UnifiedName: true,
	})

	e, _ := inv.Lookup(gameURL)
	res, err := inventory.MigrateFile(inv, invPath, gameURL, e.Files[0], "Doomslinger Dungeon", false,
		inventory.MigrateFormats{}, stubCallback{})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	if !res.ROMRenamed {
		t.Error("ROMRenamed should be true when disabling")
	}
	want := filepath.Join(dir, "Game Boy ROM.gb")
	if res.NewDestPath != want {
		t.Errorf("NewDestPath = %q, want %q", res.NewDestPath, want)
	}
	e2, _ := inv.Lookup(gameURL)
	if e2.Files[0].UnifiedName {
		t.Error("UnifiedName should be false after disable")
	}
}

func TestMigrateFile_Disable_AlreadyDisabled_NoOp(t *testing.T) {
	dir, inv, invPath := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	romPath := filepath.Join(dir, "Game Boy ROM.gb")
	os.WriteFile(romPath, []byte("rom"), 0644)
	inv.Add(gameURL, inventory.Entry{Title: "D"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: romPath, UnifiedName: false,
	})

	e, _ := inv.Lookup(gameURL)
	res, err := inventory.MigrateFile(inv, invPath, gameURL, e.Files[0], "Doomslinger Dungeon", false,
		inventory.MigrateFormats{}, stubCallback{})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	if res.ROMRenamed {
		t.Error("ROMRenamed should be false for no-op")
	}
}

func TestMigrateFile_Enable_CollisionGuard(t *testing.T) {
	dir, inv, invPath := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	romPath := filepath.Join(dir, "Game Boy ROM.gb")
	os.WriteFile(romPath, []byte("rom"), 0644)
	// Pre-create the target name so collision triggers
	os.WriteFile(filepath.Join(dir, "Doomslinger Dungeon.gb"), []byte("other"), 0644)
	inv.Add(gameURL, inventory.Entry{Title: "D"}, inventory.DownloadedFile{
		Filename: "Game Boy ROM.gb", DestPath: romPath,
	})

	e, _ := inv.Lookup(gameURL)
	res, err := inventory.MigrateFile(inv, invPath, gameURL, e.Files[0], "Doomslinger Dungeon", true,
		inventory.MigrateFormats{}, stubCallback{})
	if err != nil {
		t.Fatalf("MigrateFile: %v", err)
	}
	want := filepath.Join(dir, "Doomslinger Dungeon (2).gb")
	if res.NewDestPath != want {
		t.Errorf("NewDestPath = %q, want %q", res.NewDestPath, want)
	}
}
