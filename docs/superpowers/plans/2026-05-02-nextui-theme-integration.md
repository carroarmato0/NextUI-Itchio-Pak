# NextUI Theme Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Read NextUI's `minuisettings.txt` color palette at startup and apply it across all Itch.io Pak UI screens, falling back silently to current grayscale values on non-NextUI devices.

**Architecture:** A new `internal/theme` package parses the settings file and provides a `Theme` struct; the `Renderer` gains a `Theme` field and four new drawing helpers (`DrawPill`, `DrawCircleBadge`, `DrawTagPills`, `DrawFooterHints`); all screen `Draw` methods are updated to use themed colors and the new shape helpers. The `internal/theme` package has no SDL dependency and is fully unit-tested; the renderer helpers are SDL-only and verified visually on-device.

**Tech Stack:** Go 1.22+, SDL2 via `github.com/veandco/go-sdl2`, `//go:build !headless` for SDL files, plain Go tests (no build tag) for the theme package.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/theme/theme.go` | `Theme` struct + `Load(path) Theme` |
| Create | `internal/theme/theme_test.go` | Unit tests — no SDL, no build tag |
| Modify | `internal/renderer/renderer.go` | Add `Theme` field, `DrawPill`, `DrawCircleBadge`, `DrawTagPills`, `DrawFooterHints`, `FooterHint`; update `New` signature; update `DrawHeaderBar`/`DrawFooterBar` |
| Modify | `cmd/itchio-pak/main_sdl.go` | Call `theme.Load`, pass to `renderer.New` |
| Modify | `internal/ui/screen_list.go` | Pill selection, themed sort badge, themed colors, tag pills in right panel, DL/UPDATE overlay badges |
| Modify | `internal/ui/screen_settings.go` | Pill selection, themed colors, footer hints |
| Modify | `internal/ui/screen_detail.go` | Themed header, action pill, tag pills, themed modal border |
| Modify | `internal/ui/screen_rom_picker.go` | Pill selection, themed footer |
| Modify | `internal/ui/screen_format_picker.go` | Pill selection, themed footer |
| Modify | `internal/ui/screen_purchase_picker.go` | Pill selection, themed footer |
| Modify | `internal/ui/screen_location_picker.go` | `HeaderBG` path bar, pill selection |
| Modify | `internal/ui/screen_download.go` | Themed header/footer (semantic greens/reds stay) |

---

## Task 1: `internal/theme` package (TDD, no SDL)

**Files:**
- Create: `internal/theme/theme.go`
- Create: `internal/theme/theme_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/theme/theme_test.go`:

```go
package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_AllFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color1=0xFF0000
color2=0x00FF00
color3=0x0000FF
color4=0x112233
color5=0xAABBCC
color6=0xDDEEFF
color7=0x010203
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	if th.MainText != [3]uint8{0xFF, 0x00, 0x00} {
		t.Errorf("MainText: got %v", th.MainText)
	}
	if th.Accent != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("Accent: got %v", th.Accent)
	}
	if th.HeaderBG != [3]uint8{0x00, 0x00, 0xFF} {
		t.Errorf("HeaderBG: got %v", th.HeaderBG)
	}
	if th.ListText != [3]uint8{0x11, 0x22, 0x33} {
		t.Errorf("ListText: got %v", th.ListText)
	}
	if th.AccentText != [3]uint8{0xAA, 0xBB, 0xCC} {
		t.Errorf("AccentText: got %v", th.AccentText)
	}
	if th.HintText != [3]uint8{0xDD, 0xEE, 0xFF} {
		t.Errorf("HintText: got %v", th.HintText)
	}
	if th.Background != [3]uint8{0x01, 0x02, 0x03} {
		t.Errorf("Background: got %v", th.Background)
	}
}

func TestLoad_SubsetOfFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	// Only color2 set; others should be defaults.
	if err := os.WriteFile(p, []byte("color2=0x9B2257\n"), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	if th.Accent != [3]uint8{0x9B, 0x22, 0x57} {
		t.Errorf("Accent: got %v", th.Accent)
	}
	// Background should be the default #141414
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background default: got %v", th.Background)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	th := Load("/nonexistent/path/minuisettings.txt")
	def := defaults()
	if th != def {
		t.Errorf("expected defaults for missing file, got %+v", th)
	}
}

