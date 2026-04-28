# Animated GIF Playback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Play animated GIF cover arts in the game list and detail screen by advancing frames inside `cache.Get()`, while keeping a static PNG in `.media/` for NextUI's ROM browser.

**Architecture:** A new pure-Go file `gif_anim.go` holds the `gifAnim` struct, frame-rendering pipeline, and the `advance()` method. `image_cache.go` (SDL-guarded with `!headless`) detects animated GIFs in `fetchRaw`, creates streaming SDL textures in `uploadTexture`, and calls `advance()` each frame in `Get()`. No screen files are touched.

**Tech Stack:** Go stdlib `image/gif`, `golang.org/x/image/draw` (already a dep), go-sdl2 `sdl.TEXTUREACCESS_STREAMING` + `texture.Update()`.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/renderer/gif_anim.go` | **Create** | `gifAnim` struct, `maxGIFFrames` const, `renderGIFFrames` func, `advance()` method — pure Go, no SDL, no build tag |
| `internal/renderer/image_cache.go` | **Modify** | Add `anim *gifAnim` to `cacheEntry` and `rawImage`; buffer body + detect animated GIF in `fetchRaw`; streaming texture path in `uploadTexture`; frame-advance call in `Get` |
| `internal/renderer/image_cache_gif_test.go` | **Create** | SDL-free tests: `TestGIFAnimFrameAdvance`, `TestGIFAnimZeroDelayDefaulted`; no build tag (tests only reference `gif_anim.go` types) |

> **Note on build tags:** The spec refers to `//go:build !sdl` on the test file. Because `gifAnim` and `renderGIFFrames` live in `gif_anim.go` (no build tag, no SDL import), the test file compiles in every mode — including headless CI — without a build constraint. No build tag is needed on the test file.

---

## Task 1: `gifAnim` struct + `renderGIFFrames` + `advance` method

**Files:**
- Create: `internal/renderer/gif_anim.go`
- Create: `internal/renderer/image_cache_gif_test.go` (tests for this task only)

- [ ] **Step 1.1: Write failing tests for `renderGIFFrames`**

Create `internal/renderer/image_cache_gif_test.go`:

