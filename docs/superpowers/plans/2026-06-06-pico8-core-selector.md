# Pico-8 Core Selector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a settings toggle that switches Pico-8 downloads between FakeO8 (`Pico-8 (P8)/`) and the official Pico-8 Pak (`Pico-8 (PICO)/`), instantly migrating existing downloads via `os.Rename`.

**Architecture:** A new `Pico8Core` string field in `Config` drives a core-aware `Pico8ROMDir(core)` helper in the `roms` package. All `DestinationDir` call sites in the UI layer pass `cfg.Pico8Core`. On change, `roms.MigratePico8Files` walks the inventory, renames each ROM and its cover art, updates `DestPath` entries, and saves the inventory before the config is written.

**Tech Stack:** Go 1.22, `encoding/json`, `os.Rename`, existing `internal/inventory`, `internal/roms`, `internal/settings`, `internal/ui` packages.

---

## File Map

| Action | File | Purpose |
|--------|------|---------|
| Modify | `internal/settings/settings.go` | Add `Pico8Core` field + update `defaults()` |
| Modify | `internal/settings/settings_test.go` | Tests for new field |
| Modify | `internal/inventory/inventory.go` | Add `AllURLs() []string` method |
| Modify | `internal/roms/roms.go` | Replace `Pico8Dir` const + `Pico8GameDir` with `Pico8ROMDir(core)` + `Pico8GameSubDir(core, title)`; update `DestinationDir(ext, core)` signature |
| Modify | `internal/roms/roms_test.go` | Update existing test + add new tests |
| Create | `internal/roms/migrate.go` | `MigratePico8Files` function |
| Create | `internal/roms/migrate_test.go` | Migration tests |
| Modify | `internal/ui/screen_settings.go` | New picker item, `inv`/`invPath` fields, migration on change, status line |
| Modify | `internal/ui/screen_list.go` | Pass `inv`/`invPath` to `NewSettingsScreen` |
| Modify | `internal/ui/screen_detail.go` | Pass `inv`/`invPath` to `NewSettingsScreen` |
| Modify | `internal/ui/dev_start.go` | Pass `inv`/`invPath` to `NewSettingsScreen` |
| Modify | `internal/ui/screen_autodetect.go` | Pass `s.cfg.Pico8Core` to `DestinationDir` |
| Modify | `internal/ui/screen_fetch_uploads.go` | Pass `s.cfg.Pico8Core` to `DestinationDir` (3 sites) |
| Modify | `internal/ui/screen_format_picker.go` | Pass `s.cfg.Pico8Core` to `DestinationDir` |
| Modify | `internal/ui/screen_location_picker.go` | Pass `cfg.Pico8Core` to `DestinationDir` |
| Modify | `internal/ui/screen_purchase_picker.go` | Pass `s.cfg.Pico8Core` to `DestinationDir` |
| Modify | `internal/ui/screen_rom_picker.go` | Pass `s.cfg.Pico8Core` to `DestinationDir` |
| Modify | `internal/ui/screen_zip_inspect.go` | Pass `s.cfg.Pico8Core` to `DestinationDir` (2 sites) + replace `Pico8GameDir` |
| Modify | `internal/ui/screen_zip_download.go` | Pass `s.cfg.Pico8Core` to `DestinationDir` |

---

## Task 1: Add `Pico8Core` to settings

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/settings/settings_test.go`:

```go
func TestPico8CoreDefault(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Pico8Core != "fakeo8" {
		t.Errorf("default Pico8Core = %q, want %q", cfg.Pico8Core, "fakeo8")
	}
}

func TestPico8CoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{Pico8Core: "pico8"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Pico8Core != "pico8" {
		t.Errorf("Pico8Core = %q, want %q", loaded.Pico8Core, "pico8")
	}
}