func TestLoad_MalformedLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color2=notahex
color7=0x141414
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	// color2 malformed → stays at default
	if th.Accent != defaults().Accent {
		t.Errorf("Accent should be default after bad hex, got %v", th.Accent)
	}
	// color7 valid → parsed correctly
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background: got %v", th.Background)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	def := defaults()
	if th != def {
		t.Errorf("expected defaults for empty file, got %+v", th)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
cd .worktrees/nextui-theme
go test ./internal/theme/... -v
```

Expected: compile error — package `theme` does not exist yet.

- [ ] **Step 3: Implement `internal/theme/theme.go`**

```go
package theme

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Theme holds the 7 UI colors read from minuisettings.txt.
// All fields are [R,G,B] triples. Fields are exported so screens can
// read them via r.Theme without importing this package.
type Theme struct {
	Background [3]uint8 // color7 — clear color, panel fills
	HeaderBG   [3]uint8 // color3 — header + footer bar background
	Accent     [3]uint8 // color2 — selection pill, badges, separator
	AccentText [3]uint8 // color5 — text inside selection pill
	ListText   [3]uint8 // color4 — unselected row text
	HintText   [3]uint8 // color6 — footer hint labels
	MainText   [3]uint8 // color1 — author, metadata, description
}

// Load reads minuisettings.txt and returns a Theme.
// Missing or unreadable files return defaults silently.
// Malformed lines are skipped with a WARN log; partial themes are valid.
func Load(path string) Theme {
	th := defaults()
	f, err := os.Open(path)
	if err != nil {
		return th
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		rgb, err := parseHex(v)
		if err != nil {
			logger.Warn("theme: %s: bad value %q: %v", k, v, err)
			continue
		}
		switch k {
		case "color1":
			th.MainText = rgb
		case "color2":
			th.Accent = rgb
		case "color3":
			th.HeaderBG = rgb
		case "color4":
			th.ListText = rgb
		case "color5":
			th.AccentText = rgb
		case "color6":
			th.HintText = rgb
		case "color7":
			th.Background = rgb
		}
	}
	return th
}

// defaults returns the static grayscale fallback values.
// These match the hardcoded colors currently in the renderer and screens,
// so a missing minuisettings.txt produces no visible change.
func defaults() Theme {
	return Theme{
		Background: [3]uint8{0x14, 0x14, 0x14}, // #141414
		HeaderBG:   [3]uint8{0x1E, 0x1E, 0x1E}, // #1E1E1E
		Accent:     [3]uint8{0x3C, 0x3C, 0x5C}, // #3C3C5C
		AccentText: [3]uint8{0xDC, 0xDC, 0xDC}, // #DCDCDC
		ListText:   [3]uint8{0xDC, 0xDC, 0xDC}, // #DCDCDC
		HintText:   [3]uint8{0x8C, 0x8C, 0x8C}, // #8C8C8C
		MainText:   [3]uint8{0xDC, 0xDC, 0xDC}, // #DCDCDC
	}
}

// parseHex parses a "0xRRGGBB" string into an [R,G,B] triple.
func parseHex(s string) ([3]uint8, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return [3]uint8{}, fmt.Errorf("missing 0x prefix")
	}
	n, err := strconv.ParseUint(s[2:], 16, 32)
	if err != nil {
		return [3]uint8{}, err
	}
	if n > 0xFFFFFF {
		return [3]uint8{}, fmt.Errorf("value %s out of 24-bit range", s)
	}
	return [3]uint8{uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
cd .worktrees/nextui-theme
go test ./internal/theme/... -v
```

Expected: 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/theme/theme.go internal/theme/theme_test.go
git commit -m "feat(theme): add theme package — parses minuisettings.txt color fields"
```

---

## Task 2: Renderer — `Theme` field + new drawing helpers

**Files:**
- Modify: `internal/renderer/renderer.go`

The renderer file has `//go:build !headless`. The new SDL-dependent methods go in the same file. The `Theme` field and `FooterHint` type also live here.

- [ ] **Step 1: Add `FooterHint` type and `Theme` field to `Renderer`**

After the `import` block and before `type Renderer struct`, add:

```go
// BadgeKind distinguishes face-button badges (circle) from function-key badges (pill).
type BadgeKind int

const (
	BadgeCircle BadgeKind = iota
	BadgePill
)

// FooterHint is one item in the footer hint bar.
type FooterHint struct {
	Kind  BadgeKind
	Label string // button label inside the badge, e.g. "A", "B", "START"
	Text  string // description text after the badge, e.g. "Open"
}
```

Add `Theme theme.Theme` to the `Renderer` struct (after `W, H int32`):

```go
type Renderer struct {
	Window        *sdl.Window
	Renderer      *sdl.Renderer
	Font          *ttf.Font
	SmallFont     *ttf.Font
	primaryRanges []cmapRange
	fallbacks     []fallbackFont
	W, H          int32
	Theme         theme.Theme
}
```

Add the `theme` import to the import block:

```go
"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
```

- [ ] **Step 2: Update `New` to accept and store a `Theme`**

Change the signature of `New`:

```go
func New(title string, w, h int, th theme.Theme) (*Renderer, error) {
```

Set the field before returning:

```go
r := &Renderer{
    Window: win, Renderer: ren, Font: font, SmallFont: smallFont,
    W: int32(w), H: int32(h),
    primaryRanges: buildGlyphRanges("assets/font.ttf"),
    Theme: th,
}
```

- [ ] **Step 3: Update `DrawHeaderBar` and `DrawFooterBar` to use theme colors**

Replace the two methods:

```go
func (r *Renderer) DrawHeaderBar(h int32) int32 {
	bg := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, h, bg[0], bg[1], bg[2])
	r.DrawRect(0, h, r.W, 2, ac[0], ac[1], ac[2])
	_, fh := r.TextSize("Ag")
	return (h - fh) / 2
}

func (r *Renderer) DrawFooterBar(h int32) int32 {
	bg := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, r.H-h, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawRect(0, r.H-h+2, r.W, h-2, bg[0], bg[1], bg[2])
	_, fh := r.SmallTextSize("Ag")
	return r.H - h + 2 + (h-2-fh)/2
}
```

- [ ] **Step 4: Add `DrawPill`**

Append to `renderer.go`:

```go
// DrawPill draws a filled pill (capsule) shape.
// The border radius is clamped to h/2 so it is always a true capsule.
// It draws a center rectangle plus two filled circle caps.
func (r *Renderer) DrawPill(x, y, w, h int32, red, green, blue uint8) {
	radius := h / 2
	if radius < 1 {
		radius = 1
	}
	// Center rectangle (excluding caps)
	r.Renderer.SetDrawColor(red, green, blue, 255)
	r.Renderer.FillRect(&sdl.Rect{X: x + radius, Y: y, W: w - radius*2, H: h})
	// Left and right filled circle caps
	drawFilledCircle(r.Renderer, x+radius, y+radius, radius, red, green, blue)
	drawFilledCircle(r.Renderer, x+w-radius, y+radius, radius, red, green, blue)
}

// drawFilledCircle draws a filled circle using a midpoint algorithm.
func drawFilledCircle(ren *sdl.Renderer, cx, cy, radius int32, red, green, blue uint8) {
	ren.SetDrawColor(red, green, blue, 255)
	for dy := -radius; dy <= radius; dy++ {
		dx := int32(math.Sqrt(float64(radius*radius - dy*dy)))
		ren.DrawLine(cx-dx, cy+dy, cx+dx, cy+dy)
	}
}
```

Add `"math"` to the import block.

- [ ] **Step 5: Add `DrawCircleBadge`**

```go
// DrawCircleBadge draws a filled circle badge (used for face buttons A, B).
// cx, cy is the center; d is the diameter.
func (r *Renderer) DrawCircleBadge(cx, cy, d int32, red, green, blue uint8) {
	drawFilledCircle(r.Renderer, cx, cy, d/2, red, green, blue)
}
```

- [ ] **Step 6: Add `DrawTagPills`**

```go
// DrawTagPills renders a slice of tag strings as pill badges that wrap across
// lines. Each pill is fgR/fgG/fgB text on bgR/bgG/bgB background.
// maxW is the available pixel width; lineH is the vertical step between rows.
// Returns the total height consumed.
func (r *Renderer) DrawTagPills(tags []string, x, y, maxW, lineH int32,
	fgR, fgG, fgB, bgR, bgG, bgB uint8) int32 {

	const hPad = int32(6) // horizontal padding inside pill
	const vPad = int32(3) // vertical padding inside pill
	const gap = int32(6)  // horizontal gap between pills

	cx := x
	cy := y
	_, textH := r.SmallTextSize("Ag")
	pillH := textH + vPad*2

	for _, tag := range tags {
		tw, _ := r.SmallTextSize(tag)
		pillW := tw + hPad*2
		if cx > x && cx+pillW > x+maxW {
			cx = x
			cy += lineH
		}
		r.DrawPill(cx, cy, pillW, pillH, bgR, bgG, bgB)
		r.DrawSmallText(tag, cx+hPad, cy+vPad, fgR, fgG, fgB)
		cx += pillW + gap
	}
	if cx > x {
		return cy - y + lineH
	}
	return 0
}
```

- [ ] **Step 7: Add `DrawFooterHints`**

```go
// DrawFooterHints renders the footer hint bar from a typed slice.
// Circle badges are used for face buttons (A, B); pill badges for function keys.
// y is the vertical center line for the hint row (returned by DrawFooterBar).
func (r *Renderer) DrawFooterHints(hints []FooterHint, y int32) {
	ac := r.Theme.Accent
	acTxt := r.Theme.AccentText
	hint := r.Theme.HintText

	_, smallH := r.SmallTextSize("Ag")
	badgeDiam := smallH + 4
	cx := int32(10)

	for _, h := range hints {
		labelW, _ := r.SmallTextSize(h.Label)
		textW, _ := r.SmallTextSize(h.Text)

		switch h.Kind {
		case BadgeCircle:
			// Circle badge centered on y
			badgeCX := cx + int32(badgeDiam)/2
			badgeCY := y + smallH/2
			r.DrawCircleBadge(badgeCX, badgeCY, int32(badgeDiam), ac[0], ac[1], ac[2])
			r.DrawSmallTextCentered(h.Label, cx, badgeCY-smallH/2, int32(badgeDiam), acTxt[0], acTxt[1], acTxt[2])
			cx += int32(badgeDiam) + 4
		case BadgePill:
			const hPad = int32(5)
			pillW := labelW + hPad*2
			pillH := smallH + 4
			pillY := y - 2
			r.DrawPill(cx, pillY, pillW, pillH, ac[0], ac[1], ac[2])
			r.DrawSmallText(h.Label, cx+hPad, pillY+2, acTxt[0], acTxt[1], acTxt[2])
			cx += pillW + 4
		}
		r.DrawSmallText(h.Text, cx, y, hint[0], hint[1], hint[2])
		cx += textW + 14
	}
}
```

- [ ] **Step 8: Verify the build compiles**

```
cd .worktrees/nextui-theme
go build -tags '!headless' ./...
```

Expected: clean build (or only errors from callers that haven't been updated yet — that's fine at this step).

Actually, the `renderer.New` signature change will break `main_sdl.go`. Fix `main_sdl.go` first before checking build — see Task 3.

- [ ] **Step 9: Commit (after Task 3 build passes)**

```bash
git add internal/renderer/renderer.go
git commit -m "feat(renderer): add Theme field, DrawPill/CircleBadge/TagPills/FooterHints helpers"
```

---

## Task 3: Wire `theme.Load` into `main_sdl.go`

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`

- [ ] **Step 1: Add import and load call**

In `main_sdl.go`, add to the import block:

```go
"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
```

After `logger.Info("display: %dx%d", w, h)` and before `renderer.New(...)`, add:

```go
const miniSettingsPath = "/mnt/SDCARD/.userdata/shared/minuisettings.txt"
th := theme.Load(miniSettingsPath)
logger.Info("theme: loaded background=#%02X%02X%02X accent=#%02X%02X%02X",
    th.Background[0], th.Background[1], th.Background[2],
    th.Accent[0], th.Accent[1], th.Accent[2])
```

- [ ] **Step 2: Pass theme to `renderer.New`**

Change the `renderer.New` call:

```go
r, err := renderer.New("Itch.io", int(w), int(h), th)
```

- [ ] **Step 3: Verify clean build**

```
cd .worktrees/nextui-theme
go build -tags '!headless' ./cmd/itchio-pak/
```

Expected: clean build. If there are SDL .so errors, that's a link error in dev — it means the Go compilation itself succeeded.

- [ ] **Step 4: Run all tests**

```
cd .worktrees/nextui-theme
bash scripts/test.sh
```

Expected: all 8 suites pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/itchio-pak/main_sdl.go
git commit -m "feat(main): load NextUI theme from minuisettings.txt at startup"
```

---

## Task 4: `screen_list.go` — pill selection, themed colors, sort badge pill, tag pills

**Files:**
- Modify: `internal/ui/screen_list.go`

This is the most complex screen change. Work through it section by section.

### 4a — Replace static color constants with theme helpers

- [ ] **Step 1: Remove the three color constants and add a theme accessor helper**

Delete these lines (around line 22–25):

```go
const (
	colorBG        = uint8(20)
	colorHighlight = uint8(60)
	colorText      = uint8(220)
)
```

The `colorBG`, `colorHighlight`, and `colorText` constants are referenced throughout the screen files. Rather than passing the renderer everywhere, convert all `colorBG` usages to `r.Theme.Background[0], r.Theme.Background[1], r.Theme.Background[2]` etc. at each call site. There are ~5-8 call sites per screen; the replacements are mechanical.

> **Note:** `colorBG`, `colorHighlight`, and `colorText` are used across *all* screen files that share `package ui`. Do NOT remove these constants until every screen file has been updated. Instead, update each screen's `Draw` method to use `r.Theme.*` and only then remove the constants in the final cleanup step (Task 9). Until then, keep the constants defined but unused warnings will appear — suppress by keeping at least one usage, or do the cleanup all at once.
>
> **Practical approach:** Work screen by screen (Tasks 4–8), replacing each usage of `colorBG` / `colorHighlight` / `colorText` inline. Remove the constants once all screens are done (Task 9 step).

### 4b — Pill selection highlight (left panel)

- [ ] **Step 2: Replace full-width `DrawRect` selection highlight with `DrawPill`**

Find the selection highlight in `Draw` (around line 342):

```go
if i == s.cursor {
    r.DrawRect(0, rowTop, leftW, rowH, colorHighlight, colorHighlight, colorHighlight+20)
}
```

Replace with:

```go
if i == s.cursor {
    ac := r.Theme.Accent
    pillMargin := int32(4)
    r.DrawPill(pillMargin, rowTop+2, leftW-pillMargin*2, rowH-4, ac[0], ac[1], ac[2])
}
```

Also update selected-row text from `colorText, colorText, colorText` to use `AccentText`:

```go
// In the selected-row text draw calls (isDownloaded bold + non-bold paths):
// Replace: colorText, colorText, colorText
// With: r.Theme.AccentText[0], r.Theme.AccentText[1], r.Theme.AccentText[2]
```

There are four `DrawText`/`DrawBoldText` calls for the selected row title (lines ~382, ~396, ~401, ~410). Update all four.

Unselected rows keep `colorText` → update to `r.Theme.ListText[0], r.Theme.ListText[1], r.Theme.ListText[2]`.

### 4c — Header: themed colors + sort badge as pill

- [ ] **Step 3: Replace the header drawing block**

Find the header block (lines ~228–249):

```go
// Header
headerH := int32(72)
r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
_, fontH := r.TextSize("Ag")
headerTextY := (headerH - fontH) / 2
r.DrawText("Itch.io — GB Studio Games", 12, headerTextY, colorText, colorText, colorText)
if s.cacheReady {
    badge := itchio.SortModeBadge(s.sortMode)
    bw, _ := r.TextSize(badge)
    bx := r.W - bw - 12
    var badgeR, badgeG, badgeB uint8
    switch s.sortMode {
    case itchio.SortModeFree:
        badgeR, badgeG, badgeB = 80, 200, 80
    case itchio.SortModePaid:
        badgeR, badgeG, badgeB = 220, 180, 60
    default:
        badgeR, badgeG, badgeB = 80, 200, 220
    }
    r.DrawText(badge, bx, headerTextY, badgeR, badgeG, badgeB)
}
// Thin separator line below header
r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
```

Replace with:

```go
headerH := int32(72)
headerTextY := r.DrawHeaderBar(headerH)
mt := r.Theme.MainText
r.DrawText("Itch.io — GB Studio Games", 12, headerTextY, mt[0], mt[1], mt[2])
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
    r.DrawText(badge, pillX+hPad, headerTextY, aT[0], aT[1], aT[2])
}
_, fontH := r.TextSize("Ag")
```

Note: `fontH` is still needed below; declare it after `DrawHeaderBar` call.

### 4d — Background and right panel

- [ ] **Step 4: Replace `r.Clear(colorBG, colorBG, colorBG)` with theme color**

```go
bg := r.Theme.Background
r.Clear(bg[0], bg[1], bg[2])
```

- [ ] **Step 5: Right panel — remove price line, replace tags with `DrawTagPills`**

Find the right-panel metadata block (~lines 501–558). Remove the price text lines:

```go
// DELETE these lines:
if g.IsFree {
    r.DrawText("Free", rightX, metaY, 80, 200, 80)
} else {
    r.DrawText(fmt.Sprintf("$%.2f", g.Price), rightX, metaY, 220, 180, 60)
}
metaY += lineGap
```

Replace the tag line block (the `wrappedLines` / `tagScrollY` section) with a `DrawTagPills` call that preserves the vertical-scroll logic:

```go
if len(filteredTags) > 0 {
    ac := r.Theme.Accent
    aT := r.Theme.AccentText
    // Measure total pill height without clipping to know if scroll is needed.
    totalTagH := r.DrawTagPills(filteredTags, rightX, 0, rightW, lineGap,
        aT[0], aT[1], aT[2], ac[0]/3, ac[1]/3, ac[2]/3)
    availH := r.H - footerH - metaY
    if availH <= 0 {
        availH = 0
    }
    if totalTagH <= availH {
        s.tagScrollY = 0
        r.DrawTagPills(filteredTags, rightX, metaY, rightW, lineGap,
            aT[0], aT[1], aT[2], ac[0]/3, ac[1]/3, ac[2]/3)
        metaY += totalTagH
    } else {
        maxTagScroll := totalTagH - availH
        if s.tagScrollY > maxTagScroll {
            s.tagScrollY = maxTagScroll
        }
        if s.tagScrollY == maxTagScroll &&
            time.Since(s.tagScrollAt) > scrollDelay+time.Duration(maxTagScroll)*time.Second/time.Duration(tagScrollSpeed)+time.Second {
            s.tagScrollY = 0
            s.tagScrollAt = time.Now()
        }
        r.SetClipRect(rightX, metaY, rightW, availH)
        r.DrawTagPills(filteredTags, rightX, metaY-s.tagScrollY, rightW, lineGap,
            aT[0], aT[1], aT[2], ac[0]/3, ac[1]/3, ac[2]/3)
        r.ClearClipRect()
    }
}
```

Note: `ac[0]/3` etc. gives a darkened version of the accent color for the pill background (approximately 33% brightness), keeping tags readable on any accent.

- [ ] **Step 6: Update author text to use `MainText`**

```go
// Before: r.DrawText("by "+g.Author, rightX, metaY, 160, 160, 160)
mt := r.Theme.MainText
r.DrawText("by "+g.Author, rightX, metaY, mt[0], mt[1], mt[2])
```

### 4e — Footer hints

- [ ] **Step 7: Replace plain-text footer hints with `DrawFooterHints`**

Find the footer section (~lines 561–620) which currently does:

```go
ftrY := r.DrawFooterBar(footerH)
r.DrawSmallText("A:open  SELECT:sort  B:exit  Start:settings", 10, ftrY, 140, 140, 140)
```

(The actual hint strings vary based on narrow screen width and game state. Replace ALL `DrawSmallText` footer calls in the `Draw` method with `DrawFooterHints` equivalents.)

Standard footer (wide screen):

```go
ftrY := r.DrawFooterBar(footerH)
hints := []renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "A", Text: "open"},
    {Kind: renderer.BadgePill, Label: "SEL", Text: "sort"},
    {Kind: renderer.BadgeCircle, Label: "B", Text: "exit"},
    {Kind: renderer.BadgePill, Label: "START", Text: "settings"},
}
r.DrawFooterHints(hints, ftrY)
```

Narrow screen (≤640px) variant:

```go
hints := []renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "A", Text: "open"},
    {Kind: renderer.BadgePill, Label: "SEL", Text: "sort"},
    {Kind: renderer.BadgeCircle, Label: "B", Text: "exit"},
    {Kind: renderer.BadgePill, Label: "⚙", Text: ""},
}
```

Apply the same pattern for the "no games match" empty state footer and the error-state footer (adapt hints to context).

Error state footer:

```go
hints := []renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "A", Text: "retry"},
    {Kind: renderer.BadgeCircle, Label: "B", Text: "exit"},
}
```

For the optional dismiss hint (UPDATE/REMOVED games), append to the hints slice when applicable — keep the existing conditional logic but produce a `FooterHint` entry.

- [ ] **Step 8: Build and quick sanity check**

```
cd .worktrees/nextui-theme
go build -tags '!headless' ./internal/ui/
```

Fix any compile errors. Do not run visual test yet — accumulate all screen changes first.

- [ ] **Step 9: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): apply NextUI theme to list screen — pill selection, themed colors, sort badge pill, tag pills"
```

---

## Task 5: `screen_settings.go` — pill selection, themed colors, footer hints

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 1: Replace `DrawHeaderBar` and background clear**

`screen_settings.go` already calls `r.DrawHeaderBar(headerH)` — that now uses theme colors automatically. Replace `r.Clear(colorBG, colorBG, colorBG)`:

```go
bg := r.Theme.Background
r.Clear(bg[0], bg[1], bg[2])
```

- [ ] **Step 2: Replace full-width selection highlight with pill**

Find (around line 120):

```go
if settingsItem(i) == s.cursor {
    r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
}
```

Replace with:

```go
if settingsItem(i) == s.cursor {
    ac := r.Theme.Accent
    r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
}
```

- [ ] **Step 3: Update row text colors**

Selected row text → `AccentText`. Unselected row text → `ListText`:

```go
// Before: r.DrawText(label, 20, y, colorText, colorText, colorText)
var tr, tg, tb uint8
if settingsItem(i) == s.cursor {
    c := r.Theme.AccentText
    tr, tg, tb = c[0], c[1], c[2]
} else {
    c := r.Theme.ListText
    tr, tg, tb = c[0], c[1], c[2]
}
r.DrawText(label, 20, y, tr, tg, tb)
```

- [ ] **Step 4: API key status badges — keep semantic colors, convert to `DrawPill`**

Find the API key status block:

```go
switch s.client.GetAPIKeyStatus() {
case itchio.APIKeyStatusWorking:
    r.DrawText("WORKING", 20+labelW, y, 80, 200, 80)
case itchio.APIKeyStatusRejected:
    r.DrawText("REJECTED", 20+labelW, y, 200, 60, 60)
default:
    r.DrawText("PRESENT", 20+labelW, y, colorText, colorText, colorText)
}
```

Replace with pill badges (semantic colors intentionally not themed):

```go
var statusLabel string
var sR, sG, sB uint8
switch s.client.GetAPIKeyStatus() {
case itchio.APIKeyStatusWorking:
    statusLabel, sR, sG, sB = "WORKING", 80, 200, 80
case itchio.APIKeyStatusRejected:
    statusLabel, sR, sG, sB = "REJECTED", 200, 60, 60
default:
    statusLabel, sR, sG, sB = "PRESENT", 140, 140, 140
}
sw, sh := r.SmallTextSize(statusLabel)
const sp = int32(4)
r.DrawPill(20+labelW+4, y+(fontH-sh-sp)/2, sw+sp*2, sh+sp, sR, sG, sB)
r.DrawSmallText(statusLabel, 20+labelW+4+sp, y+(fontH-sh)/2, 20, 20, 20)
```

- [ ] **Step 5: Replace footer hints**

Find:

```go
ftrY := r.DrawFooterBar(footerH)
r.DrawSmallText("A:select  B:back  Start:settings", 10, ftrY, 140, 140, 140)
```

Replace:

```go
ftrY := r.DrawFooterBar(footerH)
r.DrawFooterHints([]renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "A", Text: "select"},
    {Kind: renderer.BadgeCircle, Label: "B", Text: "back"},
}, ftrY)
```

- [ ] **Step 6: Build check**

```
cd .worktrees/nextui-theme
go build -tags '!headless' ./internal/ui/
```

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat(ui): apply NextUI theme to settings screen — pill selection, themed colors"
```

