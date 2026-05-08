# Profiling Findings — itchio-pak (2026-05-08, round 3)

**Session:** 318.19 s wall (~5 min), 94.71 s sampled CPU (**29.76% utilisation**)
**Active CPU (excluding vsync):** ~59.8 s / 318.2 s ≈ **18.8%** (GIF-heavy browsing)
**Live heap at snapshot:** 119.97 MB (GIF frames mid-decode, see §6)
**Lifetime allocations:** 1,824.36 MB
**Alloc rate:** 5.73 MB/s (dominated by GIF image decode, see §5)
**Total alloc objects:** 634,310
**Profiles:** `debug-profiles/itchio-cpu.prof`, `debug-profiles/itchio-mem.prof`
**Binary:** `bin/tg5040/itchio-pak` (fix/code-quality-fixes branch, commit a565168)

---

## Comparison with Round 5 (2026-05-08 earlier)

| Metric | Round 5 | Round 6 | Notes |
|--------|---------|---------|-------|
| `fmt.Sprintf` badge labels | 120,151 (21.3%) | **0** | **FIXED ✅** |
| `splitTextRuns` / `drawRuns` / `DrawPill` / `DrawRect` | ~1.7M objects | **0** | **FIXED ✅** |
| `truncateToWidth` per-frame | 206,627 (6.9%) | **cold-cache only** | **FIXED ✅** |
| `ListScreen.Draw` flat | 89,568 (15.9%) | **12,288 (1.9%)** | **−86%** |
| All per-frame allocators | dominant | near-zero | **FIXED ✅** |

All six round-5 action items are confirmed resolved. The new top items are
either one-time session costs (parsing, QR, TLS) or two remaining CGo escapes.

---

## CPU — Top Findings (Round 6)

### 1. `SDL_RenderPresent` vsync — 34.88 s cumulative (36.83%)

