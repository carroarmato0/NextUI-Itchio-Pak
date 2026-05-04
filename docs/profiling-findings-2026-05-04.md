# Profiling Findings — itchio-pak (2026-05-04)

**Session:** 91.79s wall time, 61.75s sampled CPU (67% utilisation)  
**Live heap at exit:** 276 MB  
**Lifetime allocations:** 975 MB  
**Profiles:** `debug-profiles/itchio-cpu.prof`, `debug-profiles/itchio-mem.prof`  
**Binary:** `bin/tg5040/itchio-pak` (v1.0.11 / 72b5b4a-dirty)

---

## CPU — Top findings

### 1. Text re-rendered from scratch every frame — ~33% of total CPU

`CreateTextureFromSurface` (15.82s cumulative) + `SDL_DestroyTexture` (5.01s) = ~21s combined.
Both flow directly from `ListScreen.Draw` → `DrawText` / `DrawSmallText` / `DrawFooterHints`.

Root cause in `renderer.go` `DrawText`:

```go
surface, err := font.RenderUTF8Blended(run.text, color)    // SDL surface every call
texture, err := r.Renderer.CreateTextureFromSurface(surface) // GPU upload every call
texture.Destroy()                                            // GPU free every call
```

Every call to `DrawText`, `DrawSmallText`, `DrawFooterHints`, and `DrawTagPills` creates a new SDL
surface, uploads it to the GPU, renders it, and destroys it — on **every frame**.  Game titles,
tag pills, button labels, and footer text are all static between frames and re-created unnecessarily
each draw cycle.

**Fix:** Add an LRU text-texture cache keyed by `(text, fontIdx, r, g, b)`.  
Cache hit = blit cached texture, zero surface/upload/destroy overhead.  
Estimated saving: 20+ seconds over a 90s session.

---

### 2. Footer hints fully re-rendered every frame — ~4s / 6.5%

`DrawFooterHints` (`renderer.go:505`) calls `DrawSmallText`, `DrawPill`, and `DrawCircleBadge` for
every hint on every frame.  It accounts for 3.43s of `ListScreen.Draw`'s cumulative time.  The
footer hint set is **completely static** for a given screen state.

**Fix:** Render the complete footer once to an offscreen SDL texture when the hint set changes; blit
that texture each frame.  Invalidate and re-render only when hints change.

---

### 3. GIF frame advance creates and destroys a GPU texture every tick — repeated overhead

In `image_cache.go` (`Get` lines 86-106, duplicated in `Peek` lines 140-166), every time a GIF
advances a frame:

```go
surface, _ := sdl.CreateRGBSurfaceFrom(...)       // new surface
newTex, _ := r.Renderer.CreateTextureFromSurface(surface) // GPU upload
surface.Free()
entry.texture.Destroy()                            // destroy previous frame texture
entry.texture = newTex
```

This is a GPU texture round-trip on **every frame advance** for every animated cover art on screen.
At a 100 ms frame interval that is 10 texture create/destroy cycles per second per visible GIF.

**Fix:** Pre-upload all GIF frames as `[]*sdl.Texture` at load time inside `gifAnim`.  Frame advance
becomes a zero-allocation pointer swap.  The raw pixel slices (`gifAnim.frames`) can be freed after
upload, eliminating 272 MB of live heap.  Also removes the code duplication between `Get` and
`Peek`.

---

### 4. GIF compositing goes through the slow paletted path — ~13s one-time per decode

In `gif_anim.go:127`:

```go
stdraw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, stdraw.Over)
```

`frame` is `*image.Paletted`.  The `Over` operator with a paletted source has no fast path — it
calls `drawRGBA` which calls `RGBA64At` on every pixel (palette index lookup → colour conversion →
bounds check → alpha blend).  This is the 8.79s `image/draw.drawRGBA` + 4.65s
`Paletted.RGBA64At` CPU cost.

**Fix:** Pre-convert each paletted frame to RGBA before compositing:

```go
rgbaFrame := image.NewRGBA(frame.Bounds())
stdraw.Draw(rgbaFrame, frame.Bounds(), frame, frame.Bounds().Min, stdraw.Src) // Src on Paletted is optimised
stdraw.Draw(canvas, frame.Bounds(), rgbaFrame, frame.Bounds().Min, stdraw.Over) // RGBA→RGBA Over has fast path
```

One-time cost at decode time, not per rendered frame.

---

### 5. `drawFilledCircle` uses one CGo DrawLine call per scanline — ~1s / 1.6%

`drawFilledCircle` (`renderer.go:433`) issues one `DrawLine` CGo call per scanline row.  For a
badge of radius ~18px that is 36 CGo round-trips per circle, called for every A/B badge in the
footer every frame.

**Fix:** Replace scanline loop with `SDL_RenderGeometry` (triangle fan) in a single CGo call, or
pre-render badges to a texture at startup.

---

## Memory — Top findings

### 1. GIF pixel data dominates the live heap — 272 MB of 276 MB live

`inuse_space` profile: `renderGIFFrames` holds 272 MB live.  These are the pre-rendered RGBA pixel
slices in `gifAnim.frames[]` — up to 64 frames × (640 × height × 4 bytes) per cached GIF.  Retained
for the entire lifetime of the cache entry.

**Fix:** Directly resolved by CPU finding #3 — once frames are pre-uploaded as SDL textures the raw
pixel slices can be freed immediately after upload.

### 2. GIF decoding allocates 390 MB total (one-time, GC'd) — 40% of lifetime allocations

`image.NewRGBA` allocates a full-canvas RGBA image for every stored GIF frame during
`renderGIFFrames`.  For a 64-frame GIF at 640×480 this is ~77 MB per GIF, all allocated then mostly
GC'd.  The GC pressure is visible as `mallocgc` entries in the CPU trace.

**Fix:** Reuse a single scratch `image.RGBA` canvas across frame renders inside `renderGIFFrames`.
Allocate `dst` once before the loop; reset per iteration.  The `pix := make([]uint8); copy()`
snapshot line still correctly captures each frame.

---

## Priority summary

| # | Area | CPU impact | Memory impact | Effort |
|---|------|-----------|---------------|--------|
| 1 | Text texture cache (`DrawText`/`DrawSmallText`) | ~20s / **33%** | low | medium |
| 2 | Footer hints offscreen texture | ~4s / **6.5%** | low | low |
| 3 | Pre-upload GIF frames as SDL textures | per-tick overhead | **frees 272 MB live** | medium |
| 4 | GIF paletted→RGBA pre-conversion | ~13s one-time | low | low |
| 5 | Reuse scratch RGBA canvas in `renderGIFFrames` | low | reduces GC churn | low |
| 6 | `drawFilledCircle` CGo reduction | ~1s / 1.6% | low | low |

Items 1 and 3 are the dominant pathologies: per-frame texture thrash in text rendering, and
per-tick GPU upload in GIF animation.  Together they should produce a very significant drop in CPU
load and bring live heap from 276 MB down to the low tens of MB.