---

## Task 6: `screen_detail.go` — themed header, action pill, tag pills, modal border

**Files:**
- Modify: `internal/ui/screen_detail.go`

- [ ] **Step 1: Background + header**

```go
// r.Clear(colorBG, colorBG, colorBG)
bg := r.Theme.Background
r.Clear(bg[0], bg[1], bg[2])
```

The detail screen draws its own header (not via `DrawHeaderBar`) at lines ~160–165:

```go
r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
```

Replace with:

```go
hBG := r.Theme.HeaderBG
ac := r.Theme.Accent
r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
```

Update header text from `colorText` to `r.Theme.MainText`:

```go
mt := r.Theme.MainText
r.DrawText(s.game.Title, 12, 8, mt[0], mt[1], mt[2])
```

- [ ] **Step 2: Action row — replace plain text with `DrawPill`**

Find the action row rendering (the "[ A: Download ]" / "[ A: Open Itch.io ]" section, ~lines 310–335). It currently uses `r.DrawText`. Replace each action with a themed pill:

```go
// Free game download action:
ac := r.Theme.Accent
aT := r.Theme.AccentText
actionLabel := "A: Download"
aw, ah := r.TextSize(actionLabel)
const ap = int32(10)
pillW := aw + ap*2
pillH := ah + 6
r.DrawPill(margin, y, pillW, pillH, ac[0], ac[1], ac[2])
r.DrawText(actionLabel, margin+ap, y+3, aT[0], aT[1], aT[2])
```

