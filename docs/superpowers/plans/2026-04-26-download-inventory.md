# Download Inventory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track downloaded games in a persistent inventory so the UI can display download status, prevent duplicate confusion, and allow per-file or full-game deletion.

**Architecture:** New `internal/inventory` package owns the data model and JSON persistence. A `*Inventory` pointer is threaded through the screen constructor chain at startup. The list and detail screens read from it on each Draw; the download screen writes to it on success.

**Tech Stack:** Go 1.22, standard library (`encoding/json`, `os`, `path/filepath`, `net/url`), `internal/logger`, SDL2/ttf for renderer additions.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/inventory/inventory.go` | Create | Types, Load, Save, Add, Remove, Lookup, IsPresent, VerifyAndClean, CoverArtPath |
| `internal/inventory/inventory_test.go` | Create | Unit tests for all inventory methods |
| `internal/renderer/text.go` | Modify | Exempt U+1F4BE from `isEmoji` |
| `internal/renderer/text_test.go` | Modify | Add floppy disk test case |
| `internal/renderer/renderer.go` | Modify | Add `DrawBoldText`, `BoldTextSize` |
| `cmd/itchio-pak/main_sdl.go` | Modify | Load inventory, run VerifyAndClean, pass to ListScreen |
| `internal/ui/screen_list.go` | Modify | Accept inventory; show floppy icon + bold title for present games; add `truncateBoldToWidth` helper |
| `internal/ui/screen_detail.go` | Modify | Accept inventory; show download status + X button; modal kind field; single-file delete |
| `internal/ui/screen_fetch_uploads.go` | Modify | Accept + thread `inv`, `inventoryPath` |
| `internal/ui/screen_rom_picker.go` | Modify | Accept + thread `inv`, `inventoryPath` |
| `internal/ui/screen_location_picker.go` | Modify | Accept + thread `inv`, `inventoryPath` |
| `internal/ui/screen_format_picker.go` | Modify | Accept + thread `inv`, `inventoryPath` |
| `internal/ui/screen_download.go` | Modify | Accept `inv`, `inventoryPath`; record download on success |
| `internal/ui/screen_manage_downloads.go` | Create | Multi-file deletion picker screen |

---

## Task 1: Inventory package — types, Load, Save, Add, Remove, Lookup

**Files:**
- Create: `internal/inventory/inventory.go`
- Create: `internal/inventory/inventory_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/inventory/inventory_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
go test -race -tags headless ./internal/inventory/...
```

Expected: `cannot find package` or `no Go files` — the package doesn't exist yet.

- [ ] **Step 3: Create `internal/inventory/inventory.go`**

```go
package inventory

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DownloadedFile struct {
	Filename     string    `json:"filename"`
	DestPath     string    `json:"dest_path"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type Entry struct {
	GameURL    string           `json:"game_url"`
	Title      string           `json:"title"`
	Author     string           `json:"author"`
	CoverURL   string           `json:"cover_url"`
	Files      []DownloadedFile `json:"files"`
	VerifiedAt time.Time        `json:"verified_at,omitempty"`
}

type Inventory struct {
	Entries map[string]*Entry `json:"entries"`
}

// Load reads the inventory from path. Returns an empty inventory if the file
// is missing or unparseable — never returns an error for those cases.
func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return &Inventory{Entries: make(map[string]*Entry)}, nil
	}
	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return &Inventory{Entries: make(map[string]*Entry)}, nil
	}
	if inv.Entries == nil {
		inv.Entries = make(map[string]*Entry)
	}
	return &inv, nil
}

// Save writes the inventory to path atomically (write to .tmp then rename).
func (inv *Inventory) Save(path string) error {
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write inventory tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename inventory: %w", err)
	}
	return nil
}

// Add upserts an entry and appends a file, deduplicating by DestPath.
func (inv *Inventory) Add(gameURL string, e Entry, file DownloadedFile) {
	existing, ok := inv.Entries[gameURL]
	if !ok {
		entry := &Entry{
			GameURL:  gameURL,
			Title:    e.Title,
			Author:   e.Author,
			CoverURL: e.CoverURL,
		}
		inv.Entries[gameURL] = entry
		existing = entry
	} else {
		existing.Title = e.Title
		existing.Author = e.Author
		existing.CoverURL = e.CoverURL
	}
	for _, f := range existing.Files {
		if f.DestPath == file.DestPath {
			return
		}
	}
	existing.Files = append(existing.Files, file)
}

// Remove deletes the entry for gameURL.
func (inv *Inventory) Remove(gameURL string) {
	delete(inv.Entries, gameURL)
}

// Lookup returns the entry for gameURL.
func (inv *Inventory) Lookup(gameURL string) (*Entry, bool) {
	e, ok := inv.Entries[gameURL]
	return e, ok
}

// IsPresent reports whether gameURL has an inventory entry with at least one file.
// Assumes VerifyAndClean has already removed entries whose files are gone from disk.
func (inv *Inventory) IsPresent(gameURL string) bool {
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	return len(e.Files) > 0
}

// CoverArtPath returns the filesystem path for the cover art of a downloaded ROM,
// mirroring the naming convention used by itchio.DownloadCoverArt.
// Returns "" if either argument is empty.
func CoverArtPath(coverURL, romDestPath string) string {
	if coverURL == "" || romDestPath == "" {
		return ""
	}
	ext := ".png"
	if u, err := url.Parse(coverURL); err == nil {
		if e := filepath.Ext(u.Path); e != "" {
			ext = e
		}
	}
	dir := filepath.Dir(romDestPath)
	base := strings.TrimSuffix(filepath.Base(romDestPath), filepath.Ext(romDestPath))
	return filepath.Join(dir, ".media", base+ext)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -tags headless ./internal/inventory/...
```

Expected: `ok  github.com/carroarmato0/nextui-itchio-pak/internal/inventory`

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/
git commit -m "feat(inventory): add inventory package with Load, Save, Add, Remove, Lookup"
```

---

## Task 2: Inventory package — IsPresent and VerifyAndClean

**Files:**
- Modify: `internal/inventory/inventory.go` (add `VerifyAndClean`, logger import)
- Modify: `internal/inventory/inventory_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/inventory/inventory_test.go`:

```go
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

func TestCoverArtPath_WithPNGCover(t *testing.T) {
	got := inventory.CoverArtPath(
		"https://img.itch.zone/abc/cover.png",
		"/mnt/SDCARD/Roms/Game Boy (GB)/my-game.gb",
	)
	want := "/mnt/SDCARD/Roms/Game Boy (GB)/.media/my-game.png"
	if got != want {
		t.Errorf("CoverArtPath = %q, want %q", got, want)
	}
}

func TestCoverArtPath_EmptyCoverURL_DefaultsPNG(t *testing.T) {
	got := inventory.CoverArtPath("", "/roms/game.gb")
	if got != "" {
		t.Errorf("empty coverURL should return empty string, got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify new tests fail**

```bash
go test -race -tags headless ./internal/inventory/...
```

Expected: `FAIL` — `VerifyAndClean` is undefined.

- [ ] **Step 3: Add `VerifyAndClean` to `internal/inventory/inventory.go`**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/logger"` to the import block, then add the method:

```go
// VerifyAndClean removes DownloadedFile entries whose paths no longer exist on
// disk. Entries with no remaining files are deleted entirely. Saves if any
// entries were removed. Returns the number of DownloadedFile rows removed.
func (inv *Inventory) VerifyAndClean(path string) int {
	removed := 0
	for gameURL, entry := range inv.Entries {
		var kept []DownloadedFile
		for _, f := range entry.Files {
			if _, err := os.Stat(f.DestPath); err == nil {
				kept = append(kept, f)
			} else {
				logger.Debug("inventory: removing stale file=%s", f.DestPath)
				removed++
			}
		}
		if len(kept) == 0 {
			logger.Debug("inventory: removing empty entry game=%q", entry.Title)
			delete(inv.Entries, gameURL)
		} else if len(kept) < len(entry.Files) {
			entry.Files = kept
			entry.VerifiedAt = time.Now()
		} else {
			entry.VerifiedAt = time.Now()
		}
	}
	if removed > 0 {
		_ = inv.Save(path)
	}
	return removed
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -tags headless ./internal/inventory/...
```

Expected: `ok  github.com/carroarmato0/nextui-itchio-pak/internal/inventory`

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/
git commit -m "feat(inventory): add IsPresent, VerifyAndClean, CoverArtPath"
```

---

## Task 3: Renderer — exempt U+1F4BE, add DrawBoldText and BoldTextSize

**Files:**
- Modify: `internal/renderer/text.go`
- Modify: `internal/renderer/text_test.go`
- Modify: `internal/renderer/renderer.go`

- [ ] **Step 1: Add floppy disk test case to `internal/renderer/text_test.go`**

In the `cases` slice inside `TestSanitizeText`, add before the closing `}`:

```go
{"floppy disk U+1F4BE passes through", "\U0001F4BE", "\U0001F4BE"},
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -race -tags headless ./internal/renderer/...
```

Expected: `FAIL` — `sanitizeText("\U0001F4BE")` currently returns `""`.

- [ ] **Step 3: Exempt U+1F4BE in `internal/renderer/text.go`**

Replace the `isEmoji` function body:

```go
func isEmoji(r rune) bool {
	if r == 0x1F4BE { // floppy disk — used as download indicator
		return false
	}
	return (r >= 0x2600 && r <= 0x26FF) || // Miscellaneous Symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0x2B00 && r <= 0x2BFF) || // Miscellaneous Symbols and Arrows
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F650 && r <= 0x1F67F) || // Ornamental Dingbats
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1FFFF) // Various supplementary emoji blocks
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -tags headless ./internal/renderer/...
```

Expected: `ok  github.com/carroarmato0/nextui-itchio-pak/internal/renderer`

- [ ] **Step 5: Add `DrawBoldText` and `BoldTextSize` to `internal/renderer/renderer.go`**

Add after the `DrawText` method (around line 116):

```go
// DrawBoldText renders text using SDL_ttf bold style synthesis.
func (r *Renderer) DrawBoldText(text string, x, y int32, red, green, blue uint8) {
	r.Font.SetStyle(ttf.STYLE_BOLD)
	defer r.Font.SetStyle(ttf.STYLE_NORMAL)
	r.DrawText(text, x, y, red, green, blue)
}

// BoldTextSize returns the pixel width and height of text measured in bold style.
func (r *Renderer) BoldTextSize(text string) (int32, int32) {
	r.Font.SetStyle(ttf.STYLE_BOLD)
	defer r.Font.SetStyle(ttf.STYLE_NORMAL)
	return r.TextSize(text)
}
```

- [ ] **Step 6: Verify the build tag file compiles**

```bash
go build -tags headless ./internal/renderer/...
```

Expected: no output (clean build). SDL renderer files are excluded by the headless tag — this verifies the non-SDL renderer files are clean.

- [ ] **Step 7: Commit**

```bash
git add internal/renderer/text.go internal/renderer/text_test.go internal/renderer/renderer.go
git commit -m "feat(renderer): exempt floppy disk glyph, add DrawBoldText and BoldTextSize"
```

---

## Task 4: Startup wiring and ListScreen constructor

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Update `internal/ui/screen_list.go` — add inventory fields and update constructor**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"` to the import block.

Add two fields to the `ListScreen` struct (after `cachePath string`):

```go
inv           *inventory.Inventory
inventoryPath string
```

Update `NewListScreen` signature and body — replace the existing `func NewListScreen(...)` line and the first line of the body:

```go
func NewListScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, cachePath string, inv *inventory.Inventory, inventoryPath string) *ListScreen {
	s := &ListScreen{
		client:        client,
		cfg:           cfg,
		cache:         cache,
		page:          1,
		cfgPath:       cfgPath,
		cachePath:     cachePath,
		inv:           inv,
		inventoryPath: inventoryPath,
	}
```

- [ ] **Step 2: Update `cmd/itchio-pak/main_sdl.go`**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"` to the import block.

After the `cachePath` line (line 19), add:

```go
inventoryPath := filepath.Join(filepath.Dir(cfgPath), "inventory.json")
inv, _ := inventory.Load(inventoryPath)
removed := inv.VerifyAndClean(inventoryPath)
logger.Info("inventory: cleaned %d stale file(s)", removed)
```

Update the `NewListScreen` call to pass inventory:

```go
var current ui.Screen = ui.NewListScreen(client, cfg, cfgPath, cache, cachePath, inv, inventoryPath)
```

- [ ] **Step 3: Verify the project still compiles**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add cmd/itchio-pak/main_sdl.go internal/ui/screen_list.go
git commit -m "feat(inventory): wire inventory into startup and ListScreen"
```

---

## Task 5: Thread inventory through the download screen chain

This task adds `inv *inventory.Inventory` and `inventoryPath string` to every screen constructor between `ListScreen` and `DownloadScreen`. No behaviour changes yet — just plumbing.

**Files:**
- Modify: `internal/ui/screen_detail.go`
- Modify: `internal/ui/screen_fetch_uploads.go`
- Modify: `internal/ui/screen_rom_picker.go`
- Modify: `internal/ui/screen_location_picker.go`
- Modify: `internal/ui/screen_format_picker.go`
- Modify: `internal/ui/screen_download.go`

- [ ] **Step 1: Update `internal/ui/screen_detail.go`**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"`.

Add to `DetailScreen` struct (after `prev Screen`):

```go
inv           *inventory.Inventory
inventoryPath string
```

Update `NewDetailScreen` signature:

```go
func NewDetailScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, inv *inventory.Inventory, inventoryPath string, prev Screen) *DetailScreen {
	s := &DetailScreen{client: client, cfg: cfg, cfgPath: cfgPath, cache: cache, game: game, prev: prev, loading: true, inv: inv, inventoryPath: inventoryPath}
```

Update the `startDownload` method to pass inventory to `NewFetchUploadsScreen`:

```go
func (s *DetailScreen) startDownload() Screen {
	if s.loading {
		return s
	}
	if !s.game.IsFree && s.cfg.APIKey == "" {
		return s
	}
	return NewFetchUploadsScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s.inv, s.inventoryPath, s)
}
```

Update `NewDetailScreen` calls in `screen_list.go` HandleEvent (both the keyboard and controller `return NewDetailScreen(...)` lines):

```go
return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.games[s.cursor], s.inv, s.inventoryPath, s)
```

- [ ] **Step 2: Update `internal/ui/screen_fetch_uploads.go`**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"`.

Add to `FetchUploadsScreen` struct:

```go
inv           *inventory.Inventory
inventoryPath string
```

Update `NewFetchUploadsScreen` signature:

```go
func NewFetchUploadsScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail,
	inv *inventory.Inventory, inventoryPath string,
	prev Screen,
) *FetchUploadsScreen {
	s := &FetchUploadsScreen{
		client: client, cfg: cfg, cfgPath: cfgPath,
		cache: cache, game: game, detail: detail, prev: prev,
		state: fetchLoading,
		inv: inv, inventoryPath: inventoryPath,
	}
```

Update `nextScreen` to thread inventory — replace the three `NewDownloadScreen` and `NewROMPickerScreen` and `NewLocationPickerScreen` and `NewFormatPickerScreen` calls:

```go
func (s *FetchUploadsScreen) nextScreen() Screen {
	var known, unknown []roms.Upload
	for _, u := range s.uploads {
		if u.NeedsFormat {
			unknown = append(unknown, u)
		} else {
			known = append(known, u)
		}
	}

	if len(known) > 0 {
		if len(known) == 1 {
			upload := known[0]
			if s.cfg.ROMLocation == "ask" {
				return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
			}
			ext := strings.ToLower(filepath.Ext(upload.Filename))
			dest := roms.DestinationDir(ext) + upload.Filename
			return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
		}
		return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, known, s.inv, s.inventoryPath, s.prev)
	}
	return NewFormatPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, unknown, s.inv, s.inventoryPath, s.prev)
}
```

- [ ] **Step 3: Update `internal/ui/screen_rom_picker.go`**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"`.

Add to `ROMPickerScreen` struct:

```go
inv           *inventory.Inventory
inventoryPath string
```

Update `NewROMPickerScreen` signature:

```go
func NewROMPickerScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail, uploads []roms.Upload, inv *inventory.Inventory, inventoryPath string, prev Screen) *ROMPickerScreen {
	return &ROMPickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, uploads: uploads, prev: prev,
		inv: inv, inventoryPath: inventoryPath,
	}
}
```

Update `chooseUpload` to thread inventory:

```go
func (s *ROMPickerScreen) chooseUpload(upload roms.Upload) Screen {
	if s.cfg.ROMLocation == "ask" {
		return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
	}
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
}
```

- [ ] **Step 4: Update `internal/ui/screen_location_picker.go`**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"`.

Add to `LocationPickerScreen` struct (after `scrollOffset int`):

```go
inv           *inventory.Inventory
inventoryPath string
```

Update `NewLocationPickerScreen` signature — add `inv *inventory.Inventory, inventoryPath string` before `prev Screen`:

```go
func NewLocationPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail, upload roms.Upload,
	inv *inventory.Inventory, inventoryPath string,
	prev Screen,
) *LocationPickerScreen {
```

Add `inv: inv, inventoryPath: inventoryPath,` to the struct literal inside that constructor.

Find the `NewDownloadScreen` call in `screen_location_picker.go` (line 346) and update it:

```go
return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.upload, dest, s.inv, s.inventoryPath, s.prev)
```

- [ ] **Step 5: Update `internal/ui/screen_format_picker.go`**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"`.

Add to `FormatPickerScreen` struct (after `cursor int`):

```go
inv           *inventory.Inventory
inventoryPath string
```

Update `NewFormatPickerScreen` signature — add `inv *inventory.Inventory, inventoryPath string` before `prev Screen`:

```go
func NewFormatPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail,
	uploads []roms.Upload, inv *inventory.Inventory, inventoryPath string, prev Screen,
) *FormatPickerScreen {
```

Add `inv: inv, inventoryPath: inventoryPath,` to the struct literal.

Find the `NewDownloadScreen` call in `screen_format_picker.go` (line 220) and update it:

```go
return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
```

- [ ] **Step 6: Update `internal/ui/screen_download.go` constructor signature**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"`.

Add to `DownloadScreen` struct (after `err error`):

```go
inv           *inventory.Inventory
inventoryPath string
```

Update `NewDownloadScreen` signature:

```go
func NewDownloadScreen(client *itchio.Client, cfg *settings.Config, game itchio.Game, detail *itchio.GameDetail, upload roms.Upload, dest string, inv *inventory.Inventory, inventoryPath string, prev Screen) *DownloadScreen {
	s := &DownloadScreen{
		client: client, cfg: cfg, game: game, detail: detail,
		upload: upload, prev: prev, dest: dest, state: dlDownloading,
		inv: inv, inventoryPath: inventoryPath,
	}
```

- [ ] **Step 7: Verify the project compiles**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/
git commit -m "feat(inventory): thread inv and inventoryPath through download screen chain"
```

---

## Task 6: Record download in DownloadScreen

**Files:**
- Modify: `internal/ui/screen_download.go`

- [ ] **Step 1: Add inventory recording to the success branch**

In `screen_download.go`, inside the goroutine, after `logger.Info("download: complete file=%s", upload.Filename)` and before `s.state = dlDone`, add:

```go
s.inv.Add(game.URL, inventory.Entry{
    GameURL:  game.URL,
    Title:    game.Title,
    Author:   game.Author,
    CoverURL: game.CoverURL,
}, inventory.DownloadedFile{
    Filename:     upload.Filename,
    DestPath:     dest,
    DownloadedAt: time.Now(),
})
if err := s.inv.Save(s.inventoryPath); err != nil {
    logger.Warn("inventory: save failed: %v", err)
} else {
    logger.Info("inventory: recorded game=%q file=%s", game.Title, upload.Filename)
}
```

Add `"time"` to the import block if not already present.

- [ ] **Step 2: Verify the project compiles**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_download.go
git commit -m "feat(inventory): record successful download in inventory"
```

---

## Task 7: List screen — floppy icon and bold title

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Add `truncateBoldToWidth` helper**

Append to `screen_list.go` (after `truncateToWidth`):

```go
// truncateBoldToWidth truncates text with "…" so it fits within maxW pixels
// when rendered in bold.
func truncateBoldToWidth(r *renderer.Renderer, text string, maxW int32) string {
	tw, _ := r.BoldTextSize(text)
	if tw <= maxW {
		return text
	}
	ellipsisW, _ := r.BoldTextSize("…")
	target := maxW - ellipsisW
	runes := []rune(text)
	for len(runes) > 0 {
		tw, _ = r.BoldTextSize(string(runes))
		if tw <= target {
			break
		}
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), " ") + "…"
}
```

- [ ] **Step 2: Update the row rendering loop in `Draw`**

In the `for i, g := range s.games` loop, replace the existing price badge and title drawing block with:

```go
// Determine download status for this row.
isPresent := s.inv.IsPresent(g.URL)

// Badge: floppy disk if downloaded, otherwise price.
var badgeLabel string
var badgeR, badgeG, badgeB uint8
if isPresent {
    badgeLabel = "\U0001F4BE"
    badgeR, badgeG, badgeB = 80, 200, 220
} else if g.IsFree {
    badgeLabel = "Free"
    badgeR, badgeG, badgeB = 80, 200, 80
} else {
    badgeLabel = fmt.Sprintf("$%.2f", g.Price)
    badgeR, badgeG, badgeB = 220, 180, 60
}
badgeW, _ := r.TextSize(badgeLabel)
badgeX := leftW - badgeW - 8

// Title area is left of the badge.
titleAreaW := badgeX - 14

if i == s.cursor {
    if isPresent {
        titleW, _ := r.BoldTextSize(g.Title)
        if titleW <= titleAreaW {
            s.titleScrollX = 0
            r.DrawBoldText(g.Title, 10, y, colorText, colorText, colorText)
        } else {
            maxScroll := titleW - titleAreaW
            scrollX := s.titleScrollX
            if scrollX > maxScroll {
                scrollX = maxScroll
            }
            r.SetClipRect(10, rowTop, titleAreaW, rowH)
            r.DrawBoldText(g.Title, 10-scrollX, y, colorText, colorText, colorText)
            r.ClearClipRect()
            if scrollX == maxScroll && time.Since(s.titleScrollAt) > scrollDelay+time.Duration(maxScroll)*time.Second/time.Duration(scrollSpeed)+time.Second {
                s.titleScrollX = 0
                s.titleScrollAt = time.Now()
            }
        }
    } else {
        titleW, _ := r.TextSize(g.Title)
        if titleW <= titleAreaW {
            s.titleScrollX = 0
            r.DrawText(g.Title, 10, y, colorText, colorText, colorText)
        } else {
            maxScroll := titleW - titleAreaW
            scrollX := s.titleScrollX
            if scrollX > maxScroll {
                scrollX = maxScroll
            }
            r.SetClipRect(10, rowTop, titleAreaW, rowH)
            r.DrawText(g.Title, 10-scrollX, y, colorText, colorText, colorText)
            r.ClearClipRect()
            if scrollX == maxScroll && time.Since(s.titleScrollAt) > scrollDelay+time.Duration(maxScroll)*time.Second/time.Duration(scrollSpeed)+time.Second {
                s.titleScrollX = 0
                s.titleScrollAt = time.Now()
            }
        }
    }
} else {
    if isPresent {
        r.DrawBoldText(truncateBoldToWidth(r, g.Title, titleAreaW), 10, y, colorText, colorText, colorText)
    } else {
        r.DrawText(truncateToWidth(r, g.Title, titleAreaW), 10, y, colorText, colorText, colorText)
    }
}

// Badge always rendered on top of title.
r.DrawText(badgeLabel, badgeX, y, badgeR, badgeG, badgeB)
```

Remove the old `var priceLabel` / `priceR, priceG, priceB` / `priceW` / `priceX` / `titleAreaW` block and the old title/price drawing code that it replaces.

- [ ] **Step 3: Verify the project compiles**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): show floppy icon and bold title for downloaded games in list"
```

---

## Task 8: Detail screen — download status, X button, single-file delete

**Files:**
- Modify: `internal/ui/screen_detail.go`

- [ ] **Step 1: Extend `detailModal` with a `kind` field**

Replace the `detailModal` struct definition:

```go
type modalKind int

const (
	modalKindInfo          modalKind = iota
	modalKindDeleteConfirm           // A: confirm delete, B: cancel
)

type detailModal struct {
	active bool
	kind   modalKind
	title  string
	body   string
	// onConfirm is called when the user confirms a modalKindDeleteConfirm modal.
	onConfirm func()
}
```

- [ ] **Step 2: Update `drawModal` to vary the footer hint by kind**

In `drawModal`, replace the last line:

```go
r.DrawSmallTextCentered("Press any button to dismiss", boxX, y, boxW, 120, 120, 120)
```

with:

```go
if s.modal.kind == modalKindDeleteConfirm {
    r.DrawSmallTextCentered("A: confirm  B: cancel", boxX, y, boxW, 200, 100, 100)
} else {
    r.DrawSmallTextCentered("Press any button to dismiss", boxX, y, boxW, 120, 120, 120)
}
```

- [ ] **Step 3: Update `HandleEvent` modal dismissal to handle delete confirm**

Replace the modal dismissal block at the top of `HandleEvent`:

```go
if s.modal.active {
    switch ev := e.(type) {
    case *sdl.KeyboardEvent:
        if ev.Type == sdl.KEYDOWN {
            if s.modal.kind == modalKindDeleteConfirm {
                switch ev.Keysym.Sym {
                case sdl.K_RETURN: // physical A = confirm
                    if s.modal.onConfirm != nil {
                        s.modal.onConfirm()
                    }
                    s.modal.active = false
                case sdl.K_ESCAPE: // physical B = cancel
                    s.modal.active = false
                }
            } else {
                s.modal.active = false
            }
        }
    case *sdl.ControllerButtonEvent:
        if ev.Type == sdl.CONTROLLERBUTTONDOWN {
            if s.modal.kind == modalKindDeleteConfirm {
                switch ev.Button {
                case sdl.CONTROLLER_BUTTON_B: // physical A = confirm
                    if s.modal.onConfirm != nil {
                        s.modal.onConfirm()
                    }
                    s.modal.active = false
                case sdl.CONTROLLER_BUTTON_A: // physical B = cancel
                    s.modal.active = false
                }
            } else {
                s.modal.active = false
            }
        }
    case *sdl.QuitEvent:
        return nil
    }
    return s
}
```

- [ ] **Step 4: Update the action area in `Draw` to show download status**

Add import `"path/filepath"` to the import block.

Replace the existing action area block (the `if s.game.IsFree { ... }` section that draws `[ A: Download ]`) with:

```go
// ── Action area ─────────────────────────────────────────
_, smallFH := r.SmallTextSize("Ag")
isPresent := s.inv.IsPresent(s.game.URL)

if isPresent {
    // Download-again button
    if s.game.IsFree {
        r.DrawText("[ B: Download again ]", margin, y, 80, 200, 80)
    } else if s.cfg.APIKey == "" {
        r.DrawText(fmt.Sprintf("$%.2f  Purchase required", s.game.Price), margin, y, 220, 180, 60)
    } else {
        r.DrawText(fmt.Sprintf("[ B: Download again ]  $%.2f", s.game.Price), margin, y, 80, 200, 80)
    }
    y += fontH + 4

    // "Already on device" indicator
    r.DrawText("\U0001F4BE Already on device", margin, y, 80, 200, 220)
    y += fontH + 4

    // Per-file list
    if entry, ok := s.inv.Lookup(s.game.URL); ok {
        for _, f := range entry.Files {
            line := "  " + f.Filename + "  →  " + filepath.Dir(f.DestPath) + "/"
            r.DrawSmallText(line, margin, y, 120, 120, 120)
            y += smallFH + 2
        }
    }
    y += 4

    // Delete button
    r.DrawText("[ X: Delete ]", margin, y, 200, 80, 80)
    y += fontH + 8
} else {
    if s.game.IsFree {
        r.DrawText("[ B: Download ]", margin, y, 80, 200, 80)
    } else if s.cfg.APIKey == "" {
        r.DrawText(fmt.Sprintf("$%.2f  Purchase required", s.game.Price), margin, y, 220, 180, 60)
    } else {
        r.DrawText(fmt.Sprintf("[ B: Download ]  $%.2f", s.game.Price), margin, y, 80, 200, 80)
    }
    y += fontH + 8
}
```

Note: the `smallFH` variable is already declared earlier in `Draw` for the header — remove the duplicate declaration if needed (check for `_, smallFH := r.SmallTextSize("Ag")` already present in the loading block above and consolidate).

- [ ] **Step 5: Add X button handling and `performSingleFileDelete` method**

Add import `"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"` (already added in Task 5 — skip if present).

In `HandleEvent`, inside the `case *sdl.ControllerButtonEvent:` block (after the advisory check section and before `switch ev.Button`), add the X button case to the existing `switch ev.Button`:

```go
case sdl.CONTROLLER_BUTTON_X:
    s.triggerDelete()
    return s
```

Add the same for keyboard events (`sdl.K_x` or use a dedicated key — `sdl.K_x`):

```go
case sdl.K_x:
    s.triggerDelete()
```

Add these two methods to `screen_detail.go`:

```go
func (s *DetailScreen) triggerDelete() {
    entry, ok := s.inv.Lookup(s.game.URL)
    if !ok {
        return
    }
    if len(entry.Files) > 1 {
        // Multiple files — navigate to manage screen.
        // (handled as a screen transition; see screen_manage_downloads.go)
        return // replaced in Task 9
    }
    // Single file — show confirmation modal.
    var bodyLines []string
    if len(entry.Files) == 1 {
        bodyLines = append(bodyLines, entry.Files[0].Filename)
        bodyLines = append(bodyLines, filepath.Dir(entry.Files[0].DestPath)+"/")
    }
    s.modal = detailModal{
        active: true,
        kind:   modalKindDeleteConfirm,
        title:  "Delete downloaded file?",
        body:   strings.Join(bodyLines, "\n"),
        onConfirm: func() {
            s.performSingleFileDelete()
        },
    }
}

func (s *DetailScreen) performSingleFileDelete() {
    entry, ok := s.inv.Lookup(s.game.URL)
    if !ok {
        return
    }
    for _, f := range entry.Files {
        if err := os.Remove(f.DestPath); err != nil && !os.IsNotExist(err) {
            logger.Warn("inventory: delete file=%s: %v", f.DestPath, err)
        } else {
            logger.Debug("inventory: deleted file=%s", f.DestPath)
        }
        if artPath := inventory.CoverArtPath(entry.CoverURL, f.DestPath); artPath != "" {
            if err := os.Remove(artPath); err != nil && !os.IsNotExist(err) {
                logger.Warn("inventory: delete cover-art=%s: %v", artPath, err)
            } else {
                logger.Debug("inventory: deleted cover-art=%s", artPath)
            }
        }
    }
    logger.Info("inventory: deleted game=%q files=%d", entry.Title, len(entry.Files))
    s.inv.Remove(s.game.URL)
    if err := s.inv.Save(s.inventoryPath); err != nil {
        logger.Warn("inventory: save after delete failed: %v", err)
    }
}
```

Add `"os"` and `"strings"` to the import block if not already present.

- [ ] **Step 6: Verify the project compiles**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat(ui): detail screen download status, X-button delete for single-file games"
```

---

## Task 9: ManageDownloadsScreen — multi-file deletion

**Files:**
- Create: `internal/ui/screen_manage_downloads.go`
- Modify: `internal/ui/screen_detail.go` (complete `triggerDelete` multi-file branch)

- [ ] **Step 1: Create `internal/ui/screen_manage_downloads.go`**

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
	"github.com/veandco/go-sdl2/sdl"
)

