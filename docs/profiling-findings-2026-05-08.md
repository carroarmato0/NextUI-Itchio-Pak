# Profiling Findings — itchio-pak (2026-05-08)

**Session:** 901.30 s wall time (~15 min), 122.93 s sampled CPU (**13.64% utilisation**)
**Active CPU (excluding vsync wait):** ~87.6 s / 901.3 s ≈ **9.7%**
**Live heap at exit:** 6.58 MB
**Lifetime allocations:** 3,484 MB (longer session; more GIF previews viewed than round 3)
**Profiles:** `debug-profiles/itchio-cpu.prof`, `debug-profiles/itchio-mem.prof`
**Binary:** `bin/tg5040/itchio-pak` (fix/code-quality-fixes branch, commit 0281ad3)

---

## Comparison with Round 3 (2026-05-04)

| Metric | Round 3 | Round 4 | Notes |
|--------|---------|---------|-------|
| Wall time | 55.56 s | 901.30 s | Much longer session |
| Sampled CPU | 19.10 s | 122.93 s | Proportional to wall time |
| CPU utilisation | 34.38% | **13.64%** | App is more idle; GIF work in background |
| Live heap | 4.3 MB | 6.58 MB | Normal variance |
| Lifetime allocs | 893 MB | 3,484 MB | ~3.9 MB/s; GIF-heavy session |
| `drawFilledCircle` allocs | 531.99 MB (59%) | **~0** | **FIXED ✅** (`circleRectBuf` in place) |

### Round 3 action item — status

| Item | Status |
|------|--------|
| `drawFilledCircle` package-level `circleRectBuf` | **FIXED ✅** — absent from alloc_space top 33; confirmed at renderer.go:487 |

---

## CPU — Top Findings (Round 4)

### 1. `SDL_RenderPresent` vsync wait — 35.35 s cumulative (28.75%)

The app blocks on `SDL_RenderPresent` waiting for display flip. This is idle time, not
work. **No action needed.** Excluding this, active CPU is ~9.7%.

### 2. GIF decode pipeline — ~41 s cumulative (33.47%)

The image cache background goroutines spend the bulk of active CPU decoding GIF previews:

| Symbol | Flat | Flat% |
|--------|------|-------|
| `compress/lzw.(*Reader).decode` | 9.63 s | 7.83% |
| `compositePalettedOver` | 8.43 s | 6.86% |
| `io.ReadFull` (GIF byte read) | 15.87 s cum | 12.91% |
| `image/gif.(*decoder).readImageDescriptor` | 1.18 s | 0.96% |
| `image.(*Paletted).RGBA64At` | 1.21 s | 0.98% |
| `compress/lzw.(*Reader).readLSB` | 1.20 s | 0.98% |

This is all in `fetchInBackground` goroutines — it does not block the SDL draw thread.
The app stays responsive while images load.

**GIF decode is inherently expensive** (LZW decompression + per-pixel palette compositing).
Reducing it requires either: (a) skipping frames for very long GIFs, (b) capping download
size, or (c) accepting it as background cost. **No action recommended for now** — the
background goroutine model already keeps it off the main thread.

### 3. Image resize — ~6 s flat (~5%)

| Symbol | Flat |
|--------|------|
| `nnInterpolator.scale_RGBA_RGBA_Src` | 3.13 s (2.55%) |
| `kernelScaler.scaleY_RGBA_Src` | 1.81 s (1.47%) |
| `kernelScaler.scaleX_RGBA64Image` | 1.23 s (1.00%) |

Also in background goroutines. No action needed.

### 4. `drawFilledCircle` + `DrawPill` FillRects overhead — 8.20 s cumulative (6.67%)

Every `DrawPill` call (game tag badges, footer buttons) invokes `drawFilledCircle` twice.
Each `drawFilledCircle` calls `FillRects` twice (solid + fringe). Each `FillRects` is a
CGo call: `6.70 s` cumulative through `SDL_RenderFillRects`.

The `circleRectBuf` fix eliminated the heap allocation. The CGo-crossing cost itself
remains. The fix for heap escape on `DrawPill`'s body rect (see Memory §2) will not reduce
this CPU time — that is a separate concern.

### 5. `splitTextRuns` — 0.62 s flat (0.50%)

Same flat cost as `drawFilledCircle` (0.62 s). Per-frame UTF-8 scanning of game titles.
Fix in Memory §1 will also reduce the GC pressure that contributes to this figure.

### 6. Text measurement path — 3.0 s cumulative

`textSizeImpl` (0.17 s flat, 3.0 s cum) + `textCache.get` (0.19 s flat) + `fontIndex`
(0.24 s flat). The `textCache` LRU absorbs repeated measurement; residual cost is cold
misses. No further action needed.

---

## Memory — Top Findings (Round 4)

### 1. `splitTextRuns` per-call allocation — 912,050 objects (**30.28%** of all objects)

The single largest allocation source by object count. `splitTextRuns` (text.go:169)
allocates a fresh `[]textRun` on every call via `append`. With `textSizeImpl` (568,024
cumulative objects) and `drawRuns` (912,049 cumulative) both calling it, every text
measure or render produces at least one allocation.

**Fix:** Package-level reuse — same pattern as `circleRectBuf`:

```go
var splitBuf []textRun

func splitTextRuns(s string, fontIndex func(rune) int) []textRun {
    if s == "" {
        return nil
    }
    splitBuf = splitBuf[:0]
    runStart := 0
    runIdx := -1
    for i := 0; i < len(s); {
        // ... unchanged body, append to splitBuf instead of runs ...
    }
    if runIdx >= 0 && runStart < len(s) {
        splitBuf = append(splitBuf, textRun{text: s[runStart:], fontIdx: runIdx})
    }
    return splitBuf
}
```