For paid games where a price is shown, use amber semantic color (intentionally not themed — it signals "paid"):

```go
r.DrawPill(margin, y, pillW, pillH, 220, 160, 40)
r.DrawText(actionLabel, margin+ap, y+3, 20, 20, 20)
```

- [ ] **Step 3: Tag line — replace `DrawWrappedText` with `DrawTagPills`**

Find the tag rendering in the detail screen (which calls `DrawWrappedText` for tags). Replace with:

```go
if len(filteredTags) > 0 {
    ac := r.Theme.Accent
    aT := r.Theme.AccentText
    r.DrawTagPills(filteredTags, margin, y, usableW, lineH,
        aT[0], aT[1], aT[2], ac[0]/3, ac[1]/3, ac[2]/3)
}
```

- [ ] **Step 4: Modal box border — tint with `Accent`**

Find the modal border rectangle (~line 362):

```go
r.DrawRect(boxX-1, boxY-1, boxW+2, boxH+2, 70, 70, 70)
```

Replace with:

```go
ac := r.Theme.Accent
r.DrawRect(boxX-1, boxY-1, boxW+2, boxH+2, ac[0]/2, ac[1]/2, ac[2]/2)
```

(Half-brightness accent gives a subtle tint without overwhelming the modal's semantic title color.)

- [ ] **Step 5: Footer hints**

Replace `DrawSmallText` footer calls with `DrawFooterHints`:

Normal state (wide):

```go
ftrY := r.DrawFooterBar(footerH)
hints := []renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "B", Text: "back"},
    {Kind: renderer.BadgePill, Label: "L/R", Text: "screenshots"},
    {Kind: renderer.BadgePill, Label: "START", Text: "settings"},
}
if scrollHint != "" {
    hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "↕", Text: "scroll"})
}
r.DrawFooterHints(hints, ftrY)
```

Narrow screen: omit "Start:settings", abbreviate to badge-only:

```go
hints := []renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "B", Text: "back"},
    {Kind: renderer.BadgePill, Label: "L/R", Text: ""},
    {Kind: renderer.BadgePill, Label: "⚙", Text: ""},
}
```

- [ ] **Step 6: Build check**

```
cd .worktrees/nextui-theme
go build -tags '!headless' ./internal/ui/
```

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat(ui): apply NextUI theme to detail screen — themed header, action pill, tag pills, modal accent border"
```

---

## Task 7: Picker screens — pill selection, footer hints

**Files:**
- Modify: `internal/ui/screen_rom_picker.go`
- Modify: `internal/ui/screen_format_picker.go`
- Modify: `internal/ui/screen_purchase_picker.go`

These three screens share the same pattern: header bar (already via `DrawHeaderBar`), list with full-width highlight, `DrawSmallText` footer. Apply the same pill selection and footer hint substitutions.

- [ ] **Step 1: `screen_rom_picker.go` — pill selection**

Find the selection highlight:

```go
if i == s.cursor {
    r.DrawRect(0, rowTop, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
}
```

Replace:

```go
if i == s.cursor {
    ac := r.Theme.Accent
    r.DrawPill(4, rowTop+2, r.W-8, rowH-4, ac[0], ac[1], ac[2])
}
```

Update row text to `AccentText` for selected / `ListText` for unselected (same pattern as settings screen).

Background clear: `r.Clear(colorBG, colorBG, colorBG)` → use `r.Theme.Background`.

Footer hints:

```go
ftrY := r.DrawFooterBar(footerH)
r.DrawFooterHints([]renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "A", Text: "select"},
    {Kind: renderer.BadgeCircle, Label: "B", Text: "back"},
}, ftrY)
```

- [ ] **Step 2: `screen_format_picker.go` — pill selection + format badge pills**

Same pill selection + background clear + footer hints as above.

Format badges (`[GB]` / `[GBC]` / `[ZIP]`) currently use `DrawSmallText` with semantic colors. Convert to `DrawPill` but **keep their semantic colors** (intentionally not themed — they identify file types):

```go
// Find the format badge rendering block and replace DrawSmallText with:
var fmtLabel string
var fmtR, fmtG, fmtB uint8
switch ext {
case ".gb":
    fmtLabel, fmtR, fmtG, fmtB = "GB", 80, 160, 220
case ".gbc":
    fmtLabel, fmtR, fmtG, fmtB = "GBC", 80, 200, 120
default:
    fmtLabel, fmtR, fmtG, fmtB = "ZIP", 180, 140, 80
}
fw, fh := r.SmallTextSize(fmtLabel)
const fp = int32(4)
r.DrawPill(badgeX, y+(rowH-fh-fp)/2, fw+fp*2, fh+fp, fmtR, fmtG, fmtB)
r.DrawSmallText(fmtLabel, badgeX+fp, y+(rowH-fh)/2, 20, 20, 20)
```

- [ ] **Step 3: `screen_purchase_picker.go` — pill selection**

Same pill selection + background clear + footer hints.

- [ ] **Step 4: Build check**

```
cd .worktrees/nextui-theme
go build -tags '!headless' ./internal/ui/
```

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_rom_picker.go internal/ui/screen_format_picker.go internal/ui/screen_purchase_picker.go
git commit -m "feat(ui): apply NextUI theme to picker screens — pill selection, footer hints"
```

