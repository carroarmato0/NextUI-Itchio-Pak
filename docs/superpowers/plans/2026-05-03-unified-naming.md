# Unified Naming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename downloaded ROMs to match the game title, keep saves/states in sync during migration, and expose a per-game toggle alongside a global settings switch.

**Architecture:** Pure utility functions in `internal/roms/` (sanitise, save paths, state paths, zip peek) feed an `inventory.MigrateFile` function that accepts a callback interface for UI prompts. The download screen applies unified naming post-write. UI layers (settings, detail, manage-downloads) wire the callback interface through a dedicated migration-flow screen.

**Tech Stack:** Go 1.22, `archive/zip` (stdlib), SDL2 UI layer (build tag `!headless`).

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Modify | `internal/inventory/inventory.go` | Add `UnifiedName`, `UnifiedNamingDisabled`, `UpdateFile`, `SetUnifiedNamingDisabled` |
| Modify | `internal/settings/settings.go` | Add `UnifiedNaming bool` (default `true`) |
| Create | `internal/roms/sanitise.go` | `SanitiseFilename`, `ResolveUnifiedDest` |
| Create | `internal/roms/sanitise_test.go` | Spec §8.A + collision cases |
| Create | `internal/roms/ziputil.go` | `ZipInnerFilename` |
| Create | `internal/roms/ziputil_test.go` | zip peek cases |
| Create | `internal/roms/savegame.go` | `SaveGamePath`, `RomDirToSaveTag` |
| Create | `internal/roms/savegame_test.go` | Spec §8.B |
| Create | `internal/roms/savestate.go` | `SaveStatePaths`, `RomCoreInfo` |
| Create | `internal/roms/savestate_test.go` | Spec §8.B2 |
| Create | `internal/inventory/migrate.go` | `MigrateFormats`, `ReadMigrateFormats`, `MigrateFile`, `SaveDataCallback`, `MigrateResult` |
| Create | `internal/inventory/migrate_test.go` | Spec §8.C |
| Modify | `internal/ui/screen_download.go` | Apply unified naming post-download |
| Modify | `internal/ui/screen_settings.go` | Global `UnifiedNaming` toggle |
| Create | `internal/ui/screen_migrate_flow.go` | Migration flow state machine (save prompt → state prompt → execute) |
| Modify | `internal/ui/screen_manage_downloads.go` | Per-game toggle row |
| Modify | `internal/ui/screen_detail.go` | Per-game toggle action item |

---

## Task 1: Inventory schema additions

**Files:**
- Modify: `internal/inventory/inventory.go`
- Modify: `internal/inventory/inventory_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/inventory/inventory_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io/.worktrees/feature/unified-naming
go test ./internal/inventory/... 2>&1 | grep -E "FAIL|PASS|undefined|does not"
```

Expected: compile error — `UnifiedName`, `UnifiedNamingDisabled`, `SetUnifiedNamingDisabled`, `UpdateFile` undefined.

- [ ] **Step 3: Add fields and methods to `internal/inventory/inventory.go`**

Change `DownloadedFile`:
```go
type DownloadedFile struct {
	Filename     string    `json:"filename"`
	DestPath     string    `json:"dest_path"`
	DownloadedAt time.Time `json:"downloaded_at"`
	UnifiedName  bool      `json:"unified_name,omitempty"`
}
```

Change `Entry` (add one field after `RemovalDismissedAt`):
```go
UnifiedNamingDisabled bool `json:"unified_naming_disabled,omitempty"`
```

Add two new methods after `MarkReachable`:
```go
// SetUnifiedNamingDisabled sets the per-game unified-naming opt-out flag.
func (inv *Inventory) SetUnifiedNamingDisabled(gameURL string, disabled bool) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.UnifiedNamingDisabled = disabled
}

// UpdateFile replaces the DownloadedFile whose DestPath matches oldDestPath.
// Returns false if the game URL or file is not found.
func (inv *Inventory) UpdateFile(gameURL, oldDestPath string, file DownloadedFile) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	for i, f := range e.Files {
		if f.DestPath == oldDestPath {
			e.Files[i] = file
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/inventory/... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/inventory.go internal/inventory/inventory_test.go
git commit -m "feat(inventory): add UnifiedName, UnifiedNamingDisabled, UpdateFile, SetUnifiedNamingDisabled"
```

---

## Task 2: Settings schema

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/settings/settings_test.go` (create file if it doesn't exist):

```go
package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

func TestUnifiedNaming_DefaultTrue(t *testing.T) {
	dir := t.TempDir()
	cfg, err := settings.Load(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UnifiedNaming {
		t.Error("UnifiedNaming default should be true")
	}
}

func TestUnifiedNaming_OldConfigGetsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Old config with no unified_naming field
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UnifiedNaming {
		t.Error("UnifiedNaming should default to true when absent from config")
	}
}

func TestUnifiedNaming_ExplicitFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"unified_naming": false}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnifiedNaming {
		t.Error("UnifiedNaming should be false when explicitly set to false")
	}
}