```go
package renderer

import (
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"
)

// makeTestGIF returns a 2×2 animated GIF with nFrames frames.
// Frame i gets palette index (i % 2), delay d[i] centiseconds.
func makeTestGIF(nFrames int, delays []int) *gif.GIF {
	palette := color.Palette{color.Black, color.White}
	images := make([]*image.Paletted, nFrames)
	for i := range images {
		img := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		img.SetColorIndex(0, 0, uint8(i%2))
		images[i] = img
	}
	d := delays
	if d == nil {
		d = make([]int, nFrames)
	}
	return &gif.GIF{
		Image:  images,
		Delay:  d,
		Config: image.Config{Width: 2, Height: 2, ColorModel: palette},
	}
}

func TestRenderGIFFramesCount(t *testing.T) {
	g := makeTestGIF(3, []int{10, 20, 15})
	anim := renderGIFFrames(g)
	if len(anim.frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(anim.frames))
	}
	if len(anim.delays) != 3 {
		t.Fatalf("expected 3 delays, got %d", len(anim.delays))
	}
}

func TestRenderGIFFramesCapAtMax(t *testing.T) {
	g := makeTestGIF(maxGIFFrames+5, nil)
	for i := range g.Delay {
		g.Delay[i] = 5
	}
	anim := renderGIFFrames(g)
	if len(anim.frames) != maxGIFFrames {
		t.Fatalf("expected frames capped at %d, got %d", maxGIFFrames, len(anim.frames))
	}
}

func TestRenderGIFFramesDimensions(t *testing.T) {
	g := makeTestGIF(2, []int{5, 5})
	anim := renderGIFFrames(g)
	if anim.w != 2 || anim.h != 2 {
		t.Fatalf("expected 2×2, got %d×%d", anim.w, anim.h)
	}
	if anim.pitch != 2*4 {
		t.Fatalf("expected pitch 8, got %d", anim.pitch)
	}
	for i, f := range anim.frames {
		if len(f) != int(anim.w)*int(anim.h)*4 {
			t.Fatalf("frame %d: expected %d bytes, got %d", i, int(anim.w)*int(anim.h)*4, len(f))
		}
	}
}

func TestGIFAnimZeroDelayDefaulted(t *testing.T) {
	g := makeTestGIF(2, []int{0, 0})
	anim := renderGIFFrames(g)
	for i, d := range anim.delays {
		if d != 100*time.Millisecond {
			t.Errorf("frame %d: delay = %v, want 100ms (zero-delay default)", i, d)
		}
	}
}

func TestGIFAnimNonZeroDelay(t *testing.T) {
	g := makeTestGIF(2, []int{10, 25}) // 10cs = 100ms, 25cs = 250ms
	anim := renderGIFFrames(g)
	if anim.delays[0] != 100*time.Millisecond {
		t.Errorf("frame 0: got %v, want 100ms", anim.delays[0])
	}
	if anim.delays[1] != 250*time.Millisecond {
		t.Errorf("frame 1: got %v, want 250ms", anim.delays[1])
	}
}

func TestGIFAnimFrameAdvance(t *testing.T) {
	base := time.Now()
	a := &gifAnim{
		frames: [][]uint8{make([]uint8, 4), make([]uint8, 4), make([]uint8, 4)},
		delays: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 150 * time.Millisecond},
		cur:    0,
		nextAt: base, // advance due at exactly base
	}

	// time.After is strict: not after base itself → no advance.
	if _, ok := a.advance(base); ok {
		t.Fatal("advance at nextAt (not strictly after) should not fire")
	}

	// 1ns after → should advance to frame 1.
	t0 := base.Add(time.Nanosecond)
	idx, ok := a.advance(t0)
	if !ok {
		t.Fatal("expected advance=true")
	}
	if idx != 1 {
		t.Fatalf("expected frame 1, got %d", idx)
	}
	// nextAt must be set to t0 + delays[1] = t0+200ms.
	wantNextAt := t0.Add(200 * time.Millisecond)
	if !a.nextAt.Equal(wantNextAt) {
		t.Fatalf("nextAt = %v, want %v", a.nextAt, wantNextAt)
	}

	// Same timestamp: no second advance.
	if _, ok := a.advance(t0); ok {
		t.Fatal("should not advance twice at same timestamp")
	}

	// Past frame 1 delay → frame 2.
	t1 := t0.Add(200*time.Millisecond + time.Nanosecond)
	idx, ok = a.advance(t1)
	if !ok || idx != 2 {
		t.Fatalf("expected advance to frame 2, got idx=%d ok=%v", idx, ok)
	}

	// Past frame 2 delay → wrap to frame 0.
	t2 := t1.Add(150*time.Millisecond + time.Nanosecond)
	idx, ok = a.advance(t2)
	if !ok || idx != 0 {
		t.Fatalf("expected wrap to frame 0, got idx=%d ok=%v", idx, ok)
	}
}

func TestGIFAnimAdvanceSingleFrame(t *testing.T) {
	a := &gifAnim{
		frames: [][]uint8{make([]uint8, 4)},
		delays: []time.Duration{100 * time.Millisecond},
		cur:    0,
		nextAt: time.Now().Add(-time.Second), // well past due
	}
	// Single frame: advancing wraps back to 0.
	idx, ok := a.advance(time.Now())
	if !ok || idx != 0 {
		t.Fatalf("single-frame: expected advance to 0, got idx=%d ok=%v", idx, ok)
	}
}
```

- [ ] **Step 1.2: Run tests to verify they fail**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
go test ./internal/renderer/ -run 'TestRenderGIF|TestGIFAnim' -v 2>&1 | head -30
```

Expected: compilation error — `renderGIFFrames`, `gifAnim`, `maxGIFFrames` undefined.

- [ ] **Step 1.3: Create `gif_anim.go`**

Create `internal/renderer/gif_anim.go`:

```go
package renderer

