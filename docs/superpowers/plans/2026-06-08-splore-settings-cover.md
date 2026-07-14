# Splore Settings Toggle + Cover Art Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Pico-8 Splore Store settings toggle (hidden for FakeO8, default ON for official core) and bundle a custom pixel-art cover image alongside the Splore cart.

**Architecture:** A new `cmd/gen-splore-cover/` program generates the pixel-art PNG once and commits it; `internal/roms/splore.go` embeds it via `//go:embed` and writes/cleans it alongside the cart. A new `Pico8Splore bool` config field gates all three call sites (startup, migration, settings toggle). The settings screen adds one new item using the existing conditional-visibility pattern.

**Tech Stack:** Go stdlib (`image`, `image/png`, `embed`, `os`), `internal/roms`, `internal/settings`, `internal/ui`.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `cmd/gen-splore-cover/main.go` | Dev-time pixel-art PNG generator |
| Modify | `internal/roms/splore.go` | Add `//go:embed`, `//go:generate`, cover-art write/clean |
| Modify | `internal/roms/splore_test.go` | Four new cover-art tests |
| Modify | `internal/settings/settings.go` | `Pico8Splore bool` field + default |
| Modify | `internal/settings/settings_test.go` | Default + round-trip test for `Pico8Splore` |
| Modify | `internal/ui/screen_settings.go` | `sItemPico8Splore` constant, visibility, draw, activate |
| Modify | `cmd/itchio-pak/main_sdl.go` | Gate startup call on `cfg.Pico8Splore` |
| Modify | `internal/ui/screen_pico8_core_migrate.go` | Gate `EnsureSploreCart` on `cfg.Pico8Splore` |

---

## Task 1: Pixel-art cover generator

**Files:**
- Create: `cmd/gen-splore-cover/main.go`
- Produces: `internal/roms/splore_cover.png` (committed artefact)

The generator is a standalone dev tool — no TDD. Run once, commit output.

- [ ] **Step 1: Create the generator**

Create `cmd/gen-splore-cover/main.go`:

