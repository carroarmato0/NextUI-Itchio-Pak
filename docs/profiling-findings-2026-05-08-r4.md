# Profiling Findings — itchio-pak (2026-05-08, round 4)

**Session:** 381.30 s wall (~6 min), 141.05 s sampled CPU (**36.99% utilisation**)
**Active CPU (excluding vsync):** ~64.7 s / 381.3 s ≈ **17.0%** (list-heavy, PNG-heavy browsing)
**Live heap at snapshot:** 24.2 MB (PNG palette frame mid-decode at exit, see §4)
**Lifetime allocations:** 3,109.10 MB
**Alloc rate:** 8.16 MB/s (PNG + GIF image decode)
**Total alloc objects:** 440,866
**Profiles:** `debug-profiles/itchio-cpu.prof`, `debug-profiles/itchio-mem.prof`
**Binary:** `bin/tg5040/itchio-pak` (`a565168` — Round 5 fixes; Round 6 code fixes committed but **not yet built/deployed**)

---

## Comparison with Round 6 (same binary, different session activity)

| Metric | Round 6 | Round 7 | Notes |
|--------|---------|---------|-------|
| Wall time | 318.2 s | 381.3 s | longer session |
| CPU utilisation | 29.76% | 36.99% | more active |
| Total alloc objects | 634,310 | **440,866** | −31% (less GIF browsing) |
| Alloc rate | 5.73 MB/s | 8.16 MB/s | more PNG decode this session |
| `ListScreen.Draw` flat | 12,288 (1.9%) | **99,947 (22.67%)** | list-heavy session revealed new items |
| `DrawTextureAt` CGo escape | 32,768 (5.17%) | not ranked | detail screen barely used |
| `SetClipRect` CGo escape | 32,768 (5.17%) | not ranked | same |
| `DetailScreen.Draw` fmt.Sprintf | 39,322 (6.2%) | not ranked | same |

The Round 6 items (DrawTextureAt, SetClipRect, DetailScreen fmt.Sprintf) are still present in the binary but did not rank because this session was list-screen-heavy with minimal detail screen use. **They are fixed in commits acfc2d3 + 2ff0fbe — need to build and deploy.**

This list-heavy session exposed two new per-frame allocators that were previously hidden by the higher-volume detail screen items.

---

## CPU — Top Findings (Round 7)

### 1. `SDL_RenderPresent` vsync — 50.67 s cumulative (35.92%)

Normal idle wait. The user spent most of the session on `ListScreen`.
**No action needed.**

### 2. GIF + image decode — 41.98 s cumulative

| Symbol | Flat |
|--------|------|
| `compress/lzw.(*Reader).decode` | 10.38 s (7.36%) |
| `compositePalettedOver` | 8.75 s (6.20%) |
| `renderGIFFrames` cumulative | 19.39 s (13.75%) |
| `nnInterpolator` resize | 5.73 s (4.06%) |

PNG thumbnail images were the main content this session (`parsePLTE` is the top memory item). GIF decode is background-goroutine work; main thread stays responsive. **No action needed.**

### 3. `drawFilledCircle` + `DrawPill` — 13.71 s / 13.38 s cumulative

Unchanged from previous rounds. No further low-risk optimisation available. **No action needed.**

---

## Memory — Top Findings (Round 7)

### 1. `image/png.(*decoder).parsePLTE` — 98,954 objects (**22.45%**)

Each PNG with a colour-palette chunk allocates a palette slice on decode. User browsed
many static-thumbnail games this session. One-time cost per image, GC'd after GPU upload.
**No action needed.**

### 2. `ListScreen.Draw` flat — 99,947 objects (**22.67%**)

`ListScreen.Draw` flat jumped from 12,288 (R6) to 99,947 (R7) because this session
had much heavier list-screen use. Drilling into the screen_list subtree reveals two
per-frame local slices that are the root cause.

#### 2a. `filteredTags` per-game-per-frame slice (`screen_list.go:821`)

```go
var filteredTags []string
for _, tag := range g.Tags {
    // ...
    filteredTags = append(filteredTags, tag)
}
```

Declared as `nil` inside the per-game render loop. Each game's tags cause 1–3 backing-array
reallocations via `append` (nil → cap 1 → cap 2 → cap 4). With N visible games per frame
at 30 fps, this scales as `N_games × reallocs × fps`.

**Fix:** promote to a package-level `var filteredTagsBuf []string`, reset to `[:0]` before
each game's tag loop (same pattern as `splitBuf`):

```go
var filteredTagsBuf []string

// inside the per-game loop in Draw:
filteredTagsBuf = filteredTagsBuf[:0]
for _, tag := range g.Tags {
    // ...
    filteredTagsBuf = append(filteredTagsBuf, tag)
}
if len(filteredTagsBuf) > 0 {
    r.DrawTagPills(filteredTagsBuf, ...)
    // ...
}
```

**Expected impact:** eliminates the dominant share of the 99,947 ListScreen.Draw flat objects.

---

#### 2b. `footerHints` per-frame slice (`screen_list.go:878–888`)

