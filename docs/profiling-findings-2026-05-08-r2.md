# Profiling Findings — itchio-pak (2026-05-08, round 2)

**Session:** 712.33 s wall time (~12 min), 147.33 s sampled CPU (**20.68% utilisation**)
**Active CPU (excluding vsync wait):** ~62.9 s / 712.3 s ≈ **8.8%**
**Live heap at exit:** 5.93 MB
**Lifetime allocations:** 552.69 MB
**Profiles:** `debug-profiles/itchio-cpu.prof`, `debug-profiles/itchio-mem.prof`
**Binary:** `bin/tg5040/itchio-pak` (fix/code-quality-fixes branch, commit 88e0f05)

---

## Comparison with Round 4 (2026-05-08 earlier session)

| Metric | Round 4 | Round 5 | Delta |
|--------|---------|---------|-------|
| Wall time | 901.3 s | 712.3 s | shorter session |
| Sampled CPU | 122.93 s | 147.33 s | — |
| CPU utilisation | 13.64% | 20.68% | +7 pp (user more active; detail screen heavy) |
| Live heap | 6.58 MB | **5.93 MB** | −0.65 MB |
| Lifetime allocs | 3,484 MB | **552.69 MB** | **−84% absolute** |
| Alloc rate | 3.87 MB/s | **0.78 MB/s** | **−80%** |
| Total alloc objects | 3,011,760 | **564,663** | **−81%** |
| `splitTextRuns` objects | 912,050 (30%) | **0** | **FIXED ✅** |
| `drawRuns` Copy escape | 524,290 (17%) | **0** | **FIXED ✅** |
| `DrawPill`/`DrawRect` escape | 360,451 (12%) | **0** | **FIXED ✅** |
| `truncateToWidth` allocs | 206,627 (7%) | **cold-cache only** | **FIXED ✅** |

### Round 4 action items — status

| Item | Status |
|------|--------|
| `splitTextRuns` → package-level `splitBuf` | **FIXED ✅** — gone from top 25 |
| `DrawPill` `pillBodyBuf` + `DrawRect` `drawRectBuf` | **FIXED ✅** — gone from top 25 |
| `drawRuns` `copyDstBuf` for both `Copy` calls | **FIXED ✅** — gone from top 25 |
| `ListScreen.cachedTruncate` memoisation | **FIXED ✅** — only 54,613 cold-cache misses remain |

---

## CPU — Top Findings (Round 5)

### 1. `SDL_RenderPresent` vsync wait — 84.41 s cumulative (57.27%)

The user spent the majority of the session on `DetailScreen`. Both `DetailScreen.Draw`
(98.63 s cumulative) and `ListScreen.Draw` (21.29 s cumulative) spend most of their
budget waiting for the display flip. This is expected idle time. **No action needed.**

### 2. `drawFilledCircle` + `DrawPill` + `DrawTagPills` — 20.87 s cumulative (14.17%)

Now the dominant non-idle, non-GIF CPU cost. `DrawTagPills` alone is 12.18 s (8.27%).
Each tag pill on the detail screen draws 2 anti-aliased circles + 1 filled rect = 7 CGo
boundary crossings per pill. With many tags per game, this is many SDL calls per frame.

| Symbol | Cumulative |
|--------|-----------|
| `drawFilledCircle` | 20.87 s (14.17%) |
| `DrawPill` | 18.95 s (12.86%) |
| `DrawTagPills` | 12.18 s (8.27%) |
| `FillRects` CGo | 16.76 s (11.38%) |

The `circleRectBuf` fix (Round 3) eliminated all allocations. The remaining cost is
**pure CGo call overhead** — each `SDL_RenderFillRects` crossing takes ~140 ns on the
device; 7 crossings × many pills per frame adds up.

**Potential optimisation:** skip the anti-aliasing fringe pass when `radius < 4` — at
that size the single-pixel fringe is sub-pixel and visually indistinguishable. This
reduces `drawFilledCircle` from 2 `FillRects` calls to 1 for small pill end-caps.

### 3. `splitTextRuns` — 0.90 s flat (0.61%)

Pure computation cost (UTF-8 scanning). Allocations are gone; this is now bounded by
the scan itself. No further action needed.

### 4. `drawRuns` — 6.87 s cumulative (4.66%)

Driven mostly by the text LRU cache lookup and `RenderUTF8Blended` on cold misses.
`textCache.get` contributes 0.27 s flat. No further action needed.

---

## Memory — Top Findings (Round 5)

### 1. `fmt.Sprintf` — 120,151 objects (**21.28%** of all objects) — NEW TOP

The new dominant per-frame allocator. Call sites:

| Location | Call | Frequency |
|----------|------|-----------|
| `screen_list.go:625` | `fmt.Sprintf("$%.2f", g.Price)` | once per priced game per frame |
| `screen_list.go:873–875` | `fmt.Sprintf("Page %d[/%d]", ...)` | once per frame (footer) |
| `screen_detail.go:335` | `fmt.Sprintf("Image %d/%d  (←→)", ...)` | once per frame on detail |
| `screen_detail.go:363` | `fmt.Sprintf("$%.2f", price)` | once per download option per frame |
| `screen_detail.go:390` | `fmt.Sprintf(" (+%d more)", n)` | once per multi-file entry per frame |

Price formatting (`"$%.2f"`) is the highest-frequency call. Prices do not change between
view reloads.

**Fix for `screen_list.go:625`:** extend the existing `truncCache` pattern — store the
pre-formatted badge label alongside truncated title in `rebuildView`. Or use `strconv`
directly:

```go
// instead of: fmt.Sprintf("$%.2f", g.Price)
badgeLabel = "$" + strconv.FormatFloat(float64(g.Price), 'f', 2, 32)
```

`strconv.FormatFloat` still allocates a string, but avoids the `fmt` reflection overhead.
Better: cache the label string per game URL in a `map[string]string badgeLabelCache`
(same lifecycle as `truncCache`, cleared on `rebuildView`).

**Fix for page/image counters:** use `strconv.AppendInt` into a stack buffer:

```go
var pageBuf [32]byte
pageInfo := string(strconv.AppendInt(strconv.AppendInt(pageBuf[:0], int64(currentPage), 10), int64(s.totalPages), 10))
```

Or simply accept the ~3 Sprintf calls per frame as a minor residual cost.

---

### 2. `strings.(*Builder).grow` — 54,613 objects (9.67%)

These are cold-cache misses from `cachedTruncate` — the first time each `(title, maxW,
bold)` triple is seen, the underlying `truncateToWidth`/`truncateBoldToWidth` builds the
truncated string. Subsequent frames are free. **No action needed** — this is bounded by
the number of distinct titles × layout variants, not by frame rate.

---

### 3. `sdl.(*Texture).Query` — 32,768 objects (5.80%)

Called in `drawRuns` on the text-cache miss path — once per unique `(text, font, color)`
run. Since `r.texts` (the LRU) grows to cover frequently-used strings, subsequent renders
are free. The 32,768 figure is the count of unique text runs first seen in this session.
**No action needed** — bounded by unique string variety, not frame rate.

---

### 4. `sdl.WaitEventTimeout` — 24,577 objects (4.35%)

SDL event loop. Each call allocates an `sdl.Event` interface value in the go-sdl2
binding. The app processes ~34 event-poll calls per second. Fixing this would require
changes to the go-sdl2 binding. **Not worth pursuing.**

---

### 5. GIF + image decode — 506 MB (91.6% of alloc_space) — expected, no action

| Source | Alloc space |
|--------|-------------|
| `renderGIFFrames` direct | 318.07 MB (57.6%) |
| `image.NewRGBA` compositing | 67.93 MB (12.3%) |
| `image.NewPaletted` (GIF frames) | 58.76 MB (10.6%) |
| `io.ReadAll` raw download | 37.99 MB (6.9%) |
| `compress/lzw.newReader` | 11.74 MB (2.1%) |

All GC'd after GPU upload. Proportional to GIF images viewed. **No action needed.**

---

### 6. Live heap (5.93 MB)

| Holder | Live | Notes |
|--------|------|-------|
| `FetchGameDetail` response buffer | 1.23 MB | Detail screen was open at exit |
| `io.ReadAll` image buffer | 1.21 MB | Last image being fetched |
| `runtime/pprof.StartCPUProfile` | 1.18 MB | Profiling only; absent in production |
| `image.NewRGBA` (last GIF frame) | 0.77 MB | In-progress GIF decode |
| HTTP/2 write buffer | 0.52 MB | Persistent connection state |
| `pprof.profMap` | 0.52 MB | Profiling only |
| CA cert PEM decode | 0.51 MB | One-time startup; permanent |

Clean live heap. No leaks or surprises.

---

## Priority Summary (Round 5)

| # | Area | Object impact | Space impact | Effort |
|---|------|---------------|--------------|--------|
| 1 | `fmt.Sprintf("$%.2f")` badge label cache | −~100K objects (−18%) | negligible | low |
| 2 | `drawFilledCircle` skip AA fringe for radius < 4 | — | — | trivial |

Everything else is bounded by session content (unique text, unique images) rather than
frame rate. After item 1, per-frame allocation objects should drop below 100 K/session
under typical usage.
