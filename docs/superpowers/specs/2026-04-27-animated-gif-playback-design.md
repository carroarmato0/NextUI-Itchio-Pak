# Animated GIF Playback — Design Spec

**Date:** 2026-04-27  
**Status:** Approved

## Problem

Some itch.io cover arts are animated GIFs. The app currently saves a static PNG
thumbnail at download time and displays it as a still image. The GIF never plays
in the UI.

## Goal

Play animated GIF cover arts in the game list (focused entry only) and on the
game detail screen, while keeping a good-looking static PNG in `.media/` for
NextUI's ROM browser.

## Scope

- All changes are confined to `internal/renderer/image_cache.go` and a new test
  file. No screen files (`screen_list.go`, `screen_detail.go`) are modified.
- The static PNG generation fix (`compositeGIFFrames`, brightest-frame selection)
  is already in place and is not part of this work.

---

## Design

### Approach

Frame advancement lives in `cache.Get()`. `Get()` is already called every frame
only for the currently visible cover art (focused game in the list; active
screenshot on the detail screen). This means advancement is naturally gated to
"only when active" without any extra coupling between the cache and the screens.

### Data Model

Two new types, all in `image_cache.go`:

```go
// gifAnim holds the rendered RGBA frames for one animated GIF.
type gifAnim struct {
    frames  [][]uint8       // rendered RGBA slices, one per frame (capped at maxGIFFrames)
    delays  []time.Duration // per-frame display duration
    w, h    int32
    pitch   int             // w * 4
    cur     int             // current frame index
    nextAt  time.Time       // wall-clock time to advance to cur+1
}

const maxGIFFrames = 64
```

`cacheEntry` gains one field:

```go
type cacheEntry struct {
    key     string
    texture *sdl.Texture
    anim    *gifAnim    // nil for static images
}
```

`rawImage` (background → main thread) gains one field:

```go
type rawImage struct {
    url   string
    pix   []uint8   // frame 0 pixel data (or only frame for static)
    w, h  int32
    pitch int
    anim  *gifAnim  // non-nil when source is an animated GIF
}
```

`pix` always holds frame 0 so `ProcessPending` can use the same initial-upload
path regardless of whether the image is animated.

### Decode Pipeline (`fetchRaw`)

1. HTTP fetch and format detection are unchanged.
2. If format is `"gif"` and `gif.DecodeAll` returns more than one frame:
   a. Render every frame onto a canvas using the full GIF compositing algorithm
      (disposal methods: None / Background / Previous; frame offsets; background
      colour from global colour table). The algorithm is implemented as a private
      helper `renderGIFFrames` inside `image_cache.go`. It is intentionally
      separate from `compositeGIFFrames` in `cover_art.go`: that function returns
      only the single brightest frame for use as a static PNG thumbnail, whereas
      the renderer needs every frame in sequence.
   b. Resize all frames to the same dimensions (max 640 px wide, same scale
      applied to every frame for consistency).
   c. Take the first `min(frameCount, maxGIFFrames)` rendered frames. If the
      GIF has more than `maxGIFFrames` frames the excess is silently dropped;
      the stored frames loop back to index 0.
   d. Convert frame delays from GIF's 100ths-of-a-second units to
      `time.Duration`. A delay of 0 is treated as 100 ms (browser convention).
   e. Populate `rawImage.anim`; set `rawImage.pix` to frame 0.
3. For static GIFs (single frame) and all other formats the path is unchanged.

### Texture Management

**`ProcessPending` (main thread):**

When draining `readyCh`, if `raw.anim != nil`:
- Create an `SDL_TEXTUREACCESS_STREAMING` texture (via `renderer.CreateTexture`)
  instead of the usual `CreateTextureFromSurface` path. Streaming textures
  support in-place pixel updates via `texture.Update()`.
- Set `sdl.BLENDMODE_BLEND` on the texture to handle any transparent frames.
- Upload frame 0 with `texture.Update(nil, raw.pix, raw.pitch)`.
- Attach `raw.anim` to the new `cacheEntry`.

Eviction is unchanged: `texture.Destroy()` is called regardless of whether the
entry is animated. The `gifAnim` frames are regular Go heap allocations and are
collected by the GC.

**`Get` (main thread, called from `Draw` each frame):**

After the existing LRU-hit path, if `entry.anim != nil`:

```
now := time.Now()
if now.After(entry.anim.nextAt) {
    entry.anim.cur = (entry.anim.cur + 1) % len(entry.anim.frames)
    entry.texture.Update(nil, entry.anim.frames[entry.anim.cur], entry.anim.pitch)
    entry.anim.nextAt = now.Add(entry.anim.delays[entry.anim.cur])
}
```

The updated texture is returned to the caller as usual.

### Memory Budget

`maxGIFFrames = 64`. For the largest observed cover art (225 × 204, 274 frames):

- Raw frame store: 64 × 225 × 204 × 4 B ≈ **11 MB RAM**
- GPU texture: 1 × 225 × 204 × 4 B ≈ **183 KB VRAM**

At most one animated entry is cached at any time in practice (cover arts with
animation are rare; only the focused game's URL is fetched on the list screen).

### No Screen Changes

`screen_list.go` and `screen_detail.go` are unmodified. The list screen already
calls `cache.Get()` only for `games[cursor].CoverURL`; the detail screen calls
it only for `ScreenshotURLs[screenshotIdx]`. The "only when active" invariant
falls out of the existing call sites.

---

## Testing

| Test | Location | What it verifies |
|---|---|---|
| `TestGIFAnimFrameAdvance` | `internal/renderer/image_cache_gif_test.go` | Frames advance on schedule and wrap correctly (SDL-free, build-tagged) |
| `TestGIFAnimZeroDelayDefaulted` | same | Zero-delay frames default to 100 ms |
| `TestDownloadCoverArtAnimatedGIFComposited` | `internal/itchio/cover_art_test.go` | Already exists — covers decode side |

The renderer test file uses a build tag (`//go:build !sdl`) so it compiles on
the host without SDL headers.

---

## Out of Scope

- Pause / resume controls (no button is mapped to this).
- Animated screenshots (only cover art URLs are GIFs in practice).
- Adaptive frame-skip when the render loop runs slower than the GIF's frame rate.