---

## Task 8: `screen_location_picker.go` — `HeaderBG` path bar, pill selection

**Files:**
- Modify: `internal/ui/screen_location_picker.go`

- [ ] **Step 1: Background clear**

```go
bg := r.Theme.Background
r.Clear(bg[0], bg[1], bg[2])
```

- [ ] **Step 2: Path bar background from `HeaderBG`**

Find (line ~188):

```go
r.DrawRect(0, pathBarY, r.W, pathBarH, 25, 25, 25)
```

Replace:

```go
hBG := r.Theme.HeaderBG
r.DrawRect(0, pathBarY, r.W, pathBarH, hBG[0], hBG[1], hBG[2])
```

- [ ] **Step 3: Pill selection for directory rows**

Find the `rowEntry` selection highlight:

```go
r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
```

Replace:

```go
ac := r.Theme.Accent
r.DrawPill(4, y-2, r.W-8, rowH-4, ac[0], ac[1], ac[2])
```

Update row text to `AccentText` when selected / `ListText` otherwise.

**"Save here" row stays semantic green** — do not theme it:

```go
// Keep as-is:
r.DrawRect(0, confirmY, r.W, confirmH, 26, 58, 34)
r.DrawText("[ ✓  Save here ]", 12, confirmY+5, 80, 200, 120)
```