```go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// Pico-8 16-colour palette
var pal = [16]color.RGBA{
	{0x00, 0x00, 0x00, 0xff}, // 0  black
	{0x1d, 0x2b, 0x53, 0xff}, // 1  dark blue
	{0x7e, 0x25, 0x53, 0xff}, // 2  dark purple
	{0x00, 0x87, 0x51, 0xff}, // 3  dark green
	{0xab, 0x52, 0x36, 0xff}, // 4  brown
	{0x5f, 0x57, 0x4f, 0xff}, // 5  dark gray
	{0xc2, 0xc3, 0xc7, 0xff}, // 6  light gray
	{0xff, 0xf1, 0xe8, 0xff}, // 7  white
	{0xff, 0x00, 0x4d, 0xff}, // 8  red
	{0xff, 0xa3, 0x00, 0xff}, // 9  orange
	{0xff, 0xec, 0x27, 0xff}, // 10 yellow
	{0x00, 0xe4, 0x36, 0xff}, // 11 green
	{0x29, 0xad, 0xff, 0xff}, // 12 light blue
	{0x83, 0x76, 0x9c, 0xff}, // 13 lavender
	{0xff, 0x77, 0xa8, 0xff}, // 14 pink
	{0xff, 0xcc, 0xaa, 0xff}, // 15 peach
}

// 5×7 bitmap font — columns: bit 4 = left, bit 0 = right
var font = map[byte][7]uint8{
	'A': {0b01110, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'C': {0b01110, 0b10001, 0b10000, 0b10000, 0b10000, 0b10001, 0b01110},
	'E': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b11111},
	'L': {0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b11111},
	'O': {0b01110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'P': {0b11110, 0b10001, 0b10001, 0b11110, 0b10000, 0b10000, 0b10000},
	'R': {0b11110, 0b10001, 0b10001, 0b11110, 0b10100, 0b10010, 0b10001},
	'S': {0b01110, 0b10001, 0b10000, 0b01110, 0b00001, 0b10001, 0b01110},
	'T': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100},
	'V': {0b10001, 0b10001, 0b10001, 0b10001, 0b01010, 0b01010, 0b00100},
}

var img = image.NewRGBA(image.Rect(0, 0, 128, 128))

func dot(x, y, p int) {
	if x >= 0 && x < 128 && y >= 0 && y < 128 {
		img.SetRGBA(x, y, pal[p])
	}
}

func fill(x, y, w, h, p int) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			dot(x+dx, y+dy, p)
		}
	}
}

func drawText(s string, x, y, scale, p int) {
	cx := x
	for i := 0; i < len(s); i++ {
		g, ok := font[s[i]]
		if !ok {
			cx += (5 + 1) * scale
			continue
		}
		for row := 0; row < 7; row++ {
			for bit := 0; bit < 5; bit++ {
				if g[row]>>(4-bit)&1 == 1 {
					fill(cx+bit*scale, y+row*scale, scale, scale, p)
				}
			}
		}
		cx += (5 + 1) * scale
	}
}

func textWidth(s string, scale int) int {
	n := len(s)
	if n == 0 {
		return 0
	}
	return (n*5 + (n - 1)) * scale
}

func main() {
	outPath := "internal/roms/splore_cover.png"
	if len(os.Args) > 1 {
		outPath = os.Args[1]
	}

	// background
	fill(0, 0, 128, 128, 1) // dark blue

	// stars (fixed positions for a deterministic PNG)
	type star struct{ x, y, c int }
	for _, s := range []star{
		{4, 6, 7}, {18, 3, 10}, {35, 9, 7}, {52, 4, 6}, {71, 11, 10},
		{88, 5, 7}, {103, 8, 6}, {120, 3, 7}, {11, 19, 6}, {29, 22, 10},
		{47, 17, 7}, {63, 24, 6}, {82, 18, 7}, {98, 21, 10}, {116, 16, 6},
		{6, 33, 10}, {24, 38, 7}, {42, 31, 6}, {78, 29, 10}, {96, 35, 7},
		{114, 32, 6}, {3, 76, 7}, {20, 82, 10}, {37, 73, 6}, {55, 79, 7},
		{73, 75, 6}, {91, 83, 10}, {109, 77, 7}, {125, 80, 6}, {8, 96, 10},
		{26, 102, 7}, {44, 94, 6}, {62, 99, 7}, {80, 93, 10}, {97, 100, 7},
		{113, 95, 6}, {5, 113, 7}, {22, 118, 6}, {40, 111, 10}, {58, 116, 7},
		{76, 121, 6}, {93, 114, 7}, {111, 119, 10}, {127, 109, 7},
	} {
		dot(s.x, s.y, s.c)
	}
	// cross-shaped bright stars
	for _, s := range []star{{18, 3, 7}, {71, 11, 7}, {29, 22, 7}, {96, 35, 7}} {
		dot(s.x, s.y, 7)
		dot(s.x-1, s.y, 6)
		dot(s.x+1, s.y, 6)
		dot(s.x, s.y-1, 6)
		dot(s.x, s.y+1, 6)
	}

	// "SPLORE" title: 2× scale, white, y=8
	title := "SPLORE"
	drawText(title, (128-textWidth(title, 2))/2, 8, 2, 7)

	// "CARTVERSE" subtitle: 1× scale, pink, y=28
	sub := "CARTVERSE"
	subX := (128 - textWidth(sub, 1)) / 2
	drawText(sub, subX, 28, 1, 14)

	// small cart icons flanking the subtitle
	for _, cx := range []int{subX - 9, subX + textWidth(sub, 1) + 4} {
		for i := 0; i < 5; i++ {
			dot(cx+i, 28, 13) // top bar
			dot(cx+i, 31, 13) // bottom bar
		}
		dot(cx, 29, 13); dot(cx+4, 29, 13) // sides
		dot(cx, 30, 13); dot(cx+4, 30, 13)
	}

	// rocket: 10 wide, startX=59 (centred), body at y=40
	rx, ry := 59, 40
	type sprRow struct {
		pat string
		col int
	}
	for i, row := range []sprRow{
		{"....XX....", 7},  // nose tip: white
		{"...XXXX...", 12}, // body: light blue
		{"..XXXXXX..", 12},
		{"..XXXXXX..", 12},
		{".XXXXXXXX.", 12},
		{".XXXXXXXX.", 12},
		{".XXXXXXXX.", 12},
		{".XXXXXXXX.", 12},
		{"XXXXXXXXXX", 12},
		{"X.XXXXXX.X", 12}, // fins
		{"X.XXXXXX.X", 12},
		{"...XXXX...", 12},
	} {
		for j := 0; j < len(row.pat); j++ {
			if row.pat[j] == 'X' {
				dot(rx+j, ry+i, row.col)
			}
		}
	}
	fill(rx+4, ry+3, 2, 2, 7) // window: 2×2 white

	// flame below rocket
	fy := ry + 12
	type fPx struct{ x, y, c int }
	for _, f := range []fPx{
		{3, 0, 10}, {4, 0, 10}, {5, 0, 10}, {6, 0, 10},
		{3, 1, 9}, {4, 1, 10}, {5, 1, 9}, {6, 1, 9},
		{4, 2, 9}, {5, 2, 9},
		{4, 3, 8}, {5, 3, 8},
	} {
		dot(rx+f.x, fy+f.y, f.c)
	}

	f, err := os.Create(outPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
```

