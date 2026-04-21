# ROM Location Feature Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `ROM Location` setting (auto/ask) that lets power users browse the SD card filesystem to choose where a downloaded ROM is saved, with per-extension memory of the last chosen path.

**Architecture:** A new `LocationPickerScreen` is injected between the existing ROM picker and `DownloadScreen` when `cfg.ROMLocation == "ask"`. The screen reads subdirectories via `os.ReadDir`, presents them as a D-pad-navigable list with a pinned "Save here" confirm row, remembers the confirmed path per file extension in `cfg.LastROMDirs`, and validates that remembered paths still exist before using them. In auto mode the entire feature is a no-op.

**Tech Stack:** Go 1.22+, SDL2 via go-sdl2, JSON config, standard `os`/`filepath`/`sort` packages.

**Spec deviation — `NewDownloadScreen` signature:** The spec lists `screen_download.go` as untouched, but `NewDownloadScreen` currently computes `dest` internally from `roms.DestinationDir`. To let `LocationPickerScreen` supply a user-chosen path, `dest string` is added as a parameter and the computation moves to each call site. Draw, HandleEvent, and the `DownloadScreen` struct are unchanged.

---

## File Map

| File | Status | Responsibility |
|------|--------|---------------|
| `internal/settings/settings.go` | Modify | Add `ROMLocation string` + `LastROMDirs map[string]string` fields; update `defaults()` |
| `internal/settings/settings_test.go` | Modify | Tests for new fields: default value, round-trip, JSON omitempty |
| `internal/ui/screen_download.go` | Modify (signature only) | Accept `dest string` as parameter instead of computing it internally |
| `internal/ui/screen_settings.go` | Modify | Add `sItemROMLocation` enum value, render row, toggle on activate |
| `internal/ui/screen_location_picker.go` | **Create** | Full directory browser screen |
| `internal/ui/screen_fetch_uploads.go` | Modify | Route through `LocationPickerScreen` when `cfg.ROMLocation == "ask"` |
| `internal/ui/screen_rom_picker.go` | Modify | Same routing change |

**Task order is intentional:** Tasks 1–3 prepare the data layer and signature; Task 4 creates the new screen; Tasks 5–6 wire the routing that references the new screen. Each task compiles cleanly on its own.

---

## Task 1: Extend Config — tests first

**Files:**
- Modify: `internal/settings/settings_test.go`
- Modify: `internal/settings/settings.go`

- [ ] **Step 1.1: Add three failing tests to `settings_test.go`**

Update the import block at the top of `internal/settings/settings_test.go` — add `"bytes"`:

```go
import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)
```

Append to the end of `internal/settings/settings_test.go`:

```go
func TestROMLocationDefault(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ROMLocation != "auto" {
		t.Errorf("default ROMLocation = %q, want %q", cfg.ROMLocation, "auto")
	}
	if cfg.LastROMDirs != nil {
		t.Errorf("default LastROMDirs should be nil, got %v", cfg.LastROMDirs)
	}
}

func TestLastROMDirsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{
		ROMLocation: "ask",
		LastROMDirs: map[string]string{
			".gbc": "/mnt/SDCARD/Roms/RPG/GBC/",
			".gb":  "/mnt/SDCARD/Roms/RPG/GB/",
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ROMLocation != "ask" {
		t.Errorf("ROMLocation = %q, want %q", loaded.ROMLocation, "ask")
	}
	if loaded.LastROMDirs[".gbc"] != "/mnt/SDCARD/Roms/RPG/GBC/" {
		t.Errorf(".gbc dir = %q, want %q", loaded.LastROMDirs[".gbc"], "/mnt/SDCARD/Roms/RPG/GBC/")
	}
	if loaded.LastROMDirs[".gb"] != "/mnt/SDCARD/Roms/RPG/GB/" {
		t.Errorf(".gb dir = %q, want %q", loaded.LastROMDirs[".gb"], "/mnt/SDCARD/Roms/RPG/GB/")
	}
}

func TestLastROMDirsOmittedWhenNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{ROMLocation: "ask"} // LastROMDirs is nil
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte("last_rom_dirs")) {
		t.Errorf("last_rom_dirs should be omitted when nil, found in JSON:\n%s", data)
	}
}
```

