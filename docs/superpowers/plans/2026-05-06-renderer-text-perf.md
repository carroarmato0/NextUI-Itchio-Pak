# Renderer Text Pipeline Performance Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate three text-pipeline regressions identified in R4 profiling: `sanitizeText` per-call allocations, missing `fontIndex` rune cache, and missing `WrapText` output cache.

**Architecture:** All changes are contained within `internal/renderer/`. Task 1 (`sanitizeText` fast path) is unit-testable via the existing headless test suite. Tasks 2 and 3 (`fontIndex` cache, `WrapText` cache) live in the `!headless`-guarded `renderer.go` and are verified by build + deploy to device.

**Tech Stack:** Go, `internal/renderer` package, `./scripts/test.sh` for headless tests, `./scripts/build.sh tg5040` + `./scripts/deploy.sh` for device verification.

---

### Task 1: `sanitizeText` zero-alloc fast path

**Files:**
- Modify: `internal/renderer/text.go:194-207`
- Test: `internal/renderer/text_test.go` (existing `TestSanitizeText` — no new test needed; add one allocation-behaviour case)

- [ ] **Step 1: Add an allocation-free assertion test case**

Open `internal/renderer/text_test.go`. Inside `TestSanitizeText`, add this case to the `cases` slice (before the `for` loop):

```go
{"plain latin unchanged — same pointer", "Hello World", "Hello World"},
```

Also add a standalone allocation test after `TestSanitizeText`:

```go
func TestSanitizeTextNoAllocFastPath(t *testing.T) {
	input := "Hello World"
	allocs := testing.AllocsPerRun(100, func() {
		_ = sanitizeText(input)
	})
	if allocs != 0 {
		t.Errorf("sanitizeText(%q): got %.0f allocs, want 0", input, allocs)
	}
}
```

- [ ] **Step 2: Run the new test to confirm it fails**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
go test ./internal/renderer/ -run TestSanitizeTextNoAllocFastPath -v
```

Expected output:
```
--- FAIL: TestSanitizeTextNoAllocFastPath (...)
    text_test.go:...: sanitizeText("Hello World"): got 2 allocs, want 0