- [ ] **Step 2: Run the generator from the repo root**

```sh
go run ./cmd/gen-splore-cover/
```

Expected: `internal/roms/splore_cover.png` created (~1-2 KB). Open it in any image viewer to verify it shows a dark space background, stars, "SPLORE" text at the top, "CARTVERSE" subtitle in pink, a pixel-art rocket with flame in the centre, and two tiny cart-outline icons flanking the subtitle.

- [ ] **Step 3: Commit generator and PNG**

```sh
git add cmd/gen-splore-cover/main.go internal/roms/splore_cover.png
git commit -m "feat(roms): add pixel-art Splore cover generator and PNG"
```

---

## Task 2: Embed cover art + update splore.go (TDD)

**Files:**
- Modify: `internal/roms/splore_test.go`
- Modify: `internal/roms/splore.go`

The existing `ensureSploreCartInDir` currently returns early when the cart file already exists. That early return must be removed so cover-art idempotency is also checked independently.

- [ ] **Step 1: Write failing tests**

Add to the bottom of `internal/roms/splore_test.go`:

```go
func TestEnsureSploreCartInDir_CreatesCoverArt(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("ensureSploreCartInDir: %v", err)
	}
	artPath := dir + ".media/Splore.png"
	info, err := os.Stat(artPath)
	if err != nil {
		t.Fatalf(".media/Splore.png not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("cover art file is empty")
	}
}

func TestEnsureSploreCartInDir_CoverArtIdempotent(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	artPath := dir + ".media/Splore.png"
	stat1, err := os.Stat(artPath)
	if err != nil {
		t.Fatalf("stat after first call: %v", err)
	}
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
	stat2, err := os.Stat(artPath)
	if err != nil {
		t.Fatalf("stat after second call: %v", err)
	}
	if stat1.ModTime() != stat2.ModTime() {
		t.Error("second call rewrote cover art (not idempotent)")
	}
}

func TestCleanSploreCartInDir_RemovesCoverArt(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := cleanSploreCartInDir(dir); err != nil {
		t.Fatalf("cleanSploreCartInDir: %v", err)
	}
	if _, err := os.Stat(dir + ".media/Splore.png"); !os.IsNotExist(err) {
		t.Errorf(".media/Splore.png should be gone; stat returned: %v", err)
	}
}

func TestCleanSploreCartInDir_ToleratesMissingCoverArt(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	// Seed only the cart, not the cover art directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := cleanSploreCartInDir(dir); err != nil {
		t.Errorf("expected no error for missing cover art, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```sh
./scripts/test.sh
```

Expected: `TestEnsureSploreCartInDir_CreatesCoverArt` fails — `.media/Splore.png not created`.

- [ ] **Step 3: Update `internal/roms/splore.go`**

Replace the entire file with:

```go
// To regenerate splore_cover.png run from the repo root:
//   go run ./cmd/gen-splore-cover/
//go:generate go run ../../cmd/gen-splore-cover splore_cover.png

package roms