type ManageDownloadsScreen struct {
	inv           *inventory.Inventory
	inventoryPath string
	gameURL       string
	cursor        int // 0..len(files)-1 = file rows; len(files) = "Delete all"

	confirmActive  bool
	confirmFileIdx int // -1 = delete all, otherwise index into entry.Files

	prev Screen
}

func NewManageDownloadsScreen(inv *inventory.Inventory, inventoryPath string, gameURL string, prev Screen) *ManageDownloadsScreen {
	return &ManageDownloadsScreen{
		inv:           inv,
		inventoryPath: inventoryPath,
		gameURL:       gameURL,
		prev:          prev,
	}
}

func (s *ManageDownloadsScreen) Draw(r *renderer.Renderer) {
	entry, ok := s.inv.Lookup(s.gameURL)
	if !ok {
		r.Present()
		return
	}

	r.Clear(colorBG, colorBG, colorBG)
	footerH := int32(40)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	// Header
	headerH := fontH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
	title := truncateToWidth(r, "Manage Downloads — "+entry.Title, r.W-24)
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawSmallText("by "+entry.Author, 12, 8+fontH+4, 140, 140, 140)

	contentTop := headerH + 10
	rowH := fontH + 14
	margin := int32(20)

	// File rows
	for i, f := range entry.Files {
		y := contentTop + int32(i)*rowH
		if i == s.cursor && !s.confirmActive {
			r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}
		nameW, _ := r.TextSize(f.Filename)
		r.DrawText(f.Filename, margin, y, colorText, colorText, colorText)
		dirLabel := "→  " + f.DestPath
		r.DrawSmallText(dirLabel, margin+nameW+12, y+(fontH-smallFH)/2, 120, 120, 120)
	}

	// Separator + "Delete all" row
	sepY := contentTop + int32(len(entry.Files))*rowH
	r.DrawRect(margin, sepY, r.W-margin*2, 1, 50, 50, 50)
	deleteAllY := sepY + 8
	deleteAllIdx := len(entry.Files)
	if s.cursor == deleteAllIdx && !s.confirmActive {
		r.DrawRect(0, deleteAllY-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
	}
	r.DrawText("Delete all", margin, deleteAllY, 200, 80, 80)

	// Footer
	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText("B: select  |  A: back", 10, ftrY, 140, 140, 140)

	// Confirmation overlay
	if s.confirmActive {
		s.drawConfirmOverlay(r, entry)
	}

	r.Present()
}

func (s *ManageDownloadsScreen) drawConfirmOverlay(r *renderer.Renderer, entry *inventory.Entry) {
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	pad := int32(16)

	var title, body string
	if s.confirmFileIdx == -1 {
		title = fmt.Sprintf("Delete all %d file(s)?", len(entry.Files))
		var names []string
		for _, f := range entry.Files {
			names = append(names, f.Filename)
		}
		body = strings.Join(names, "\n")
	} else {
		title = "Delete this file?"
		f := entry.Files[s.confirmFileIdx]
		body = f.Filename + "\n" + f.DestPath
	}

	lineH := smallFH + 4
	bodyLineCount := int32(strings.Count(body, "\n") + 1)
	boxW := r.W * 2 / 3
	boxH := pad + fontH + pad + 2 + pad + lineH*bodyLineCount + pad + 2 + pad + smallFH + pad
	boxX := (r.W - boxW) / 2
	boxY := (r.H - boxH) / 2

	r.DrawRect(0, 0, r.W, r.H, 10, 10, 15)
	r.DrawRect(boxX-1, boxY-1, boxW+2, boxH+2, 70, 70, 70)
	r.DrawRect(boxX, boxY, boxW, boxH, 25, 25, 35)

	y := boxY + pad
	r.DrawTextCentered(title, boxX, y, boxW, 240, 180, 60)
	y += fontH + pad
	r.DrawRect(boxX+pad, y, boxW-pad*2, 1, 60, 60, 60)
	y += 1 + pad

	for _, line := range strings.Split(body, "\n") {
		r.DrawSmallText(line, boxX+pad, y, 200, 200, 200)
		y += lineH
	}
	y += pad
	r.DrawRect(boxX+pad, y, boxW-pad*2, 1, 60, 60, 60)
	y += 1 + pad
	r.DrawSmallTextCentered("A: confirm  B: cancel", boxX, y, boxW, 200, 100, 100)
}

func (s *ManageDownloadsScreen) HandleEvent(e sdl.Event) Screen {
	entry, ok := s.inv.Lookup(s.gameURL)
	if !ok {
		return s.prev
	}
	rowCount := len(entry.Files) + 1 // files + "Delete all"

	if s.confirmActive {
		switch ev := e.(type) {
		case *sdl.ControllerButtonEvent:
			if ev.Type != sdl.CONTROLLERBUTTONDOWN {
				return s
			}
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_B: // physical A = confirm
				allGone := s.performDelete(entry, s.confirmFileIdx)
				s.confirmActive = false
				s.confirmFileIdx = -1
				if allGone {
					return s.prev
				}
				if s.cursor >= len(entry.Files) {
					s.cursor = len(entry.Files) // keep on "Delete all" if files remain
				}
			case sdl.CONTROLLER_BUTTON_A: // physical B = cancel
				s.confirmActive = false
				s.confirmFileIdx = -1
			}
		case *sdl.KeyboardEvent:
			if ev.Type != sdl.KEYDOWN {
				return s
			}
			switch ev.Keysym.Sym {
			case sdl.K_RETURN: // confirm
				allGone := s.performDelete(entry, s.confirmFileIdx)
				s.confirmActive = false
				s.confirmFileIdx = -1
				if allGone {
					return s.prev
				}
				if s.cursor >= len(entry.Files) {
					s.cursor = len(entry.Files)
				}
			case sdl.K_ESCAPE: // cancel
				s.confirmActive = false
				s.confirmFileIdx = -1
			}
		case *sdl.QuitEvent:
			return nil
		}
		return s
	}

	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < rowCount-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			s.confirmActive = true
			if s.cursor == len(entry.Files) {
				s.confirmFileIdx = -1
			} else {
				s.confirmFileIdx = s.cursor
			}
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < rowCount-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B: // physical A = select
			s.confirmActive = true
			if s.cursor == len(entry.Files) {
				s.confirmFileIdx = -1
			} else {
				s.confirmFileIdx = s.cursor
			}
		case sdl.CONTROLLER_BUTTON_A: // physical B = back
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

// performDelete deletes the file at fileIdx (or all files if fileIdx == -1),
// removes cover art, updates the inventory, and saves. Returns true if all
// files for the game are now gone.
func (s *ManageDownloadsScreen) performDelete(entry *inventory.Entry, fileIdx int) bool {
	var toDelete []inventory.DownloadedFile
	if fileIdx == -1 {
		toDelete = make([]inventory.DownloadedFile, len(entry.Files))
		copy(toDelete, entry.Files)
		entry.Files = nil
	} else {
		toDelete = []inventory.DownloadedFile{entry.Files[fileIdx]}
		var remaining []inventory.DownloadedFile
		for i, f := range entry.Files {
			if i != fileIdx {
				remaining = append(remaining, f)
			}
		}
		entry.Files = remaining
	}

	for _, f := range toDelete {
		if err := os.Remove(f.DestPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("inventory: delete file=%s: %v", f.DestPath, err)
		} else {
			logger.Debug("inventory: deleted file=%s", f.DestPath)
		}
		if artPath := inventory.CoverArtPath(entry.CoverURL, f.DestPath); artPath != "" {
			if err := os.Remove(artPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("inventory: delete cover-art=%s: %v", artPath, err)
			} else {
				logger.Debug("inventory: deleted cover-art=%s", artPath)
			}
		}
	}
	logger.Info("inventory: deleted game=%q files=%d", entry.Title, len(toDelete))

	if len(entry.Files) == 0 {
		s.inv.Remove(entry.GameURL)
	}
	if err := s.inv.Save(s.inventoryPath); err != nil {
		logger.Warn("inventory: save after delete failed: %v", err)
	}
	return len(entry.Files) == 0
}
```

- [ ] **Step 2: Complete the multi-file branch in `screen_detail.go` `triggerDelete`**

Replace the `// Multiple files — navigate to manage screen.` comment block in `triggerDelete`:

```go
if len(entry.Files) > 1 {
    return // this line is removed; the whole block below replaces it
}
```

The full updated `triggerDelete`:

```go
func (s *DetailScreen) triggerDelete() Screen {
	entry, ok := s.inv.Lookup(s.game.URL)
	if !ok {
		return s
	}
	if len(entry.Files) > 1 {
		return NewManageDownloadsScreen(s.inv, s.inventoryPath, s.game.URL, s)
	}
	var bodyLines []string
	if len(entry.Files) == 1 {
		bodyLines = append(bodyLines, entry.Files[0].Filename)
		bodyLines = append(bodyLines, filepath.Dir(entry.Files[0].DestPath)+"/")
	}
	s.modal = detailModal{
		active: true,
		kind:   modalKindDeleteConfirm,
		title:  "Delete downloaded file?",
		body:   strings.Join(bodyLines, "\n"),
		onConfirm: func() {
			s.performSingleFileDelete()
		},
	}
	return s
}
```

Note: `triggerDelete` now returns `Screen`. Update all call sites in `HandleEvent` from `s.triggerDelete()` to `return s.triggerDelete()` (both the keyboard and controller branches).

- [ ] **Step 3: Verify the project compiles**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 4: Run the full test suite**

```bash
go test -race -tags headless ./...
```

Expected: all packages pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_manage_downloads.go internal/ui/screen_detail.go
git commit -m "feat(ui): ManageDownloadsScreen for multi-file deletion; complete delete flow"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|-----------------|------|
| Inventory keyed by game URL | Task 1 |
| Track multiple files per game | Task 1 (DownloadedFile slice) |
| Load/Save JSON, atomic write | Task 1 |
| Record download on success | Task 6 |
| VerifyAndClean at startup, Info/Debug logging | Tasks 2, 4 |
| Floppy disk indicator in list (U+1F4BE) | Tasks 3, 7 |
| Bold title for present games | Tasks 3, 7 |
| Title scroll respects badge width | Task 7 (titleAreaW reused) |
| Detail screen: "Already on device" + file list | Task 8 |
| Detail screen: X button triggers delete | Task 8 |
| Single-file delete: confirmation modal | Task 8 |
| Multi-file delete: ManageDownloadsScreen | Task 9 |
| Per-file delete in ManageDownloadsScreen | Task 9 |
| "Delete all" option | Task 9 |
| Cover art deletion on file delete | Tasks 8, 9 |
| CoverArtPath helper | Task 2 |
| Inventory threaded through all screens | Task 5 |

All spec requirements covered. No gaps.
