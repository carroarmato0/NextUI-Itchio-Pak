# UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add platform filtering, search, alpha-jump navigation, a virtual keyboard, and dynamic layout scaling to the Itch.io NextUI Pak, scaling gracefully from 640×480 (Miyoo Flip) to 1280×720 (TrimUI).

**Architecture:** Three new `Screen` types (`FilterScreen`, `KeyboardScreen`) wrap the screen beneath them via the existing `prev Screen` pattern and communicate results via callbacks. Pure logic (alpha-jump, search, platform filter, layout constants) lives in headless-testable files with no build tag. SDL2 rendering code carries `//go:build !headless` as all existing UI screens do.

**Tech Stack:** Go 1.22+, SDL2 via `github.com/veandco/go-sdl2`, CGo. Tests run via `./scripts/test.sh` (containerised, headless build tag). SDL2 screens verified via `./scripts/dev-screenshot.sh`.

**Button mapping reminder (NextUI convention — SDL names are inverted from labels):**
- Physical A (confirm) = `CONTROLLER_BUTTON_B` = `K_RETURN`
- Physical B (back) = `CONTROLLER_BUTTON_A` = `K_ESCAPE`
- Physical X (secondary) = `CONTROLLER_BUTTON_X` = `K_x`
- Physical Y (clear/reset) = `CONTROLLER_BUTTON_Y` = `K_y`
- L1/R1 shoulders = `CONTROLLER_BUTTON_LEFTSHOULDER` / `CONTROLLER_BUTTON_RIGHTSHOULDER` = `K_PAGEUP` / `K_PAGEDOWN`
- START = `CONTROLLER_BUTTON_START` = `K_s`
- SELECT = `CONTROLLER_BUTTON_BACK` = `K_TAB`

---

### Task 1: Config — add PlatformFilter field

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/settings/settings_test.go`:

```go
func TestPlatformFilterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{PlatformFilter: "GBC"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PlatformFilter != "GBC" {
		t.Errorf("PlatformFilter = %q, want %q", loaded.PlatformFilter, "GBC")
	}
}

func TestPlatformFilterDefaultsToEmpty(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PlatformFilter != "" {
		t.Errorf("default PlatformFilter = %q, want %q", cfg.PlatformFilter, "")
	}
}

func TestPlatformFilterOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte("platform_filter")) {
		t.Errorf("platform_filter should be omitted when empty, found in JSON:\n%s", data)
	}
}

func TestPlatformFilterBackwardsCompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"unified_naming":true}`), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PlatformFilter != "" {
		t.Errorf("old config PlatformFilter = %q, want empty string", loaded.PlatformFilter)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure on PlatformFilter undefined**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|PlatformFilter"
```

Expected: `settings.Config undefined field PlatformFilter` or similar compile error.

- [ ] **Step 3: Add PlatformFilter to Config struct**

In `internal/settings/settings.go`, add one line to the `Config` struct after `SortMode`:

```go
	SortMode       string            `json:"sort_mode,omitempty"`
	PlatformFilter string            `json:"platform_filter,omitempty"` // "" = All; persisted to config.json
```

- [ ] **Step 4: Run tests — expect all pass**

```bash
./scripts/test.sh 2>&1 | tail -5
```

Expected: `ok  	github.com/carroarmato0/nextui-itchio-pak/internal/settings`

- [ ] **Step 5: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat(settings): add PlatformFilter field"
```

---

### Task 2: Layout system

**Files:**
- Create: `internal/ui/layout.go`
- Create: `internal/ui/layout_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/ui/layout_test.go`:

```go
package ui

import "testing"

func TestLayoutForWideScreen(t *testing.T) {
	l := LayoutFor(1280, 720)
	if l.HeaderPad != 6 {
		t.Errorf("HeaderPad = %d, want 6", l.HeaderPad)
	}
	if l.RowPad != 4 {
		t.Errorf("RowPad = %d, want 4", l.RowPad)
	}
	if l.FooterPad != 5 {
		t.Errorf("FooterPad = %d, want 5", l.FooterPad)
	}
	if l.ContentGap != 6 {
		t.Errorf("ContentGap = %d, want 6", l.ContentGap)
	}
	if l.CoverMaxW != 0.75 {
		t.Errorf("CoverMaxW = %v, want 0.75", l.CoverMaxW)
	}
}

func TestLayoutForSmallScreen(t *testing.T) {
	l := LayoutFor(640, 480)
	if l.HeaderPad != 3 {
		t.Errorf("HeaderPad = %d, want 3", l.HeaderPad)
	}
	if l.RowPad != 2 {
		t.Errorf("RowPad = %d, want 2", l.RowPad)
	}
	if l.FooterPad != 2 {
		t.Errorf("FooterPad = %d, want 2", l.FooterPad)
	}
	if l.ContentGap != 3 {
		t.Errorf("ContentGap = %d, want 3", l.ContentGap)
	}
	if l.CoverMaxW != 1.0 {
		t.Errorf("CoverMaxW = %v, want 1.0", l.CoverMaxW)
	}
}

func TestLayoutForOverlayMargin(t *testing.T) {
	wide := LayoutFor(1280, 720)
	if wide.OverlayMarginX != 1280*14/100 {
		t.Errorf("wide OverlayMarginX = %d, want %d", wide.OverlayMarginX, 1280*14/100)
	}
	small := LayoutFor(640, 480)
	if small.OverlayMarginX != 640*3/100 {
		t.Errorf("small OverlayMarginX = %d, want %d", small.OverlayMarginX, 640*3/100)
	}
}