- [ ] **Step 4: Footer hints**

```go
ftrY := r.DrawFooterBar(footerH)
r.DrawFooterHints([]renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "A", Text: "choose"},
    {Kind: renderer.BadgeCircle, Label: "B", Text: "back"},
}, ftrY)
```

- [ ] **Step 5: Build check**

```
cd .worktrees/nextui-theme
go build -tags '!headless' ./internal/ui/
```

- [ ] **Step 6: Commit**

```bash
git add internal/ui/screen_location_picker.go
git commit -m "feat(ui): apply NextUI theme to location picker — HeaderBG path bar, pill selection"
```

---

## Task 9: Remaining screens + final cleanup

**Files:**
- Modify: `internal/ui/screen_download.go`
- Any other screen files in `internal/ui/` not yet touched (fetch_uploads, about, apikey_check, cache_refresh, content_moderation, manage_downloads, tag_filter)

### 9a — `screen_download.go`

- [ ] **Step 1: Background + header + footer — themed; semantic colors stay unchanged**

```go
bg := r.Theme.Background
r.Clear(bg[0], bg[1], bg[2])
```

`DrawHeaderBar` already themed. `DrawFooterBar` already themed.

Progress bar fill stays green (`80, 200, 120`) — semantic success color. "Download complete!" text stays green. Error text stays red. These are not themed.

