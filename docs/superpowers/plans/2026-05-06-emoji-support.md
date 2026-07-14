# Emoji Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render emoji in all UI text via NotoEmoji-Regular as a fallback font, silently dropping emoji that no font covers, and stripping emoji from ROM filenames and A-Z/Z-A sort keys.

**Architecture:** Bundle NotoEmoji-Regular.ttf as `assets/font_fallback_emoji.ttf` so the existing `font_fallback_*.ttf` glob auto-loads it. A new `internal/text` package owns the emoji range table and exports `IsEmoji`/`StripEmoji`. The renderer's `fontIndex` returns -1 for emoji that no font covers; `splitTextRuns` skips -1 runes so they produce no glyph instead of tofu.

**Tech Stack:** Go 1.22, SDL2_ttf (unchanged), NotoEmoji-Regular.ttf (new asset), `internal/text` (new package)

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/text/text.go` | `IsEmoji(r rune) bool`, `StripEmoji(s string) string` |
| Create | `internal/text/text_test.go` | Unit tests for both functions |
| Create | `assets/font_fallback_emoji.ttf` | NotoEmoji-Regular font (downloaded) |
| Create | `assets/OFL-1.1-NotoEmoji.txt` | OFL 1.1 license for NotoEmoji |
| Modify | `internal/renderer/text.go` | Remove `sanitizeText`/`isEmoji`; update `splitTextRuns` to skip -1 |
| Modify | `internal/renderer/text_test.go` | Remove sanitize tests; add `splitTextRuns` skip test |
| Modify | `internal/renderer/renderer.go` | Import `internal/text`; update `fontIndex`; remove `sanitizeText` from 3 call sites |
| Modify | `internal/roms/sanitise.go` | Call `text.StripEmoji` at top of `SanitiseFilename` |
| Modify | `internal/roms/sanitise_test.go` | Add emoji title test cases |
| Modify | `internal/itchio/sort.go` | Add `sortKey` helper; update `SortModeAZ`/`SortModeZA` |
| Modify | `internal/itchio/sort_test.go` | Add emoji prefix sort test cases |

---

## Task 1: Create `internal/text` package

**Files:**
- Create: `internal/text/text.go`
- Create: `internal/text/text_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/text/text_test.go`:

```go
package text_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/text"
)