import (
	_ "embed"
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

//go:embed splore_cover.png
var sploreCartCoverPNG []byte

const sploreCartFilename = "Splore.p8"

const sploreCartContent = `pico-8 cartridge // http://www.pico-8.com
version 16
__lua__
function _init()
  if type(splore)=="function" then
    splore()
  else
    cls(1)
    print("splore not available",4,52,7)
    print("requires official pico-8",4,60,6)
    print("not supported by this core",4,68,13)
  end
end
`

// EnsureSploreCart creates the Pico-8 ROM directory for the official pico8
// core and writes Splore.p8 and its cover art if not already present.
// No-op for fakeo8, which does not support splore(). Idempotent.
func EnsureSploreCart(core string) error {
	if core != "pico8" {
		return nil
	}
	return ensureSploreCartInDir(Pico8ROMDir(core))
}

// CleanSploreCart removes Splore.p8 and its cover art from the Pico-8 ROM
// directory for core. Missing files are not an error.
func CleanSploreCart(core string) error {
	return cleanSploreCartInDir(Pico8ROMDir(core))
}

func ensureSploreCartInDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("splore: mkdir %s: %v", dir, err)
		return err
	}

	// cart file
	cartPath := dir + sploreCartFilename
	if _, err := os.Stat(cartPath); err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("splore: stat %s: %v", cartPath, err)
		}
		if err := os.WriteFile(cartPath, []byte(sploreCartContent), 0644); err != nil {
			logger.Error("splore: write %s: %v", cartPath, err)
			return err
		}
		logger.Info("splore: seeded %s", cartPath)
	}

	// cover art
	mediaDir := dir + ".media/"
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		logger.Error("splore: mkdir %s: %v", mediaDir, err)
		return err
	}
	artPath := mediaDir + "Splore.png"
	if _, err := os.Stat(artPath); err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("splore: stat %s: %v", artPath, err)
		}
		if err := os.WriteFile(artPath, sploreCartCoverPNG, 0644); err != nil {
			logger.Error("splore: write cover art %s: %v", artPath, err)
			return err
		}
		logger.Info("splore: seeded cover art %s", artPath)
	}

	return nil
}

func cleanSploreCartInDir(dir string) error {
	// cart file
	cartPath := dir + sploreCartFilename
	if err := os.Remove(cartPath); err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("splore: remove %s: %v", cartPath, err)
			return err
		}
		logger.Debug("splore: clean skipped (not present): %s", cartPath)
	} else {
		logger.Info("splore: cleaned %s", cartPath)
	}

	// cover art — .media/ directory is left in place (other games' art lives there)
	artPath := dir + ".media/Splore.png"
	if err := os.Remove(artPath); err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("splore: remove cover art %s: %v", artPath, err)
			return err
		}
		logger.Debug("splore: clean cover art skipped (not present): %s", artPath)
	} else {
		logger.Info("splore: cleaned cover art %s", artPath)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```sh
./scripts/test.sh
```

Expected: all 11 packages pass, including the 4 new cover-art tests.

- [ ] **Step 5: Commit**

```sh
git add internal/roms/splore.go internal/roms/splore_test.go
git commit -m "feat(roms): embed Splore cover art, write/clean alongside cart"
```

---

## Task 3: Add `Pico8Splore` to settings (TDD)

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/settings/settings_test.go`:

```go
func TestPico8SploreDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Pico8Splore {
		t.Error("default Pico8Splore should be true")
	}
}