Footer hints:

```go
ftrY := r.DrawFooterBar(footerH)
r.DrawFooterHints([]renderer.FooterHint{
    {Kind: renderer.BadgeCircle, Label: "B", Text: "back"},
}, ftrY)
```

### 9b — All other `screen_*.go` files

- [ ] **Step 2: For each remaining screen, apply the same three-step pattern:**

1. `r.Clear(colorBG, colorBG, colorBG)` → `bg := r.Theme.Background; r.Clear(bg[0], bg[1], bg[2])`
2. Selection highlight `DrawRect` → `DrawPill` with `r.Theme.Accent`
3. `DrawSmallText` footer hints → `DrawFooterHints`

Check which files remain:

```
grep -rl "colorHighlight\|colorBG\|colorText" internal/ui/
```

Apply the pattern to each. Text color updates: selected row → `AccentText`, unselected → `ListText` or `MainText` depending on context.

### 9c — Remove now-unused constants

- [ ] **Step 3: Remove the `colorBG`, `colorHighlight`, `colorText` constants from `screen_list.go`**

After all screen files are updated, verify nothing in `package ui` still references these constants:

```
grep -n "colorBG\|colorHighlight\|colorText" internal/ui/*.go
```

If the list is empty, delete the constant block from `screen_list.go` (lines ~22–25).