- [ ] **Step 1.2: Run tests to confirm they fail**

```bash
go test -race -tags headless ./internal/settings/...
```

Expected: compilation error — `settings.Config` has no field `ROMLocation` or `LastROMDirs`.

- [ ] **Step 1.3: Implement the config changes in `settings.go`**

Replace the `Config` struct and `defaults()` function:

```go
// Config is the top-level application configuration.
type Config struct {
	APIKey       string            `json:"api_key"`
	ROMSelection string            `json:"rom_selection"`
	ROMLocation  string            `json:"rom_location"`
	LastROMDirs  map[string]string `json:"last_rom_dirs,omitempty"`
	Filter       ContentFilter     `json:"content_filter"`
}

func defaults() *Config {
	return &Config{
		APIKey:       "",
		ROMSelection: "auto",
		ROMLocation:  "auto",
		Filter: ContentFilter{
			AdultContent: CategoryFilter{Enabled: true},
			HeavyThemes:  CategoryFilter{Enabled: true},
			SubstanceUse: CategoryFilter{Enabled: true},
			// QueerContent defaults to disabled (zero value).
		},
	}
}
```

- [ ] **Step 1.4: Run tests to confirm they pass**

```bash
go test -race -tags headless ./internal/settings/...
```

Expected: `ok  github.com/carroarmato0/nextui-itchio-pak/internal/settings`

- [ ] **Step 1.5: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat(settings): add ROMLocation and LastROMDirs config fields"
```

---

## Task 2: Migrate NewDownloadScreen to accept explicit dest

**Files:**
- Modify: `internal/ui/screen_download.go`
- Modify: `internal/ui/screen_fetch_uploads.go`
- Modify: `internal/ui/screen_rom_picker.go`

This task only changes the `dest` parameter and existing callers. Routing to `LocationPickerScreen` comes in Tasks 5–6 after the new screen exists.

- [ ] **Step 2.1: Update `NewDownloadScreen` in `screen_download.go`**

**Old import block:**
```go
import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)
```

**New import block** (remove `"path/filepath"` and `"strings"`):
```go
import (
	"fmt"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)
```

**Old function start:**
```go
func NewDownloadScreen(client *itchio.Client, cfg *settings.Config, game itchio.Game, detail *itchio.GameDetail, upload roms.Upload, prev Screen) *DownloadScreen {
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename
```

**New function start** (accepts `dest string`; removes the two internal computation lines):
```go
func NewDownloadScreen(client *itchio.Client, cfg *settings.Config, game itchio.Game, detail *itchio.GameDetail, upload roms.Upload, dest string, prev Screen) *DownloadScreen {
```

The rest of the function body is unchanged.

- [ ] **Step 2.2: Update the caller in `screen_fetch_uploads.go`**

Add `"path/filepath"` and `"strings"` to the import block (they move here from screen_download.go):

```go
import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)
```

Replace `nextScreen()` — compute dest at the call site, no routing to LocationPickerScreen yet:

```go
func (s *FetchUploadsScreen) nextScreen() Screen {
	if len(s.uploads) == 1 {
		upload := s.uploads[0]
		ext := strings.ToLower(filepath.Ext(upload.Filename))
		dest := roms.DestinationDir(ext) + upload.Filename
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.prev)
	}
	// Multiple files — always show picker so the user can choose
	return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s.uploads, s.prev)
}
```

- [ ] **Step 2.3: Update the caller in `screen_rom_picker.go`**

Add `"path/filepath"` and `"strings"` to the import block:

```go
import (
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)
```

Replace both `NewDownloadScreen` calls in `HandleEvent` (keyboard `K_RETURN` and controller `CONTROLLER_BUTTON_B`) with a call to a new helper method:

**Old keyboard case:**
```go
case sdl.K_RETURN:
	if s.cursor < len(s.uploads) {
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.uploads[s.cursor], s.prev)
	}