Both callers (`drawRuns` and `textSizeImpl`) iterate the returned slice immediately and do
not store it, so sharing `splitBuf` across calls is safe. SDL main goroutine only — no
locking needed.

**Expected impact:** ~30% reduction in total allocation object count per session.

---

### 2. `DrawPill` body rect CGo escape — 262,147 objects (8.70%)

`DrawPill` (renderer.go:466) passes a composite literal pointer to `FillRect`:

```go
r.Renderer.FillRect(&sdl.Rect{X: x + radius, Y: y, W: w - radius*2, H: h})
```

The pointer escapes to the heap at the CGo boundary. Apply the existing `circleRectBuf`
pattern:

```go
var pillBodyBuf sdl.Rect

func (r *Renderer) DrawPill(x, y, w, h int32, red, green, blue uint8) {
    radius := h / 2
    if radius < 1 {
        radius = 1
    }
    r.Renderer.SetDrawColor(red, green, blue, 255)
    pillBodyBuf = sdl.Rect{X: x + radius, Y: y, W: w - radius*2, H: h}
    r.Renderer.FillRect(&pillBodyBuf)
    drawFilledCircle(r.Renderer, x+radius, y+radius, radius, red, green, blue)
    drawFilledCircle(r.Renderer, x+w-radius, y+radius, radius, red, green, blue)
}
```

**Expected impact:** −262,147 objects per session (~8.7% of object count).

---

### 3. `DrawRect` CGo escape — 98,304 objects (3.26%)

Same issue as `DrawPill`. `DrawRect` passes an `&sdl.Rect{...}` to a CGo `FillRect` or
`RenderCopy` call. Needs a `drawRectBuf sdl.Rect` package-level variable using the same
pattern.

**Expected impact:** −98,304 objects per session (~3.3% of object count).

---

### 4. `truncateToWidth` per-frame `[]rune` allocation — 206,627 objects (6.86%)

`truncateToWidth` and `truncateBoldToWidth` (screen_list.go:1106, :1146) convert the game
title to `[]rune` then call `string(runes)` in a shrink loop — both operations allocate.
These are called once per visible game per frame on `ListScreen`.

The result is constant for a given (title, maxW) pair. Cache at the `ListScreen` level,
keyed to the current page, rebuilt on page change:

```go
type ListScreen struct {
    ...
    truncCache []string // one entry per game in current page; rebuilt on page/layout change
    truncMaxW  int32    // invalidated when column width changes
}
```

Populate in the page-load path; read zero-alloc in `Draw`.

**Expected impact:** eliminates per-frame string allocs for list titles (~153 K objects from
`strings.(*Builder).grow` also go away, since these are driven by the same calls).

---

### 5. GIF decode lifetime allocations — 3,244 MB (93.1%) — expected, no action

| Source | Alloc space |
|--------|-------------|
| `renderGIFFrames` direct (composite RGBA frames) | 1,523.70 MB (43.7%) |
| `image.NewPaletted` (GIF per-frame palette buffers) | 843.59 MB (24.2%) |
| `io.ReadAll` (raw GIF download) | 310.09 MB (8.9%) |
| `image.NewRGBA` (renderGIFFrames compositing) | 287.41 MB (8.2%) |
| `kernelScaler.makeTmpBuf` (resize intermediate) | 194.06 MB (5.6%) |
| `compress/lzw.newReader` | 118.42 MB (3.4%) |

All GC'd after GPU texture upload. Proportional to the number of GIF images viewed in a
session. The LRU cache (50 entries) holds decoded textures; re-decode only happens on
cache eviction. **No action needed.**

---

### 6. Live heap composition (6.58 MB)

| Holder | Live | Notes |
|--------|------|-------|
| `encoding/json.literalStore` | 1.54 MB | Parsed game list response in memory |
| `textSizeImpl` cache | 0.67 MB | Text measurement LRU; expected |
| `FetchGameDetail` buffer | 0.64 MB | Last detail screen response |
| `runtime/pprof.StartCPUProfile` | 0.58 MB | Profiling only; absent in production |
| `golang.org/x/net/html` init table | 0.56 MB | HTML tokenizer; one-time startup |
| `parseCmap12` font table | 0.54 MB | Fallback font cmap; expected |
| `http2.NewFramer` write buffer | 0.52 MB | HTTP/2 connection; permanent |
| `pprof.profMap.lookup` | 0.52 MB | Profiling only |
| `encoding/json.typeFields` | 0.51 MB | Reflection cache; one-time |
| `crypto/nistec.P384Point` | 0.51 MB | TLS ECDSA; one-time after handshake |

No live heap concerns. All retained memory is intentional or profiling-induced.

---

## Priority Summary (Round 4)

| # | Area | Object impact | Space impact | Effort |
|---|------|---------------|--------------|--------|
| 1 | `splitTextRuns` package-level `splitBuf` | −912 K objects (−30%) | −~10 MB alloc | trivial |
| 2 | `DrawPill` `pillBodyBuf` package-level | −262 K objects (−8.7%) | negligible | one line |
| 3 | `DrawRect` `drawRectBuf` package-level | −98 K objects (−3.3%) | negligible | one line |
| 4 | `truncateToWidth` cache on `ListScreen` | −206 K objects (−6.9%) | negligible | small |

Items 1–3 are trivial mechanical changes following the existing `circleRectBuf` pattern.
Item 4 requires a small cache field and rebuild-on-page-change logic.

Combined, items 1–4 would eliminate approximately **49%** of all allocation objects per
session, reducing GC pressure significantly on the memory-constrained ARM64 device.