func TestPico8SploreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{Pico8Splore: false}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Pico8Splore != false {
		t.Errorf("Pico8Splore round-trip: got %v, want false", loaded.Pico8Splore)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```sh
./scripts/test.sh
```

Expected: `TestPico8SploreDefault` fails — `cfg.Pico8Splore` field does not exist yet.

- [ ] **Step 3: Add the field to `internal/settings/settings.go`**

In the `Config` struct, add after the `Pico8Core` line:

```go
	Pico8Core      string            `json:"pico8_core,omitempty"`     // "fakeo8" | "pico8"
	Pico8Splore    bool              `json:"pico8_splore"`             // no omitempty — false must survive save/load
```

In `defaults()`, add after `Pico8Core`:

```go
		Pico8Core:     "fakeo8",
		Pico8Splore:   true,
```

- [ ] **Step 4: Run tests to confirm they pass**

```sh
./scripts/test.sh
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```sh
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat(settings): add Pico8Splore field (default true)"
```

---

## Task 4: Add settings toggle to the UI

**Files:**
- Modify: `internal/ui/screen_settings.go`

No unit tests for this file (SDL2 UI, excluded from headless CI). Build verification only.

- [ ] **Step 1: Add the constant**

In `screen_settings.go`, add `sItemPico8Splore` immediately after `sItemPico8Core` in the iota:

```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMLocation
	sItemPico8Core
	sItemPico8Splore // ← new
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

- [ ] **Step 2: Add skip logic in `moveCursor`**

In `moveCursor`, after the `sItemMusicLocation` skip block, add:

```go
	// Skip Pico-8 Splore Store when official core is not selected.
	if s.cursor == sItemPico8Splore && s.cfg.Pico8Core != "pico8" {
		if dir >= 0 {
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			} else {
				s.cursor--
			}
		} else {
			if s.cursor > 0 {
				s.cursor--
			} else {
				s.cursor++
			}
		}
	}
```

- [ ] **Step 3: Add the item to `Draw`**

In the `Draw` method, after the `sItemPico8Core` append:

```go
	items = append(items, menuItem{sItemPico8Core, "Pico-8 Core: " + pico8CoreLabel})
	if s.cfg.Pico8Core == "pico8" {
		sploreVal := "OFF"
		if s.cfg.Pico8Splore {
			sploreVal = "ON"
		}
		items = append(items, menuItem{sItemPico8Splore, "Pico-8 Splore Store: " + sploreVal})
	}
```

- [ ] **Step 4: Add the activate case**

In `activate()`, add after the `sItemPico8Core` case:

```go
	case sItemPico8Splore:
		s.cfg.Pico8Splore = !s.cfg.Pico8Splore
		if s.cfg.Pico8Splore {
			if err := roms.EnsureSploreCart(s.cfg.Pico8Core); err != nil {
				logger.Warn("settings: splore cart: %v", err)
			}
		} else {
			if err := roms.CleanSploreCart(s.cfg.Pico8Core); err != nil {
				logger.Warn("settings: splore cart: %v", err)
			}
		}
		if err := s.cfg.Save(s.cfgPath); err != nil {
			logger.Warn("settings: save: %v", err)
		}
		logger.Info("settings: pico8 splore store changed to %v", s.cfg.Pico8Splore)
```

`roms` is not currently imported in this file — add it to the import block:

```go
import (
	"fmt"
	"os"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
)
```

- [ ] **Step 5: Build to verify**

```sh
./scripts/build.sh native
```

Expected: build succeeds with no errors.

- [ ] **Step 6: Commit**

```sh
git add internal/ui/screen_settings.go
git commit -m "feat(ui): add Pico-8 Splore Store toggle in settings"
```

---

## Task 5: Update startup call

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`

- [ ] **Step 1: Replace the startup call**

In `main_sdl.go`, find the current block (inserted by the previous splore feature):

```go
	if err := roms.EnsureSploreCart(cfg.Pico8Core); err != nil {
		logger.Warn("startup: splore cart: %v", err)
	}
```

Replace with:

```go
	if cfg.Pico8Splore {
		if err := roms.EnsureSploreCart(cfg.Pico8Core); err != nil {
			logger.Warn("startup: splore cart: %v", err)
		}
	} else {
		if err := roms.CleanSploreCart(cfg.Pico8Core); err != nil {
			logger.Warn("startup: splore cart: %v", err)
		}
	}
```

- [ ] **Step 2: Build to verify**

```sh
./scripts/build.sh native
```

Expected: build succeeds.

- [ ] **Step 3: Commit**

```sh
git add cmd/itchio-pak/main_sdl.go
git commit -m "feat(startup): gate Splore cart seeding on Pico8Splore setting"
```

---

## Task 6: Update core migration

**Files:**
- Modify: `internal/ui/screen_pico8_core_migrate.go`

- [ ] **Step 1: Gate `EnsureSploreCart` on the setting**

In `startMigration`, find the current block:

```go
		if err := roms.CleanSploreCart(s.oldCore); err != nil {
			logger.Warn("pico8-migrate: clean splore cart: %v", err)
		}
		if err := roms.EnsureSploreCart(s.newCore); err != nil {
			logger.Warn("pico8-migrate: seed splore cart: %v", err)
		}
```

Replace with:

```go
		if err := roms.CleanSploreCart(s.oldCore); err != nil {
			logger.Warn("pico8-migrate: clean splore cart: %v", err)
		}
		if s.cfg.Pico8Splore {
			if err := roms.EnsureSploreCart(s.newCore); err != nil {
				logger.Warn("pico8-migrate: seed splore cart: %v", err)
			}
		}
```

- [ ] **Step 2: Build and run full test suite**

```sh
./scripts/build.sh native && ./scripts/test.sh
```

Expected: build succeeds, all 11 packages pass.

- [ ] **Step 3: Commit**

```sh
git add internal/ui/screen_pico8_core_migrate.go
git commit -m "feat(migration): gate Splore cart seeding on Pico8Splore setting"
```