Normal idle wait. The user spent time on both `ListScreen` and `DetailScreen` this
session (roughly equal split vs round 5's detail-heavy run). **No action needed.**

### 2. GIF decode pipeline — 26.62 s cumulative (28.11%)

| Symbol | Flat |
|--------|------|
| `compress/lzw.(*Reader).decode` | 6.04 s (6.38%) |
| `compositePalettedOver` | 5.60 s (5.91%) |
| `renderGIFFrames` cumulative | 11.93 s (12.60%) |
| `nnInterpolator` resize | 3.23 s (3.41%) |

User browsed many GIF-preview games this session. All in background goroutines;
main thread stays responsive. **No action needed.**

### 3. `drawFilledCircle` + `DrawPill` — 7.78 s / 7.33 s cumulative

Unchanged pattern from previous rounds. The `radius < 4` AA-skip fix is in place and
covers tiny circles. Larger pills (radius ≥ 8) still pay the full 7-CGo-call cost.
No further low-risk optimisation identified. **No action needed.**

---

## Memory — Top Findings (Round 6)

### 1. `DrawTextureAt` CGo escape — 32,768 objects (**5.17%**)

`DrawTextureAt` (renderer.go:362) passes a composite literal directly to `Copy`:

```go
r.Renderer.Copy(tex, nil, &sdl.Rect{X: x, Y: y, W: w, H: h})
```

The pointer escapes at the CGo boundary. Same pattern as the previous `copyDstBuf` fix.

**Fix:** reuse `copyDstBuf` (already declared for `drawRuns`):

```go
func (r *Renderer) DrawTextureAt(tex *sdl.Texture, x, y, w, h int32) {
    copyDstBuf = sdl.Rect{X: x, Y: y, W: w, H: h}
    r.Renderer.Copy(tex, nil, &copyDstBuf)
}
```

**Expected impact:** −32,768 objects per session (−5.2%).

---

### 2. `SetClipRect` CGo escape — 32,768 objects (**5.17%**)

`SetClipRect` (renderer.go:367) declares a local `rect` and passes `&rect` to the CGo
call. The local variable escapes to the heap through the CGo boundary:

```go
rect := sdl.Rect{X: x, Y: y, W: w, H: h}
r.Renderer.SetClipRect(&rect)
```

**Fix:** add a package-level `clipRectBuf sdl.Rect`:

```go
var clipRectBuf sdl.Rect

func (r *Renderer) SetClipRect(x, y, w, h int32) {
    clipRectBuf = sdl.Rect{X: x, Y: y, W: w, H: h}
    r.Renderer.SetClipRect(&clipRectBuf)
}
```

**Expected impact:** −32,768 objects per session (−5.2%).

---

### 3. `DetailScreen.Draw` flat — 39,322 objects (**6.20%**)

`ListScreen.Draw` flat dropped from 89,568 to 12,288 (−86%) after the badge price
cache. `DetailScreen.Draw` flat remains at 39,322 — the unsolved `fmt.Sprintf` calls
in `screen_detail.go`:

| Line | Call | Frequency |
|------|------|-----------|
| 335 | `fmt.Sprintf("Image %d/%d  (←→)", screenshotIdx+1, n)` | once per frame on detail |
| 363 | `fmt.Sprintf("$%.2f", price)` | once per download option per frame |
| 390 | `fmt.Sprintf(" (+%d more)", n)` | once per multi-file entry per frame |

The screenshot counter and download prices are static once detail data loads. Pre-format
them in `DetailScreen` when `s.detail` is populated (same pattern as `badgePriceCache`).

**Expected impact:** −~39,322 objects per session (−6.2%).

---

### 4. One-time session costs — not actionable per-frame

| Allocator | Objects | Source |
|-----------|---------|--------|
| `html.(*Tokenizer).Token` | 62,929 | Game description HTML parse (once per game) |
| `encoding/xml.*` | ~105,000 | RSS feed / game data XML parse |
| `go-qrcode/reedsolomon.*` | ~90,700 | QR code generation (once per game, texture cached) |
| `compress/flate.*` | 40,172 | Image/network decompression |
| TLS/crypto | ~57,000 | One-time per HTTPS connection |
| `regexp.FindAllStringSubmatch` | 16,384 | Game data field extraction |
| `time.NewTimer` | 12,015 | Animation/scroll timers (short-lived) |

All bounded by session activity (games viewed, connections made), not frame rate.
**No action needed.**

---

### 5. GIF + image decode — 1,741 MB (95.4% of alloc_space)

| Source | Space |
|--------|-------|
| `renderGIFFrames` direct | 731.69 MB (40.1%) |
| `image.NewPaletted` (GIF frames) | 519.60 MB (28.5%) |
| `io.ReadAll` raw download | 198.97 MB (10.9%) |
| `image.NewRGBA` compositing | 164.55 MB (9.0%) |
| `compress/lzw.newReader` | 44.41 MB (2.4%) |

All GC'd after GPU upload. High this session because the user browsed many GIF-preview
games. **No action needed.**

---

### 6. Live heap (119.97 MB) — GIF mid-decode at exit

The unusually large live heap (vs ~6 MB in previous rounds) is explained entirely by
two in-progress GIF frame buffers retained at snapshot time:

| Holder | Live |
|--------|------|
| `renderGIFFrames` (decoded RGBA frames) | 55.57 MB |
| `image.NewPaletted` (GIF palette frames) | 53.16 MB |

These are the CPU-side frame buffers for a large GIF that was decoded but not yet GC'd
at exit. They are temporary allocations — GC'd after GPU texture upload. The app exited
while a background fetch was still mid-decode. **No live heap leak; no action needed.**

---

## Priority Summary (Round 6)

| # | Area | Object impact | Effort |
|---|------|---------------|--------|
| 1 | `DrawTextureAt` → reuse `copyDstBuf` | −32,768 (−5.2%) | trivial |
| 2 | `SetClipRect` → `clipRectBuf` package-level | −32,768 (−5.2%) | trivial |
| 3 | `DetailScreen.Draw` `fmt.Sprintf` → pre-format on detail load | −~39,322 (−6.2%) | low |

After items 1–2 the only remaining per-frame allocators are in `DetailScreen.Draw`.
After item 3, per-frame allocation should be near-zero across both screens.