FAIL
```

- [ ] **Step 3: Replace `sanitizeText` in `text.go` with the fast-path version**

Replace the current body of `sanitizeText` (`internal/renderer/text.go` lines 194–207) with:

```go
func sanitizeText(s string) string {
	if s == "" {
		return s
	}
	hasEmoji := false
	for _, r := range s {
		if isEmoji(r) {
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
		if !isEmoji(r) {
			out = append(out, s[i:i+size]...)
		}
		i += size
	}
	return string(out)
}
```

- [ ] **Step 4: Run the full text test suite**

```bash
go test ./internal/renderer/ -run TestSanitize -v
```

Expected output:
```
--- PASS: TestSanitizeText (...)
--- PASS: TestSanitizeTextNoAllocFastPath (...)
PASS
```

- [ ] **Step 5: Run the full headless test suite**

```bash
./scripts/test.sh
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/renderer/text.go internal/renderer/text_test.go
git commit -m "perf(renderer): zero-alloc fast path for sanitizeText"
```

---

### Task 2: `fontIndex` per-rune cache

**Files:**
- Modify: `internal/renderer/renderer.go` — `Renderer` struct, `New`, `fontIndex`

No headless unit test is possible (`renderer.go` is `//go:build !headless`). Correctness is verified by the existing `TestBuildGlyphRanges` / `TestSplitTextRuns` tests (which exercise `inRanges` and `fontIndex` logic indirectly) plus device smoke test.

- [ ] **Step 1: Add `runeFont` field to `Renderer` struct**

In `internal/renderer/renderer.go`, the `Renderer` struct currently ends with:

```go
	texts         *textCache
	sizes         map[sizeKey][2]int32 // SizeUTF8 measurement cache (no GPU resources; no LRU needed)
```

Add one field after `sizes`:

```go
	texts         *textCache
	sizes         map[sizeKey][2]int32 // SizeUTF8 measurement cache (no GPU resources; no LRU needed)
	runeFont      map[rune]int         // fontIndex result per rune; populated lazily, never evicted
```

- [ ] **Step 2: Initialise `runeFont` in `New`**

In `New`, find the line:

```go
		sizes:         make(map[sizeKey][2]int32),
```

Add the initialisation on the next line:

```go
		sizes:         make(map[sizeKey][2]int32),
		runeFont:      make(map[rune]int),
```

- [ ] **Step 3: Update `fontIndex` to consult and populate the cache**

The current `fontIndex` body (`renderer.go:173–183`):

```go
func (r *Renderer) fontIndex(ch rune) int {
	if r.primaryRanges == nil || inRanges(r.primaryRanges, ch) {
		return 0
	}
	for i, fb := range r.fallbacks {
		if fb.ranges == nil || inRanges(fb.ranges, ch) {
			return i + 1
		}
	}
	return 0
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
	r.runeFont[ch] = idx
	return idx
}
```

- [ ] **Step 4: Run the headless test suite**

```bash
./scripts/test.sh
```

Expected: all tests pass (no renderer tests exercise `fontIndex` directly, but `splitTextRuns` tests and the build itself will catch compilation errors).

- [ ] **Step 5: Build for device**

```bash
./scripts/build.sh tg5040
```

Expected: `Built: bin/tg5040/itchio-pak (...)` with no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/renderer/renderer.go
git commit -m "perf(renderer): cache fontIndex result per rune"
```

---

### Task 3: `WrapText` output cache

**Files:**
- Modify: `internal/renderer/text_cache.go` — add `wrapKey` type
- Modify: `internal/renderer/renderer.go` — `Renderer` struct, `New`, `WrapText`

- [ ] **Step 1: Add `wrapKey` type to `text_cache.go`**

In `internal/renderer/text_cache.go`, add after the `sizeKey` type definition (after line 20):

```go
// wrapKey is the cache key for WrapText results.
type wrapKey struct {
	text     string
	maxWidth int32
}
```

- [ ] **Step 2: Add `wrapCache` field to `Renderer` struct**

In `internal/renderer/renderer.go`, update the `Renderer` struct to add `wrapCache` after `runeFont`:

```go
	runeFont      map[rune]int         // fontIndex result per rune; populated lazily, never evicted
	wrapCache     map[wrapKey][]string // WrapText output keyed on (text, maxWidth); no LRU needed
```

- [ ] **Step 3: Initialise `wrapCache` in `New`**

After the `runeFont` initialisation line:

```go
		runeFont:      make(map[rune]int),
		wrapCache:     make(map[wrapKey][]string),
```

- [ ] **Step 4: Update `WrapText` to consult and populate the cache**

The current `WrapText` body (`renderer.go:348–376`):

```go
func (r *Renderer) WrapText(text string, maxWidth int32) []string {
	// Sanitize once up front so measurements (via TextSize) match rendering.
	text = sanitizeText(text)
	var lines []string
	for _, paragraph := range splitLines(text) {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := splitWords(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			test := current + " " + word
			tw, _ := r.TextSize(test)
			if tw > maxWidth {
				lines = append(lines, current)
				current = word
			} else {
				current = test
			}
		}
		lines = append(lines, current)
	}
	return lines
}
```

Replace with:

```go
func (r *Renderer) WrapText(text string, maxWidth int32) []string {
	key := wrapKey{text, maxWidth}
	if lines, ok := r.wrapCache[key]; ok {
		return lines
	}
	// Sanitize once up front so measurements (via TextSize) match rendering.
	sanitized := sanitizeText(text)
	var lines []string
	for _, paragraph := range splitLines(sanitized) {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := splitWords(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			test := current + " " + word
			tw, _ := r.TextSize(test)
			if tw > maxWidth {
				lines = append(lines, current)
				current = word
			} else {
				current = test
			}
		}
		lines = append(lines, current)
	}
	r.wrapCache[key] = lines
	return lines
}
```

Note: the cache key uses the raw `text` (before sanitize) so callers passing the same original string always hit the cache regardless of sanitization output.

- [ ] **Step 5: Run the headless test suite**

```bash
./scripts/test.sh
```

Expected: all tests pass.

- [ ] **Step 6: Build for device**

```bash
./scripts/build.sh tg5040
```

Expected: `Built: bin/tg5040/itchio-pak (...)` with no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/renderer/text_cache.go internal/renderer/renderer.go
git commit -m "perf(renderer): cache WrapText output keyed on (text, maxWidth)"
```

---

### Task 4: Deploy and smoke-test on device

- [ ] **Step 1: Deploy to connected device**

```bash
./scripts/deploy.sh
```

Expected: binary pushed, no ADB errors.

- [ ] **Step 2: Launch Itch.io from NextUI and navigate**

- Open the list screen and scroll with L1/R1 held for several seconds — verify smooth scrolling with no hitches.
- Open a game's detail screen — verify description text renders correctly.
- Confirm emoji in titles/descriptions are stripped (no tofu boxes, no crashes).

- [ ] **Step 3: Run a profile round to confirm improvements (optional)**

If a follow-up profile is desired:

```bash
./scripts/debug.sh profile
# launch from NextUI, browse for ~60s, then exit
./scripts/debug.sh pull-profile
go tool pprof -text -cum bin/tg5040/itchio-pak debug-profiles/itchio-cpu.prof | head -30
```

Key things to check:
- `textSizeImpl` / `WrapText` cumulative cost should be substantially lower than R4's 16%
- `inRanges` flat cost should drop from 6.93% toward noise
- `sanitizeText` should have near-zero alloc_space
