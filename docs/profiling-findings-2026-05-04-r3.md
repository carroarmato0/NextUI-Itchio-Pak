# Profiling Findings — itchio-pak (2026-05-04, round 3)

**Session:** 55.56s wall time, 19.10s sampled CPU (34.38% utilisation)
**Live heap at exit:** 4.3 MB
**Lifetime allocations:** 893.32 MB
**Profiles:** `debug-profiles/itchio-cpu.prof`, `debug-profiles/itchio-mem.prof`
**Binary:** `bin/tg5040/itchio-pak` (perf/render-optimizations-r2 branch, commit fe0fb08)

---

## Comparison with Round 2 (2026-05-04)

| Metric | Round 2 | Round 3 | Delta |
|--------|---------|---------|-------|
| Wall time | 106.69 s | 55.56 s | −51 s (shorter run) |
| Sampled CPU | 47.02 s | **19.10 s** | **−28 s (−59%)** |
| CPU utilisation | 44% | **34.38%** | **−10 pp** |
| Live heap | 3.3 MB | 4.3 MB | +1 MB (AA regression) |
| Lifetime allocs | 1,994 MB | **893 MB** | **−55%** |
| QR CPU | ~14 s / 30% | ~0 | **eliminated ✅** |
| `textSizeImpl` / `SizeUTF8` | ~4.2 s / 8.9% | 0.88 s / 4.61% | **−79% ✅** |
| `drawFilledCircle` allocs | 43 MB | **531 MB** | **12× WORSE ❌** |

---

## Round 2 Findings — Status After Fixes

### ✅ QR texture re-generated every frame — fixed

`DetailScreen.drawQR` now caches the `*sdl.Texture` in `s.qrTex`; generated once per game
URL, destroyed on screen close. QR-related symbols are absent from the R3 CPU profile.
The 615 MB flate round-trip is gone: `qrcode.New().Image()` returns pixels directly,
skipping the PNG encode→decode cycle. Lifetime QR allocations effectively zero.

### ✅ `TTF_SizeUTF8` called every frame — largely fixed

`textSizeImpl` falls from 4.2 s to **0.88 s** (−79%). The measurement cache introduced in
round 2 absorbs repeated layout probes. Remaining cost is cold-cache misses on first render
of each unique string.

### ⚠️ `drawFilledCircle` stack arrays — AA rewrite introduced heap regression (531 MB)

The round 2 fix replaced `make([]sdl.Rect, …)` with fixed-size stack arrays to eliminate
the 43 MB per-session heap cost. The round 3 rewrite added anti-aliasing by introducing a
second `fringeBuf [maxCircleRadius*4 + 2]sdl.Rect` (8,224 bytes). Both `solidBuf` and
`fringeBuf` are declared on the stack, but **CGo's escape analysis boundary forces them to
the heap on every call** — the Go compiler cannot prove that `FillRects` (a CGo call) does
not retain the pointer. Result: **531 MB lifetime allocations**, 12× worse than before the
round 2 fix. See fix below.

---

## CPU — Top Findings (Round 3)

### 1. `cgocall` overhead — 7.08 s / 37.07% of sampled CPU

The dominant category is now raw CGo call overhead: every SDL renderer call crosses the
CGo boundary. `drawFilledCircle` contributes **7 CGo calls per invocation** (vs. 2 before
the AA rewrite): `SetDrawBlendMode` ×3, `SetDrawColor` ×2, `FillRects` ×2.

```
DrawPill           1.51 s (7.91%) cumulative
  → drawFilledCircle  1.67 s cumulative (8.74%)
      → SetDrawBlendMode  ×3
      → SetDrawColor      ×2
      → FillRects         ×2
```

**Fix:** Move both rect buffers to a package-level array (avoids per-call heap escape). CGo
call count remains 7 but allocations drop to zero.

---

### 2. `textSizeImpl` / `SizeUTF8` — 0.88 s / 4.61%

Residual cold-cache misses. No further action needed; cost is now proportional to unique
string variety per session, not per-frame frequency.

---

### 3. All other hot paths — within noise

No other symbol exceeds 1 s cumulative. The round 2 fixes (QR caching, text-run LRU,
GIF pre-upload) have eliminated the previous dominant costs.

---

## Memory — Top Findings (Round 3)

### 1. `drawFilledCircle` AA heap escape — 531.99 MB / 59.55% of lifetime allocations

`solidBuf [257]sdl.Rect` (4,112 bytes) + `fringeBuf [514]sdl.Rect` (8,224 bytes) escape to
heap on every `drawFilledCircle` call because Go's escape analysis does not track pointers
through CGo calls (`FillRects`). At ~43,000 circle draws per 55-second session this
produces 531 MB of GC churn.

**Fix:** Replace both stack-local arrays with a single package-level
`[maxCircleRadius*6 + 3]sdl.Rect` buffer. Package-level variables are already on the heap
at program start and are never subject to per-call escape analysis. Since `drawFilledCircle`
is only called from the SDL main goroutine, no synchronisation is needed. Per-call
allocation drops to zero; the 531 MB regression is eliminated.

```go
// circleRectBuf is reused across calls; only accessed from the SDL main goroutine.
var circleRectBuf [maxCircleRadius*6 + 3]sdl.Rect

func drawFilledCircle(ren *sdl.Renderer, cx, cy, radius int32, ...) {
    n := int(radius*2 + 1)
    // solid at [0:ns], fringe at [n : n+nf]
    for i := 0; i < n; i++ {
        circleRectBuf[i]   = sdl.Rect{...solid...}
        circleRectBuf[n+i*2]   = sdl.Rect{...fringe left...}
        circleRectBuf[n+i*2+1] = sdl.Rect{...fringe right...}
    }
    ren.FillRects(circleRectBuf[:n])
    ren.FillRects(circleRectBuf[n : n+n*2])
}
```

### 2. GIF one-time decode — expected ~250 MB lifetime

`image.NewPaletted` + `image.NewRGBA` inside `renderGIFFrames`. One-time decode cost,
GC'd after GPU upload. Unchanged from round 2; acceptable.

### 3. Remaining allocations — ~110 MB

Spread across `compress/flate` (residual from non-QR paths), text-run LRU cache misses,
and network I/O buffers. No single hotspot exceeds 50 MB; no action needed.

---

## Priority Summary (Round 3)

| # | Area | CPU impact | Memory impact | Effort |
|---|------|-----------|---------------|--------|
| 1 | `drawFilledCircle` package-level buffer | −`drawFilledCircle` heap alloc overhead | **−531 MB (−59%)** | trivial |

All other round 2 hotspots have been resolved. After fixing item 1, the application should
exhibit near-zero per-frame heap allocation under steady-state browsing.