func TestIsEmoji(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want bool
	}{
		{"ASCII letter A", 'A', false},
		{"CJK ideograph", '中', false},
		{"Arabic letter", 'ا', false},
		{"accented Latin", 'é', false},
		{"Misc Symbols start U+2600", 0x2600, true},
		{"Misc Symbols end U+26FF", 0x26FF, true},
		{"Dingbats start U+2700", 0x2700, true},
		{"Dingbats end U+27BF", 0x27BF, true},
		{"Misc Symbols Arrows U+2B00", 0x2B00, true},
		{"Misc Pictographs U+1F300", 0x1F300, true},
		{"Emoticons U+1F600", 0x1F600, true},
		{"Transport U+1F680", 0x1F680, true},
		{"Floppy disk U+1F4BE", 0x1F4BE, true},
		{"Supplementary U+1F700", 0x1F700, true},
		{"Supplementary end U+1FFFF", 0x1FFFF, true},
		{"just below Misc Symbols U+25FF", 0x25FF, false},
		{"just above Supplementary U+20000", 0x20000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := text.IsEmoji(tc.r)
			if got != tc.want {
				t.Errorf("IsEmoji(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

func TestStripEmoji(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain ASCII unchanged", "Hello World", "Hello World"},
		{"CJK unchanged", "日本語", "日本語"},
		{"Arabic unchanged", "مرحبا", "مرحبا"},
		{"single emoji stripped", "🎮", ""},
		{"leading emoji stripped", "🎮 Adventure", " Adventure"},
		{"embedded emoji stripped", "Night 🌙 Crawler", "Night  Crawler"},
		{"trailing emoji stripped", "Dungeon ⚔️", "Dungeon "},
		{"emoji-only title becomes empty", "🎮🌙⚔️", ""},
		{"floppy disk U+1F4BE stripped", "\U0001F4BE", ""},
		{"misc symbol stripped", "★ cool", " cool"},
		{"dingbat stripped", "✂ cut", " cut"},
		{"mixed CJK and emoji", "かぞくロボット 🎮", "かぞくロボット "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := text.StripEmoji(tc.input)
			if got != tc.want {
				t.Errorf("StripEmoji(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestStripEmojiNoAllocFastPath(t *testing.T) {
	input := "Hello World"
	allocs := testing.AllocsPerRun(100, func() {
		_ = text.StripEmoji(input)
	})
	if allocs != 0 {
		t.Errorf("StripEmoji(%q): got %.0f allocs, want 0", input, allocs)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
./scripts/test.sh
```

Expected: build failure — `cannot find package "github.com/carroarmato0/nextui-itchio-pak/internal/text"`

- [ ] **Step 3: Create the implementation**

Create `internal/text/text.go`:

```go
package text

import "unicode/utf8"

// IsEmoji reports whether r is an emoji or symbol codepoint.
func IsEmoji(r rune) bool {
	return (r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0x2B00 && r <= 0x2BFF) ||
		(r >= 0x1F300 && r <= 0x1F5FF) ||
		(r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F650 && r <= 0x1F67F) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x1F700 && r <= 0x1FFFF)
}

// StripEmoji returns s with all emoji codepoints removed.
// Fast path: if no emoji are present, s is returned unchanged (zero allocations).
func StripEmoji(s string) string {
	if s == "" {
		return s
	}
	hasEmoji := false
	for _, r := range s {
		if IsEmoji(r) {
			hasEmoji = true
			break
		}
	}
	if !hasEmoji {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !IsEmoji(r) {
			out = append(out, s[i:i+size]...)
		}
		i += size
	}
	return string(out)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
./scripts/test.sh
```

Expected: all tests pass, including the new `internal/text` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/text/text.go internal/text/text_test.go
git commit -m "feat(text): add IsEmoji and StripEmoji to internal/text package"
```

---

## Task 2: Update `splitTextRuns` to skip runes with fontIdx -1

**Files:**
- Modify: `internal/renderer/text.go` (lines 168–189)
- Modify: `internal/renderer/text_test.go` (add new test)

- [ ] **Step 1: Add the failing test**

Append to `internal/renderer/text_test.go` (after the existing `TestBuildGlyphRanges` test, before `TestSanitizeText`):

```go
func TestSplitTextRunsSkip(t *testing.T) {
	// mockSkipIndex: emoji-range runes return -1 (drop), ASCII returns 0, Arabic returns 1.
	mockSkipIndex := func(r rune) int {
		if r >= 0x1F300 && r <= 0x1FFFF {
			return -1
		}
		if r >= 0x0600 && r <= 0x06FF {
			return 1
		}
		return 0
	}

	cases := []struct {
		name  string
		input string
		want  []textRun
	}{
		{
			"skipped rune in middle produces two runs",
			"Hi\U0001F4BEbye",
			[]textRun{{"Hi", 0}, {"bye", 0}},
		},
		{
			"skipped rune at start is dropped",
			"\U0001F300Hello",
			[]textRun{{"Hello", 0}},
		},
		{
			"skipped rune at end is dropped",
			"Hello\U0001F300",
			[]textRun{{"Hello", 0}},
		},
		{
			"all skipped returns nil",
			"\U0001F300\U0001F680",
			nil,
		},
		{
			"multiple skipped between two font runs",
			"Hi\U0001F300\U0001F680خ",
			[]textRun{{"Hi", 0}, {"خ", 1}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitTextRuns(tc.input, mockSkipIndex)
			if len(got) != len(tc.want) {
				t.Fatalf("splitTextRuns(%q) = %v (len %d), want %v (len %d)",
					tc.input, got, len(got), tc.want, len(tc.want))
			}
			for i, run := range got {
				if run != tc.want[i] {
					t.Errorf("run[%d]: got %+v, want %+v", i, run, tc.want[i])
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to confirm the new test fails**

```bash
./scripts/test.sh
```

Expected: `TestSplitTextRunsSkip` fails — skipped rune still appears in the run.

- [ ] **Step 3: Update `splitTextRuns` in `internal/renderer/text.go`**

Replace the existing `splitTextRuns` function (lines 168–189) with:

```go
// splitTextRuns segments s into runs where consecutive runes that resolve to
// the same font index are merged. fontIndex(r) returns 0 for the primary font,
// a positive index for a fallback font, or -1 to drop the rune entirely.
// An empty input returns nil.
func splitTextRuns(s string, fontIndex func(rune) int) []textRun {
	if s == "" {
		return nil
	}
	var runs []textRun
	runStart := 0
	runIdx := -1
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		idx := fontIndex(r)
		if idx == -1 {
			// Flush the current run (if any) and skip this rune.
			if runIdx >= 0 {
				runs = append(runs, textRun{text: s[runStart:i], fontIdx: runIdx})
			}
			i += size
			runStart = i
			runIdx = -1
			continue
		}
		if runIdx < 0 {
			runIdx = idx
			runStart = i
		} else if idx != runIdx {
			runs = append(runs, textRun{text: s[runStart:i], fontIdx: runIdx})
			runStart = i
			runIdx = idx
		}
		i += size
	}
	if runIdx >= 0 && runStart < len(s) {
		runs = append(runs, textRun{text: s[runStart:], fontIdx: runIdx})
	}
	return runs
}
```

- [ ] **Step 4: Run tests to confirm all pass**

```bash
./scripts/test.sh
```

Expected: all tests pass including `TestSplitTextRunsSkip` and all existing `TestSplitTextRuns` cases.

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/text.go internal/renderer/text_test.go
git commit -m "feat(renderer): splitTextRuns skips runes with fontIdx -1"
```

---

## Task 3: Wire `fontIndex` to return -1, remove `sanitizeText`

**Files:**
- Modify: `internal/renderer/renderer.go` (fontIndex + 3 call sites)
- Modify: `internal/renderer/text.go` (remove sanitizeText, isEmoji)
- Modify: `internal/renderer/text_test.go` (remove sanitize tests)

These three files must change together — removing `sanitizeText` from `text.go` before removing its call sites would fail to compile.

- [ ] **Step 1: Update `fontIndex` in `internal/renderer/renderer.go`**

Add the import `itext "github.com/carroarmato0/nextui-itchio-pak/internal/text"` to the import block at the top of `renderer.go`. The import block currently reads:

```go
import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)
```

Replace with:

```go
import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	itext "github.com/carroarmato0/nextui-itchio-pak/internal/text"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)
```

- [ ] **Step 2: Update `fontIndex` to return -1 for unrenderable emoji**

The existing `fontIndex` method reads:

```go
func (r *Renderer) fontIndex(ch rune) int {
	if idx, ok := r.runeFont[ch]; ok {
		return idx
	}
	var idx int
	if r.primaryRanges != nil && !inRanges(r.primaryRanges, ch) {
		for i, fb := range r.fallbacks {
			if fb.ranges == nil || inRanges(fb.ranges, ch) {
				idx = i + 1
				break
			}
		}
	}
	r.runeFont[ch] = idx
	return idx
}
```

Replace with:

```go
func (r *Renderer) fontIndex(ch rune) int {
	if idx, ok := r.runeFont[ch]; ok {
		return idx
	}
	var idx int
	if r.primaryRanges != nil && !inRanges(r.primaryRanges, ch) {
		for i, fb := range r.fallbacks {
			if fb.ranges == nil || inRanges(fb.ranges, ch) {
				idx = i + 1
				break
			}
		}
	}
	// Drop emoji that no font covers rather than falling through to tofu.
	if idx == 0 && r.primaryRanges != nil && itext.IsEmoji(ch) {
		idx = -1
	}
	r.runeFont[ch] = idx
	return idx
}
```

- [ ] **Step 3: Remove `sanitizeText` from 3 call sites in `renderer.go`**

**Call site 1** — `drawRuns` function. Find:

```go
	runs := splitTextRuns(sanitizeText(text), r.fontIndex)
```

Replace with (there are two of these — fix both in one edit):

```go
	runs := splitTextRuns(text, r.fontIndex)
```

**Call site 2** — `textSizeImpl` function. Same pattern — `sanitizeText(text)` → `text` (note: `text` here is the parameter name, not the `itext` alias; the parameter is named `text string`).

**Call site 3** — `WrapText` function. Find:

```go
	sanitized := sanitizeText(text)
	var lines []string
	for _, paragraph := range splitLines(sanitized) {
```

Replace with:

```go
	var lines []string
	for _, paragraph := range splitLines(text) {
```

- [ ] **Step 4: Remove `sanitizeText` and `isEmoji` from `internal/renderer/text.go`**

Delete the entire `sanitizeText` function (lines 191–219) and the entire `isEmoji` function (lines 221–235) from `text.go`. The file should end after the closing brace of `inRanges`.

- [ ] **Step 5: Remove the sanitize tests from `internal/renderer/text_test.go`**

Delete the `TestSanitizeText` function and the `TestSanitizeTextNoAllocFastPath` function from `text_test.go`. These tested functionality that has been removed.

- [ ] **Step 6: Run all tests**

```bash
./scripts/test.sh
```

Expected: all tests pass. `TestSplitTextRuns` and `TestSplitTextRunsSkip` and `TestBuildGlyphRanges` all pass. No compile errors.

- [ ] **Step 7: Commit**

```bash
git add internal/renderer/renderer.go internal/renderer/text.go internal/renderer/text_test.go
git commit -m "feat(renderer): route emoji through fallback chain; drop unrenderable emoji via fontIndex -1"
```

---

## Task 4: Bundle the NotoEmoji font asset

**Files:**
- Create: `assets/font_fallback_emoji.ttf`
- Create: `assets/OFL-1.1-NotoEmoji.txt`

- [ ] **Step 1: Download NotoEmoji-Regular.ttf**

```bash
curl -L -o assets/font_fallback_emoji.ttf \
  https://github.com/googlefonts/noto-emoji/raw/main/fonts/NotoEmoji-Regular.ttf
```

Verify the file downloaded and is non-empty:

```bash
ls -lh assets/font_fallback_emoji.ttf
```

Expected: file exists, size roughly 5–10 MB.

- [ ] **Step 2: Create the license file**

Create `assets/OFL-1.1-NotoEmoji.txt` with the following content (the copyright line comes from the font's metadata; the rest is the standard OFL 1.1 text matching the format of the other OFL files in `assets/`):

```
Copyright 2013 Google LLC

This Font Software is licensed under the SIL Open Font License, Version 1.1.

This license is available with a FAQ at: https://scripts.sil.org/OFL
```

- [ ] **Step 3: Run the full test + build to confirm the font loads**

```bash
./scripts/test.sh
```

Expected: all tests pass. If a device is available, `./scripts/deploy.sh` and verify emoji render in game titles.

- [ ] **Step 4: Commit**

```bash
git add assets/font_fallback_emoji.ttf assets/OFL-1.1-NotoEmoji.txt
git commit -m "feat(assets): bundle NotoEmoji-Regular as emoji fallback font"
```

---

## Task 5: Strip emoji from ROM filenames

**Files:**
- Modify: `internal/roms/sanitise.go`
- Modify: `internal/roms/sanitise_test.go`

- [ ] **Step 1: Add failing test cases to `sanitise_test.go`**

In `TestSanitiseFilename`, add these cases to the existing `cases` slice:

```go
{"🎮 Adventure Quest", ".gb", "Adventure Quest.gb"},
{"Night 🌙 Crawler", ".gb", "Night  Crawler.gb"},
{"⚔️Dungeon", ".gbc", "Dungeon.gbc"},
{"🎮🌙", ".gb", ""},
```

- [ ] **Step 2: Run tests to confirm new cases fail**

```bash
./scripts/test.sh
```

Expected: `TestSanitiseFilename` fails on the four new cases — emoji still present in filename.

- [ ] **Step 3: Update `SanitiseFilename` in `sanitise.go`**

Add the import for `internal/text` at the top of the file. The existing imports are:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)
```

Replace with:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/text"
)
```

Then update `SanitiseFilename` to strip emoji as its first step. The existing function body opens with:

```go
func SanitiseFilename(title, ext string) string {
	if title == "" {
		return ""
	}
	const strip = `/:?*"<>|`
```

Replace with:

```go
func SanitiseFilename(title, ext string) string {
	if title == "" {
		return ""
	}
	title = text.StripEmoji(title)
	const strip = `/:?*"<>|`
```

- [ ] **Step 4: Run tests to confirm all pass**

```bash
./scripts/test.sh
```

Expected: all tests pass including the four new emoji cases.

- [ ] **Step 5: Commit**

```bash
git add internal/roms/sanitise.go internal/roms/sanitise_test.go
git commit -m "feat(roms): strip emoji from ROM filenames in SanitiseFilename"
```

---

## Task 6: Strip emoji from A-Z/Z-A sort keys

**Files:**
- Modify: `internal/itchio/sort.go`
- Modify: `internal/itchio/sort_test.go`

- [ ] **Step 1: Add failing test cases to `sort_test.go`**

Add a new test function after `TestApplySort_AZ_CaseInsensitive`:

```go
func TestApplySort_AZ_EmojiPrefixSortsByText(t *testing.T) {
	games := []itchio.Game{
		{Title: "🎮 Zelda"},
		{Title: "Apple"},
		{Title: "⚔️ Banana"},
	}
	result := itchio.ApplySort(games, itchio.SortModeAZ, nil, nil, nil)
	want := []string{"Apple", "⚔️ Banana", "🎮 Zelda"}
	if !equalTitles(result, want) {
		t.Errorf("AZ emoji prefix: got %v, want %v", titles(result), want)
	}
}

func TestApplySort_ZA_EmojiPrefixSortsByText(t *testing.T) {
	games := []itchio.Game{
		{Title: "🎮 Zelda"},
		{Title: "Apple"},
		{Title: "⚔️ Banana"},
	}
	result := itchio.ApplySort(games, itchio.SortModeZA, nil, nil, nil)
	want := []string{"🎮 Zelda", "⚔️ Banana", "Apple"}
	if !equalTitles(result, want) {
		t.Errorf("ZA emoji prefix: got %v, want %v", titles(result), want)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
./scripts/test.sh
```

Expected: the two new tests fail — emoji-prefixed titles sort by emoji codepoint instead of their text.

- [ ] **Step 3: Update `sort.go`**

Add the import for `internal/text`. The existing imports are:

```go
import (
	"sort"
	"strings"
)
```

Replace with:

```go
import (
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/text"
)
```

Add a `sortKey` helper immediately before `ApplySort`:

```go
// sortKey returns a normalised sort key for a game title: emoji stripped, lowercased.
func sortKey(s string) string {
	return strings.ToLower(text.StripEmoji(s))
}
```

In `ApplySort`, update the `SortModeAZ` case. Find:

```go
	case SortModeAZ:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		})
		return out
```

Replace with:

```go
	case SortModeAZ:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return sortKey(out[i].Title) < sortKey(out[j].Title)
		})
		return out
```

Update the `SortModeZA` case. Find:

```go
	case SortModeZA:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Title) > strings.ToLower(out[j].Title)
		})
		return out
```

Replace with:

```go
	case SortModeZA:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return sortKey(out[i].Title) > sortKey(out[j].Title)
		})
		return out
```

- [ ] **Step 4: Run all tests**

```bash
./scripts/test.sh
```

Expected: all tests pass including the two new emoji sort tests.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/sort.go internal/itchio/sort_test.go
git commit -m "feat(itchio): strip emoji from A-Z/Z-A sort keys so emoji-prefixed titles sort by text"
```