func TestPico8CoreOldConfigDefaultsFakeo8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Old config JSON without pico8_core field.
	if err := os.WriteFile(path, []byte(`{"unified_naming":true}`), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Pico8Core != "fakeo8" {
		t.Errorf("old config Pico8Core = %q, want %q", loaded.Pico8Core, "fakeo8")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/settings/ -run "TestPico8Core" -v
```

Expected: FAIL — `cfg.Pico8Core` is undefined.

- [ ] **Step 3: Add `Pico8Core` field and update defaults**

In `internal/settings/settings.go`, add the field to `Config`:

```go
Pico8Core      string            `json:"pico8_core,omitempty"`  // "fakeo8" | "pico8"
```

Place it after `MusicLocation`:

```go
	MusicDownload string            `json:"music_download,omitempty"` // "auto" | "ask" | "off"
	MusicLocation string            `json:"music_location,omitempty"` // "auto" | "ask"
	Pico8Core     string            `json:"pico8_core,omitempty"`     // "fakeo8" | "pico8"
```

Update `defaults()` to set it:

```go
func defaults() *Config {
	return &Config{
		APIKey:        "",
		ROMLocation:   "auto",
		UnifiedNaming: true,
		MusicDownload: "off",
		MusicLocation: "auto",
		Pico8Core:     "fakeo8",
		Filter: ContentFilter{
			AdultContent: CategoryFilter{Enabled: true},
			HeavyThemes:  CategoryFilter{Enabled: true},
			SubstanceUse: CategoryFilter{Enabled: true},
		},
	}
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
go test ./internal/settings/ -run "TestPico8Core" -v
```

Expected: PASS for all three new tests.

- [ ] **Step 5: Run full settings test suite**

```
go test ./internal/settings/ -v
```

Expected: all existing tests still pass.

- [ ] **Step 6: Commit**

```
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat: add Pico8Core setting (fakeo8|pico8, default fakeo8)"
```

---

## Task 2: Add `AllURLs()` to inventory

**Files:**
- Modify: `internal/inventory/inventory.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/inventory/inventory_test.go` (or create it if it only has the existing test file):

```go
func TestAllURLs(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Add("https://a.itch.io/game-a", inventory.Entry{Title: "A"}, inventory.DownloadedFile{Filename: "a.p8", DestPath: "/roms/a.p8"})
	inv.Add("https://b.itch.io/game-b", inventory.Entry{Title: "B"}, inventory.DownloadedFile{Filename: "b.p8", DestPath: "/roms/b.p8"})

	urls := inv.AllURLs()
	if len(urls) != 2 {
		t.Fatalf("AllURLs() len = %d, want 2", len(urls))
	}
	urlSet := map[string]bool{}
	for _, u := range urls {
		urlSet[u] = true
	}
	if !urlSet["https://a.itch.io/game-a"] {
		t.Error("expected https://a.itch.io/game-a in AllURLs()")
	}
	if !urlSet["https://b.itch.io/game-b"] {
		t.Error("expected https://b.itch.io/game-b in AllURLs()")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```
go test ./internal/inventory/ -run "TestAllURLs" -v
```

Expected: FAIL — `AllURLs` undefined.

- [ ] **Step 3: Implement `AllURLs`**

Add to `internal/inventory/inventory.go` after `LatestCheckedAt`:

```go
// AllURLs returns the game URLs of all inventory entries.
func (inv *Inventory) AllURLs() []string {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	urls := make([]string, 0, len(inv.Entries))
	for url := range inv.Entries {
		urls = append(urls, url)
	}
	return urls
}
```

- [ ] **Step 4: Run test to confirm it passes**

```
go test ./internal/inventory/ -run "TestAllURLs" -v
```

Expected: PASS.

- [ ] **Step 5: Run full inventory test suite**

```
go test ./internal/inventory/ -v
```

Expected: all existing tests still pass.

- [ ] **Step 6: Commit**

```
git add internal/inventory/inventory.go internal/inventory/inventory_test.go
git commit -m "feat: add AllURLs() to inventory for bulk iteration"
```

---

## Task 3: Replace `Pico8Dir` / `Pico8GameDir` with core-aware functions in `roms`

**Files:**
- Modify: `internal/roms/roms.go`
- Modify: `internal/roms/roms_test.go`

- [ ] **Step 1: Write failing tests**

In `internal/roms/roms_test.go`, add:

```go
func TestPico8ROMDir(t *testing.T) {
	cases := []struct {
		core string
		want string
	}{
		{"fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{"pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
		{"", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},        // unknown → default
		{"other", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},   // unknown → default
	}
	for _, tc := range cases {
		got := roms.Pico8ROMDir(tc.core)
		if got != tc.want {
			t.Errorf("Pico8ROMDir(%q) = %q, want %q", tc.core, got, tc.want)
		}
	}
}

func TestPico8GameSubDir(t *testing.T) {
	cases := []struct {
		core  string
		title string
		want  string
	}{
		{"fakeo8", "Poom", "/mnt/SDCARD/Roms/Pico-8 (P8)/Poom/"},
		{"pico8", "Poom", "/mnt/SDCARD/Roms/Pico-8 (PICO)/Poom/"},
		{"fakeo8", "Celeste", "/mnt/SDCARD/Roms/Pico-8 (P8)/Celeste/"},
	}
	for _, tc := range cases {
		got := roms.Pico8GameSubDir(tc.core, tc.title)
		if got != tc.want {
			t.Errorf("Pico8GameSubDir(%q, %q) = %q, want %q", tc.core, tc.title, got, tc.want)
		}
	}
}
```

Also update the existing `TestDestinationDir` to use the new signature (it will fail to compile until the implementation is done — that's fine, the compile error IS the failure):

```go
func TestDestinationDir(t *testing.T) {
	tests := []struct {
		ext  string
		core string
		want string
	}{
		{".gbc", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".GBC", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".gb", "fakeo8", "/mnt/SDCARD/Roms/Game Boy (GB)/"},
		{".gba", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"},
		{".nes", "fakeo8", "/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/"},
		{".md", "fakeo8", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".gen", "fakeo8", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".smd", "fakeo8", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".p8", "fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".P8", "fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".p8.png", "fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".p8", "pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
		{".p8.png", "pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
		{".zip", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".unknown", "fakeo8", ""},
	}
	for _, tt := range tests {
		got := roms.DestinationDir(tt.ext, tt.core)
		if got != tt.want {
			t.Errorf("DestinationDir(%q, %q) = %q, want %q", tt.ext, tt.core, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/roms/ -run "TestPico8ROMDir|TestPico8GameSubDir|TestDestinationDir" -v
```

Expected: build/FAIL — `Pico8ROMDir`, `Pico8GameSubDir` undefined; `DestinationDir` wrong arg count.

- [ ] **Step 3: Update `internal/roms/roms.go`**

Replace:

```go
// Pico8Dir is the NextUI Pico-8 ROM directory.
const Pico8Dir = "/mnt/SDCARD/Roms/Pico-8 (P8)/"

func DestinationDir(ext string) string {
	switch strings.ToLower(ext) {
	case ".gbc":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	case ".gb":
		return "/mnt/SDCARD/Roms/Game Boy (GB)/"
	case ".gba":
		return GBADir
	case ".nes":
		return NESDir
	case ".md", ".gen", ".smd":
		return GenesisDir
	case ".p8", ".p8.png":
		return Pico8Dir
	case ".zip":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	default:
		return ""
	}
}
```

With:

```go
// Pico8ROMDir returns the Pico-8 ROM directory for the given core.
// core: "fakeo8" | "pico8" — any other value falls back to "fakeo8".
func Pico8ROMDir(core string) string {
	if core == "pico8" {
		return "/mnt/SDCARD/Roms/Pico-8 (PICO)/"
	}
	return "/mnt/SDCARD/Roms/Pico-8 (P8)/"
}

func DestinationDir(ext, pico8Core string) string {
	switch strings.ToLower(ext) {
	case ".gbc":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	case ".gb":
		return "/mnt/SDCARD/Roms/Game Boy (GB)/"
	case ".gba":
		return GBADir
	case ".nes":
		return NESDir
	case ".md", ".gen", ".smd":
		return GenesisDir
	case ".p8", ".p8.png":
		return Pico8ROMDir(pico8Core)
	case ".zip":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	default:
		return ""
	}
}
```

Replace:

```go
// Pico8GameDir returns the subdirectory for a Pico-8 game that ships with
// multiple files (.p8/.p8.png/.lua). All game files are extracted here.
func Pico8GameDir(gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return Pico8Dir + safe + "/"
}
```

With:

```go
// Pico8GameSubDir returns the subdirectory for a Pico-8 game that ships with
// multiple files (.p8/.p8.png/.lua). All game files are extracted here.
func Pico8GameSubDir(core, gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return Pico8ROMDir(core) + safe + "/"
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
go test ./internal/roms/ -run "TestPico8ROMDir|TestPico8GameSubDir|TestDestinationDir" -v
```

Expected: PASS.

- [ ] **Step 5: Run full roms test suite**

```
go test ./internal/roms/ -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```
git add internal/roms/roms.go internal/roms/roms_test.go
git commit -m "feat: replace Pico8Dir const with core-aware Pico8ROMDir and Pico8GameSubDir"
```

---

## Task 4: Implement `MigratePico8Files`

**Files:**
- Create: `internal/roms/migrate.go`
- Create: `internal/roms/migrate_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/roms/migrate_test.go`:

```go
package roms_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestMigratePico8Files_movesROMAndCoverArt(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "Pico-8 (P8)") + "/"
	newDir := filepath.Join(base, "Pico-8 (PICO)") + "/"

	// Create ROM file and cover art in old dir.
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

	inv := &inventory.Inventory{}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "Game"}, inventory.DownloadedFile{
		Filename:     "game.p8.png",
		DestPath:     romPath,
		DownloadedAt: time.Now(),
		FileType:     inventory.FileTypeROM,
	})

	invPath := filepath.Join(base, "inventory.json")
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	if err := roms.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files: %v", err)
	}

	// ROM must be at new path.
	newROMPath := filepath.Join(base, "Pico-8 (PICO)", "game.p8.png")
	if _, err := os.Stat(newROMPath); err != nil {
		t.Errorf("ROM not found at new path %s: %v", newROMPath, err)
	}
	// ROM must not exist at old path.
	if _, err := os.Stat(romPath); !os.IsNotExist(err) {
		t.Errorf("ROM still exists at old path %s", romPath)
	}

	// Cover art must be at new path.
	newArtPath := filepath.Join(base, "Pico-8 (PICO)", ".media", "game.p8.png")
	if _, err := os.Stat(newArtPath); err != nil {
		t.Errorf("cover art not found at new path %s: %v", newArtPath, err)
	}

	// Inventory DestPath must be updated.
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

	// Multi-file game in subdirectory.
	gameSubDir := filepath.Join(base, "Pico-8 (P8)", "Poom")
	if err := os.MkdirAll(gameSubDir, 0755); err != nil {
		t.Fatal(err)
	}
	file1 := filepath.Join(gameSubDir, "poom_0.p8")
	file2 := filepath.Join(gameSubDir, "poom_1.p8")
	for _, f := range []string{file1, file2} {
		if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	inv := &inventory.Inventory{}
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

	if err := roms.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files: %v", err)
	}

	for _, name := range []string{"poom_0.p8", "poom_1.p8"} {
		newPath := filepath.Join(base, "Pico-8 (PICO)", "Poom", name)
		if _, err := os.Stat(newPath); err != nil {
			t.Errorf("file %s not at new path: %v", name, err)
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

	inv := &inventory.Inventory{}
	inv.Add("https://dev.itch.io/gbgame", inventory.Entry{Title: "GBC Game"}, inventory.DownloadedFile{
		Filename: "game.gbc",
		DestPath: gbcROM,
	})

	invPath := filepath.Join(base, "inventory.json")
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	if err := roms.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files: %v", err)
	}

	// GBC file must be untouched.
	if _, err := os.Stat(gbcROM); err != nil {
		t.Errorf("GBC ROM unexpectedly gone: %v", err)
	}
	entry, _ := inv.Lookup("https://dev.itch.io/gbgame")
	if entry.Files[0].DestPath != gbcROM {
		t.Errorf("GBC inventory DestPath changed unexpectedly to %q", entry.Files[0].DestPath)
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
	missingROM := filepath.Join(base, "Pico-8 (P8)", "missing.p8.png") // not created on disk

	if err := os.WriteFile(goodROM, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}

	inv := &inventory.Inventory{}
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

	// MigratePico8Files must not return error even though one file doesn't exist.
	if err := roms.MigratePico8Files(inv, invPath, oldDir, newDir); err != nil {
		t.Fatalf("MigratePico8Files returned error on partial failure: %v", err)
	}

	// Good game must have moved.
	newGoodPath := filepath.Join(base, "Pico-8 (PICO)", "good.p8.png")
	if _, err := os.Stat(newGoodPath); err != nil {
		t.Errorf("good ROM not moved: %v", err)
	}
	goodEntry, _ := inv.Lookup("https://dev.itch.io/good")
	if goodEntry.Files[0].DestPath != newGoodPath {
		t.Errorf("good inventory DestPath = %q, want %q", goodEntry.Files[0].DestPath, newGoodPath)
	}

	// Missing game's inventory entry must be unchanged.
	missingEntry, _ := inv.Lookup("https://dev.itch.io/missing")
	if missingEntry.Files[0].DestPath != missingROM {
		t.Errorf("missing inventory DestPath changed to %q", missingEntry.Files[0].DestPath)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/roms/ -run "TestMigrate" -v
```

Expected: FAIL — `roms.MigratePico8Files` undefined.

- [ ] **Step 3: Create `internal/roms/migrate.go`**

```go
package roms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// MigratePico8Files moves all Pico-8 ROM files (and their cover art) whose
// DestPath is under oldDir to newDir, updates the inventory, and saves it.
// Individual file failures are logged as warnings but do not abort the migration.
func MigratePico8Files(inv *inventory.Inventory, invPath, oldDir, newDir string) error {
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("migrate pico8: create dest dir: %w", err)
	}

	urls := inv.AllURLs()
	for _, gameURL := range urls {
		entry, ok := inv.Lookup(gameURL)
		if !ok {
			continue
		}
		for _, f := range entry.Files {
			if !strings.HasPrefix(f.DestPath, oldDir) {
				continue
			}
			rel := strings.TrimPrefix(f.DestPath, oldDir)
			newPath := filepath.Join(newDir, rel)

			if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
				logger.Warn("migrate pico8: mkdir %s: %v", filepath.Dir(newPath), err)
				continue
			}
			if err := os.Rename(f.DestPath, newPath); err != nil {
				logger.Warn("migrate pico8: rename %s → %s: %v", f.DestPath, newPath, err)
				continue
			}
			logger.Info("migrate pico8: moved %s → %s", f.DestPath, newPath)

			// Move cover art (best-effort).
			oldArt := inventory.CoverArtPath(entry.CoverURL, f.DestPath)
			if oldArt != "" {
				if _, err := os.Stat(oldArt); err == nil {
					newArt := inventory.CoverArtPath(entry.CoverURL, newPath)
					if err := os.MkdirAll(filepath.Dir(newArt), 0755); err == nil {
						if err := os.Rename(oldArt, newArt); err != nil {
							logger.Warn("migrate pico8: cover art rename %s: %v", oldArt, err)
						}
					}
				}
			}

			updated := f
			updated.DestPath = newPath
			inv.UpdateFile(gameURL, f.DestPath, updated)
		}
	}

	if err := inv.Save(invPath); err != nil {
		return fmt.Errorf("migrate pico8: save inventory: %w", err)
	}

	// Best-effort cleanup of old .media dir and old root dir.
	_ = os.Remove(filepath.Join(strings.TrimSuffix(oldDir, "/"), ".media"))
	_ = os.Remove(strings.TrimSuffix(oldDir, "/"))

	return nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
go test ./internal/roms/ -run "TestMigrate" -v
```

Expected: all four migration tests PASS.

- [ ] **Step 5: Run full roms test suite**

```
go test ./internal/roms/ -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```
git add internal/roms/migrate.go internal/roms/migrate_test.go
git commit -m "feat: add MigratePico8Files for atomic core-switch file relocation"
```

---

## Task 5: Update all `DestinationDir` and `Pico8GameDir` call sites

**Files:**
- Modify: `internal/ui/screen_autodetect.go`
- Modify: `internal/ui/screen_fetch_uploads.go`
- Modify: `internal/ui/screen_format_picker.go`
- Modify: `internal/ui/screen_location_picker.go`
- Modify: `internal/ui/screen_purchase_picker.go`
- Modify: `internal/ui/screen_rom_picker.go`
- Modify: `internal/ui/screen_zip_inspect.go`
- Modify: `internal/ui/screen_zip_download.go`

All these files call `roms.DestinationDir(ext)` with one argument; the new signature requires two. All screens have a `s.cfg` field of type `*settings.Config`. The fix in each case is: `roms.DestinationDir(ext)` → `roms.DestinationDir(ext, s.cfg.Pico8Core)`.

- [ ] **Step 1: Update `screen_autodetect.go`**

```
# Line 134:
dest := roms.DestinationDir(ext) + upload.Filename
# becomes:
dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + upload.Filename
```

- [ ] **Step 2: Update `screen_fetch_uploads.go`**

Three call sites (lines 286, 308, and one additional):

```
# All three occurrences of:
roms.DestinationDir(ext)
# become:
roms.DestinationDir(ext, s.cfg.Pico8Core)
```

- [ ] **Step 3: Update `screen_format_picker.go`**

```
# Line 309:
dest := roms.DestinationDir(chosenExt) + upload.Filename
# becomes:
dest := roms.DestinationDir(chosenExt, s.cfg.Pico8Core) + upload.Filename
```

- [ ] **Step 4: Update `screen_location_picker.go`**

The call is inside `resolveStartDir(cfg *settings.Config, ext, cfgPath string)`:

```
# Line 101:
return roms.DestinationDir(ext)
# becomes:
return roms.DestinationDir(ext, cfg.Pico8Core)
```

- [ ] **Step 5: Update `screen_purchase_picker.go`**

```
# Line 205:
dest := roms.DestinationDir(ext) + upload.Filename
# becomes:
dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + upload.Filename
```

- [ ] **Step 6: Update `screen_rom_picker.go`**

```
# Line 147:
dest := roms.DestinationDir(ext) + upload.Filename
# becomes:
dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + upload.Filename
```

- [ ] **Step 7: Update `screen_zip_inspect.go`**

Three changes:

```
# Line 217 (validation check inside ZIP scan loop):
if inner := strings.ToLower(roms.ROMExt(e.Name)); roms.DestinationDir(inner) != "" {
# becomes:
if inner := strings.ToLower(roms.ROMExt(e.Name)); roms.DestinationDir(inner, s.cfg.Pico8Core) != "" {

# Line 232:
dest := roms.DestinationDir(ext) + s.upload.Filename
# becomes:
dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + s.upload.Filename

# Line 245:
gameDir := roms.Pico8GameDir(s.game.Title)
# becomes:
gameDir := roms.Pico8GameSubDir(s.cfg.Pico8Core, s.game.Title)
```

- [ ] **Step 8: Update `screen_zip_download.go`**

```
# Line 236:
destDir = roms.DestinationDir(ext)
# becomes:
destDir = roms.DestinationDir(ext, s.cfg.Pico8Core)
```

- [ ] **Step 9: Verify the project builds**

```
./scripts/build.sh native
```

Expected: `Built: bin/native/itchio-pak ...` with no errors.

- [ ] **Step 10: Commit**

```
git add internal/ui/screen_autodetect.go \
        internal/ui/screen_fetch_uploads.go \
        internal/ui/screen_format_picker.go \
        internal/ui/screen_location_picker.go \
        internal/ui/screen_purchase_picker.go \
        internal/ui/screen_rom_picker.go \
        internal/ui/screen_zip_inspect.go \
        internal/ui/screen_zip_download.go
git commit -m "feat: pass Pico8Core to DestinationDir and Pico8GameSubDir at all call sites"
```

---

## Task 6: Settings screen — picker item, inventory wiring, migration on change

**Files:**
- Modify: `internal/ui/screen_settings.go`
- Modify: `internal/ui/screen_list.go`
- Modify: `internal/ui/screen_detail.go`
- Modify: `internal/ui/dev_start.go`

- [ ] **Step 1: Add `sItemPico8Core` to the settings enum**

In `screen_settings.go`, add the new item after `sItemROMLocation`:

```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMLocation
	sItemPico8Core      // ← new
	sItemMusicDownload
	sItemMusicLocation
	sItemUnifiedNaming
	sItemNextUITheme
	sItemLogLevel
	sItemClearCache
	sItemRefreshCache
	sItemUpdateInventory
	sItemContentModeration
	sItemAbout
	sItemCount
)
```

- [ ] **Step 2: Add `inv`, `invPath`, and `statusMsg` to `SettingsScreen`**

```go
type SettingsScreen struct {
	client         *itchio.Client
	cfg            *settings.Config
	cfgPath        string
	inv            *inventory.Inventory   // ← new
	invPath        string                 // ← new
	cache          *renderer.ImageCache
	cursor         settingsItem
	prev           Screen
	onRefreshGames func(Screen) Screen
	updateSvc      UpdateServicer

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	onThemeToggle  func(bool)
	onOwnedReady   func([]itchio.OwnedGame)

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time

	showAPIKeyHelp bool
	apiKeyHelpQR   *sdl.Texture
	statusMsg      string  // ← new: shown briefly after migration
}
```

Add the import for inventory at the top of the file imports:

```go
"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
```

- [ ] **Step 3: Update `NewSettingsScreen` signature and body**

```go
func NewSettingsScreen(
	client *itchio.Client,
	cfg *settings.Config,
	cfgPath string,
	inv *inventory.Inventory,
	invPath string,
	cache *renderer.ImageCache,
	prev Screen,
	onRefreshGames func(Screen) Screen,
	updateSvc UpdateServicer,
	nextUITheme theme.Theme,
	defaultTheme theme.Theme,
	themeAvailable bool,
	onThemeToggle func(bool),
	onOwnedReady func([]itchio.OwnedGame),
) *SettingsScreen {
	s := &SettingsScreen{
		client:         client,
		cfg:            cfg,
		cfgPath:        cfgPath,
		inv:            inv,
		invPath:        invPath,
		cache:          cache,
		prev:           prev,
		onRefreshGames: onRefreshGames,
		updateSvc:      updateSvc,
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		onThemeToggle:  onThemeToggle,
		onOwnedReady:   onOwnedReady,
	}
	// ... rest of constructor unchanged
```

- [ ] **Step 4: Add the picker row to `Draw`**

After the ROM Location item in the `items` slice (inside `Draw`):

```go
items = append(items, menuItem{sItemROMLocation, "ROM Location: " + s.cfg.ROMLocation})
pico8CoreLabel := "FakeO8 (default)"
if s.cfg.Pico8Core == "pico8" {
    pico8CoreLabel = "Pico-8 (official)"
}
items = append(items, menuItem{sItemPico8Core, "Pico-8 Core: " + pico8CoreLabel})
items = append(items, menuItem{sItemMusicDownload, "Music Download: " + musicDownloadLabel(s.cfg.MusicDownload)})
```

Add status message rendering after the items loop (before the footer), still inside `Draw`:

```go
if s.statusMsg != "" {
    sm := r.Theme.MainText
    r.DrawText(s.statusMsg, 20, r.H-footerH-rowH-4, sm[0], sm[1], sm[2])
}
```

- [ ] **Step 5: Add the `activate` case for `sItemPico8Core`**

Add to the `activate` switch in `screen_settings.go`:

```go
case sItemPico8Core:
    oldCore := s.cfg.Pico8Core
    if oldCore == "pico8" {
        s.cfg.Pico8Core = "fakeo8"
    } else {
        s.cfg.Pico8Core = "pico8"
    }
    oldDir := roms.Pico8ROMDir(oldCore)
    newDir := roms.Pico8ROMDir(s.cfg.Pico8Core)
    if err := roms.MigratePico8Files(s.inv, s.invPath, oldDir, newDir); err != nil {
        logger.Warn("settings: pico8 core migration failed: %v", err)
        s.cfg.Pico8Core = oldCore // revert
        s.statusMsg = "Migration failed — check log"
        return nil
    }
    if err := s.cfg.Save(s.cfgPath); err != nil {
        logger.Warn("settings: save failed after pico8 core switch: %v", err)
    }
    s.statusMsg = "Pico-8 files moved to " + newDir
    logger.Info("settings: pico8 core changed to %s", s.cfg.Pico8Core)
```

Also clear `statusMsg` when the cursor moves. In `moveCursor`, add at the end:

```go
s.statusMsg = ""
```

- [ ] **Step 6: Update all `NewSettingsScreen` call sites**

In `internal/ui/screen_list.go` (2 occurrences), add `s.inv, s.inventoryPath,` after `s.cfgPath,`:

```go
return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s.inv, s.inventoryPath, s.cache, s, s.newCacheRefreshScreen, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle, s.onOwnedReady)
```

In `internal/ui/screen_detail.go` (2 occurrences), add `s.inv, s.inventoryPath,` after `s.cfgPath,`:

```go
return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s.inv, s.inventoryPath, s.cache, s, nil, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle, nil)
```

In `internal/ui/dev_start.go` (1 occurrence), add `inv, inventoryPath,` after `cfgPath,`:

```go
return NewSettingsScreen(client, cfg, cfgPath, inv, inventoryPath, cache, list, nil, updateSvc, nextUITheme, defaultTheme, themeAvailable, onThemeToggle, nil)
```

- [ ] **Step 7: Build to confirm no compile errors**

```
./scripts/build.sh native
```

Expected: `Built: bin/native/itchio-pak ...` with no errors.

- [ ] **Step 8: Run full test suite**

```
./scripts/test.sh
```

Expected: all packages pass.

- [ ] **Step 9: Commit**

```
git add internal/ui/screen_settings.go \
        internal/ui/screen_list.go \
        internal/ui/screen_detail.go \
        internal/ui/dev_start.go
git commit -m "feat: Pico-8 Core picker in settings with instant file migration"
```

---

## Task 7: Build, deploy, and verify on device

- [ ] **Step 1: Cross-compile for tg5040**

```
./scripts/build.sh tg5040
```

Expected: `Built: bin/tg5040/itchio-pak ...`

- [ ] **Step 2: Push binary to device**

```
adb push bin/tg5040/itchio-pak /mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio-pak
```

- [ ] **Step 3: Verify on device**

Restart the pak. Navigate to Settings. Confirm "Pico-8 Core: FakeO8 (default)" row appears. Press A to toggle to "Pico-8 (official)". Confirm any previously downloaded `.p8`/`.p8.png` files appear in `/mnt/SDCARD/Roms/Pico-8 (PICO)/`. Press A again to toggle back and confirm files return to `/mnt/SDCARD/Roms/Pico-8 (P8)/`.

- [ ] **Step 4: Download a new Pico-8 game and confirm it lands in the active core's directory**