func TestUnifiedNaming_RoundTrip_False(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg, _ := settings.Load(filepath.Join(dir, "missing.json"))
	cfg.UnifiedNaming = false
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.UnifiedNaming {
		t.Error("UnifiedNaming=false should survive save/load round-trip")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/settings/... 2>&1
```

Expected: compile error — `UnifiedNaming` undefined on `Config`.

- [ ] **Step 3: Add field to `internal/settings/settings.go`**

In `Config`, add after `NextUITheme`:
```go
UnifiedNaming bool `json:"unified_naming"` // default true — no omitempty so false survives save/load
```

In `defaults()`, add after `NextUITheme` (or inside return):
```go
UnifiedNaming: true,
```

The full updated `defaults()`:
```go
func defaults() *Config {
	return &Config{
		APIKey:       "",
		ROMSelection: "auto",
		ROMLocation:  "auto",
		UnifiedNaming: true,
		Filter: ContentFilter{
			AdultContent: CategoryFilter{Enabled: true},
			HeavyThemes:  CategoryFilter{Enabled: true},
			SubstanceUse: CategoryFilter{Enabled: true},
		},
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/settings/... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat(settings): add UnifiedNaming bool (default true)"
```

---

## Task 3: Filename sanitisation

**Files:**
- Create: `internal/roms/sanitise.go`
- Create: `internal/roms/sanitise_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/roms/sanitise_test.go`:

```go
package roms_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestSanitiseFilename(t *testing.T) {
	cases := []struct {
		title string
		ext   string
		want  string
	}{
		{"Doomslinger Dungeon", ".gb", "Doomslinger Dungeon.gb"},
		{"Solastra", ".gbc", "Solastra.gbc"},
		{"My Game: Subtitle", ".gb", "My Game Subtitle.gb"},
		{"Game/Title", ".gb", "GameTitle.gb"},
		{"  Spaced  Title  ", ".gb", "Spaced Title.gb"},
		{"Game * Name", ".gb", "Game Name.gb"},
		{"Game Boy ROM", ".gb", "Game Boy ROM.gb"},
		{"", ".gb", ""},
	}
	for _, c := range cases {
		got := roms.SanitiseFilename(c.title, c.ext)
		if got != c.want {
			t.Errorf("SanitiseFilename(%q, %q) = %q, want %q", c.title, c.ext, got, c.want)
		}
	}
}

func TestResolveUnifiedDest_NoCollision(t *testing.T) {
	dir := t.TempDir()
	// Create the ROM at its upstream name
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon")
	want := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !renamed {
		t.Error("renamed should be true")
	}
}

func TestResolveUnifiedDest_SameNameNoRename(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon")
	if got != current {
		t.Errorf("got %q, want %q", got, current)
	}
	if renamed {
		t.Error("renamed should be false when name is already correct")
	}
}

func TestResolveUnifiedDest_Collision(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing file at the target name
	existing := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if err := os.WriteFile(existing, []byte("other"), 0644); err != nil {
		t.Fatal(err)
	}

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon")
	want := filepath.Join(dir, "Doomslinger Dungeon (2).gb")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !renamed {
		t.Error("renamed should be true")
	}
}

func TestResolveUnifiedDest_EmptyTitle_NoRename(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	got, renamed := roms.ResolveUnifiedDest(current, "")
	if got != current {
		t.Errorf("empty title: got %q, want %q", got, current)
	}
	if renamed {
		t.Error("empty title: renamed should be false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/roms/... 2>&1
```

Expected: compile error — `SanitiseFilename`, `ResolveUnifiedDest` undefined.

- [ ] **Step 3: Create `internal/roms/sanitise.go`**

```go
package roms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SanitiseFilename builds a safe filename from a game title and extension.
// Strips / : ? * " < > | from the title, trims and collapses whitespace.
// Returns "" when title is empty (caller should use the upstream filename instead).
func SanitiseFilename(title, ext string) string {
	if title == "" {
		return ""
	}
	const strip = `/:?*"<>|`
	var b strings.Builder
	for _, r := range title {
		if strings.ContainsRune(strip, r) {
			continue
		}
		b.WriteRune(r)
	}
	s := strings.Join(strings.Fields(b.String()), " ")
	if s == "" {
		return ""
	}
	return s + ext
}

// ResolveUnifiedDest returns the desired on-disk path for a ROM after applying
// unified naming. currentPath is where the file was written. gameTitle is the
// itch.io game title (used to derive the target filename).
//
// Returns (currentPath, false) when no rename is needed (name already correct,
// or title is empty). Returns (targetPath, true) when a rename is required.
// Appends " (2)", " (3)" etc. to avoid colliding with existing files.
func ResolveUnifiedDest(currentPath, gameTitle string) (string, bool) {
	ext := filepath.Ext(currentPath)
	candidate := SanitiseFilename(gameTitle, ext)
	if candidate == "" || candidate == filepath.Base(currentPath) {
		return currentPath, false
	}
	dir := filepath.Dir(currentPath)
	target := filepath.Join(dir, candidate)
	if _, err := os.Stat(target); err == nil && target != currentPath {
		stem := strings.TrimSuffix(candidate, ext)
		for n := 2; ; n++ {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
			target = filepath.Join(dir, candidate)
			if _, err := os.Stat(target); os.IsNotExist(err) {
				break
			}
		}
	}
	return target, target != currentPath
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/roms/... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/roms/sanitise.go internal/roms/sanitise_test.go
git commit -m "feat(roms): add SanitiseFilename and ResolveUnifiedDest"
```

---

## Task 4: ZIP inner filename

**Files:**
- Create: `internal/roms/ziputil.go`
- Create: `internal/roms/ziputil_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/roms/ziputil_test.go`:

```go
package roms_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func makeTestZip(t *testing.T, innerName string) string {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "rom.zip")
	w, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(w)
	f, err := zw.Create(innerName)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("rom data"))
	zw.Close()
	w.Close()
	return zipPath
}

func TestZipInnerFilename_GB(t *testing.T) {
	zipPath := makeTestZip(t, "Pokemon - Red Version (USA, Europe).gb")
	got := roms.ZipInnerFilename(zipPath)
	if got != "Pokemon - Red Version (USA, Europe).gb" {
		t.Errorf("got %q", got)
	}
}

func TestZipInnerFilename_GBC(t *testing.T) {
	zipPath := makeTestZip(t, "Solastra.gbc")
	got := roms.ZipInnerFilename(zipPath)
	if got != "Solastra.gbc" {
		t.Errorf("got %q", got)
	}
}

func TestZipInnerFilename_NoROMInside(t *testing.T) {
	zipPath := makeTestZip(t, "readme.txt")
	got := roms.ZipInnerFilename(zipPath)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestZipInnerFilename_MissingFile(t *testing.T) {
	got := roms.ZipInnerFilename("/nonexistent/path.zip")
	if got != "" {
		t.Errorf("expected empty for missing file, got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/roms/... 2>&1
```

Expected: compile error — `ZipInnerFilename` undefined.

- [ ] **Step 3: Create `internal/roms/ziputil.go`**

```go
package roms

import (
	"archive/zip"
	"path/filepath"
	"strings"
)

// ZipInnerFilename returns the filename of the first recognized ROM file inside
// a zip archive. Returns "" if the zip cannot be opened or contains no ROM.
func ZipInnerFilename(zipPath string) string {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return ""
	}
	defer r.Close()
	for _, f := range r.File {
		switch strings.ToLower(filepath.Ext(f.Name)) {
		case ".gb", ".gbc", ".gba":
			return f.Name
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/roms/... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/roms/ziputil.go internal/roms/ziputil_test.go
git commit -m "feat(roms): add ZipInnerFilename"
```

---

## Task 5: Save game path

**Files:**
- Create: `internal/roms/savegame.go`
- Create: `internal/roms/savegame_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/roms/savegame_test.go`:

```go
package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestSaveGamePath(t *testing.T) {
	base := "/mnt/SDCARD"
	cases := []struct {
		saveFormat    int
		innerFilename string
		romPath       string
		want          string
	}{
		// format 0 (MinUI) — full filename + .sav
		{0, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.gb.sav"},
		{0, "", base + "/Roms/Game Boy Color (GBC)/Solastra.gbc", base + "/Saves/GBC/Solastra.gbc.sav"},
		{0, "", base + "/Roms/Game Boy (GB)/My Game v1.2.gb", base + "/Saves/GB/My Game v1.2.gb.sav"},
		// format 1 (Retroarch SRM compressed) — strip ext, .srm
		{1, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.srm"},
		{1, "", base + "/Roms/Game Boy Color (GBC)/Solastra.gbc", base + "/Saves/GBC/Solastra.srm"},
		// format 2 (Generic) — strip ext, .sav
		{2, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.sav"},
		{2, "", base + "/Roms/Game Boy (GB)/My Game v1.2.gb", base + "/Saves/GB/My Game v1.2.sav"},
		// format 3 (Retroarch SRM uncompressed) — same as format 1
		{3, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.srm"},
		// unrecognised directory
		{0, "", base + "/Roms/Unknown Emulator/foo.rom", ""},
		// zip + format 0 + no innerFilename → zip.sav
		{0, "", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).zip.sav"},
		// zip + format 0 + innerFilename → inner.gb.sav
		{0, "Pokemon - Red Version (USA, Europe).gb", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).gb.sav"},
		// zip + format 1 + innerFilename → same stem as without (both stripped)
		{1, "Pokemon - Red Version (USA, Europe).gb", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).srm"},
		{1, "", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).srm"},
	}
	for _, c := range cases {
		got := roms.SaveGamePath(c.romPath, c.saveFormat, c.innerFilename)
		if got != c.want {
			t.Errorf("SaveGamePath(%q, %d, %q)\n  got  %q\n  want %q", c.romPath, c.saveFormat, c.innerFilename, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/roms/... 2>&1
```

Expected: compile error — `SaveGamePath` undefined.

- [ ] **Step 3: Create `internal/roms/savegame.go`**

```go
package roms

import (
	"path/filepath"
	"strings"
)

// RomDirToSaveTag maps a ROM directory name to its NextUI save tag.
// Returns "" for unrecognised directories.
func RomDirToSaveTag(romDestPath string) string {
	switch filepath.Base(filepath.Dir(romDestPath)) {
	case "Game Boy (GB)":
		return "GB"
	case "Game Boy Color (GBC)":
		return "GBC"
	case "Game Boy Advance (GBA)":
		return "GBA"
	default:
		return ""
	}
}

// SaveGamePath derives the SRAM save file path for a downloaded ROM.
//
// saveFormat:
//   0 = MinUI (default)   — full ROM filename + ".sav"  (e.g. Game.gb.sav)
//   1 = Retroarch SRM     — extension stripped + ".srm" (e.g. Game.srm)
//   2 = Generic           — extension stripped + ".sav" (e.g. Game.sav)
//   3 = Retroarch SRM     — same as 1, uncompressed
//
// innerFilename: when the ROM is a .zip and NextUI's useExtractedFileName is
// enabled, pass the filename of the ROM inside the zip. Only affects format 0
// output; formats 1–3 produce the same result either way.
//
// Returns "" for unrecognised ROM directories.
func SaveGamePath(romDestPath string, saveFormat int, innerFilename string) string {
	tag := RomDirToSaveTag(romDestPath)
	if tag == "" {
		return ""
	}
	baseName := filepath.Base(romDestPath)
	if innerFilename != "" && saveFormat == 0 {
		baseName = innerFilename
	}
	ext := filepath.Ext(baseName)
	savesDir := filepath.Join(filepath.Dir(filepath.Dir(romDestPath)), "Saves")
	switch saveFormat {
	case 0:
		return filepath.Join(savesDir, tag, baseName+".sav")
	case 1, 3:
		stem := strings.TrimSuffix(baseName, ext)
		return filepath.Join(savesDir, tag, stem+".srm")
	case 2:
		stem := strings.TrimSuffix(baseName, ext)
		return filepath.Join(savesDir, tag, stem+".sav")
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/roms/... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/roms/savegame.go internal/roms/savegame_test.go
git commit -m "feat(roms): add SaveGamePath with format-aware and zip support"
```

---

## Task 6: Save state paths

**Files:**
- Create: `internal/roms/savestate.go`
- Create: `internal/roms/savestate_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/roms/savestate_test.go`:

```go
package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestSaveStatePaths_Format0_GB(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Doomslinger Dungeon.gb"
	paths := roms.SaveStatePaths(romPath, 0, "", "GB", "gambatte")
	if len(paths) != 10 {
		t.Fatalf("format 0 should return 10 paths, got %d", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Doomslinger Dungeon.gb"
	if paths[0] != base+".st0" {
		t.Errorf("slot 0: got %q", paths[0])
	}
	if paths[5] != base+".st5" {
		t.Errorf("slot 5: got %q", paths[5])
	}
	if paths[9] != base+".st9" {
		t.Errorf("auto (slot 9): got %q", paths[9])
	}
}

func TestSaveStatePaths_Format1_GB(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Doomslinger Dungeon.gb"
	paths := roms.SaveStatePaths(romPath, 1, "", "GB", "gambatte")
	if len(paths) != 9 {
		t.Fatalf("format 1 should return 9 paths, got %d", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Doomslinger Dungeon"
	if paths[0] != base+".state.1" {
		t.Errorf("slot 1: got %q", paths[0])
	}
	if paths[4] != base+".state.5" {
		t.Errorf("slot 5: got %q", paths[4])
	}
	if paths[8] != base+".state.auto" {
		t.Errorf("auto: got %q", paths[8])
	}
}

func TestSaveStatePaths_Format3_GB(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Doomslinger Dungeon.gb"
	paths := roms.SaveStatePaths(romPath, 3, "", "GB", "gambatte")
	if len(paths) != 10 {
		t.Fatalf("format 3 should return 10 paths, got %d", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Doomslinger Dungeon"
	if paths[0] != base+".state" {
		t.Errorf("slot 0: got %q", paths[0])
	}
	if paths[5] != base+".state5" {
		t.Errorf("slot 5: got %q", paths[5])
	}
	if paths[9] != base+".state.auto" {
		t.Errorf("auto: got %q", paths[9])
	}
}

func TestSaveStatePaths_ZipWithInnerFilename_Format0(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip"
	inner := "Pokemon - Red Version (USA, Europe).gb"
	paths := roms.SaveStatePaths(romPath, 0, inner, "GB", "gambatte")
	if len(paths) != 10 {
		t.Fatalf("got %d paths", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Pokemon - Red Version (USA, Europe).gb"
	if paths[0] != base+".st0" {
		t.Errorf("slot 0 with inner filename: got %q", paths[0])
	}
}

func TestSaveStatePaths_ZipWithInnerFilename_Format1_SameAsStemOnly(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip"
	inner := "Pokemon - Red Version (USA, Europe).gb"
	withInner := roms.SaveStatePaths(romPath, 1, inner, "GB", "gambatte")
	withoutInner := roms.SaveStatePaths(romPath, 1, "", "GB", "gambatte")
	if len(withInner) != len(withoutInner) {
		t.Fatalf("format 1 zip paths should be equal with or without innerFilename: %d vs %d", len(withInner), len(withoutInner))
	}
	for i := range withInner {
		if withInner[i] != withoutInner[i] {
			t.Errorf("path[%d] differs: %q vs %q", i, withInner[i], withoutInner[i])
		}
	}
}

func TestSaveStatePaths_UnrecognisedDir_ReturnsNil(t *testing.T) {
	paths := roms.SaveStatePaths("/mnt/SDCARD/Roms/Unknown/foo.rom", 0, "", "", "")
	if paths != nil {
		t.Errorf("expected nil for unrecognised dir, got %v", paths)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/roms/... 2>&1
```

Expected: compile error — `SaveStatePaths` undefined.

- [ ] **Step 3: Create `internal/roms/savestate.go`**

```go
package roms

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SaveStatePaths returns the set of save-state paths that could exist for a ROM.
// The caller should filter to only paths that exist on disk before prompting.
//
// stateFormat:
//   0 = MinUI      — <full>.st0 … .st9   (10 paths; .st9 = auto-resume)
//   1/2 = Retroarch-ish — <stem>.state.1 … .state.8 + .state.auto  (9 paths)
//   3/4 = Retroarch     — <stem>.state, .state1…8, .state.auto      (10 paths)
//
// innerFilename: same semantics as SaveGamePath — only affects format 0.
// coreTag / coreName: must match the NextUI core directory (e.g. "GB", "gambatte").
// Returns nil for empty coreTag/coreName or unrecognised ROM directories.
func SaveStatePaths(romDestPath string, stateFormat int, innerFilename, coreTag, coreName string) []string {
	if coreTag == "" || coreName == "" {
		return nil
	}
	statesDir := filepath.Join("/mnt/SDCARD/.userdata/shared", coreTag+"-"+coreName)
	baseName := filepath.Base(romDestPath)
	if innerFilename != "" && stateFormat == 0 {
		baseName = innerFilename
	}
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)

	switch stateFormat {
	case 0:
		paths := make([]string, 10)
		for i := 0; i <= 9; i++ {
			paths[i] = filepath.Join(statesDir, fmt.Sprintf("%s.st%d", baseName, i))
		}
		return paths
	case 1, 2:
		paths := make([]string, 9)
		for i := 1; i <= 8; i++ {
			paths[i-1] = filepath.Join(statesDir, fmt.Sprintf("%s.state.%d", stem, i))
		}
		paths[8] = filepath.Join(statesDir, stem+".state.auto")
		return paths
	case 3, 4:
		paths := make([]string, 10)
		paths[0] = filepath.Join(statesDir, stem+".state")
		for i := 1; i <= 8; i++ {
			paths[i] = filepath.Join(statesDir, fmt.Sprintf("%s.state%d", stem, i))
		}
		paths[9] = filepath.Join(statesDir, stem+".state.auto")
		return paths
	default:
		return nil
	}
}

// RomCoreInfo returns the coreTag and coreName for a ROM path.
// Returns ("", "") for unrecognised directories.
func RomCoreInfo(romDestPath string) (coreTag, coreName string) {
	switch filepath.Base(filepath.Dir(romDestPath)) {
	case "Game Boy (GB)":
		return "GB", "gambatte"
	case "Game Boy Color (GBC)":
		return "GBC", "gambatte"
	case "Game Boy Advance (GBA)":
		return "GBA", "gpsp"
	default:
		return "", ""
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/roms/... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/roms/savestate.go internal/roms/savestate_test.go
git commit -m "feat(roms): add SaveStatePaths and RomCoreInfo"
```

---

## Task 7: MigrateFormats and settings reader

**Files:**
- Create: `internal/inventory/migrate.go`
- Create: `internal/inventory/migrate_test.go` (types only so far)

- [ ] **Step 1: Write a failing test for `ReadMigrateFormats`**

Create `internal/inventory/migrate_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/inventory/... 2>&1
```

Expected: compile error — `ReadMigrateFormats`, `MigrateFormats` undefined.

- [ ] **Step 3: Create `internal/inventory/migrate.go`** (types + reader only)

```go
package inventory

import (
	"fmt"
	"os"
	"strings"
)

// MigrateFormats carries the user's configured save and state format indices,
// read from /mnt/SDCARD/.userdata/shared/minuisettings.txt before calling MigrateFile.
type MigrateFormats struct {
	SaveFormat           int  // 0=MinUI, 1=Retroarch SRM compressed, 2=Generic, 3=Retroarch SRM uncompressed
	StateFormat          int  // 0=MinUI, 1/2=Retroarch-ish (legacy), 3/4=Retroarch
	UseExtractedFileName bool // mirrors useExtractedFileName from minuisettings.txt
}

// NXSettingsPath is the on-device path to NextUI's shared settings file.
const NXSettingsPath = "/mnt/SDCARD/.userdata/shared/minuisettings.txt"

// ReadMigrateFormats reads saveFormat, stateFormat, and useExtractedFileName
// from path. Missing or unreadable file returns all-zero (MinUI defaults).
func ReadMigrateFormats(path string) MigrateFormats {
	data, err := os.ReadFile(path)
	if err != nil {
		return MigrateFormats{}
	}
	var f MigrateFormats
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		var n int
		if _, err := fmt.Sscanf(line, "saveFormat=%d", &n); err == nil {
			f.SaveFormat = n
			continue
		}
		if _, err := fmt.Sscanf(line, "stateFormat=%d", &n); err == nil {
			f.StateFormat = n
			continue
		}
		if _, err := fmt.Sscanf(line, "useExtractedFileName=%d", &n); err == nil {
			f.UseExtractedFileName = n != 0
		}
	}
	return f
}

// SaveDataCallback surfaces save-game and save-state prompts to the caller.
// In production the UI screens implement this interface.
// In tests a stub struct returns predetermined answers.
type SaveDataCallback interface {
	AskRenameExistingSave(savePath string) bool
	AskOverwriteExistingSave(newSavePath string) bool
	AskRenameExistingStates(statePaths []string) bool
}

// MigrateResult reports what MigrateFile changed.
type MigrateResult struct {
	ROMRenamed      bool
	CoverArtRenamed bool
	SaveRenamed     bool
	SaveSkipped     bool
	StatesRenamed   []string
	StatesSkipped   []string
	NewDestPath     string
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/inventory/... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/migrate.go internal/inventory/migrate_test.go
git commit -m "feat(inventory): add MigrateFormats, ReadMigrateFormats, SaveDataCallback, MigrateResult"
```

---

## Task 8: MigrateFile implementation

**Files:**
- Modify: `internal/inventory/migrate.go`
- Modify: `internal/inventory/migrate_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/inventory/migrate_test.go`:

```go
// stubCallback is a test double for SaveDataCallback.
type stubCallback struct {
	askRename    bool
	askOverwrite bool
	askStates    bool
}

func (s stubCallback) AskRenameExistingSave(_ string) bool        { return s.askRename }
func (s stubCallback) AskOverwriteExistingSave(_ string) bool     { return s.askOverwrite }
func (s stubCallback) AskRenameExistingStates(_ []string) bool    { return s.askStates }

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
	// Update entry with cover URL
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
	dir, inv, invPath := setupMigrateTest(t)
	gameURL := "https://playinstinct.itch.io/doomslinger-dungeon"
	addTestFile(t, inv, dir, gameURL, "Doomslinger Dungeon", "Game Boy ROM.gb", "Game Boy ROM.gb")
	// Create a fake save at the expected MinUI format-0 path
	savesDir := filepath.Join(dir, "Saves", "GB")
	os.MkdirAll(savesDir, 0755)
	oldSave := filepath.Join(savesDir, "Game Boy ROM.gb.sav")
	os.WriteFile(oldSave, []byte("save"), 0644)

	// Patch DestPath to use dir-based path that resolves to our test saves dir
	// We need a custom path that SaveGamePath can resolve. Instead, we test
	// the callback-based path: provide a ROM dir that maps to a known save dir.
	// For testability, we skip the real SaveGamePath and test the callback path.
	// The full integration is covered by TestMigrateFile_EnableUnifiedName_NoSave.
	// This test verifies that when a save IS found, user confirming renames it.

	// Use a real GB dir structure in tempdir.
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
	dir, inv, invPath := setupMigrateTest(t)
	_ = inv
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
	// Save at old path should still exist
	if _, err := os.Stat(saveFile); err != nil {
		t.Errorf("old save should remain: %v", err)
	}
	// ROM should still have been renamed
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
	// File already has unified name; upstream = "Game Boy ROM.gb"
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/inventory/... 2>&1
```

Expected: compile error — `MigrateFile` undefined.

- [ ] **Step 3: Implement `MigrateFile` in `internal/inventory/migrate.go`**

Append to `internal/inventory/migrate.go`:

```go
import (
	// add to existing imports:
	"errors"
	"path/filepath"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

// MigrateFile renames a downloaded ROM (and optionally its save/state files)
// between the upstream filename and the game-title-based name.
//
// enable=true:  rename to game title (UnifiedName=true).
// enable=false: revert to upstream filename (UnifiedName=false).
func MigrateFile(
	inv *Inventory,
	invPath string,
	gameURL string,
	file DownloadedFile,
	gameTitle string,
	enable bool,
	formats MigrateFormats,
	cb SaveDataCallback,
) (MigrateResult, error) {
	var res MigrateResult

	entry, ok := inv.Lookup(gameURL)
	if !ok {
		return res, errors.New("migrate: game not found in inventory")
	}

	currentPath := file.DestPath

	// Determine target path.
	var targetPath string
	if enable {
		targetPath, _ = roms.ResolveUnifiedDest(currentPath, gameTitle)
	} else {
		if !file.UnifiedName {
			return res, nil // already disabled — no-op
		}
		targetPath = filepath.Join(filepath.Dir(currentPath), file.Filename)
	}

	// No-op when path is already correct.
	if targetPath == currentPath {
		file.UnifiedName = enable
		inv.UpdateFile(gameURL, currentPath, file)
		_ = inv.Save(invPath)
		return res, nil
	}

	// Resolve inner filename for zip+useExtractedFileName.
	var innerFilename string
	if formats.UseExtractedFileName && filepath.Ext(currentPath) == ".zip" {
		innerFilename = roms.ZipInnerFilename(currentPath)
	}

	// --- Save game handling ---
	oldSavePath := roms.SaveGamePath(currentPath, formats.SaveFormat, innerFilename)
	newSavePath := roms.SaveGamePath(targetPath, formats.SaveFormat, innerFilename)
	var renameThisSave, skipThisSave bool

	if oldSavePath != "" && oldSavePath != newSavePath {
		if _, err := os.Stat(oldSavePath); err == nil {
			// Save exists at old path. Check for overwrite conflict first.
			if _, err2 := os.Stat(newSavePath); err2 == nil {
				// Save exists at new path too.
				if !cb.AskRenameExistingSave(oldSavePath) {
					skipThisSave = true
				} else if !cb.AskOverwriteExistingSave(newSavePath) {
					return res, fmt.Errorf("migrate: cancelled by user (save overwrite declined)")
				} else {
					renameThisSave = true
				}
			} else {
				if cb.AskRenameExistingSave(oldSavePath) {
					renameThisSave = true
				} else {
					skipThisSave = true
				}
			}
		}
	}

	// --- Save state handling ---
	coreTag, coreName := roms.RomCoreInfo(currentPath)
	var existingStatePaths []string
	if coreTag != "" {
		allOldStates := roms.SaveStatePaths(currentPath, formats.StateFormat, innerFilename, coreTag, coreName)
		for _, sp := range allOldStates {
			if _, err := os.Stat(sp); err == nil {
				existingStatePaths = append(existingStatePaths, sp)
			}
		}
	}
	renameStates := len(existingStatePaths) > 0 && cb.AskRenameExistingStates(existingStatePaths)

	// --- Rename ROM ---
	if err := os.Rename(currentPath, targetPath); err != nil {
		return res, fmt.Errorf("migrate: rename ROM: %w", err)
	}
	res.ROMRenamed = true
	res.NewDestPath = targetPath

	// --- Rename cover art (non-fatal) ---
	oldCover := CoverArtPath(entry.CoverURL, currentPath)
	if oldCover != "" {
		newCover := CoverArtPath(entry.CoverURL, targetPath)
		if err := os.Rename(oldCover, newCover); err != nil {
			logger.Warn("migrate: cover art rename failed: %v", err)
		} else {
			res.CoverArtRenamed = true
		}
	}

	// --- Rename save ---
	if renameThisSave {
		if err := os.Rename(oldSavePath, newSavePath); err != nil {
			logger.Warn("migrate: save rename failed: %v", err)
			res.SaveSkipped = true
		} else {
			res.SaveRenamed = true
		}
	} else if skipThisSave {
		res.SaveSkipped = true
	}

	// --- Rename state files (non-fatal per file) ---
	if renameStates {
		allNewStates := roms.SaveStatePaths(targetPath, formats.StateFormat, innerFilename, coreTag, coreName)
		for i, oldState := range existingStatePaths {
			// Find matching new path by index in the full list
			oldAll := roms.SaveStatePaths(currentPath, formats.StateFormat, innerFilename, coreTag, coreName)
			idx := -1
			for j, p := range oldAll {
				if p == oldState {
					idx = j
					break
				}
			}
			if idx < 0 || idx >= len(allNewStates) {
				res.StatesSkipped = append(res.StatesSkipped, oldState)
				continue
			}
			_ = i
			newState := allNewStates[idx]
			if err := os.Rename(oldState, newState); err != nil {
				logger.Warn("migrate: state rename %s: %v", oldState, err)
				res.StatesSkipped = append(res.StatesSkipped, oldState)
			} else {
				res.StatesRenamed = append(res.StatesRenamed, newState)
			}
		}
	}

	// --- Update inventory ---
	file.DestPath = targetPath
	file.UnifiedName = enable
	inv.UpdateFile(gameURL, currentPath, file)
	if err := inv.Save(invPath); err != nil {
		logger.Warn("migrate: save inventory: %v", err)
	}
	return res, nil
}
```

You will also need to add the missing imports. The top of `migrate.go` should include:

```go
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)
```

Remove the duplicate `fmt` and `os` from the reader section if needed — consolidate into one `import` block.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/inventory/... -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/migrate.go internal/inventory/migrate_test.go
git commit -m "feat(inventory): implement MigrateFile with save/state support"
```

---

## Task 9: Download flow — apply unified naming

**Files:**
- Modify: `internal/ui/screen_download.go`

No unit tests are possible for this file (SDL2 `!headless` build tag). Verify via deploy and manual test.

- [ ] **Step 1: Modify the goroutine in `NewDownloadScreen`**

In `screen_download.go`, after the download succeeds (after line ~73 `logger.Info("download: complete file=%s", upload.Filename)`), apply unified naming before `inv.Add`. Replace the success block:

```go
} else {
    logger.Info("download: complete file=%s", upload.Filename)

    // Apply unified naming if enabled for this game.
    finalDest := dest
    unifiedName := false
    if cfg.UnifiedNaming {
        entry, entryExists := inv.Lookup(game.URL)
        disabled := entryExists && entry.UnifiedNamingDisabled
        if !disabled {
            newDest, didRename := roms.ResolveUnifiedDest(dest, game.Title)
            if didRename {
                if renameErr := os.Rename(dest, newDest); renameErr != nil {
                    logger.Warn("unified-naming: rename failed: %v", renameErr)
                } else {
                    logger.Info("unified-naming: renamed %q → %q", filepath.Base(dest), filepath.Base(newDest))
                    finalDest = newDest
                    unifiedName = true
                }
            } else {
                unifiedName = true // name already correct
            }
        }
    }

    if artErr := client.DownloadCoverArt(game.CoverURL, finalDest); artErr != nil {
        logger.Warn("cover-art: game=%q url=%s: %v", game.Title, game.CoverURL, artErr)
    }
    s.inv.Add(game.URL, inventory.Entry{
        GameURL:  game.URL,
        Title:    game.Title,
        Author:   game.Author,
        CoverURL: game.CoverURL,
        IsFree:   game.IsFree,
    }, inventory.DownloadedFile{
        Filename:     upload.Filename,
        DestPath:     finalDest,
        DownloadedAt: time.Now(),
        UnifiedName:  unifiedName,
    })
    if saveErr := s.inv.Save(s.inventoryPath); saveErr != nil {
        logger.Warn("inventory: save failed: %v", saveErr)
    } else {
        logger.Info("inventory: recorded game=%q file=%s unified=%v", game.Title, filepath.Base(finalDest), unifiedName)
    }
    s.dest = finalDest
    s.state = dlDone
}
```

Add the missing imports to `screen_download.go`:

```go
import (
    // existing ...
    "os"
    "path/filepath"

    "github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)
```

Also update the `dlDone` display in `Draw` to show `s.dest` (which is already used on line 143 as `s.dest`). The struct needs `dest` to be updated from `finalDest`:

```go
// In DownloadScreen struct, dest is already there. Update it in the goroutine:
s.dest = finalDest
```

- [ ] **Step 2: Build to verify it compiles**

```bash
go build -tags '!headless' ./... 2>&1
```

Expected: no errors. (Cross-compile is not needed at this stage.)

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_download.go
git commit -m "feat(ui): apply unified naming in download flow"
```

---

## Task 10: Settings UI — global toggle

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 1: Add `sItemUnifiedNaming` to the settings item enum**

In `screen_settings.go`, find the `settingsItem` const block and insert `sItemUnifiedNaming` between `sItemROMLocation` and `sItemNextUITheme` (or at end before `sItemCount`):

```go
const (
    sItemAPIKey settingsItem = iota
    sItemROMMode
    sItemROMLocation
    sItemUnifiedNaming  // ← add this
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

- [ ] **Step 2: Add the toggle row to `Draw`**

Find the existing pattern of how other boolean toggles are drawn (e.g. `NextUITheme`). Add a similar row for `sItemUnifiedNaming`. In the `Draw` method, find the section that renders the settings list rows and add:

```go
case sItemUnifiedNaming:
    label := "Use game title as filename"
    val := "ON"
    if !s.cfg.UnifiedNaming {
        val = "OFF"
    }
    drawRow(label, val, s.cursor == sItemUnifiedNaming)
```

The exact draw code depends on the existing `drawRow` helper pattern. Follow the same pattern used for `sItemNextUITheme`.

- [ ] **Step 3: Handle input for `sItemUnifiedNaming`**

In `HandleEvent` (or the `activate` helper), find the switch on `s.cursor` and add:

```go
case sItemUnifiedNaming:
    s.cfg.UnifiedNaming = !s.cfg.UnifiedNaming
    if err := s.cfg.Save(s.cfgPath); err != nil {
        logger.Warn("settings: save failed: %v", err)
    }
```

- [ ] **Step 4: Build to verify it compiles**

```bash
go build -tags '!headless' ./... 2>&1
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat(ui/settings): add Use game title as filename toggle"
```

---

## Task 11: Migration flow screen

**Files:**
- Create: `internal/ui/screen_migrate_flow.go`

This screen orchestrates the full migration: detect → prompt save → prompt states → run. It implements `SaveDataCallback` so `MigrateFile` can be called synchronously.

- [ ] **Step 1: Create `internal/ui/screen_migrate_flow.go`**

```go
//go:build !headless

package ui

import (
    "fmt"
    "os"
    "strings"

    "github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
    "github.com/carroarmato0/nextui-itchio-pak/internal/logger"
    "github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
    "github.com/carroarmato0/nextui-itchio-pak/internal/roms"
    "github.com/veandco/go-sdl2/sdl"
)

type migrateFlowState int

const (
    mfsCheckSave    migrateFlowState = iota // checking / prompting save
    mfsCheckStates                          // prompting state files
    mfsRunning                              // executing migration
    mfsDone                                 // finished — show result
    mfsError                                // error
)

// MigrateFlowScreen guides the user through the unified-naming migration for
// a single DownloadedFile. It implements SaveDataCallback synchronously by
// collecting answers during the prompt states before calling MigrateFile.
type MigrateFlowScreen struct {
    inv           *inventory.Inventory
    inventoryPath string
    gameURL       string
    gameTitle     string
    file          inventory.DownloadedFile
    enable        bool
    formats       inventory.MigrateFormats
    prev          Screen

    state migrateFlowState

    // Prompt state — save
    saveExists    bool
    savePath      string
    newSavePath   string
    saveConflict  bool // save exists at new path too
    saveAnswer    bool // true = rename, false = skip
    saveAsked     bool

    // Prompt state — overwrite guard
    overwriteAsked  bool
    overwriteAnswer bool

    // Prompt state — states
    existingStates []string
    statesAnswer   bool
    statesAsked    bool

    // Result
    result inventory.MigrateResult
    err    error
}

// NewMigrateFlowScreen creates a migration flow screen and immediately starts
// detection. Caller must push the returned screen onto the screen stack.
func NewMigrateFlowScreen(
    inv *inventory.Inventory,
    invPath string,
    gameURL, gameTitle string,
    file inventory.DownloadedFile,
    enable bool,
    formats inventory.MigrateFormats,
    prev Screen,
) *MigrateFlowScreen {
    s := &MigrateFlowScreen{
        inv: inv, inventoryPath: invPath,
        gameURL: gameURL, gameTitle: gameTitle,
        file: file, enable: enable, formats: formats, prev: prev,
    }
    s.detect()
    return s
}

// detect runs the pre-flight checks to decide whether prompts are needed.
func (s *MigrateFlowScreen) detect() {
    currentPath := s.file.DestPath

    // Determine target path for path comparison.
    var targetPath string
    if s.enable {
        targetPath, _ = roms.ResolveUnifiedDest(currentPath, s.gameTitle)
    } else {
        import_filepath := func(p string) string {
            // inline filepath.Join equivalent — use roms package
            _ = p
            return ""
        }
        _ = import_filepath
        // Use file.Filename as target stem in same dir.
        // (We can't import filepath directly here — use roms helper.)
        // Actually we CAN import filepath; the UI package can use stdlib.
        targetPath = currentPath[:len(currentPath)-len(filepath.Base(currentPath))] + s.file.Filename
    }

    var innerFilename string
    if s.formats.UseExtractedFileName && len(currentPath) > 4 && currentPath[len(currentPath)-4:] == ".zip" {
        innerFilename = roms.ZipInnerFilename(currentPath)
    }

    oldSave := roms.SaveGamePath(currentPath, s.formats.SaveFormat, innerFilename)
    newSave := roms.SaveGamePath(targetPath, s.formats.SaveFormat, innerFilename)

    if oldSave != "" && oldSave != newSave {
        if _, err := os.Stat(oldSave); err == nil {
            s.saveExists = true
            s.savePath = oldSave
            s.newSavePath = newSave
            if _, err2 := os.Stat(newSave); err2 == nil {
                s.saveConflict = true
            }
        }
    }

    coreTag, coreName := roms.RomCoreInfo(currentPath)
    if coreTag != "" {
        allStates := roms.SaveStatePaths(currentPath, s.formats.StateFormat, innerFilename, coreTag, coreName)
        for _, sp := range allStates {
            if _, err := os.Stat(sp); err == nil {
                s.existingStates = append(s.existingStates, sp)
            }
        }
    }

    // Decide initial state.
    if s.saveExists {
        s.state = mfsCheckSave
    } else if len(s.existingStates) > 0 {
        s.state = mfsCheckStates
    } else {
        s.runMigration()
    }
}

// AskRenameExistingSave implements SaveDataCallback.
func (s *MigrateFlowScreen) AskRenameExistingSave(_ string) bool { return s.saveAnswer }

// AskOverwriteExistingSave implements SaveDataCallback.
func (s *MigrateFlowScreen) AskOverwriteExistingSave(_ string) bool { return s.overwriteAnswer }

// AskRenameExistingStates implements SaveDataCallback.
func (s *MigrateFlowScreen) AskRenameExistingStates(_ []string) bool { return s.statesAnswer }

func (s *MigrateFlowScreen) runMigration() {
    s.state = mfsRunning
    res, err := inventory.MigrateFile(
        s.inv, s.inventoryPath, s.gameURL, s.file,
        s.gameTitle, s.enable, s.formats, s,
    )
    if err != nil {
        s.err = err
        s.state = mfsError
    } else {
        s.result = res
        s.state = mfsDone
    }
}

func (s *MigrateFlowScreen) Draw(r *renderer.Renderer) {
    bg := r.Theme.Background
    r.Clear(bg[0], bg[1], bg[2])

    footerH := int32(52)
    _, fontH := r.TextSize("Ag")
    _, smallFH := r.SmallTextSize("Ag")
    hdr := r.Theme.HeaderBG
    ac := r.Theme.Accent
    mt := r.Theme.MainText
    ht := r.Theme.HintText

    headerH := fontH + smallFH + 16
    r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
    r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])

    action := "Enabling"
    if !s.enable {
        action = "Disabling"
    }
    r.DrawText(truncateToWidth(r, action+" title filename — "+s.gameTitle, r.W-24), 12, 8, mt[0], mt[1], mt[2])

    mid := headerH + (r.H-headerH-footerH)/2

    switch s.state {
    case mfsCheckSave:
        saveBase := s.savePath
        if len(saveBase) > 40 {
            saveBase = "…" + saveBase[len(saveBase)-40:]
        }
        r.DrawTextCentered("Save file detected", 0, mid-fontH*2, r.W, 240, 180, 60)
        r.DrawSmallTextCentered(saveBase, 0, mid-fontH, r.W, ht[0], ht[1], ht[2])
        if s.saveConflict && s.saveAsked {
            r.DrawSmallTextCentered("A save already exists at the new path.", 0, mid, r.W, 200, 100, 100)
            r.DrawSmallTextCentered("Overwrite it?", 0, mid+smallFH+4, r.W, ht[0], ht[1], ht[2])
            ftrY := r.DrawFooterBar(footerH)
            r.DrawFooterHints([]renderer.FooterHint{
                {Kind: renderer.BadgeCircle, Label: "B", Text: "Overwrite"},
                {Kind: renderer.BadgeCircle, Label: "A", Text: "Cancel"},
            }, ftrY)
        } else {
            r.DrawSmallTextCentered("Rename it to match the new ROM name?", 0, mid, r.W, ht[0], ht[1], ht[2])
            r.DrawSmallTextCentered("If you skip, your save will not load until renamed manually.", 0, mid+smallFH+4, r.W, 120, 120, 120)
            ftrY := r.DrawFooterBar(footerH)
            r.DrawFooterHints([]renderer.FooterHint{
                {Kind: renderer.BadgeCircle, Label: "B", Text: "Rename save"},
                {Kind: renderer.BadgeCircle, Label: "A", Text: "Skip"},
            }, ftrY)
        }

    case mfsCheckStates:
        r.DrawTextCentered("Save states detected", 0, mid-fontH*2, r.W, 240, 180, 60)
        r.DrawSmallTextCentered(fmt.Sprintf("%d save state(s) found.", len(s.existingStates)), 0, mid-fontH, r.W, ht[0], ht[1], ht[2])
        r.DrawSmallTextCentered("Rename them to match the new ROM name?", 0, mid, r.W, ht[0], ht[1], ht[2])
        r.DrawSmallTextCentered("If you skip, they will not load until renamed manually.", 0, mid+smallFH+4, r.W, 120, 120, 120)
        ftrY := r.DrawFooterBar(footerH)
        r.DrawFooterHints([]renderer.FooterHint{
            {Kind: renderer.BadgeCircle, Label: "B", Text: "Rename states"},
            {Kind: renderer.BadgeCircle, Label: "A", Text: "Skip"},
        }, ftrY)

    case mfsRunning:
        r.DrawTextCentered("Renaming…", 0, mid, r.W, mt[0], mt[1], mt[2])

    case mfsDone:
        r.DrawTextCentered("Done!", 0, mid-fontH, r.W, 80, 200, 80)
        summary := s.buildSummary()
        r.DrawSmallTextCentered(summary, 0, mid+4, r.W, ht[0], ht[1], ht[2])
        ftrY := r.DrawFooterBar(footerH)
        r.DrawFooterHints([]renderer.FooterHint{
            {Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
        }, ftrY)

    case mfsError:
        msg := "Error"
        if s.err != nil {
            msg = s.err.Error()
        }
        r.DrawTextCentered("Migration failed", 0, mid-fontH, r.W, 200, 60, 60)
        r.DrawSmallTextCentered(msg, 0, mid+4, r.W, 200, 100, 100)
        ftrY := r.DrawFooterBar(footerH)
        r.DrawFooterHints([]renderer.FooterHint{
            {Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
        }, ftrY)
    }

    r.Present()
}

func (s *MigrateFlowScreen) buildSummary() string {
    var parts []string
    if s.result.ROMRenamed {
        parts = append(parts, "ROM renamed")
    }
    if s.result.SaveRenamed {
        parts = append(parts, "save renamed")
    }
    if s.result.SaveSkipped {
        parts = append(parts, "save skipped")
    }
    if len(s.result.StatesRenamed) > 0 {
        parts = append(parts, fmt.Sprintf("%d state(s) renamed", len(s.result.StatesRenamed)))
    }
    if len(parts) == 0 {
        return "No changes needed."
    }
    return strings.Join(parts, ", ") + "."
}

func (s *MigrateFlowScreen) HandleEvent(e sdl.Event) Screen {
    switch ev := e.(type) {
    case *sdl.KeyboardEvent:
        if ev.Type != sdl.KEYDOWN {
            return s
        }
        return s.handleKey(ev.Keysym.Sym)
    case *sdl.ControllerButtonEvent:
        if ev.Type != sdl.CONTROLLERBUTTONDOWN {
            return s
        }
        return s.handleButton(ev.Button)
    }
    return s
}

func (s *MigrateFlowScreen) handleKey(sym sdl.Keycode) Screen {
    switch s.state {
    case mfsCheckSave:
        switch sym {
        case sdl.K_RETURN: // B = Rename / Overwrite
            return s.handleSaveConfirm()
        case sdl.K_ESCAPE: // A = Skip / Cancel
            return s.handleSaveSkip()
        }
    case mfsCheckStates:
        switch sym {
        case sdl.K_RETURN: // B = Rename
            s.statesAnswer = true
        case sdl.K_ESCAPE: // A = Skip
            s.statesAnswer = false
        }
        s.runMigration()
    case mfsDone, mfsError:
        switch sym {
        case sdl.K_RETURN, sdl.K_ESCAPE:
            return s.prev
        }
    }
    return s
}

func (s *MigrateFlowScreen) handleButton(btn uint8) Screen {
    switch s.state {
    case mfsCheckSave:
        switch btn {
        case sdl.CONTROLLER_BUTTON_B: // physical A = Rename / Overwrite
            return s.handleSaveConfirm()
        case sdl.CONTROLLER_BUTTON_A: // physical B = Skip / Cancel
            return s.handleSaveSkip()
        }
    case mfsCheckStates:
        switch btn {
        case sdl.CONTROLLER_BUTTON_B: // Rename
            s.statesAnswer = true
        case sdl.CONTROLLER_BUTTON_A: // Skip
            s.statesAnswer = false
        }
        s.runMigration()
    case mfsDone, mfsError:
        switch btn {
        case sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A:
            return s.prev
        }
    }
    return s
}

func (s *MigrateFlowScreen) handleSaveConfirm() Screen {
    if s.saveConflict && !s.saveAsked {
        // First A/B press = user confirmed rename; now ask about overwrite.
        s.saveAnswer = true
        s.saveAsked = true
        return s // stay on screen to show overwrite prompt
    }
    if s.saveConflict && s.saveAsked {
        // Second press = user confirmed overwrite.
        s.overwriteAnswer = true
    } else {
        s.saveAnswer = true
    }
    s.advanceFromSave()
    return s
}

func (s *MigrateFlowScreen) handleSaveSkip() Screen {
    if s.saveConflict && s.saveAsked {
        // Cancel overwrite → abort migration.
        s.overwriteAnswer = false
        s.advanceFromSave()
        return s
    }
    s.saveAnswer = false
    s.advanceFromSave()
    return s
}

func (s *MigrateFlowScreen) advanceFromSave() {
    if len(s.existingStates) > 0 {
        s.state = mfsCheckStates
    } else {
        s.runMigration()
    }
}
```

Note: you will need to add `"path/filepath"` to the import block of this file.

- [ ] **Step 2: Build to verify it compiles**

```bash
go build -tags '!headless' ./... 2>&1
```

Fix any compile errors (missing imports, unused variables). The `import_filepath` placeholder in `detect()` should be replaced with a proper `filepath.Dir(currentPath) + "/" + s.file.Filename` or equivalent. Use `filepath.Join(filepath.Dir(currentPath), s.file.Filename)` instead.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_migrate_flow.go
git commit -m "feat(ui): add MigrateFlowScreen for guided ROM/save/state migration"
```

---

## Task 12: Per-game toggle — screen_manage_downloads + screen_detail

**Files:**
- Modify: `internal/ui/screen_manage_downloads.go`
- Modify: `internal/ui/screen_detail.go`

### screen_manage_downloads.go

- [ ] **Step 1: Add toggle row to the cursor space**

In `ManageDownloadsScreen`, the cursor currently covers `0..len(files)` (len = "Delete all"). Add one more row: `len(files)+1` = unified naming toggle.

In `HandleEvent`, update `rowCount`:
```go
rowCount := len(entry.Files) + 2 // +1 delete-all, +1 unified-naming toggle
```

In `Draw`, after the "Delete all" row, add:

```go
sep2Y := deleteAllY + rowH + 4
r.DrawRect(margin, sep2Y, r.W-margin*2, 1, 50, 50, 50)
toggleY := sep2Y + 8
toggleIdx := len(entry.Files) + 1
unifiedDisabled := entry.UnifiedNamingDisabled
toggleLabel := "Use game title as filename"
toggleVal := "ON"
if unifiedDisabled {
    toggleVal = "OFF"
}
if s.cursor == toggleIdx && !s.confirmActive {
    r.DrawPill(4, toggleY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
    r.DrawText(toggleLabel, margin, toggleY, at[0], at[1], at[2])
    tw, _ := r.TextSize(toggleLabel)
    r.DrawText(toggleVal, margin+tw+16, toggleY, at[0], at[1], at[2])
} else {
    // greyed out when global UnifiedNaming is off
    textColor := [3]uint8{lt[0], lt[1], lt[2]}
    if !s.cfg.UnifiedNaming {
        textColor = [3]uint8{80, 80, 80}
    }
    r.DrawText(toggleLabel, margin, toggleY, textColor[0], textColor[1], textColor[2])
    tw, _ := r.TextSize(toggleLabel)
    r.DrawText(toggleVal, margin+tw+16, toggleY, textColor[0], textColor[1], textColor[2])
}
```

`ManageDownloadsScreen` will need access to `*settings.Config`. Add it to the struct and constructor:

```go
type ManageDownloadsScreen struct {
    // ... existing ...
    cfg *settings.Config
}

func NewManageDownloadsScreen(inv *inventory.Inventory, inventoryPath string, gameURL string, cfg *settings.Config, prev Screen) *ManageDownloadsScreen {
    return &ManageDownloadsScreen{
        inv: inv, inventoryPath: inventoryPath, gameURL: gameURL,
        cfg: cfg, prev: prev, confirmFileIdx: -1,
    }
}
```

Update all callers of `NewManageDownloadsScreen` (currently one, in `screen_detail.go:782`) to pass `s.cfg`.

In `HandleEvent`, add handling for `toggleIdx`:

```go
case sdl.CONTROLLER_BUTTON_B: // physical A = select
    if s.cursor == len(entry.Files) {
        // Delete all
        s.confirmActive = true
        s.confirmFileIdx = -1
    } else if s.cursor == len(entry.Files)+1 {
        // Unified naming toggle
        if s.cfg.UnifiedNaming {
            s.activateUnifiedNamingToggle(entry)
        }
    } else {
        s.confirmActive = true
        s.confirmFileIdx = s.cursor
    }
```

Add the method:

```go
func (s *ManageDownloadsScreen) activateUnifiedNamingToggle(entry inventory.Entry) {
    if len(entry.Files) == 0 {
        return
    }
    // Toggle per-game setting and migrate each file.
    newDisabled := !entry.UnifiedNamingDisabled
    s.inv.SetUnifiedNamingDisabled(s.gameURL, newDisabled)
    formats := inventory.ReadMigrateFormats(inventory.NXSettingsPath)
    enable := !newDisabled
    for _, f := range entry.Files {
        screen := NewMigrateFlowScreen(s.inv, s.inventoryPath, s.gameURL, entry.Title, f, enable, formats, s)
        _ = screen // push onto stack — see note below
    }
}
```

**Note:** For simplicity in this first implementation, `activateUnifiedNamingToggle` will push only the first file's migration flow. Multi-file migration (iterating over all files) requires chaining screens, which is complex. Ship single-file support first; multi-file can be a follow-up.

Replace the `_ = screen` with a proper screen push. Since `HandleEvent` returns a `Screen`, return the MigrateFlowScreen instead:

```go
func (s *ManageDownloadsScreen) HandleEvent(e sdl.Event) Screen {
    // ...
    case sdl.CONTROLLER_BUTTON_B:
        if s.cursor == len(entry.Files)+1 && s.cfg.UnifiedNaming {
            return s.startUnifiedNamingMigration(entry)
        }
        // ... existing cases
}

func (s *ManageDownloadsScreen) startUnifiedNamingMigration(entry inventory.Entry) Screen {
    if len(entry.Files) == 0 {
        return s
    }
    newDisabled := !entry.UnifiedNamingDisabled
    s.inv.SetUnifiedNamingDisabled(s.gameURL, newDisabled)
    formats := inventory.ReadMigrateFormats(inventory.NXSettingsPath)
    return NewMigrateFlowScreen(s.inv, s.inventoryPath, s.gameURL, entry.Title,
        entry.Files[0], !newDisabled, formats, s)
}
```

### screen_detail.go

- [ ] **Step 2: Add per-game toggle to the detail screen**

In `screen_detail.go`, in the `Draw` method, after the existing Download/Delete action rows, add a unified-naming toggle action row (visible only when a download exists):

```go
if isPresent && s.cfg.UnifiedNaming {
    entry, _ := s.inv.Lookup(s.game.URL)
    toggleLabel := "Disable title filename"
    if entry.UnifiedNamingDisabled {
        toggleLabel = "Enable title filename"
    }
    drawActionRow("Y", toggleLabel, mt[0], mt[1], mt[2], ac[0], ac[1], ac[2], 0)
}
```

In `HandleEvent`, add Y-button handling:

```go
case sdl.CONTROLLER_BUTTON_Y:
    if s.inv.IsPresent(s.game.URL) && s.cfg.UnifiedNaming {
        return s.startUnifiedNamingToggle()
    }
```

Add the method:

```go
func (s *DetailScreen) startUnifiedNamingToggle() Screen {
    entry, ok := s.inv.Lookup(s.game.URL)
    if !ok || len(entry.Files) == 0 {
        return s
    }
    newDisabled := !entry.UnifiedNamingDisabled
    s.inv.SetUnifiedNamingDisabled(s.game.URL, newDisabled)
    formats := inventory.ReadMigrateFormats(inventory.NXSettingsPath)
    return NewMigrateFlowScreen(s.inv, s.inventoryPath, s.game.URL, s.game.Title,
        entry.Files[0], !newDisabled, formats, s)
}
```

Import `inventory` package in `screen_detail.go` (it's already imported).

- [ ] **Step 3: Build to verify it compiles**

```bash
go build -tags '!headless' ./... 2>&1
```

Fix any compile errors.

- [ ] **Step 4: Run full test suite**

```bash
go test ./... 2>&1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_manage_downloads.go internal/ui/screen_detail.go
git commit -m "feat(ui): add per-game unified naming toggle to detail and manage-downloads screens"
```

---

## Final verification

- [ ] **Run all tests**

```bash
go test ./... 2>&1
```

Expected output: all packages pass, zero failures.

- [ ] **Build cross-compile check (optional but recommended)**

```bash
scripts/build.sh 2>&1 | tail -20
```

Expected: binaries produced for all platforms without errors.

---

## Post-implementation fixes

The following bugs were found during device testing after the initial plan was
completed. All are committed on the same feature branch.

### UI: Migrate flow A/B button labels swapped
`internal/ui/screen_migrate_flow.go` — The three confirmation prompts (save
rename, overwrite, save-state rename) had SDL `CONTROLLER_BUTTON_B` labelled
as "B: Rename" and `CONTROLLER_BUTTON_A` as "A: Skip". NextUI convention is
physical-A = SDL-B (confirm) and physical-B = SDL-A (back/cancel). Labels
corrected to match button identity.

### UI: Migrate flow header not vertically centred
`internal/ui/screen_migrate_flow.go` — Header text Y was hardcoded to `8`.
Changed to `(headerH - fontH) / 2` to centre it within the header bar
regardless of font size.

### Inventory: HasPendingUpdates false positive with format-picker files
`internal/inventory/inventory.go` — `HasPendingUpdates` compared upstream
filenames against the raw `f.Filename` stored in the inventory. The
format-picker appends an extension the upstream name doesn't carry
(`"Game Boy ROM.gbc"` vs `"Game Boy ROM"`), causing the downloaded file
to be treated as missing and generating a spurious [UP] badge. Fixed by also
indexing the stem (filename minus extension) in the `downloaded` map.
Added two tests: `TestHasPendingUpdates_FormatPickerExtensionMismatch` and
`TestHasPendingUpdates_FormatPickerWithGenuineNewFile`.

### UI: List not rebuilt after game deletion in DL sort mode
`internal/ui/screen_list.go`, `screen_detail.go`, `screen_manage_downloads.go`
— Added `Rebuildable` interface with `ScheduleRebuild()` on `ListScreen`. A
`needsRebuild` flag is consumed at the top of `Draw()`. All deletion paths
(detail screen, manage-downloads screen) now call `ScheduleRebuild()` on their
`prev` chain.

### UI: List not rebuilt when update-svc completes; DL separator drawn behind titles
`cmd/itchio-pak/main_sdl.go`, `internal/ui/screen_list.go` — Added
`userEventInventoryUpdate` interception in the main SDL loop to call
`listScreen.ScheduleRebuild()`. Moved the DL separator draw to after the row
loop so it renders on top of row content rather than behind it.

### UI: Fix blank ROM picker (FetchUploadsScreen)
`cmd/itchio-pak/main_sdl.go` — The `userEventInventoryUpdate` intercept used
`continue` which prevented the event from reaching `current.HandleEvent(e)`.
`FetchUploadsScreen` uses the same UserEvent code 0 for its goroutine-done
signal, so `HandleEvent` was never called and the screen stayed blank until
the user pressed a button. Removed the `continue`.

### UI: 'Saved to' path overflows on narrow screens
`internal/ui/screen_download.go` — The single-line `"Saved to: " + dest` was
replaced with three centred lines: label, directory part, and filename.
Added `truncateSmallToWidth` helper in `screen_list.go`.

### UI: DL separator visual overhaul
`internal/ui/screen_list.go` — Replaced the thin 1 px line + floating text
with a filled bar (`rgb(40,40,40)`, height `smallFH + 8`) with centred label.
Rows in the downloaded group are shifted down by `dlSepBarH` so the bar
occupies its own dedicated slot with no overlap on either adjacent row's
selection highlight. The loop termination changed from `rowIdx >= visibleRows`
to a coordinate-based check (`rowTop >= screenBottom`) to account for the
offset correctly.

### UI: DL-mode scroll broken when cursor is in downloaded group
`internal/ui/screen_list.go` — `startIdx` was computed from plain
`visibleRows` without accounting for the separator gap. When the separator is
in the viewport, the effective visible rows for the downloaded group is
`(contentH - dlSepBarH) / rowH`. Added a two-case calculation: separator
in viewport (reduced effective rows) and separator scrolled off the top
(plain formula, `yOff = 0`).

### UI: Redundant X/Y footer hints on detail screen
`internal/ui/screen_detail.go` — The footer bar repeated hints for X (Delete)
and Y (Title filename) that are already shown as labelled action rows in the
content area. Removed them to give the footer enough room to fit on the
Miyoo Flip (640 px wide).

### Inventory: Dismissed updates reappear after restart
`internal/inventory/inventory.go` — `SetUpstreamFiles` now preserves `SeenAt`
for files that already appear in `KnownUpstreamFiles`. Previously every check
cycle stamped `SeenAt = time.Now()`, making `SeenAt > UpdateDismissedAt` after
any restart and causing dismissed updates to reappear. Added regression test
`TestUpdateService_DismissedUpdateDoesNotReappearOnRestart`.

### Inventory: DL sort list jumps during update-svc run
`internal/inventory/updater.go` — `checkFreeGame` now returns `[]UpstreamFile`
instead of calling `SetUpstreamFiles` directly. `runCheck` collects all file
lists into a `pendingFiles` map and applies them in a single batch at the end,
followed by a single `Save`. Previously `SetUpstreamFiles` was called
per-entry mid-check, so `HasPendingUpdates` (read on every draw tick) would
flip for individual games while sort order was still stale, causing the list
to visibly re-sort game-by-game until the final rebuild.
