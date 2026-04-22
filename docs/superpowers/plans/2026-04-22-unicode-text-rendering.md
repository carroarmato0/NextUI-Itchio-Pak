# Unicode Text Rendering Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix unreadable Japanese/CJK game titles and strip emoji characters so all game text renders correctly on device.

**Architecture:** Two independent changes — replace `assets/font.ttf` with Noto Sans JP (covers CJK/Cyrillic/Latin), and add a pure-Go `sanitizeText` helper that strips emoji ranges before any string reaches SDL2_ttf. `sanitizeText` lives in its own file with no build tag so it compiles under `-tags headless` and is fully unit testable.

**Tech Stack:** Go 1.22, SDL2_ttf via `go-sdl2/ttf`, `unicode/utf8`, `unicode` stdlib

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `assets/font.ttf` | Replace | Swap DejaVu Sans (270KB) for Noto Sans JP Regular (~3.7MB) |
| `internal/renderer/text.go` | Create | `sanitizeText` — strips emoji Unicode ranges |
| `internal/renderer/text_test.go` | Create | Unit tests for `sanitizeText` |
| `internal/renderer/renderer.go` | Modify | Call `sanitizeText` at entry of all text functions |

---

## Task 1: Add sanitizeText with tests (TDD)

**Files:**
- Create: `internal/renderer/text.go`
- Create: `internal/renderer/text_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/renderer/text_test.go`:

```go
package renderer

import "testing"

func TestSanitizeText(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain latin unchanged", "Hello World", "Hello World"},
		{"accented latin unchanged", "Feliz polaridad Maré", "Feliz polaridad Maré"},
		{"cyrillic unchanged", "Привет", "Привет"},
		{"japanese unchanged", "彼は私の中の少女", "彼は私の中の少女"},
		{"traditional chinese unchanged", "桑之巫韻", "桑之巫韻"},
		{"single emoji stripped", "🎳", ""},
		{"emoji in title stripped", "B🎳wling", "Bwling"},
		{"emoji in description stripped", "Great game 🦾", "Great game "},
		{"misc symbol stripped", "★ cool", " cool"},
		{"dingbat stripped", "✂ cut", " cut"},
		{"transport emoji stripped", "🚀 launch", " launch"},
		{"emoticon stripped", "😀 fun", " fun"},
		{"mixed cjk and emoji", "かぞくロボット 🎮", "かぞくロボット "},
		{"empty string", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeText(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -tags headless ./internal/renderer/ -run TestSanitizeText -v
```

Expected: `FAIL — undefined: sanitizeText`

- [ ] **Step 3: Implement sanitizeText**

Create `internal/renderer/text.go` — no build tag, pure Go:

```go
package renderer

import "unicode/utf8"

// sanitizeText strips emoji and symbol characters that the bundled font cannot
// render. This prevents SDL2_ttf from emitting tofu boxes for missing glyphs.
// CJK, Cyrillic, and accented Latin are preserved — the font covers them.
func sanitizeText(s string) string {
	if s == "" {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !isEmoji(r) {
			out = append(out, s[i:i+size]...)
		}
		i += size
	}
	return string(out)
}

func isEmoji(r rune) bool {
	return (r >= 0x2600 && r <= 0x26FF) || // Miscellaneous Symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F650 && r <= 0x1F67F) || // Ornamental Dingbats
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1FFFF) // Various supplementary emoji blocks
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test -tags headless ./internal/renderer/ -run TestSanitizeText -v
```

Expected: all 14 subtests `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/text.go internal/renderer/text_test.go
git commit -m "feat(renderer): add sanitizeText to strip emoji before SDL2_ttf rendering"
```

---

## Task 2: Wire sanitizeText into the renderer

**Files:**
- Modify: `internal/renderer/renderer.go`

The functions to update are `DrawText`, `TextSize`, `DrawSmallText`, `SmallTextSize`, `WrapText`, and `DrawWrappedText`. Apply `sanitizeText` at the top of each function that receives a `text string` parameter, before the string is passed to any SDL2_ttf call or used in line-breaking logic.

- [ ] **Step 1: Update DrawText**

In `renderer.go`, change `DrawText`:

```go
func (r *Renderer) DrawText(text string, x, y int32, red, green, blue uint8) error {
	text = sanitizeText(text)
	surface, err := r.Font.RenderUTF8Blended(text, sdl.Color{R: red, G: green, B: blue, A: 255})
	// ... rest unchanged
```