```

**New keyboard case:**
```go
case sdl.K_RETURN:
	if s.cursor < len(s.uploads) {
		return s.chooseUpload(s.uploads[s.cursor])
	}
```

**Old controller case:**
```go
case sdl.CONTROLLER_BUTTON_B:
	if s.cursor < len(s.uploads) {
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.uploads[s.cursor], s.prev)
	}
```

**New controller case:**
```go
case sdl.CONTROLLER_BUTTON_B:
	if s.cursor < len(s.uploads) {
		return s.chooseUpload(s.uploads[s.cursor])
	}
```

Add the helper method at the bottom of `screen_rom_picker.go` — no routing to LocationPickerScreen yet:

```go
func (s *ROMPickerScreen) chooseUpload(upload roms.Upload) Screen {
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.prev)
}
```

- [ ] **Step 2.4: Verify build**

```bash
go build -tags headless ./...
```

Expected: no output (clean build).

- [ ] **Step 2.5: Commit**

```bash
git add internal/ui/screen_download.go internal/ui/screen_fetch_uploads.go internal/ui/screen_rom_picker.go
git commit -m "refactor(download): accept explicit dest parameter in NewDownloadScreen"
```

---

## Task 3: Add ROM Location row to Settings screen

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 3.1: Add `sItemROMLocation` to the iota block**

**Old:**
```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemClearCache
	sItemContentModeration
	sItemAbout
	sItemCount
)
```

**New:**
```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemROMLocation
	sItemClearCache
	sItemContentModeration
	sItemAbout
	sItemCount
)
```

- [ ] **Step 3.2: Add the row label in `Draw`**

**Old `items` slice:**
```go
items := []string{
	"API Key: ",
	"ROM Selection: " + s.cfg.ROMSelection,
	"Clear Image Cache",
	"Content Moderation >",
	"About",
}
```

**New:**
```go
items := []string{
	"API Key: ",
	"ROM Selection: " + s.cfg.ROMSelection,
	"ROM Location: " + s.cfg.ROMLocation,
	"Clear Image Cache",
	"Content Moderation >",
	"About",
}
```

- [ ] **Step 3.3: Add the toggle in `activate`**

**Old `activate` (partial):**
```go
func (s *SettingsScreen) activate() Screen {
	switch s.cursor {
	case sItemROMMode:
		if s.cfg.ROMSelection == "auto" {
			s.cfg.ROMSelection = "ask"
		} else {
			s.cfg.ROMSelection = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case sItemClearCache:
```

**New (insert the `sItemROMLocation` case between the two existing ones):**
```go
func (s *SettingsScreen) activate() Screen {
	switch s.cursor {
	case sItemROMMode:
		if s.cfg.ROMSelection == "auto" {
			s.cfg.ROMSelection = "ask"
		} else {
			s.cfg.ROMSelection = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case sItemROMLocation:
		if s.cfg.ROMLocation == "auto" {
			s.cfg.ROMLocation = "ask"
		} else {
			s.cfg.ROMLocation = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case sItemClearCache:
```

- [ ] **Step 3.4: Verify build**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 3.5: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat(settings): add ROM Location toggle (auto/ask)"
```

---

## Task 4: Implement LocationPickerScreen

**Files:**
- Create: `internal/ui/screen_location_picker.go`

- [ ] **Step 4.1: Create the file**

Create `internal/ui/screen_location_picker.go` with this content:

```go
//go:build !headless

package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// locationRoot is the highest directory the user can navigate to.
const locationRoot = "/mnt/SDCARD"

type rowKind int

const (
	rowSaveHere rowKind = iota
	rowUp
	rowEntry
)

// pickerRow represents one navigable item in the directory browser.
type pickerRow struct {
	kind rowKind
	name string // only set for rowEntry; holds the subdirectory name
}

// LocationPickerScreen lets the user navigate the SD card filesystem and
// choose a destination directory for a ROM download.
type LocationPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	game    itchio.Game
	detail  *itchio.GameDetail
	upload  roms.Upload
	prev    Screen

	ext        string      // lowercase extension e.g. ".gbc"
	currentDir string      // always ends with "/"
	rows       []pickerRow // [rowSaveHere, optional rowUp, zero or more rowEntry]
	cursor     int         // index into rows
}

// NewLocationPickerScreen creates a directory browser that opens at the
// remembered path for this file extension (or the default destination if no
// remembered path exists or the remembered path no longer exists on disk).
func NewLocationPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail, upload roms.Upload, prev Screen,
) *LocationPickerScreen {
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	startDir := resolveStartDir(cfg, ext, cfgPath)
	s := &LocationPickerScreen{
		client:  client,
		cfg:     cfg,
		cfgPath: cfgPath,
		game:    game,
		detail:  detail,
		upload:  upload,
		prev:    prev,
		ext:     ext,
	}
	s.loadDir(startDir)
	return s
}

// resolveStartDir returns the directory the browser should open at.
// If the remembered path for ext no longer exists on disk it is removed from
// cfg and cfg is saved before returning the default destination.
func resolveStartDir(cfg *settings.Config, ext, cfgPath string) string {
	if cfg.LastROMDirs != nil {
		if dir, ok := cfg.LastROMDirs[ext]; ok && dir != "" {
			if _, err := os.Stat(dir); err == nil {
				return dir // remembered path is valid — use it
			}
			// Stale path — forget it and fall through to default
			delete(cfg.LastROMDirs, ext)
			if len(cfg.LastROMDirs) == 0 {
				cfg.LastROMDirs = nil
			}
			cfg.Save(cfgPath) //nolint:errcheck — best-effort cleanup
		}
	}
	return roms.DestinationDir(ext)
}

// loadDir switches the browser to dir, rebuilds the row list, and resets the
// cursor to "Save here" (index 0).
func (s *LocationPickerScreen) loadDir(dir string) {
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	s.currentDir = dir
	s.rows = buildRows(dir)
	s.cursor = 0
}

// buildRows constructs the navigable row list for dir:
//   - index 0:   rowSaveHere  (always)
//   - index 1:   rowUp        (omitted when dir == locationRoot)
//   - index 1+:  rowEntry     (one per visible subdirectory, sorted case-insensitively)
func buildRows(dir string) []pickerRow {
	rows := []pickerRow{{kind: rowSaveHere}}

	atRoot := strings.TrimRight(dir, "/") == locationRoot
	if !atRoot {
		rows = append(rows, pickerRow{kind: rowUp})
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return rows // e.g. permission denied — graceful degradation
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	for _, name := range names {
		rows = append(rows, pickerRow{kind: rowEntry, name: name})
	}
	return rows
}

// atRoot reports whether the browser is already at locationRoot.
func (s *LocationPickerScreen) atRoot() bool {
	return strings.TrimRight(s.currentDir, "/") == locationRoot
}

func (s *LocationPickerScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	footerH := int32(40)

	// ── Header ──────────────────────────────────────────────────────────────
	headerH := mainFH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, 140, 140, 140)

	// ── Path bar ────────────────────────────────────────────────────────────
	pathBarY := headerH + 2
	pathBarH := smallFH + 10
	r.DrawRect(0, pathBarY, r.W, pathBarH, 25, 25, 25)
	pathText := leftTruncatePath(r, s.currentDir, r.W-24)
	r.DrawSmallText(pathText, 12, pathBarY+5, 120, 160, 200)

	// ── Confirm row (pinned first, distinct green tint) ──────────────────────
	confirmY := pathBarY + pathBarH
	confirmH := mainFH + 10
	if s.cursor == 0 {
		r.DrawRect(0, confirmY, r.W, confirmH, 26, 58, 34)
	} else {
		r.DrawRect(0, confirmY, r.W, confirmH, 15, 32, 22)
	}
	r.DrawText("[ \u2713  Save here ]", 12, confirmY+5, 80, 200, 120)
	r.DrawRect(0, confirmY+confirmH, r.W, 1, 28, 58, 28)

	// ── Directory list (rows[1:]) ────────────────────────────────────────────
	listTop := confirmY + confirmH + 2
	rowH := mainFH + 14
	entryCount := 0
	listRowsDrawn := int32(0)

	for i := 1; i < len(s.rows); i++ {
		row := s.rows[i]
		y := listTop + listRowsDrawn*rowH
		if y+rowH > r.H-footerH {
			break
		}
		selected := s.cursor == i
		switch row.kind {
		case rowUp:
			if selected {
				r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
			}
			r.DrawSmallText("\u2191  .. (go up)", 20, y+(rowH-smallFH)/2, 100, 140, 180)
		case rowEntry:
			entryCount++
			if selected {
				r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
			}
			r.DrawText("\u25b8 "+row.name, 20, y, colorText, colorText, colorText)
		}
		listRowsDrawn++
	}

	// Show placeholder when no subdirectories exist in this folder.
	if entryCount == 0 {
		y := listTop + listRowsDrawn*rowH
		r.DrawSmallText("  (no subfolders)", 20, y, 80, 80, 80)
	}

	// ── Footer ───────────────────────────────────────────────────────────────
	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText(s.footerHint(), 10, ftrY, 140, 140, 140)
	r.Present()
}

// footerHint returns the context-sensitive button hint line for the footer.
func (s *LocationPickerScreen) footerHint() string {
	const sep = "  \u00b7  "
	switch {
	case s.cursor == 0 && s.atRoot():
		return "B: confirm" + sep + "Start: cancel"
	case s.cursor == 0:
		return "B: confirm" + sep + "A: go up" + sep + "Start: cancel"
	case s.atRoot():
		return "B: enter dir" + sep + "Start: cancel"
	default:
		return "B: confirm / enter dir" + sep + "A: go up" + sep + "Start: cancel"
	}
}

func (s *LocationPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN: // B button
			return s.activate()
		case sdl.K_ESCAPE: // A button
			return s.goUp()
		case sdl.K_s: // Start button
			return s.prev // cancel, no download
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.activate()
		case sdl.CONTROLLER_BUTTON_A:
			return s.goUp()
		case sdl.CONTROLLER_BUTTON_START:
			return s.prev // cancel, no download
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

// activate handles a B-button press based on what the cursor points to.
func (s *LocationPickerScreen) activate() Screen {
	if s.cursor >= len(s.rows) {
		return s
	}
	row := s.rows[s.cursor]
	switch row.kind {
	case rowSaveHere:
		return s.confirm()
	case rowUp:
		return s.goUp()
	case rowEntry:
		s.loadDir(s.currentDir + row.name + "/")
	}
	return s
}

// goUp navigates to the parent directory. Does nothing when already at root.
func (s *LocationPickerScreen) goUp() Screen {
	if s.atRoot() {
		return s
	}
	parent := filepath.Dir(strings.TrimRight(s.currentDir, "/"))
	s.loadDir(parent)
	return s
}

// confirm saves the chosen directory to config and proceeds to download.
func (s *LocationPickerScreen) confirm() Screen {
	if s.cfg.LastROMDirs == nil {
		s.cfg.LastROMDirs = make(map[string]string)
	}
	s.cfg.LastROMDirs[s.ext] = s.currentDir
	s.cfg.Save(s.cfgPath) //nolint:errcheck — best-effort persistence
	dest := s.currentDir + s.upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.upload, dest, s.prev)
}

// leftTruncatePath shortens text from the left with a "…" prefix so it fits
// within maxW pixels when rendered with the small font. Used for long paths.
func leftTruncatePath(r *renderer.Renderer, text string, maxW int32) string {
	w, _ := r.SmallTextSize(text)
	if int32(w) <= maxW {
		return text
	}
	const ellipsis = "\u2026"
	for len(text) > 1 {
		text = text[1:]
		w, _ = r.SmallTextSize(ellipsis + text)
		if int32(w) <= maxW {
			return ellipsis + text
		}
	}
	return ellipsis
}
```

- [ ] **Step 4.2: Verify build**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 4.3: Commit**

```bash
git add internal/ui/screen_location_picker.go
git commit -m "feat(ui): add LocationPickerScreen directory browser"
```

---

## Task 5: Wire routing — FetchUploadsScreen

**Files:**
- Modify: `internal/ui/screen_fetch_uploads.go`

- [ ] **Step 5.1: Add the location picker branch to `nextScreen`**

Replace `nextScreen()` — the only change is adding the `cfg.ROMLocation == "ask"` branch before the existing single-upload path:

```go
func (s *FetchUploadsScreen) nextScreen() Screen {
	if len(s.uploads) == 1 {
		upload := s.uploads[0]
		if s.cfg.ROMLocation == "ask" {
			return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.prev)
		}
		ext := strings.ToLower(filepath.Ext(upload.Filename))
		dest := roms.DestinationDir(ext) + upload.Filename
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.prev)
	}
	// Multiple files — always show picker so the user can choose
	return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s.uploads, s.prev)
}
```

- [ ] **Step 5.2: Verify build**

```bash
go build -tags headless ./...
```

Expected: no output.

---

## Task 6: Wire routing — ROMPickerScreen

**Files:**
- Modify: `internal/ui/screen_rom_picker.go`

- [ ] **Step 6.1: Add the location picker branch to `chooseUpload`**

Replace the `chooseUpload` helper added in Task 2 — add the `cfg.ROMLocation == "ask"` branch:

```go
func (s *ROMPickerScreen) chooseUpload(upload roms.Upload) Screen {
	if s.cfg.ROMLocation == "ask" {
		return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.prev)
	}
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.prev)
}
```

- [ ] **Step 6.2: Run full test suite**

```bash
go test -race -tags headless ./...
```

Expected: all packages show `ok`.

- [ ] **Step 6.3: Commit**

```bash
git add internal/ui/screen_fetch_uploads.go internal/ui/screen_rom_picker.go
git commit -m "feat(ui): route through LocationPickerScreen when ROMLocation=ask"
```

---

## Task 7: Final verification

- [ ] **Step 7.1: Clean headless build**

```bash
go build -tags headless ./...
```

Expected: no output.

- [ ] **Step 7.2: All tests pass**

```bash
go test -race -tags headless ./...
```

Expected: `ok` for all packages.

- [ ] **Step 7.3: Cross-compile for target platforms (requires Docker or Podman)**

```bash
make build-tg5040
```

Expected: binary produced in `bin/tg5040/` with no errors.

- [ ] **Step 7.4: On-device smoke checklist (manual)**

Deploy with `make deploy-adb` or `make deploy-sd`, then verify:

| Scenario | Expected |
|----------|----------|
| `ROM Location` row appears in Settings between "ROM Selection" and "Clear Image Cache" | ✓ |
| Toggling ROM Location cycles `auto` ↔ `ask` and persists after restart | ✓ |
| With `ROM Location: auto`, download proceeds without any new screen | ✓ |
| With `ROM Location: ask`, LocationPickerScreen appears after selecting a ROM file | ✓ |
| Default open location matches the file's normal destination (`.gbc` → `Game Boy Color (GBC)/`) | ✓ |
| D-pad navigates the directory list; B enters a directory; A goes up | ✓ |
| Navigating to a folder, then pressing B on "Save here" downloads the ROM there | ✓ |
| Chosen path persists as the default on next download for the same extension | ✓ |
| `.gbc` and `.gb` remember separate paths independently | ✓ |
| Start cancels the picker and returns to the game detail screen without downloading | ✓ |
| At `/mnt/SDCARD/`, the ".." row is absent; A button does nothing | ✓ |
| Entering an empty directory shows "(no subfolders)" dimmed text | ✓ |
| Deleting a previously remembered folder externally: picker falls back to default, no crash | ✓ |