func TestLayoutForBoundary(t *testing.T) {
	// exactly at narrowScreenW should use small layout
	l := LayoutFor(narrowScreenW, 480)
	if l.RowPad != 2 {
		t.Errorf("at boundary RowPad = %d, want 2 (small)", l.RowPad)
	}
	// one pixel wider → wide layout
	l = LayoutFor(narrowScreenW+1, 720)
	if l.RowPad != 4 {
		t.Errorf("above boundary RowPad = %d, want 4 (wide)", l.RowPad)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|LayoutFor|Layout"
```

Expected: `undefined: LayoutFor`

- [ ] **Step 3: Create `internal/ui/layout.go`**

```go
package ui

// Layout holds screen-size-dependent spacing constants derived at draw time.
// Use LayoutFor(r.W, r.H) to obtain the appropriate layout for the current screen.
type Layout struct {
	HeaderPad      int32   // vertical padding inside the header bar
	RowPad         int32   // vertical padding inside each list row
	FooterPad      int32   // vertical padding inside the footer bar
	ContentGap     int32   // gap between header and content area
	CoverMaxW      float32 // cover art width as fraction of the right panel width (0–1)
	OverlayMarginX int32   // horizontal margin for centered overlay panels (pixels each side)
}

// LayoutFor returns the layout constants appropriate for a screen of size w×h.
// Two size classes: small (w ≤ narrowScreenW) and wide (w > narrowScreenW).
func LayoutFor(w, h int32) Layout {
	if w <= narrowScreenW {
		return Layout{
			HeaderPad:      3,
			RowPad:         2,
			FooterPad:      2,
			ContentGap:     3,
			CoverMaxW:      1.0,
			OverlayMarginX: w * 3 / 100,
		}
	}
	return Layout{
		HeaderPad:      6,
		RowPad:         4,
		FooterPad:      5,
		ContentGap:     6,
		CoverMaxW:      0.75,
		OverlayMarginX: w * 14 / 100,
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL" | grep ui
```

Expected: `ok  	github.com/carroarmato0/nextui-itchio-pak/internal/ui`

- [ ] **Step 5: Commit**

```bash
git add internal/ui/layout.go internal/ui/layout_test.go
git commit -m "feat(ui): add dynamic Layout system for screen-size-aware spacing"
```

---

### Task 3: Pure filter functions (platform, search, alpha-jump)

**Files:**
- Create: `internal/ui/search_filter.go`
- Create: `internal/ui/search_filter_test.go`
- Create: `internal/ui/alpha_jump.go`
- Create: `internal/ui/alpha_jump_test.go`

- [ ] **Step 1: Write failing tests for search/platform filter**

Create `internal/ui/search_filter_test.go`:

```go
package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

var filterTestGames = []itchio.Game{
	{Title: "Alwa's Awakening", Author: "Elden Pixels", Platform: "GB"},
	{Title: "Arkade Boy", Author: "Retro Dev", Platform: "GBC"},
	{Title: "Balloon Trip Remake", Author: "Balloon Dev", Platform: "GBC"},
	{Title: "Zelda Clone", Author: "Link Fan", Platform: "NES"},
}

func TestApplyPlatformFilter_Empty(t *testing.T) {
	out := applyPlatformFilter(filterTestGames, "")
	if len(out) != len(filterTestGames) {
		t.Errorf("empty filter: got %d games, want %d", len(out), len(filterTestGames))
	}
}

func TestApplyPlatformFilter_GBC(t *testing.T) {
	out := applyPlatformFilter(filterTestGames, "GBC")
	if len(out) != 2 {
		t.Fatalf("GBC filter: got %d games, want 2", len(out))
	}
	for _, g := range out {
		if g.Platform != "GBC" {
			t.Errorf("expected GBC game, got Platform=%q", g.Platform)
		}
	}
}

func TestApplyPlatformFilter_NoMatch(t *testing.T) {
	out := applyPlatformFilter(filterTestGames, "P8")
	if len(out) != 0 {
		t.Errorf("P8 filter: got %d games, want 0", len(out))
	}
}

func TestApplySearchFilter_Empty(t *testing.T) {
	out := applySearchFilter(filterTestGames, "")
	if len(out) != len(filterTestGames) {
		t.Errorf("empty search: got %d games, want %d", len(out), len(filterTestGames))
	}
}

func TestApplySearchFilter_TitleMatch(t *testing.T) {
	out := applySearchFilter(filterTestGames, "alwa")
	if len(out) != 1 || out[0].Title != "Alwa's Awakening" {
		t.Errorf("title search: unexpected result %v", out)
	}
}

func TestApplySearchFilter_AuthorMatch(t *testing.T) {
	out := applySearchFilter(filterTestGames, "Elden")
	if len(out) != 1 || out[0].Author != "Elden Pixels" {
		t.Errorf("author search: unexpected result %v", out)
	}
}

func TestApplySearchFilter_CaseInsensitive(t *testing.T) {
	out := applySearchFilter(filterTestGames, "ZELDA")
	if len(out) != 1 || out[0].Title != "Zelda Clone" {
		t.Errorf("case-insensitive search: unexpected result %v", out)
	}
}

func TestApplySearchFilter_NoMatch(t *testing.T) {
	out := applySearchFilter(filterTestGames, "xyzzy")
	if len(out) != 0 {
		t.Errorf("no-match search: got %d games, want 0", len(out))
	}
}
```

- [ ] **Step 2: Write failing tests for alpha-jump**

Create `internal/ui/alpha_jump_test.go`:

```go
package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

var jumpGames = []itchio.Game{
	{Title: "Alwa's Awakening"},  // 0 → a
	{Title: "Arkade Boy"},        // 1 → a
	{Title: "Balloon Trip"},      // 2 → b
	{Title: "Byte Defender"},     // 3 → b
	{Title: "Cave Crawler"},      // 4 → c
	{Title: "Dino Quest"},        // 5 → d
}

func TestAlphaJumpForward(t *testing.T) {
	// From cursor=0 (a), forward should land at first 'b' (index 2)
	got := alphaJumpIndex(jumpGames, 0, 1)
	if got != 2 {
		t.Errorf("forward from 0: got %d, want 2", got)
	}
}

func TestAlphaJumpForwardMidLetter(t *testing.T) {
	// From cursor=1 (also 'a'), forward should land at first 'b' (index 2)
	got := alphaJumpIndex(jumpGames, 1, 1)
	if got != 2 {
		t.Errorf("forward from 1: got %d, want 2", got)
	}
}

func TestAlphaJumpBackward(t *testing.T) {
	// From cursor=3 (b), backward should land at first 'a' that starts a different letter from b → index 1 which is still 'a'
	// Actually: scanning back from 3 (b), next different letter is index 1 (a)
	got := alphaJumpIndex(jumpGames, 3, -1)
	if got != 1 {
		t.Errorf("backward from 3: got %d, want 1", got)
	}
}

func TestAlphaJumpAtLastLetter(t *testing.T) {
	// From cursor=5 (d), forward clamps to last game
	got := alphaJumpIndex(jumpGames, 5, 1)
	if got != 5 {
		t.Errorf("forward from last letter: got %d, want 5 (clamped)", got)
	}
}

func TestAlphaJumpAtFirstLetter(t *testing.T) {
	// From cursor=0 (a), backward clamps to 0
	got := alphaJumpIndex(jumpGames, 0, -1)
	if got != 0 {
		t.Errorf("backward from first: got %d, want 0 (clamped)", got)
	}
}

func TestAlphaJumpEmptyList(t *testing.T) {
	got := alphaJumpIndex([]itchio.Game{}, 0, 1)
	if got != 0 {
		t.Errorf("empty list: got %d, want 0", got)
	}
}

func TestAlphaJumpSingleGame(t *testing.T) {
	games := []itchio.Game{{Title: "Solo"}}
	if got := alphaJumpIndex(games, 0, 1); got != 0 {
		t.Errorf("single game forward: got %d, want 0", got)
	}
	if got := alphaJumpIndex(games, 0, -1); got != 0 {
		t.Errorf("single game backward: got %d, want 0", got)
	}
}
```

- [ ] **Step 3: Run tests — expect compile failures**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|undefined"
```

Expected: `undefined: applyPlatformFilter`, `undefined: applySearchFilter`, `undefined: alphaJumpIndex`

- [ ] **Step 4: Create `internal/ui/search_filter.go`**

```go
package ui

import (
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// applyPlatformFilter returns games whose Platform field matches platform
// (case-insensitive). An empty platform string returns all games unchanged.
func applyPlatformFilter(games []itchio.Game, platform string) []itchio.Game {
	if platform == "" {
		return games
	}
	out := make([]itchio.Game, 0, len(games))
	for _, g := range games {
		if strings.EqualFold(g.Platform, platform) {
			out = append(out, g)
		}
	}
	return out
}

// applySearchFilter returns games whose Title or Author contains query
// (case-insensitive). An empty query returns all games unchanged.
func applySearchFilter(games []itchio.Game, query string) []itchio.Game {
	if query == "" {
		return games
	}
	q := strings.ToLower(query)
	out := make([]itchio.Game, 0, len(games))
	for _, g := range games {
		if strings.Contains(strings.ToLower(g.Title), q) ||
			strings.Contains(strings.ToLower(g.Author), q) {
			out = append(out, g)
		}
	}
	return out
}
```

- [ ] **Step 5: Create `internal/ui/alpha_jump.go`**

```go
package ui

import "github.com/carroarmato0/nextui-itchio-pak/internal/itchio"

// alphaJumpIndex returns the index of the first game whose sort-key first rune
// differs from games[cursor]'s, scanning in direction dir (+1 = forward, -1 = backward).
// Returns the clamped list boundary if no new letter is found.
// Returns cursor unchanged when games is empty or cursor is out of range.
func alphaJumpIndex(games []itchio.Game, cursor, dir int) int {
	if len(games) == 0 || cursor < 0 || cursor >= len(games) {
		return cursor
	}
	curFirst := firstRune(itchio.SortKey(games[cursor].Title))
	i := cursor + dir
	for i >= 0 && i < len(games) {
		if firstRune(itchio.SortKey(games[i].Title)) != curFirst {
			return i
		}
		i += dir
	}
	// No boundary found: clamp to list edge.
	if dir > 0 {
		return len(games) - 1
	}
	return 0
}

// firstRune returns the first rune of s, or 0 if s is empty.
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}
```

- [ ] **Step 6: Run tests — expect all pass**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL" | grep ui
```

Expected: `ok  	github.com/carroarmato0/nextui-itchio-pak/internal/ui`

- [ ] **Step 7: Commit**

```bash
git add internal/ui/search_filter.go internal/ui/search_filter_test.go \
        internal/ui/alpha_jump.go internal/ui/alpha_jump_test.go
git commit -m "feat(ui): add pure filter functions: platform, search, alpha-jump"
```

---

### Task 4: Renderer — DrawModal helper

**Files:**
- Modify: `internal/renderer/renderer.go`

No unit test — SDL2 only. Verified visually via device screenshot at the end.

- [ ] **Step 1: Add DrawModal to `internal/renderer/renderer.go`**

Add after the existing `DrawFooterHints` method (around line 780):

```go
// DrawModal draws a centred modal overlay with a title, wrapped body text, and
// footer hints. Used for confirmations and informational dialogs across all screens.
func (r *Renderer) DrawModal(title, body string, hints []FooterHint) {
	_, fontH := r.TextSize("Ag")
	lineH := fontH + 4

	marginX := r.W / 8
	pad := int32(20)
	panelW := r.W - marginX*2
	bodyMaxW := panelW - pad*2

	bodyLines := r.WrapText(body, bodyMaxW)
	bodyH := int32(len(bodyLines)) * lineH
	hintsH := int32(44)
	panelH := pad + fontH + pad/2 + bodyH + pad + hintsH
	if panelH > r.H*4/5 {
		panelH = r.H * 4 / 5
	}

	panelX := marginX
	panelY := (r.H - panelH) / 2

	bg := r.Theme.Background
	// Fill
	r.DrawRect(panelX, panelY, panelW, panelH, bg[0]+20, bg[1]+20, bg[2]+20)
	// Border (1px on each edge)
	r.DrawRect(panelX, panelY, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY+panelH-1, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY, 1, panelH, 70, 70, 100)
	r.DrawRect(panelX+panelW-1, panelY, 1, panelH, 70, 70, 100)

	// Title
	mt := r.Theme.MainText
	r.DrawTextCentered(title, panelX, panelY+pad, panelW, mt[0], mt[1], mt[2])

	// Body
	ht := r.Theme.HintText
	r.DrawWrappedText(body, panelX+pad, panelY+pad+fontH+pad/2, bodyMaxW, lineH, ht[0], ht[1], ht[2])

	// Hints
	r.DrawFooterHints(hints, panelY+panelH-hintsH)
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/renderer/renderer.go
git commit -m "feat(renderer): add DrawModal helper for consistent overlay dialogs"
```

---

### Task 5: Virtual Keyboard screen

**Files:**
- Create: `internal/ui/screen_keyboard.go`

No unit test (SDL2). Verified visually.

- [ ] **Step 1: Create `internal/ui/screen_keyboard.go`**

```go
//go:build !headless

package ui

import (
	"time"
	"unicode/utf8"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

// kbGrid defines the 4×8 character grid for each keyboard page.
// 0=lowercase, 1=uppercase, 2=digits+symbols.
// Empty strings are non-interactive spacers; the cursor skips them.
var kbGrid = [3][4][8]string{
	{ // page 0: lowercase a–z
		{"a", "b", "c", "d", "e", "f", "g", "h"},
		{"i", "j", "k", "l", "m", "n", "o", "p"},
		{"q", "r", "s", "t", "u", "v", "w", "x"},
		{"y", "z", "", "", "", "SPC", "⌫", "✓"},
	},
	{ // page 1: uppercase A–Z
		{"A", "B", "C", "D", "E", "F", "G", "H"},
		{"I", "J", "K", "L", "M", "N", "O", "P"},
		{"Q", "R", "S", "T", "U", "V", "W", "X"},
		{"Y", "Z", "", "", "", "SPC", "⌫", "✓"},
	},
	{ // page 2: digits + common symbols
		{"0", "1", "2", "3", "4", "5", "6", "7"},
		{"8", "9", ".", "-", "_", "'", "!", "?"},
		{"@", "#", ":", ";", "(", ")", "+", "="},
		{"", "", "", "", "", "SPC", "⌫", "✓"},
	},
}

var kbPageLabels = [3]string{"abc", "ABC", "0-9"}

// KeyboardScreen is a full-screen virtual keyboard.
// Pressing ✓ fires onConfirm(typed value) and returns prev.
// Pressing B fires onConfirm(seed) (cancel, unchanged value) and returns prev.
type KeyboardScreen struct {
	prev      Screen
	value     []rune
	seed      string
	onConfirm func(string)

	page int // 0=lower, 1=upper, 2=digits
	row  int // 0–3; -1 = text field focused
	col  int // 0–7

	blinkOn   bool
	lastBlink time.Time
}

// NewKeyboardScreen returns a KeyboardScreen pre-filled with seed.
// onConfirm is called with the result when the user confirms or cancels.
func NewKeyboardScreen(prev Screen, seed string, onConfirm func(string)) *KeyboardScreen {
	return &KeyboardScreen{
		prev:      prev,
		value:     []rune(seed),
		seed:      seed,
		onConfirm: onConfirm,
		row:       0,
		col:       0,
		blinkOn:   true,
		lastBlink: time.Now(),
	}
}

func (s *KeyboardScreen) NeedsRedraw() bool      { return true }
func (s *KeyboardScreen) HasPendingAnimation() bool { return false }

func (s *KeyboardScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	headerH := int32(48) // compact header — more room for grid
	footerH := int32(44)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Enter text", 12, textY, mt[0], mt[1], mt[2])

	// Page indicator right-aligned in header
	pageLabel := kbPageLabels[s.page]
	ht := r.Theme.HintText
	pw, _ := r.SmallTextSize(pageLabel)
	r.DrawSmallText(pageLabel, r.W-pw-12, textY, ht[0], ht[1], ht[2])

	// Blink update
	if time.Since(s.lastBlink) > 500*time.Millisecond {
		s.blinkOn = !s.blinkOn
		s.lastBlink = time.Now()
	}

	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	contentY := headerH + 6

	// Text input field
	fieldH := fontH + 16
	fieldX := int32(8)
	fieldW := r.W - 16

	// Field background
	r.DrawRect(fieldX, contentY, fieldW, fieldH, 25, 25, 38)
	// Border: accent when text field focused, dim otherwise
	var bR, bG, bB uint8
	if s.row == -1 {
		ac := r.Theme.Accent
		bR, bG, bB = ac[0], ac[1], ac[2]
	} else {
		bR, bG, bB = 60, 60, 80
	}
	r.DrawRect(fieldX, contentY, fieldW, 1, bR, bG, bB)
	r.DrawRect(fieldX, contentY+fieldH-1, fieldW, 1, bR, bG, bB)
	r.DrawRect(fieldX, contentY, 1, fieldH, bR, bG, bB)
	r.DrawRect(fieldX+fieldW-1, contentY, 1, fieldH, bR, bG, bB)

	displayText := string(s.value)
	if s.blinkOn {
		displayText += "▌"
	}
	r.DrawText(displayText, fieldX+8, contentY+(fieldH-fontH)/2, mt[0], mt[1], mt[2])

	contentY += fieldH + 4

	// Page tabs (small indicator row)
	tabY := contentY
	tabH := smallFH + 4
	tabW := r.W / 3
	for i, label := range kbPageLabels {
		tx := int32(i) * tabW
		if i == s.page {
			ac := r.Theme.Accent
			aT := r.Theme.AccentText
			r.DrawRect(tx, tabY, tabW, tabH, ac[0], ac[1], ac[2])
			r.DrawSmallTextCentered(label, tx, tabY+(tabH-smallFH)/2, tabW, aT[0], aT[1], aT[2])
		} else {
			r.DrawSmallTextCentered(label, tx, tabY+(tabH-smallFH)/2, tabW, 80, 80, 100)
		}
	}
	contentY += tabH + 4

	// Character grid
	cols := int32(8)
	rows := int32(4)
	margin := int32(4)
	availW := r.W - margin*2
	cellW := availW / cols
	availH := r.H - footerH - contentY - 4
	cellH := availH / rows

	for row := 0; row < 4; row++ {
		for col := 0; col < 8; col++ {
			ch := kbGrid[s.page][row][col]
			if ch == "" {
				continue
			}
			cx := margin + int32(col)*cellW
			cy := contentY + int32(row)*cellH
			isSelected := s.row == row && s.col == col

			var cellR, cellG, cellB uint8
			if isSelected {
				ac := r.Theme.Accent
				cellR, cellG, cellB = ac[0], ac[1], ac[2]
			} else {
				cellR, cellG, cellB = 28, 28, 42
			}
			r.DrawPill(cx+1, cy+1, cellW-2, cellH-2, cellR, cellG, cellB)

			var fR, fG, fB uint8
			if isSelected {
				aT := r.Theme.AccentText
				fR, fG, fB = aT[0], aT[1], aT[2]
			} else {
				fR, fG, fB = 190, 190, 210
			}
			r.DrawSmallTextCenteredInRect(ch, cx+1, cy+1, cellW-2, cellH-2, fR, fG, fB)
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Type/Confirm"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		{Kind: renderer.BadgePill, Label: "L1R1", Text: "Page"},
	}, ftrY)
	r.Present()
}

func (s *KeyboardScreen) HandleEvent(e sdl.Event) Screen {
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

func (s *KeyboardScreen) handleKey(sym sdl.Keycode) Screen {
	switch sym {
	case sdl.K_RETURN: // physical A
		return s.activate()
	case sdl.K_ESCAPE: // physical B — cancel
		if s.onConfirm != nil {
			s.onConfirm(s.seed)
		}
		return s.prev
	case sdl.K_UP:
		s.moveUp()
	case sdl.K_DOWN:
		s.moveDown()
	case sdl.K_LEFT:
		if s.row >= 0 {
			s.col = kbSkipLeft(s.page, s.row, s.col)
		}
	case sdl.K_RIGHT:
		if s.row >= 0 {
			s.col = kbSkipRight(s.page, s.row, s.col)
		}
	case sdl.K_PAGEUP: // L1 — previous page
		s.page = (s.page + 2) % 3
		s.clampCol()
	case sdl.K_PAGEDOWN: // R1 — next page
		s.page = (s.page + 1) % 3
		s.clampCol()
	}
	return s
}

func (s *KeyboardScreen) handleButton(btn uint8) Screen {
	switch btn {
	case sdl.CONTROLLER_BUTTON_B: // physical A — type/confirm
		return s.activate()
	case sdl.CONTROLLER_BUTTON_A: // physical B — cancel
		if s.onConfirm != nil {
			s.onConfirm(s.seed)
		}
		return s.prev
	case sdl.CONTROLLER_BUTTON_DPAD_UP:
		s.moveUp()
	case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
		s.moveDown()
	case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
		if s.row >= 0 {
			s.col = kbSkipLeft(s.page, s.row, s.col)
		}
	case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
		if s.row >= 0 {
			s.col = kbSkipRight(s.page, s.row, s.col)
		}
	case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
		s.page = (s.page + 2) % 3
		s.clampCol()
	case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
		s.page = (s.page + 1) % 3
		s.clampCol()
	}
	return s
}

func (s *KeyboardScreen) activate() Screen {
	if s.row == -1 {
		// Text field focused — backspace on A
		if len(s.value) > 0 {
			s.value = s.value[:len(s.value)-1]
		}
		return s
	}
	ch := kbGrid[s.page][s.row][s.col]
	switch ch {
	case "✓":
		logger.Debug("keyboard: confirmed value len=%d", len(s.value))
		if s.onConfirm != nil {
			s.onConfirm(string(s.value))
		}
		return s.prev
	case "⌫":
		if len(s.value) > 0 {
			s.value = s.value[:len(s.value)-1]
		}
	case "SPC":
		s.value = append(s.value, ' ')
	default:
		r, size := utf8.DecodeRuneInString(ch)
		if size > 0 && r != utf8.RuneError {
			s.value = append(s.value, r)
		}
	}
	return s
}

func (s *KeyboardScreen) moveUp() {
	if s.row <= 0 {
		s.row = -1
		return
	}
	s.row--
	s.clampCol()
}

func (s *KeyboardScreen) moveDown() {
	if s.row == -1 {
		s.row = 0
		s.clampCol()
		return
	}
	if s.row < 3 {
		s.row++
		s.clampCol()
	}
}

// clampCol moves col to the nearest non-empty cell on the current row/page.
func (s *KeyboardScreen) clampCol() {
	if s.row < 0 {
		return
	}
	if kbGrid[s.page][s.row][s.col] != "" {
		return
	}
	for d := 1; d < 8; d++ {
		if c := (s.col + d) % 8; kbGrid[s.page][s.row][c] != "" {
			s.col = c
			return
		}
		if c := (s.col - d + 8) % 8; kbGrid[s.page][s.row][c] != "" {
			s.col = c
			return
		}
	}
}

// kbSkipRight returns the column of the next non-empty cell to the right, wrapping.
func kbSkipRight(page, row, col int) int {
	for d := 1; d <= 8; d++ {
		c := (col + d) % 8
		if kbGrid[page][row][c] != "" {
			return c
		}
	}
	return col
}

// kbSkipLeft returns the column of the next non-empty cell to the left, wrapping.
func kbSkipLeft(page, row, col int) int {
	for d := 1; d <= 8; d++ {
		c := (col - d + 8) % 8
		if kbGrid[page][row][c] != "" {
			return c
		}
	}
	return col
}
```

- [ ] **Step 2: Build to confirm it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_keyboard.go
git commit -m "feat(ui): add KeyboardScreen — 4×8 console-style virtual keyboard"
```

---

### Task 6: Filter Overlay screen

**Files:**
- Create: `internal/ui/screen_filter.go`

No unit test (SDL2). Verified visually.

- [ ] **Step 1: Create `internal/ui/screen_filter.go`**

```go
//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

type filterSection int

const (
	filterSectionSearch   filterSection = iota
	filterSectionPlatform
	filterSectionSort
)

// filterPlatforms is the ordered list of platform codes shown in the overlay.
// "" is "All platforms".
var filterPlatforms = []string{"", "GB", "GBC", "GBA", "NES", "MD", "P8"}
var filterPlatformLabels = []string{"All", "GB", "GBC", "GBA", "NES", "MD", "P8"}

// FilterScreen is a SELECT overlay showing Search / Platform / Sort sections.
// It wraps prev and calls onApply(platform, sort, query) on SELECT/apply.
// It calls no callback and returns prev on B (cancel).
type FilterScreen struct {
	prev    Screen
	onApply func(platform, sort, query string)

	// working copies modified by the user; not committed until onApply fires
	platform string
	sort     string
	query    string

	// original values (for cancel)
	origPlatform string
	origSort     string
	origQuery    string

	section filterSection // which section has focus
	platCol int           // cursor within platform pill row
	sortCol int           // cursor within sort pill row
}

// NewFilterScreen constructs the filter overlay with the current active values.
// onApply is called when the user presses SELECT to apply.
func NewFilterScreen(
	prev Screen,
	platform, sort, query string,
	onApply func(platform, sort, query string),
) *FilterScreen {
	platCol := 0
	for i, p := range filterPlatforms {
		if p == platform {
			platCol = i
			break
		}
	}
	sortModes := []itchio.SortMode{
		itchio.SortModeRSS, itchio.SortModeAZ, itchio.SortModeZA,
		itchio.SortModeNew, itchio.SortModeFree, itchio.SortModePaid,
		itchio.SortModeDL, itchio.SortModeOwned,
	}
	sortCol := 0
	for i, m := range sortModes {
		if string(m) == sort {
			sortCol = i
			break
		}
	}
	return &FilterScreen{
		prev:         prev,
		onApply:      onApply,
		platform:     platform,
		sort:         sort,
		query:        query,
		origPlatform: platform,
		origSort:     sort,
		origQuery:    query,
		section:      filterSectionSearch,
		platCol:      platCol,
		sortCol:      sortCol,
	}
}

func (s *FilterScreen) NeedsRedraw() bool      { return false }
func (s *FilterScreen) HasPendingAnimation() bool { return false }

func (s *FilterScreen) Draw(r *renderer.Renderer) {
	lyt := LayoutFor(r.W, r.H)

	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	// Semi-transparent dark overlay to hint list is behind
	r.DrawRect(0, 0, r.W, r.H, bg[0]/3, bg[1]/3, bg[2]/3)

	// Panel bounds
	panelX := lyt.OverlayMarginX
	panelW := r.W - lyt.OverlayMarginX*2
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	footerH := int32(44)
	lineH := fontH + 4

	// Estimate panel height
	sectionGap := int32(10)
	headerH := fontH + 16
	searchH := fontH + 16
	labelH := smallFH + 2
	pillRowH := smallFH + 10
	panelH := headerH + sectionGap +
		labelH + searchH + sectionGap +
		labelH + pillRowH + sectionGap +
		labelH + pillRowH + sectionGap +
		footerH
	if panelH > r.H-20 {
		panelH = r.H - 20
	}
	panelY := (r.H - panelH) / 2

	// Panel background + border
	r.DrawRect(panelX, panelY, panelW, panelH, bg[0]+22, bg[1]+22, bg[2]+22)
	r.DrawRect(panelX, panelY, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY+panelH-1, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY, 1, panelH, 70, 70, 100)
	r.DrawRect(panelX+panelW-1, panelY, 1, panelH, 70, 70, 100)

	// Panel title
	mt := r.Theme.MainText
	pad := int32(12)
	r.DrawTextCentered("Filter & Search", panelX, panelY+pad, panelW, mt[0], mt[1], mt[2])
	y := panelY + pad + fontH + pad

	// Helper: section label
	drawSectionLabel := func(label string, focused bool) {
		var lr, lg, lb uint8
		if focused {
			ac := r.Theme.Accent
			lr, lg, lb = ac[0], ac[1], ac[2]
		} else {
			lr, lg, lb = 100, 110, 130
		}
		r.DrawSmallText(label, panelX+pad, y, lr, lg, lb)
		y += labelH
	}

	// Helper: pill row
	drawPillRow := func(labels []string, activeIdx int, focused bool) {
		x := panelX + pad
		for i, label := range labels {
			isActive := i == activeIdx
			isCursor := focused && i == s.pillCursorForSection(s.section)
			w, _ := r.SmallTextSize(label)
			pw := w + 12
			ph := smallFH + 6

			var bgR, bgG, bgB uint8
			var fgR, fgG, fgB uint8

			switch {
			case isActive && isCursor:
				ac := r.Theme.Accent
				bgR, bgG, bgB = ac[0], ac[1], ac[2]
				aT := r.Theme.AccentText
				fgR, fgG, fgB = aT[0], aT[1], aT[2]
			case isActive:
				ac := r.Theme.Accent
				bgR = ac[0]/2 + 20
				bgG = ac[1]/2 + 20
				bgB = ac[2]/2 + 20
				aT := r.Theme.AccentText
				fgR, fgG, fgB = aT[0], aT[1], aT[2]
			case isCursor:
				bgR, bgG, bgB = 50, 50, 70
				fgR, fgG, fgB = 200, 200, 220
			default:
				bgR, bgG, bgB = 30, 30, 45
				fgR, fgG, fgB = 120, 120, 140
			}
			r.DrawPill(x, y, pw, ph, bgR, bgG, bgB)
			r.DrawSmallTextCenteredInRect(label, x, y, pw, ph, fgR, fgG, fgB)
			x += pw + 4
			if x+pw > panelX+panelW-pad {
				x = panelX + pad
				y += int32(ph) + 3
			}
		}
		y += pillRowH
	}

	// ── Search section ──
	searchFocused := s.section == filterSectionSearch
	drawSectionLabel("SEARCH", searchFocused)
	fieldH := fontH + 12
	var sfR, sfG, sfB uint8
	if searchFocused {
		ac := r.Theme.Accent
		sfR, sfG, sfB = ac[0], ac[1], ac[2]
	} else {
		sfR, sfG, sfB = 60, 60, 80
	}
	r.DrawRect(panelX+pad, y, panelW-pad*2, fieldH, 22, 22, 35)
	r.DrawRect(panelX+pad, y, panelW-pad*2, 1, sfR, sfG, sfB)
	r.DrawRect(panelX+pad, y+fieldH-1, panelW-pad*2, 1, sfR, sfG, sfB)
	displayQuery := s.query
	if displayQuery == "" {
		ht := r.Theme.HintText
		r.DrawText("(tap A to type)", panelX+pad+6, y+(fieldH-fontH)/2, ht[0], ht[1], ht[2])
	} else {
		r.DrawText(displayQuery, panelX+pad+6, y+(fieldH-fontH)/2, mt[0], mt[1], mt[2])
	}
	if s.query != "" {
		clearW, _ := r.SmallTextSize("✕ clear")
		clearX := panelX + panelW - pad - clearW - 4
		ht := r.Theme.HintText
		r.DrawSmallText("✕ clear", clearX, y+(fieldH-smallFH)/2, ht[0], ht[1], ht[2])
	}
	y += fieldH + sectionGap

	// ── Platform section ──
	platFocused := s.section == filterSectionPlatform
	drawSectionLabel("PLATFORM", platFocused)
	platActiveIdx := 0
	for i, p := range filterPlatforms {
		if p == s.platform {
			platActiveIdx = i
			break
		}
	}
	_ = lineH // suppress unused warning
	drawPillRow(filterPlatformLabels, platActiveIdx, platFocused)
	y += sectionGap

	// ── Sort section ──
	sortFocused := s.section == filterSectionSort
	drawSectionLabel("SORT", sortFocused)
	sortLabels := []string{"RSS", "A-Z", "Z-A", "New", "Free", "Paid", "DL", "Owned"}
	sortValues := []string{"", "az", "za", "new", "free", "paid", "dl", "owned"}
	sortActiveIdx := 0
	for i, v := range sortValues {
		if v == s.sort {
			sortActiveIdx = i
			break
		}
	}
	drawPillRow(sortLabels, sortActiveIdx, sortFocused)

	// Footer hints
	ftrY := r.DrawFooterBar(footerH)
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		{Kind: renderer.BadgePill, Label: "SELECT", Text: "Apply"},
	}
	if s.query != "" || s.platform != "" || s.sort != "" {
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "Y", Text: "Clear all"})
	}
	r.DrawFooterHints(hints, ftrY)

	r.Present()
}

// pillCursorForSection returns the pill column index for the given section.
func (s *FilterScreen) pillCursorForSection(sec filterSection) int {
	switch sec {
	case filterSectionPlatform:
		return s.platCol
	case filterSectionSort:
		return s.sortCol
	}
	return 0
}

func (s *FilterScreen) HandleEvent(e sdl.Event) Screen {
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

func (s *FilterScreen) handleKey(sym sdl.Keycode) Screen {
	switch sym {
	case sdl.K_UP:
		s.moveSectionUp()
	case sdl.K_DOWN:
		s.moveSectionDown()
	case sdl.K_LEFT:
		s.movePillLeft()
	case sdl.K_RIGHT:
		s.movePillRight()
	case sdl.K_RETURN: // physical A
		return s.activate()
	case sdl.K_ESCAPE: // physical B — cancel
		logger.Debug("filter: cancelled")
		return s.prev
	case sdl.K_TAB: // SELECT — apply
		return s.apply()
	case sdl.K_y: // Y — clear all
		s.clearAll()
	}
	return s
}

func (s *FilterScreen) handleButton(btn uint8) Screen {
	switch btn {
	case sdl.CONTROLLER_BUTTON_DPAD_UP:
		s.moveSectionUp()
	case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
		s.moveSectionDown()
	case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
		s.movePillLeft()
	case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
		s.movePillRight()
	case sdl.CONTROLLER_BUTTON_B: // physical A — select/activate
		return s.activate()
	case sdl.CONTROLLER_BUTTON_A: // physical B — cancel
		logger.Debug("filter: cancelled")
		return s.prev
	case sdl.CONTROLLER_BUTTON_BACK: // SELECT — apply
		return s.apply()
	case sdl.CONTROLLER_BUTTON_Y: // Y — clear all
		s.clearAll()
	}
	return s
}

func (s *FilterScreen) moveSectionUp() {
	if s.section > filterSectionSearch {
		s.section--
	}
}

func (s *FilterScreen) moveSectionDown() {
	if s.section < filterSectionSort {
		s.section++
	}
}

func (s *FilterScreen) movePillLeft() {
	switch s.section {
	case filterSectionPlatform:
		if s.platCol > 0 {
			s.platCol--
		}
	case filterSectionSort:
		if s.sortCol > 0 {
			s.sortCol--
		}
	}
}

func (s *FilterScreen) movePillRight() {
	switch s.section {
	case filterSectionPlatform:
		if s.platCol < len(filterPlatforms)-1 {
			s.platCol++
		}
	case filterSectionSort:
		sortLen := 8 // RSS, AZ, ZA, New, Free, Paid, DL, Owned
		if s.sortCol < sortLen-1 {
			s.sortCol++
		}
	}
}

func (s *FilterScreen) activate() Screen {
	sortValues := []string{"", "az", "za", "new", "free", "paid", "dl", "owned"}
	switch s.section {
	case filterSectionSearch:
		// Open keyboard for search
		return NewKeyboardScreen(s, s.query, func(result string) {
			s.query = result
		})
	case filterSectionPlatform:
		s.platform = filterPlatforms[s.platCol]
	case filterSectionSort:
		s.sort = sortValues[s.sortCol]
	}
	return s
}

func (s *FilterScreen) apply() Screen {
	logger.Info("filter: applying platform=%q sort=%q query=%q", s.platform, s.sort, s.query)
	if s.onApply != nil {
		s.onApply(s.platform, s.sort, s.query)
	}
	return s.prev
}

func (s *FilterScreen) clearAll() {
	s.platform = ""
	s.sort = ""
	s.query = ""
	s.platCol = 0
	s.sortCol = 0
	logger.Debug("filter: cleared all")
}
```

- [ ] **Step 2: Build to confirm it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_filter.go
git commit -m "feat(ui): add FilterScreen — SELECT overlay for platform, sort, and search"
```

---

### Task 7: ListScreen — new fields, SetFilter, rebuildView update

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Add new state fields to `ListScreen` struct**

In `screen_list.go`, add these fields to the `ListScreen` struct after the existing `sortMode` and `viewGames` fields:

```go
	// Filter/search state — applied in rebuildView before ApplySort.
	platformFilter string // "" = All; persisted to config.json
	searchQuery    string // "" = no filter; session-only
```

- [ ] **Step 2: Load platformFilter from config in `NewListScreen`**

In `NewListScreen`, after `s.sortMode = itchio.SortMode(cfg.SortMode)`, add:

```go
	s.platformFilter = cfg.PlatformFilter
```

- [ ] **Step 3: Add `SetFilter` method**

Add this method to `screen_list.go`:

```go
// SetFilter updates the active platform filter, sort mode, and search query,
// then rebuilds the view and persists platform + sort to config.
func (s *ListScreen) SetFilter(platform, sort, query string) {
	s.platformFilter = platform
	s.sortMode = itchio.SortMode(sort)
	s.searchQuery = query
	s.rebuildView()
	s.cursor = 0
	s.titleScrollX = 0
	s.titleScrollAt = time.Now()
	s.tagScrollY = 0
	s.tagScrollAt = time.Now()
	s.lastCursorMove = time.Now()
	s.warmedGameURL = ""
	s.cfg.PlatformFilter = platform
	s.cfg.SortMode = string(s.sortMode)
	go s.cfg.Save(s.cfgPath)
	logger.Info("filter: platform=%q sort=%q query=%q", platform, sort, query)
}
```

- [ ] **Step 4: Update `rebuildView` to apply platform and search filters**

In `rebuildView()`, replace the line:

```go
	s.viewGames = itchio.ApplySort(s.cachedGames, s.sortMode, downloaded, pendingUpdates, removed, s.ownedURLs)
```

with:

```go
	filtered := s.cachedGames
	if s.platformFilter != "" {
		filtered = applyPlatformFilter(filtered, s.platformFilter)
	}
	if s.searchQuery != "" {
		filtered = applySearchFilter(filtered, s.searchQuery)
	}
	s.viewGames = itchio.ApplySort(filtered, s.sortMode, downloaded, pendingUpdates, removed, s.ownedURLs)
```

- [ ] **Step 5: Build to confirm it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): add ListScreen.SetFilter and platform/search filtering in rebuildView"
```

---

### Task 8: ListScreen — header pills + SELECT handler

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Replace sort badge with two filter pills in `Draw`**

In `screen_list.go`'s `Draw` method, find the existing sort badge block and replace it:

**Remove this block (existing sort badge):**

```go
		if s.cacheReady {
			badge := itchio.SortModeBadge(s.sortMode)
			bw, bh := r.TextSize(badge)
			const hPad = int32(8)
			pillW := bw + hPad*2
			pillH := bh + 4
			pillX := r.W - pillW - 12
			pillY := headerTextY - 2
			ac := r.Theme.Accent
			aT := r.Theme.AccentText
			r.DrawPill(pillX, pillY, pillW, pillH, ac[0], ac[1], ac[2])
			r.DrawTextCenteredInRect(badge, pillX, pillY, pillW, pillH, aT[0], aT[1], aT[2])
		}
```

**Replace with:**

```go
		// Filter pills — platform and sort — right-aligned in header.
		// Shown whenever the cache is ready or when filters/sort are non-default.
		{
			_, smallFH := r.SmallTextSize("Ag")
			pillH := smallFH + 6
			pillY := headerTextY - 2

			// Sort pill
			sortLabel := "● " + itchio.SortModeBadge(s.sortMode)
			sw, _ := r.SmallTextSize(sortLabel)
			sortPillW := sw + 12
			sortPillX := r.W - sortPillW - 10
			var sortBgR, sortBgG, sortBgB uint8
			if s.sortMode == itchio.SortModeRSS {
				sortBgR, sortBgG, sortBgB = 35, 50, 35
			} else {
				ac := r.Theme.Accent
				sortBgR, sortBgG, sortBgB = ac[0]/2+18, ac[1]/2+18, ac[2]/2+18
			}
			r.DrawPill(sortPillX, pillY, sortPillW, pillH, sortBgR, sortBgG, sortBgB)
			aT := r.Theme.AccentText
			r.DrawSmallTextCenteredInRect(sortLabel, sortPillX, pillY, sortPillW, pillH, aT[0], aT[1], aT[2])

			// Platform pill (to the left of sort pill)
			platLabel := "● " + s.platformLabel()
			pw, _ := r.SmallTextSize(platLabel)
			platPillW := pw + 12
			platPillX := sortPillX - platPillW - 6
			var platBgR, platBgG, platBgB uint8
			if s.platformFilter == "" {
				platBgR, platBgG, platBgB = 30, 40, 55
			} else {
				platBgR, platBgG, platBgB = 30, 55, 80
			}
			r.DrawPill(platPillX, pillY, platPillW, pillH, platBgR, platBgG, platBgB)
			r.DrawSmallTextCenteredInRect(platLabel, platPillX, pillY, platPillW, pillH, aT[0], aT[1], aT[2])
		}
```

- [ ] **Step 2: Add `platformLabel` helper method**

Add to `screen_list.go`:

```go
// platformLabel returns the display label for the current platform filter.
// Empty filter returns "All".
func (s *ListScreen) platformLabel() string {
	if s.platformFilter == "" {
		return "All"
	}
	return s.platformFilter
}
```

- [ ] **Step 3: Add SELECT button handler in `HandleEvent`**

In the keyboard event switch in `HandleEvent`, add after the existing `case sdl.K_s:` (START → settings):

```go
			case sdl.K_TAB: // SELECT → filter overlay
				return NewFilterScreen(s, s.platformFilter, string(s.sortMode), s.searchQuery,
					func(platform, sort, query string) {
						s.SetFilter(platform, sort, query)
					})
```

In the controller button switch, add after `case sdl.CONTROLLER_BUTTON_START:`:

```go
			case sdl.CONTROLLER_BUTTON_BACK: // SELECT → filter overlay
				return NewFilterScreen(s, s.platformFilter, string(s.sortMode), s.searchQuery,
					func(platform, sort, query string) {
						s.SetFilter(platform, sort, query)
					})
```

- [ ] **Step 4: Update footer hints to include SELECT**

In the `Draw` method, replace the footer hints block:

```go
	footerHintsBuf = footerHintsBuf[:0]
	footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"})
	footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"})
	footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgePill, Label: "SELECT", Text: "Filter"})
	if s.cacheReady {
		if s.sortMode == itchio.SortModeAZ || s.sortMode == itchio.SortModeZA {
			if r.W <= narrowScreenW {
				footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgePill, Label: "LR", Text: "A→Z"})
			} else {
				footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgePill, Label: "L1R1", Text: "A→Z"})
			}
		} else {
			if r.W <= narrowScreenW {
				footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgePill, Label: "LR", Text: "Page"})
			} else {
				footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgePill, Label: "L1R1", Text: "Page"})
			}
		}
	}
	if r.W <= narrowScreenW {
		footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Set"})
	} else {
		footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Settings"})
	}
	r.DrawFooterHints(footerHintsBuf, ftrY)
```

- [ ] **Step 5: Build to confirm it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): replace sort badge with filter pills; add SELECT → FilterScreen"
```

---

### Task 9: ListScreen — right panel redesign

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Update right panel draw code to use LayoutFor and smaller cover art**

In the `Draw` method of `screen_list.go`, find the right panel block starting with:

```go
	// Right panel: cover art (or placeholder) + metadata
	if s.cursor < len(s.viewGames) {
		g := s.viewGames[s.cursor]
		metaY := contentTop
		boxW := rightW
		boxH := rightW * 3 / 4 // 4:3 aspect ratio box
```

Replace the entire right panel section (from there until the footer hints block) with:

```go
	// Right panel: cover art + metadata below it
	if s.cursor < len(s.viewGames) {
		g := s.viewGames[s.cursor]
		lyt := LayoutFor(r.W, r.H)
		_, fontH2 := r.TextSize("Ag")
		_, smallFH2 := r.SmallTextSize("Ag")

		metaY := contentTop

		// Cover art box — width is CoverMaxW fraction of the right panel, centred.
		artW := int32(float32(rightW) * lyt.CoverMaxW)
		artH := artW * 3 / 4 // 4:3
		artX := rightX + (rightW-artW)/2

		r.DrawRect(artX, metaY, artW, artH, bg[0], bg[1], bg[2])

		if g.CoverURL != "" {
			tex := s.cache.Peek(r, g.CoverURL)
			if tex != nil {
				_, _, tw, th, _ := tex.Query()
				scaleW := float32(artW) / float32(tw)
				scaleH := float32(artH) / float32(th)
				scale := scaleW
				if scaleH < scaleW {
					scale = scaleH
				}
				dw := int32(float32(tw) * scale)
				dh := int32(float32(th) * scale)
				imgX := artX + (artW-dw)/2
				imgY := metaY + (artH-dh)/2
				r.DrawTextureAt(tex, imgX, imgY, dw, dh)
				// Status badge overlay on cover art
				if s.inv.HasPendingUpdates(g.URL) || s.inv.IsRemoved(g.URL) || s.inv.IsPresent(g.URL) {
					var pillLabel string
					var pillR, pillG, pillB uint8
					var shadowR, shadowG, shadowB uint8
					var textR, textG, textB uint8
					if s.inv.HasPendingUpdates(g.URL) {
						pillLabel = "UPDATE"
						pillR, pillG, pillB = 240, 160, 40
						shadowR, shadowG, shadowB = 160, 96, 16
						textR, textG, textB = 20, 20, 20
					} else if s.inv.IsRemoved(g.URL) {
						pillLabel = "REMOVED"
						pillR, pillG, pillB = 200, 60, 60
						shadowR, shadowG, shadowB = 122, 16, 16
						textR, textG, textB = 255, 255, 255
					} else {
						pillLabel = "DL"
						pillR, pillG, pillB = 80, 200, 220
						shadowR, shadowG, shadowB = 30, 130, 150
						textR, textG, textB = 20, 20, 20
					}
					lw, lh := r.SmallTextSize(pillLabel)
					const pad = int32(5)
					overlayPillW := lw + pad*2
					overlayPillH := lh + 4
					overlayPillX := imgX + dw - overlayPillW - 6
					overlayPillY := imgY + 6
					r.DrawPill(overlayPillX+1, overlayPillY+1, overlayPillW, overlayPillH, shadowR, shadowG, shadowB)
					r.DrawPill(overlayPillX, overlayPillY, overlayPillW, overlayPillH, pillR, pillG, pillB)
					r.DrawSmallTextCenteredInRect(pillLabel, overlayPillX, overlayPillY, overlayPillW, overlayPillH, textR, textG, textB)
				}
			} else if s.cache.Failed(g.CoverURL) {
				r.DrawTextCenteredInRect("No Image", artX, metaY, artW, artH, 80, 80, 80)
			} else {
				r.DrawTextCenteredInRect("Loading...", artX, metaY, artW, artH, 80, 80, 80)
			}
		} else {
			r.DrawRect(artX+2, metaY+2, artW-4, artH-4, bg[0], bg[1], bg[2])
			r.DrawRect(artX+3, metaY+3, artW-6, artH-6, 35, 35, 35)
			r.DrawTextCenteredInRect("No Image", artX, metaY, artW, artH, 80, 80, 80)
		}
		metaY += artH + int32(lyt.ContentGap)

		// Metadata below cover art
		availMetaH := r.H - footerH - metaY
		if availMetaH > 0 {
			// Title (bold if downloaded)
			mt2 := r.Theme.MainText
			if g.Title != "" && metaY < r.H-footerH {
				titleMaxW := rightW - 4
				if s.inv.IsPresent(g.URL) || s.inv.HasPendingUpdates(g.URL) || s.inv.IsRemoved(g.URL) {
					r.DrawBoldText(truncateBoldToWidth(r, g.Title, titleMaxW), rightX, metaY, mt2[0], mt2[1], mt2[2])
				} else {
					r.DrawText(truncateToWidth(r, g.Title, titleMaxW), rightX, metaY, mt2[0], mt2[1], mt2[2])
				}
				metaY += fontH2 + 2
			}
			// Author
			if g.Author != "" && metaY < r.H-footerH {
				ht2 := r.Theme.HintText
				r.DrawSmallText("by "+g.Author, rightX, metaY, ht2[0], ht2[1], ht2[2])
				metaY += smallFH2 + 4
			}
			// Tags as pills (clipped to available height)
			filteredTagsBuf = filteredTagsBuf[:0]
			for _, tag := range g.Tags {
				if strings.EqualFold(tag, "free") {
					continue
				}
				if len(tag) > 0 && strings.ContainsRune("$€£¥", rune(tag[0])) {
					continue
				}
				filteredTagsBuf = append(filteredTagsBuf, tag)
			}
			if len(filteredTagsBuf) > 0 && metaY < r.H-footerH {
				ac := r.Theme.Accent
				aT2 := r.Theme.AccentText
				bgPill := [3]uint8{
					uint8((int(ac[0]) + 35) / 2),
					uint8((int(ac[1]) + 35) / 2),
					uint8((int(ac[2]) + 35) / 2),
				}
				tagAreaH := r.H - footerH - metaY - (smallFH2 + 8)
				if tagAreaH > 0 {
					r.SetClipRect(rightX, metaY, rightW, tagAreaH)
					lineGap := smallFH2 + 4
					r.DrawTagPills(filteredTagsBuf, rightX, metaY, rightW, lineGap,
						aT2[0], aT2[1], aT2[2], bgPill[0], bgPill[1], bgPill[2])
					r.ClearClipRect()
					metaY += tagAreaH + 2
				}
			}
			// Price / status badge at the bottom of the metadata area
			if metaY < r.H-footerH {
				var priceLabel string
				var priceR, priceG, priceB uint8
				switch {
				case s.inv.HasPendingUpdates(g.URL):
					priceLabel, priceR, priceG, priceB = "UPDATE", 240, 160, 40
				case s.inv.IsRemoved(g.URL):
					priceLabel, priceR, priceG, priceB = "REMOVED", 200, 60, 60
				case s.inv.IsPresent(g.URL):
					priceLabel, priceR, priceG, priceB = "Downloaded", 80, 200, 220
				case s.ownedURLs[g.URL]:
					priceLabel, priceR, priceG, priceB = "Owned", 60, 200, 120
				case g.IsFree:
					priceLabel, priceR, priceG, priceB = "Free", 80, 200, 80
				default:
					priceLabel = s.badgePrice(g.URL, g.Price)
					priceR, priceG, priceB = 220, 180, 60
				}
				pw, _ := r.SmallTextSize(priceLabel)
				pillW2 := pw + 12
				pillH2 := smallFH2 + 6
				r.DrawPill(rightX, metaY, pillW2, pillH2, priceR, priceG, priceB)
				r.DrawSmallTextCenteredInRect(priceLabel, rightX, metaY, pillW2, pillH2, 20, 20, 20)
			}
		}
	}
```

- [ ] **Step 2: Build to confirm it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): redesign right panel — smaller cover art, static metadata below"
```

---

### Task 10: ListScreen — alpha-jump L1/R1 wiring

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Add `isAlphaJumpMode` helper**

Add to `screen_list.go`:

```go
// isAlphaJumpMode reports whether the current sort mode uses alpha-jump
// navigation for L1/R1 instead of page-scroll.
func (s *ListScreen) isAlphaJumpMode() bool {
	return s.sortMode == itchio.SortModeAZ || s.sortMode == itchio.SortModeZA
}
```

- [ ] **Step 2: Replace `startShoulderHold` / `stopShoulderHold` calls with alpha-aware versions**

In `HandleEvent`, for both keyboard and controller events, replace the `K_RIGHT` / `K_LEFT` / `CONTROLLER_BUTTON_DPAD_RIGHT` / `CONTROLLER_BUTTON_DPAD_LEFT` handlers with:

```go
		case sdl.K_RIGHT:
			if ev.Type == sdl.KEYDOWN {
				if s.isAlphaJumpMode() && s.cacheReady {
					s.jumpCursor(alphaJumpIndex(s.viewGames, s.cursor, 1) - s.cursor)
				} else {
					s.startShoulderHold(1)
				}
			} else {
				s.stopShoulderHold(1)
			}
			return s
		case sdl.K_LEFT:
			if ev.Type == sdl.KEYDOWN {
				if s.isAlphaJumpMode() && s.cacheReady {
					s.jumpCursor(alphaJumpIndex(s.viewGames, s.cursor, -1) - s.cursor)
				} else {
					s.startShoulderHold(-1)
				}
			} else {
				s.stopShoulderHold(-1)
			}
			return s
```

And for controller:

```go
		case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				if s.isAlphaJumpMode() && s.cacheReady {
					s.jumpCursor(alphaJumpIndex(s.viewGames, s.cursor, 1) - s.cursor)
				} else {
					s.startShoulderHold(1)
				}
			} else {
				s.stopShoulderHold(1)
			}
			return s
		case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				if s.isAlphaJumpMode() && s.cacheReady {
					s.jumpCursor(alphaJumpIndex(s.viewGames, s.cursor, -1) - s.cursor)
				} else {
					s.startShoulderHold(-1)
				}
			} else {
				s.stopShoulderHold(-1)
			}
			return s
```

- [ ] **Step 3: Remove now-unused `K_PAGEDOWN` / `K_PAGEUP` sort mode cycling**

The `K_PAGEDOWN` / `K_PAGEUP` keyboard handlers and `CONTROLLER_BUTTON_RIGHTSHOULDER` / `CONTROLLER_BUTTON_LEFTSHOULDER` used for sort-mode cycling are no longer needed (sort changed via FilterScreen). Remove them:

Find and delete the keyboard cases:

```go
		case sdl.K_PAGEDOWN:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.nextSortMode())
			return s
		case sdl.K_PAGEUP:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.prevSortMode())
			return s
```

Find and delete the controller cases:

```go
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.nextSortMode())
			return s
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.prevSortMode())
			return s
```

Also remove the now-unused `nextSortMode()`, `prevSortMode()`, and `changeSortMode()` methods, replacing them with a single note that sorting is now handled by `SetFilter`.

**Keep** `nextSortMode()` and `prevSortMode()` if they are still referenced elsewhere — check with:

```bash
grep -n "nextSortMode\|prevSortMode\|changeSortMode" internal/ui/screen_list.go
```

Remove only if the grep shows no remaining uses after the deletions above.

- [ ] **Step 4: Build to confirm it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 5: Run tests**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```

Expected: all `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): wire alpha-jump to L1/R1 in A-Z mode; remove sort-cycling from shoulders"
```

---

### Task 11: SettingsScreen — KeyboardScreen for API key entry

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 1: Replace `showAPIKeyHelp` overlay with `KeyboardScreen` in `activate()`**

In `screen_settings.go`, find the `activate()` method's `sItemAPIKey` case:

```go
	case sItemAPIKey:
		if s.cfg.APIKey == "" {
			s.showAPIKeyHelp = true
			logger.Info("settings: API key help overlay shown")
		}
```

Replace with:

```go
	case sItemAPIKey:
		if s.cfg.APIKey == "" {
			return NewKeyboardScreen(s, "", func(value string) {
				if value == "" {
					return
				}
				s.cfg.APIKey = value
				go s.cfg.Save(s.cfgPath)
				logger.Info("settings: API key set via keyboard, len=%d", len(value))
				if s.onOwnedReady != nil {
					go func() {
						_, owned, err := s.client.ValidateAPIKey(value)
						if err != nil {
							logger.Warn("settings: API key validation failed: %v", err)
							return
						}
						s.onOwnedReady(owned)
					}()
				}
			})
		}
```

- [ ] **Step 2: Remove `showAPIKeyHelp` field and related code**

a. Remove `showAPIKeyHelp bool` and `apiKeyHelpQR *sdl.Texture` from the `SettingsScreen` struct.

b. Remove the `drawAPIKeyHelpOverlay` method entirely (it's replaced by `KeyboardScreen`).

c. In `Draw`, remove the call `if s.showAPIKeyHelp { s.drawAPIKeyHelpOverlay(r) }`.

d. In `HandleEvent`, remove the `if s.showAPIKeyHelp { ... }` early-return block at the top.

- [ ] **Step 3: Fix footer hint order in SettingsScreen.Draw**

Find the footer hints block in `Draw`:

```go
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
	}
```

Replace with correct order (A first, then B):

```go
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}
```

Also update the conditional text tweak that follows (change `hints[1].Text` to `hints[0].Text` since A is now index 0):

```go
	if s.cursor == sItemAPIKey {
		if s.cfg.APIKey != "" {
			hints[0].Text = "Test API key"
		} else {
			hints[0].Text = "Enter API key"
		}
	}
```

- [ ] **Step 4: Build to confirm it compiles**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat(ui): replace API key QR help with KeyboardScreen; fix footer hint order"
```

---

### Task 12: DrawModal migration + empty-filter message

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Update "no games" empty-state message to reference filter**

In `screen_list.go`'s `Draw`, find the empty-list block:

```go
	if len(s.viewGames) == 0 && s.cacheReady {
		ht := r.Theme.HintText
		r.DrawTextCentered("No games match this filter.", 0, r.H/2-fontH, leftW, ht[0], ht[1], ht[2])
		r.DrawTextCentered("Press L1/R1 to change sort.", 0, r.H/2+4, leftW, 80, 160, 180)
```

Replace the body text with:

```go
	if len(s.viewGames) == 0 && s.cacheReady {
		ht := r.Theme.HintText
		r.DrawTextCentered("No games match the active filter.", 0, r.H/2-fontH, leftW, ht[0], ht[1], ht[2])
		r.DrawTextCentered("Press SELECT to change filters.", 0, r.H/2+4, leftW, 80, 160, 180)
```

- [ ] **Step 2: Update empty-state footer hints to show SELECT instead of L1R1**

In the same empty-list block, replace both narrow and wide footer hint sets with:

```go
		ftrY := r.DrawFooterBar(footerH)
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"},
			{Kind: renderer.BadgePill, Label: "SELECT", Text: "Filter"},
			{Kind: renderer.BadgePill, Label: "START", Text: "Settings"},
		}, ftrY)
```

- [ ] **Step 3: Build and run full test suite**

```bash
./scripts/build.sh native 2>&1 | tail -5 && ./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```

Expected: clean build, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "fix(ui): update empty-state message and footer to reference SELECT filter"
```

---

### Task 13: Cross-compile and visual verification

- [ ] **Step 1: Cross-compile for all platforms**

```bash
./scripts/build.sh all 2>&1 | tail -10
```

Expected: three binaries built with no errors.

- [ ] **Step 2: Deploy to connected device**

Connect TrimUI or Miyoo device via USB (ADB), then:

```bash
./scripts/deploy.sh
```

Expected: binary pushed, pak restarted.

- [ ] **Step 3: Capture screenshots of key screens**

```bash
./scripts/dev-screenshot.sh --all --out-dir /tmp/itchio-screenshots
```

Verify in `/tmp/itchio-screenshots/`:
- List screen: header shows two filter pills (All + RSS)
- Right panel: cover art smaller, title/author/tags/status visible below
- Footer: A → B → SELECT → L1R1 → START order
- Press SELECT: filter overlay opens with Search / Platform / Sort sections
- Navigate to Search, press A: keyboard opens
- Type a character, press ✓: search applied, list filters
- Cycle L1/R1 in A-Z sort: cursor jumps to next letter boundary
- Open Settings → API Key row with no key → keyboard opens for entry

- [ ] **Step 4: Final commit + summary**

```bash
git add -A
git status  # verify nothing unexpected is staged
git commit -m "chore: final cross-compile verification — ui-redesign branch ready"
```