- [ ] **Step 2: Update TextSize**

```go
func (r *Renderer) TextSize(text string) (int32, int32) {
	text = sanitizeText(text)
	w, h, err := r.Font.SizeUTF8(text)
	// ... rest unchanged
```

- [ ] **Step 3: Update DrawSmallText**

```go
func (r *Renderer) DrawSmallText(text string, x, y int32, red, green, blue uint8) error {
	text = sanitizeText(text)
	surface, err := r.SmallFont.RenderUTF8Blended(text, sdl.Color{R: red, G: green, B: blue, A: 255})
	// ... rest unchanged
```

- [ ] **Step 4: Update SmallTextSize**

```go
func (r *Renderer) SmallTextSize(text string) (int32, int32) {
	text = sanitizeText(text)
	w, h, err := r.SmallFont.SizeUTF8(text)
	// ... rest unchanged
```

- [ ] **Step 5: Update WrapText**

`WrapText` calls `TextSize` internally (which is now sanitized), but it also iterates over the raw `text` argument. Sanitize at entry:

```go
func (r *Renderer) WrapText(text string, maxWidth int32) []string {
	text = sanitizeText(text)
	var lines []string
	// ... rest unchanged
```

- [ ] **Step 6: Update DrawWrappedText**

`DrawWrappedText` calls `WrapText` (now sanitized) and `DrawText` (now sanitized). Its own `text` parameter does not need an extra sanitize call because both callees sanitize. Leave it unchanged.

- [ ] **Step 7: Run all headless tests to confirm nothing is broken**

```bash
./scripts/test.sh
```

Expected: all tests `PASS`, no new failures

- [ ] **Step 8: Commit**

```bash
git add internal/renderer/renderer.go
git commit -m "feat(renderer): wire sanitizeText into all SDL2_ttf text functions"
```

---

## Task 3: Replace font with Noto Sans JP Regular

**Files:**
- Replace: `assets/font.ttf`

- [ ] **Step 1: Download Noto Sans JP Regular**

```bash
curl -L \
  "https://github.com/googlefonts/noto-fonts/raw/main/hinted/ttf/NotoSansJP/NotoSansJP-Regular.ttf" \
  -o assets/font.ttf
```

Verify the file is a valid TTF and roughly the right size:

```bash
file assets/font.ttf
ls -lh assets/font.ttf
```

Expected: `TrueType Font data` and size around 3–4MB. If the curl 404s, download `NotoSansJP-Regular.ttf` manually from https://fonts.google.com/noto/specimen/Noto+Sans+JP (Download Family button) and copy the Regular weight to `assets/font.ttf`.

- [ ] **Step 2: Verify the font reports CJK coverage**

```bash
fc-query assets/font.ttf 2>/dev/null | grep -E "family:|fullname:" | head -4
```

Expected output includes `Noto Sans JP` in the family name.

- [ ] **Step 3: Run a headless build to confirm assets compile in**

```bash
./scripts/test.sh
```

Expected: all tests pass (font is a binary asset, not compiled, so this confirms the build pipeline is intact)

- [ ] **Step 4: Do a local non-headless build to confirm the font loads**

```bash
go build -o /tmp/itchio-pak-test ./cmd/itchio-pak/
```

Expected: clean build, no errors

- [ ] **Step 5: Commit**

```bash
git add assets/font.ttf
git commit -m "feat(assets): replace DejaVu Sans with Noto Sans JP for CJK/Unicode support"
```

---

## Self-Review

**Spec coverage:**
- ✅ Replace font.ttf with Noto Sans JP → Task 3
- ✅ Strip emoji Unicode ranges → Task 1 (sanitizeText) + Task 2 (wired into renderer)
- ✅ CJK/Cyrillic/accented Latin preserved (tested in Task 1 test cases)
- ✅ Stripping at render time, not at data layer → Tasks 1–2 only touch `internal/renderer/`
- ✅ WrapText correctness maintained → Step 5 of Task 2 sanitizes before line-breaking

**Placeholder scan:** None found.

**Type consistency:** `sanitizeText(string) string` defined in Task 1 Step 3, called in Task 2 Steps 1–5. Signature consistent throughout.