import (
	"image"
	"image/color"
	stdraw "image/draw"
	"image/gif"
	"time"

	"golang.org/x/image/draw"
)

const maxGIFFrames = 64

// gifAnim holds the pre-rendered RGBA frames for one animated GIF.
type gifAnim struct {
	frames [][]uint8       // RGBA pixel data per frame, capped at maxGIFFrames
	delays []time.Duration // per-frame display duration
	w, h   int32
	pitch  int       // w * 4
	cur    int       // current frame index
	nextAt time.Time // wall-clock time to advance to cur+1
}

// advance moves to the next frame if now is strictly after nextAt.
// Returns the current (possibly updated) frame index and whether the frame changed.
func (a *gifAnim) advance(now time.Time) (int, bool) {
	if !now.After(a.nextAt) {
		return a.cur, false
	}
	a.cur = (a.cur + 1) % len(a.frames)
	a.nextAt = now.Add(a.delays[a.cur])
	return a.cur, true
}

// renderGIFFrames renders every frame of g using the standard GIF compositing
// algorithm (disposal methods, frame offsets, background colour) and returns a
// gifAnim with one RGBA pixel slice per frame. Frames beyond maxGIFFrames are
// silently dropped. All frames are scaled to at most 640 px wide (same factor
// for every frame so dimensions are consistent).
func renderGIFFrames(g *gif.GIF) *gifAnim {
	srcW, srcH := g.Config.Width, g.Config.Height
	if srcW == 0 || srcH == 0 {
		b := g.Image[0].Bounds()
		srcW, srcH = b.Dx(), b.Dy()
	}
	srcBounds := image.Rect(0, 0, srcW, srcH)

	// Compute destination size (applied identically to every frame).
	dstW, dstH := srcW, srcH
	if srcW > 640 {
		dstH = srcH * 640 / srcW
		dstW = 640
	}
	dstBounds := image.Rect(0, 0, dstW, dstH)

	bgColor := color.Color(color.RGBA{A: 255}) // opaque black default
	if pal, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(pal) {
		bgColor = pal[g.BackgroundIndex]
	}
	bgFill := image.NewUniform(bgColor)

	canvas := image.NewRGBA(srcBounds)
	stdraw.Draw(canvas, srcBounds, bgFill, image.Point{}, stdraw.Src)

	limit := len(g.Image)
	if limit > maxGIFFrames {
		limit = maxGIFFrames
	}

	frames := make([][]uint8, limit)
	delays := make([]time.Duration, limit)

	for i := 0; i < limit; i++ {
		frame := g.Image[i]
		disposal := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}

		// Save canvas before drawing so DisposalPrevious can restore it.
		var preCanvas *image.RGBA
		if disposal == gif.DisposalPrevious {
			preCanvas = image.NewRGBA(srcBounds)
			stdraw.Draw(preCanvas, srcBounds, canvas, image.Point{}, stdraw.Src)
		}

		stdraw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, stdraw.Over)

		// Scale and capture pixel data for this frame.
		dst := image.NewRGBA(dstBounds)
		if srcW == dstW {
			stdraw.Draw(dst, dstBounds, canvas, image.Point{}, stdraw.Src)
		} else {
			draw.BiLinear.Scale(dst, dstBounds, canvas, srcBounds, draw.Over, nil)
		}
		pix := make([]uint8, len(dst.Pix))
		copy(pix, dst.Pix)
		frames[i] = pix

		d := 0
		if i < len(g.Delay) {
			d = g.Delay[i]
		}
		if d == 0 {
			delays[i] = 100 * time.Millisecond
		} else {
			delays[i] = time.Duration(d) * 10 * time.Millisecond
		}

		// Prepare canvas for next frame per disposal method.
		switch disposal {
		case gif.DisposalBackground:
			stdraw.Draw(canvas, frame.Bounds(), bgFill, image.Point{}, stdraw.Src)
		case gif.DisposalPrevious:
			if preCanvas != nil {
				stdraw.Draw(canvas, frame.Bounds(), preCanvas, frame.Bounds().Min, stdraw.Src)
			}
		}
	}

	return &gifAnim{
		frames: frames,
		delays: delays,
		w:      int32(dstW),
		h:      int32(dstH),
		pitch:  dstW * 4,
	}
}
```

- [ ] **Step 1.4: Run tests to verify they pass**

```bash
go test ./internal/renderer/ -run 'TestRenderGIF|TestGIFAnim' -v
```

Expected output:
```
=== RUN   TestRenderGIFFramesCount
--- PASS: TestRenderGIFFramesCount
=== RUN   TestRenderGIFFramesCapAtMax
--- PASS: TestRenderGIFFramesCapAtMax
=== RUN   TestRenderGIFFramesDimensions
--- PASS: TestRenderGIFFramesDimensions
=== RUN   TestGIFAnimZeroDelayDefaulted
--- PASS: TestGIFAnimZeroDelayDefaulted
=== RUN   TestGIFAnimNonZeroDelay
--- PASS: TestGIFAnimNonZeroDelay
=== RUN   TestGIFAnimFrameAdvance
--- PASS: TestGIFAnimFrameAdvance
=== RUN   TestGIFAnimAdvanceSingleFrame
--- PASS: TestGIFAnimAdvanceSingleFrame
PASS
```

- [ ] **Step 1.5: Run full test suite to confirm no regressions**

```bash
go test -tags headless ./... 2>&1
```

Expected: all pass (headless skips SDL files, gif_anim.go still compiles).

- [ ] **Step 1.6: Commit**

```bash
git add internal/renderer/gif_anim.go internal/renderer/image_cache_gif_test.go
git commit -m "feat(renderer): add gifAnim struct, renderGIFFrames, and frame advance tests"
```

---

## Task 2: Extend `cacheEntry`, `rawImage`, and `fetchRaw` to produce animation data

**Files:**
- Modify: `internal/renderer/image_cache.go`

- [ ] **Step 2.1: Add `anim` field to `cacheEntry` and `rawImage`**

In `internal/renderer/image_cache.go`, replace the two existing type declarations:

```go
// Old cacheEntry:
type cacheEntry struct {
	key     string
	texture *sdl.Texture
}