### 9d — Full test run

- [ ] **Step 4: Run all tests**

```
cd .worktrees/nextui-theme
bash scripts/test.sh
```

Expected: all suites pass (including the new `internal/theme` suite).

- [ ] **Step 5: Build cross-compile check**

```
cd .worktrees/nextui-theme
bash scripts/build.sh tg5040
```

Expected: clean build. Fix any issues before deploying.

- [ ] **Step 6: Deploy to device and visual verification**

```
bash scripts/deploy.sh
```

Check each screen on-device with default settings (should look identical to before — defaults match current hardcoded values). Then set a custom accent in NextUI settings and relaunch to verify theming applies.

- [ ] **Step 7: Final commit**

```bash
git add internal/ui/
git commit -m "feat(ui): apply NextUI theme across all remaining screens — pill selection, footer hints, themed colors"
```

---

## Completion Checklist

Before requesting merge into `main`:

- [ ] `bash scripts/test.sh` passes (all suites)
- [ ] `bash scripts/build.sh tg5040` produces clean binary
- [ ] Deployed and visually verified on device with default theme (no visible change)
- [ ] Deployed and visually verified on device with custom NextUI accent color (theming applies)
- [ ] All screen draw paths use `r.Theme.*` rather than the removed `colorBG`/`colorHighlight`/`colorText` constants
- [ ] `grep -r "colorBG\|colorHighlight\|colorText" internal/ui/` returns empty
- [ ] Semantic colors (download progress green, error red, status badges, "Save here" row) are NOT themed
