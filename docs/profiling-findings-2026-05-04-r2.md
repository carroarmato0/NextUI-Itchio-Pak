# Profiling Findings — itchio-pak (2026-05-04, round 2)

**Session:** 106.69s wall time, 47.02s sampled CPU (44% utilisation)
**Live heap at exit:** 3.3 MB
**Lifetime allocations:** 1,994 MB
**Profiles:** `debug-profiles/itchio-cpu.prof`, `debug-profiles/itchio-mem.prof`
**Binary:** `bin/tg5040/itchio-pak` (post perf/render-optimizations merge)

---

## Comparison with Round 1 (2026-05-04 baseline)

| Metric | Round 1 | Round 2 | Delta |
|--------|---------|---------|-------|
| Wall time | 91.79 s | 106.69 s | +14.9 s (longer run) |
| Sampled CPU | 61.75 s | **47.02 s** | **−14.7 s (−24%)** |
| CPU utilisation | 67% | **44%** | **−23 pp** |
| Live heap | 276 MB | **3.3 MB** | **−272 MB (−99%)** |
| Lifetime allocs | 975 MB | 1,994 MB | +1 GB (longer session + new QR hotspot) |

---

## Round 1 Findings — Status After Fixes

### ✅ Text texture churn — fixed (6.3×)

`SDL_CreateTextureFromSurface` + `SDL_DestroyTexture` for text: **~3.3 s cumulative** (was ~20.8 s).
The text-run LRU cache absorbs repeated renders of identical strings. Remaining cost is QR
upload and one-time GIF pre-upload, not per-frame text.

### ✅ Footer hints re-rendered every frame — fixed (9×)

`DrawFooterHints` now totals 0.44 s cumulative and 12 MB lifetime. Effectively eliminated.

### ✅ GIF per-frame texture create/destroy — eliminated

`renderGIFFrames` is 3.45 s cumulative — all one-time load cost. The per-tick
`CreateRGBSurfaceFrom → CreateTextureFromSurface → Destroy` loop is gone.

### ✅ GIF paletted compositing slow path — fixed

`image/draw.drawRGBA` (3.22 s) and `Paletted.RGBA64At` (2.15 s) now appear only inside
the one-time `renderGIFFrames` load pass. `compositePalettedOver` (1.43 s flat) handles
the per-frame overlay compositing at ~9× lower cost.

### ✅ Live heap from GIF pixel data — eliminated

276 MB → 3.3 MB. Raw pixel slices freed after GPU upload.

### ⚠️ `drawFilledCircle` — partial fix

CGo-per-scanline removed; `FillRects` is 0.74 s cumulative. But `make([]sdl.Rect, radius*2+1)`
still escapes to heap every call: **43 MB lifetime**. Fixed in round 2.

---

## CPU — Top Findings (Round 2)

### 1. QR texture re-generated on every frame — ~14s / 30% of CPU, 1,272 MB lifetime

`DetailScreen.drawQR` calls `Renderer.QRTexture` **on every frame** the detail screen is
visible. Each call runs the full QR encode pipeline from scratch and immediately destroys
the result:

```
DetailScreen.drawQR
  → Renderer.QRTexture
      → qrcode.Encode        8.29 s cumulative — Reed-Solomon + symbol + PNG compress
          → compress/flate   615 MB lifetime allocations
      → png.Decode           1.89 s
      → image.NewRGBA        (pixel buffer)
      → CreateTextureFromSurface
  → tex.Destroy()            called by drawQR on the same frame
```

This is the same create-and-destroy-every-frame anti-pattern previously fixed for text and GIFs.

Additionally, the encode→PNG→decode round-trip is wasteful: `qrcode.Encode` produces a PNG
that is immediately decoded back to pixels. `qrcode.New(url, level).Image(size)` returns an
`image.Image` directly, skipping both operations.

**Fix A:** Cache the `*sdl.Texture` in `DetailScreen` (field `qrTex *sdl.Texture`); generate
once per game URL, destroy on screen close.
**Fix B:** In `QRTexture` (or its replacement), use `qrcode.New().Image()` instead of
`qrcode.Encode` + `png.Decode` to eliminate the 615 MB flate round-trip.

---

### 2. `TTF_SizeUTF8` called every frame for layout — ~4.2s / 8.9%

`WrapText`, `TextSize`, and `SmallTextSize` issue a CGo `SizeUTF8` call for every word
probe during line-breaking. The same strings are measured on every frame; none of the
measurement results are cached. The text-run texture cache avoids re-uploading glyphs but
does not help here.

```
WrapText          3.70 s cumulative
  → SizeUTF8     (per word-probe)
SmallTextSize     (called from DrawFooterHints, DrawTagPills, etc.)
TextSize          (called from DrawHeaderBar, DrawTextCentered, etc.)
```

`splitTextRuns` (1.78 s cumulative) + `inRanges` (0.88 s flat) add overhead on every
measurement call as well.

**Fix:** Add a `(fontID uint8, text string) → (w, h int32)` measurement cache in the renderer,
checked before every `SizeUTF8` CGo call in `TextSize` and `SmallTextSize`. `WrapText` would
benefit automatically since it calls `TextSize` per word probe.

---

### 3. `drawFilledCircle` heap-allocates rect slice every call — 43 MB lifetime

`make([]sdl.Rect, radius*2+1)` inside `drawFilledCircle` escapes to the heap on every
invocation (the slice is passed to `FillRects` which is a CGo call, preventing stack
escape analysis from keeping it on the stack).

**Fix:** Use a fixed-size array on the stack and slice it:
```go
var buf [512]sdl.Rect  // radius ≤ 255 covers all device resolutions
n := int(radius*2 + 1)
for i := range n { ... fill buf[i] ... }
ren.FillRects(buf[:n])
```

---

## Memory — Top Findings (Round 2)

### 1. QR flate allocations — 615 MB lifetime (30.8%)

`compress/flate.NewWriter` inside `qrcode.Encode` — eliminated by Fix B above.

### 2. GIF one-time decode — expected ~560 MB lifetime

`image.NewPaletted` (246 MB) + `image.NewRGBA` (280 MB) inside `renderGIFFrames`.
These are one-time decode costs, GC'd immediately after. Acceptable.

### 3. `drawFilledCircle` slice — 43 MB lifetime

Fixed by task 3 above.

---

## Priority Summary (Round 2)

| # | Area | CPU impact | Memory impact | Effort |
|---|------|-----------|---------------|--------|
| 1 | Cache QR texture in `DetailScreen` | ~14 s / **30%** | eliminates 615 MB flate | low |
| 2 | Skip PNG round-trip in `QRTexture` | ~2 s (one-time) | −615 MB | low |
| 3 | `SizeUTF8` measurement cache | ~4.2 s / **8.9%** | low | low |
| 4 | `drawFilledCircle` stack array | low | −43 MB | trivial |

Items 1+2 address the same code path; implement together.