// New cacheEntry:
type cacheEntry struct {
	key     string
	texture *sdl.Texture
	anim    *gifAnim // nil for static images
}
```

```go
// Old rawImage:
type rawImage struct {
	url   string
	pix   []uint8
	w, h  int32
	pitch int
}

// New rawImage:
type rawImage struct {
	url   string
	pix   []uint8   // frame 0 pixel data (or only frame for static images)
	w, h  int32
	pitch int
	anim  *gifAnim  // non-nil when source is an animated GIF
}
```

- [ ] **Step 2.2: Add required imports to `image_cache.go`**

The current import block in `image_cache.go` starts at line 5. Replace the full import block:

```go
import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/draw"
)
```

> Note: `_ "image/gif"` becomes `"image/gif"` (we need `gif.DecodeAll`). `bytes` and `io` are new for body buffering. `logger` is new for animated GIF log lines.

- [ ] **Step 2.3: Update `fetchRaw` to detect animated GIFs**

Replace the existing `fetchRaw` function body (lines ~186–214 in the current file):

```go
func (c *ImageCache) fetchRaw(url string) (rawImage, error) {
	resp, err := c.client.Get(url)
	if err != nil {
		return rawImage{}, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rawImage{}, fmt.Errorf("fetch image: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return rawImage{}, fmt.Errorf("fetch image: read body: %w", err)
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return rawImage{}, fmt.Errorf("decode image: %w", err)
	}

	// Detect animated GIF: re-decode with gif.DecodeAll and render all frames.
	if format == "gif" {
		if g, err2 := gif.DecodeAll(bytes.NewReader(data)); err2 == nil && len(g.Image) > 1 {
			logger.Debug("image cache: animated GIF %s (%d frames)", url, len(g.Image))
			anim := renderGIFFrames(g)
			return rawImage{
				url:   url,
				pix:   anim.frames[0],
				w:     anim.w,
				h:     anim.h,
				pitch: anim.pitch,
				anim:  anim,
			}, nil
		}
	}

	// Static image path (unchanged).
	img = resizeMax(img, 640)
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	return rawImage{
		url:   url,
		pix:   rgba.Pix,
		w:     int32(bounds.Dx()),
		h:     int32(bounds.Dy()),
		pitch: bounds.Dx() * 4,
	}, nil
}
```

- [ ] **Step 2.4: Verify compilation**

```bash
go build -tags headless ./internal/renderer/
```

Expected: no errors (headless skips SDL-dependent parts but gif_anim.go compiles).

```bash
go vet -tags headless ./internal/renderer/
```

Expected: no issues.

- [ ] **Step 2.5: Run full test suite**

```bash
go test -tags headless ./... 2>&1
```

Expected: all pass.

- [ ] **Step 2.6: Commit**

```bash
git add internal/renderer/image_cache.go
git commit -m "feat(renderer): extend rawImage/cacheEntry for animation; detect animated GIF in fetchRaw"
```

---

## Task 3: Streaming texture upload for animated entries (`uploadTexture`)

**Files:**
- Modify: `internal/renderer/image_cache.go` — `uploadTexture` function

- [ ] **Step 3.1: Replace `uploadTexture` with animated-aware version**

Replace the existing `uploadTexture` function:

```go
func (c *ImageCache) uploadTexture(r *Renderer, raw rawImage) {
	if len(raw.pix) == 0 || raw.w <= 0 || raw.h <= 0 {
		return
	}

	var tex *sdl.Texture
	var err error

	if raw.anim != nil {
		// Streaming texture: supports in-place pixel updates via texture.Update().
		tex, err = r.Renderer.CreateTexture(
			sdl.PIXELFORMAT_ABGR8888,
			sdl.TEXTUREACCESS_STREAMING,
			raw.w, raw.h,
		)
		if err != nil {
			log.Printf("image cache: create streaming texture: %v", err)
			return
		}
		if err = tex.SetBlendMode(sdl.BLENDMODE_BLEND); err != nil {
			log.Printf("image cache: set blend mode: %v", err)
			tex.Destroy()
			return
		}
		if err = tex.Update(nil, raw.pix, raw.pitch); err != nil {
			log.Printf("image cache: texture update (frame 0): %v", err)
			tex.Destroy()
			return
		}
	} else {
		// Static image: existing surface-based path.
		surface, surfErr := sdl.CreateRGBSurfaceFrom(
			unsafe.Pointer(&raw.pix[0]),
			raw.w, raw.h,
			32, raw.pitch,
			0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
		)
		if surfErr != nil {
			return
		}
		tex, err = r.Renderer.CreateTextureFromSurface(surface)
		surface.Free()
		runtime.KeepAlive(raw.pix)
		if err != nil {
			return
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[raw.url]; exists {
		tex.Destroy()
		return
	}
	entry := &cacheEntry{key: raw.url, texture: tex, anim: raw.anim}
	el := c.lru.PushFront(entry)
	c.items[raw.url] = el
	for c.lru.Len() > c.max {
		back := c.lru.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*cacheEntry)
		evicted.texture.Destroy()
		delete(c.items, evicted.key)
		c.lru.Remove(back)
	}
}
```

> SDL pixel format note: `sdl.PIXELFORMAT_ABGR8888` stores (in a 32-bit int, MSB→LSB) A, B, G, R. On little-endian this is R, G, B, A in memory — matching `image.RGBA.Pix` layout.

- [ ] **Step 3.2: Verify compilation**

```bash
go build ./internal/renderer/ 2>&1
```

Expected: no errors (this requires SDL headers; if building on a headless machine, use `-tags headless` for the pure-Go parts instead and trust cross-compilation for the SDL path).

- [ ] **Step 3.3: Commit**

```bash
git add internal/renderer/image_cache.go
git commit -m "feat(renderer): create streaming SDL texture for animated GIF entries"
```

---

## Task 4: Frame advance in `Get`

**Files:**
- Modify: `internal/renderer/image_cache.go` — `Get` method

- [ ] **Step 4.1: Add frame-advance call inside the LRU-hit path of `Get`**

Replace the existing `Get` function:

```go
// Get returns the cached SDL2 texture for url, or nil if not yet loaded.
// When nil is returned a background fetch is started; call Get again on the
// next Draw cycle to pick up the result once it arrives.
// Must be called from the SDL main thread.
func (c *ImageCache) Get(_ *Renderer, url string) *sdl.Texture {
	c.mu.Lock()
	if el, ok := c.items[url]; ok {
		c.lru.MoveToFront(el)
		entry := el.Value.(*cacheEntry)
		if entry.anim != nil {
			if idx, advanced := entry.anim.advance(time.Now()); advanced {
				if err := entry.texture.Update(nil, entry.anim.frames[idx], entry.anim.pitch); err != nil {
					log.Printf("image cache: texture update frame %d: %v", idx, err)
				}
			}
		}
		tex := entry.texture
		c.mu.Unlock()
		return tex
	}
	if _, bad := c.failed[url]; bad {
		c.mu.Unlock()
		return nil
	}
	if _, pending := c.fetching[url]; !pending {
		c.fetching[url] = struct{}{}
		c.mu.Unlock()
		go c.fetchInBackground(url)
	} else {
		c.mu.Unlock()
	}
	return nil
}
```

- [ ] **Step 4.2: Verify compilation**

```bash
go build ./internal/renderer/ 2>&1
```

Expected: no errors.

- [ ] **Step 4.3: Run full test suite**

```bash
go test -tags headless ./... 2>&1
```

Expected: all pass.

- [ ] **Step 4.4: Commit**

```bash
git add internal/renderer/image_cache.go
git commit -m "feat(renderer): advance animated GIF frames in Get() each render cycle"
```

---

## Task 5: Cross-compilation and device smoke test

**Files:** none (build verification only)

- [ ] **Step 5.1: Cross-compile for ARM (TrimUI / Miyoo target)**

```bash
make build 2>&1
```

Or if that's not available:

```bash
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 \
  CC=arm-linux-gnueabihf-gcc \
  go build -tags trimui ./cmd/itchio-pak/ 2>&1
```

Expected: no compilation errors.

- [ ] **Step 5.2: Run host tests one final time**

```bash
go test -tags headless ./... -count=1 2>&1
```

Expected: all pass.

- [ ] **Step 5.3: Commit if any fixup was needed; otherwise tag complete**

If no changes were needed, the feature is done. If the cross-compiler revealed issues, fix them and commit with `fix(renderer): cross-compile fix for animated GIF`.

---

## Self-Review Checklist

### Spec Coverage

| Spec requirement | Task(s) |
|---|---|
| `gifAnim` struct with `frames`, `delays`, `w`, `h`, `pitch`, `cur`, `nextAt` | Task 1 |
| `maxGIFFrames = 64` | Task 1 |
| `cacheEntry.anim *gifAnim` | Task 2 |
| `rawImage.anim *gifAnim`; `pix` = frame 0 | Task 2 |
| `renderGIFFrames`: disposal algorithm, resize, cap at maxGIFFrames, zero-delay=100ms | Task 1 |
| `fetchRaw`: buffer body, `gif.DecodeAll`, animated detection | Task 2 |
| `ProcessPending` / `uploadTexture`: streaming texture + blend mode + frame-0 upload | Task 3 |
| `Get`: advance frame on schedule, `texture.Update` | Task 4 |
| `TestGIFAnimFrameAdvance` | Task 1 |
| `TestGIFAnimZeroDelayDefaulted` | Task 1 |
| No screen file changes | Maintained throughout |

### Logging Coverage (per project standards)

| Event | Level | Location |
|---|---|---|
| Animated GIF detected (N frames) | Debug | `fetchRaw` |
| Streaming texture creation error | (log.Printf, existing pattern) | `uploadTexture` |
| Texture update error on frame advance | (log.Printf, existing pattern) | `Get` |
