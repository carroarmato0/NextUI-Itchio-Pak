# Design: Renderer Text Pipeline Performance Fixes

**Date:** 2026-05-06
**Source:** Profiling round 4 (`docs/profiling-findings-2026-05-06-r4.md`)
**Scope:** `internal/renderer/` only — no UI, no scraper, no settings changes

---

## Context

Round 4 profiling (main branch, post-seamless-scroll) identified three regressions in the
text rendering pipeline. The seamless scroll feature increased the rate at which game titles
and descriptions pass through `WrapText` and `textSizeImpl`, exposing cache misses that were
acceptable at lower scroll speeds.

| Issue | CPU impact | Memory impact |
|-------|-----------|---------------|
| `fontIndex` called per-rune, per-frame with no rune-level cache | ~3 s / 7% | negligible |
| `WrapText` re-wraps same description every frame | ~7 s / 16% | moderate |
| `sanitizeText` always allocates even for emoji-free input | minimal | 72 MB / 8% |

---

## Fix 1 — `fontIndex` per-rune cache

**File:** `internal/renderer/renderer.go`

Add a `runeFont map[rune]int` field to `Renderer`, initialised in `New`. In `fontIndex`,
check the map before running the binary search / fallback loop; store the result on miss.

```go
// Renderer struct
runeFont map[rune]int  // fontIndex result per rune; populated lazily, never evicted

// fontIndex — before existing logic
if idx, ok := r.runeFont[ch]; ok {
    return idx
}
// ... existing binary-search + fallback loop ...
r.runeFont[ch] = result
return result
```

The map is unbounded but naturally bounded by the Unicode codepoints the user encounters
(a few thousand at most across a session). Values are deterministic: the result for a rune
depends only on font cmap ranges loaded at init and never changes.

---

## Fix 2 — `WrapText` output cache

**File:** `internal/renderer/renderer.go`

Add a `wrapCache map[wrapKey][]string` field to `Renderer`, initialised in `New`.

```go
type wrapKey struct {
    text     string
    maxWidth int32
}
```

In `WrapText`, check the cache before doing any work; store the result on miss. The returned
slice is safe to share across frames because callers only range over it for drawing — no
caller mutates it.

```go
func (r *Renderer) WrapText(text string, maxWidth int32) []string {
    key := wrapKey{text, maxWidth}
    if lines, ok := r.wrapCache[key]; ok {
        return lines
    }
    // ... existing implementation ...
    r.wrapCache[key] = lines
    return lines
}
```

No LRU is needed. Values are pure `[]string` with no GPU resources, and the number of
distinct `(description, maxWidth)` pairs is bounded by the game catalog. Window dimensions
are fixed on target devices, so maxWidth never changes at runtime.

The `wrapKey` type goes in `text_cache.go` alongside the other key types.

---

## Fix 3 — `sanitizeText` zero-alloc fast path

**File:** `internal/renderer/text.go`

Add a pre-scan before the existing allocation path. If no rune satisfies `isEmoji`, return
the original string unchanged — zero allocation.

```go
func sanitizeText(s string) string {
    if s == "" {
        return s
    }
    // Fast path: no emoji found, nothing to strip.
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
    // Slow path: rebuild without emoji characters.
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

Most game titles are plain Latin/ASCII; the fast path eliminates the 72 MB of per-call
`make([]byte, …)` for those inputs.

---

## Files Changed

| File | Change |
|------|--------|
| `internal/renderer/renderer.go` | Add `runeFont` and `wrapCache` fields; update `New`, `fontIndex`, `WrapText` |
| `internal/renderer/text_cache.go` | Add `wrapKey` type |
| `internal/renderer/text.go` | Add fast path to `sanitizeText` |

No new files. No interface changes. No test fixtures needed (existing headless tests cover
`WrapText` and `sanitizeText`).

---

## Out of Scope

- LRU eviction for `wrapCache` or `runeFont` — not needed given bounded inputs
- Changing `DrawWrappedText` signature — callers are unchanged
- Any non-renderer code