```go
var footerHints []renderer.FooterHint
footerHints = append(footerHints, renderer.FooterHint{...})  // × 3–5
r.DrawFooterHints(footerHints, ftrY)
```

Built from `nil` every frame. 3–5 appends cause ~3 backing-array reallocations per frame.

**Fix:** package-level `var footerHintsBuf []renderer.FooterHint`, reset to `[:0]` before
building:

```go
var footerHintsBuf []renderer.FooterHint

// in Draw, replacing the local var:
footerHintsBuf = footerHintsBuf[:0]
footerHintsBuf = append(footerHintsBuf, renderer.FooterHint{...})
// ... remaining appends ...
r.DrawFooterHints(footerHintsBuf, ftrY)
```

**Expected impact:** ~3–4 objects/frame eliminated.

---

### 3. `fmt.Sprintf` — 32,768 objects (**7.43%**)

The focus analysis confirms this comes entirely from `ListScreen.Draw` — the page counter
at `screen_list.go:894–896`:

```go
pageInfo := fmt.Sprintf("Page %d", currentPage)
if tp := s.totalPages.Load(); tp > 0 {
    pageInfo = fmt.Sprintf("Page %d/%d", currentPage, tp)
}
```

`currentPage = s.cursor/itchio.PerPage + 1`. Both `cursor` and `totalPages` change rarely
(only on page turn or data fetch). Pre-formatting on change eliminates the per-frame alloc.

**Fix:** cache the formatted string; rebuild only when inputs change:

```go
// fields on ListScreen:
pageInfoStr   string
pageInfoPage  int
pageInfoTotal int32

// helper called from Draw before rendering the footer:
func (s *ListScreen) cachedPageInfo() string {
    cp := s.cursor/itchio.PerPage + 1
    tp := s.totalPages.Load()
    if s.pageInfoStr == "" || cp != s.pageInfoPage || tp != s.pageInfoTotal {
        if tp > 0 {
            s.pageInfoStr = "Page " + strconv.Itoa(cp) + "/" + strconv.Itoa(int(tp))
        } else {
            s.pageInfoStr = "Page " + strconv.Itoa(cp)
        }
        s.pageInfoPage = cp
        s.pageInfoTotal = tp
    }
    return s.pageInfoStr
}
```

**Expected impact:** −32,768 objects per session (−7.4%).

---

### 4. `ttf.(*Font).SizeUTF8` — 32,768 objects (**7.43%**) — not actionable

These come via `ListScreen.Draw` → `cachedTruncate` → `truncateToWidth` → `textSizeImpl`
→ `SizeUTF8`. The go-sdl2 binding converts the Go string to a C string (or similar
CGo allocation) on each call to the underlying `TTF_SizeUTF8`. `textSizeImpl` already
caches results in `r.sizes`; these 32,768 objects are the cold-cache population cost for
unique `(run.text, fontID)` pairs seen for the first time this session. Bounded by unique
content, not frame rate. **No action needed.**

---

### 5. GIF + image decode — 3,109 MB (97.8% of alloc_space) — expected

| Source | Space |
|--------|-------|
| `renderGIFFrames` direct | 1,383.81 MB (44.5%) |
| `image.NewPaletted` (GIF/PNG frames) | 880.56 MB (28.3%) |
| `image.NewRGBA` compositing | 333.42 MB (10.7%) |
| `io.ReadAll` raw download | 258.16 MB (8.3%) |
| `compress/lzw.newReader` | 75.04 MB (2.4%) |

All GC'd after GPU upload. High `NewPaletted` share reflects the PNG-heavy session (each PNG palette-mode image allocates a paletted frame buffer). **No action needed.**

---

### 6. Live heap (24.2 MB) — PNG mid-decode at exit

| Holder | Live |
|--------|------|
| `image.NewPaletted` (PNG/GIF palette frame) | 21.7 MB |
| `io.ReadAll` image buffer | 1.5 MB |
| `FetchGameDetail` response buffer | 0.5 MB |
| `bufio.NewReaderSize` (HTTP) | 0.5 MB |
| `encoding/json` game cache unmarshal | 0.5 MB |

The large `NewPaletted` live object (21.7 MB) is a palette-mode image buffer mid-decode
that had not been GC'd at snapshot time. Temporary; no leak. **No action needed.**

---

## Priority Summary (Round 7)

| # | Area | Object impact | Effort |
|---|------|---------------|--------|
| 0 | **Build + deploy R6 fixes** (DrawTextureAt, SetClipRect, DetailScreen fmt.Sprintf) | −~105K objects | build step |
| 1 | `filteredTags` → package-level `filteredTagsBuf[:0]` | largest share of 99,947 ListScreen flat | trivial |
| 2 | `footerHints` → package-level `footerHintsBuf[:0]` | ~3–4 objects/frame | trivial |
| 3 | Page counter `fmt.Sprintf` → `cachedPageInfo` helper | −32,768 (−7.4%) | low |

After items 1–3, per-frame allocation across both screens should be at or very near zero.
