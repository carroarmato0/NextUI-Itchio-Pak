//go:build !headless

package renderer

import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
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

type cacheEntry struct {
	key      string
	texture  *sdl.Texture   // current displayed texture (points into textures[] for GIFs)
	textures []*sdl.Texture // pre-uploaded frame textures; nil for static images
	anim     *gifAnim       // nil for static images
}

// destroyTextures destroys all GPU textures held by this entry.
func (e *cacheEntry) destroyTextures() {
	if len(e.textures) > 0 {
		for _, t := range e.textures {
			if t != nil {
				t.Destroy()
			}
		}
	} else if e.texture != nil {
		e.texture.Destroy()
	}
}

// rawImage holds decoded pixel data ready to be uploaded to a GPU texture.
type rawImage struct {
	url   string
	pix   []uint8  // frame 0 pixel data (or only frame for static images)
	w, h  int32
	pitch int
	anim  *gifAnim // non-nil when source is an animated GIF
}

type ImageCache struct {
	mu       sync.Mutex
	lru      *list.List
	items    map[string]*list.Element
	fetching map[string]struct{} // URLs currently being fetched in background
	failed   map[string]struct{} // URLs that permanently failed (bad format)
	max      int
	client   *http.Client
	readyCh  chan rawImage // pixel data ready for main-thread texture upload
	sem      chan struct{} // concurrency limiter for background fetches
	notify   func()       // optional: called when an image lands in readyCh
}

// SetNotify registers a callback invoked once each time a decoded image is
// queued for upload. Use it to push an SDL UserEvent so the main loop can
// block in WaitEvent() instead of polling with a short timeout.
// Safe to call before any fetches start; not safe to change while fetches run.
func (c *ImageCache) SetNotify(fn func()) { c.notify = fn }

const maxConcurrentFetches = 2

func NewImageCache(maxEntries int) *ImageCache {
	return &ImageCache{
		lru:      list.New(),
		items:    make(map[string]*list.Element),
		fetching: make(map[string]struct{}),
		failed:   make(map[string]struct{}),
		max:      maxEntries,
		client:   &http.Client{Timeout: 20 * time.Second},
		readyCh:  make(chan rawImage, 32),
		sem:      make(chan struct{}, maxConcurrentFetches),
	}
}

// advanceGIFFrame advances the animation for entry if the current frame has
// expired and swaps entry.texture to the next pre-uploaded GPU texture.
// Zero-allocation: no surface or texture creation — pure pointer swap.
// Must be called with c.mu held.
func advanceGIFFrame(entry *cacheEntry) {
	if entry.anim == nil || len(entry.textures) == 0 {
		return
	}
	idx, advanced := entry.anim.advance(time.Now())
	if !advanced {
		return
	}
	if idx < len(entry.textures) && entry.textures[idx] != nil {
		entry.texture = entry.textures[idx]
	}
}

// Get returns the cached SDL2 texture for url, or nil if not yet loaded.
// When nil is returned a background fetch is started; call Get again on the
// next Draw cycle to pick up the result once it arrives.
// Must be called from the SDL main thread.
func (c *ImageCache) Get(r *Renderer, url string) *sdl.Texture {
	c.mu.Lock()
	if el, ok := c.items[url]; ok {
		c.lru.MoveToFront(el)
		entry := el.Value.(*cacheEntry)
		advanceGIFFrame(entry)
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
		logger.Debug("image cache: queuing fetch %s", url)
		c.mu.Unlock()
		go c.fetchInBackground(url)
	} else {
		c.mu.Unlock()
	}
	return nil
}

// Peek returns the cached SDL2 texture for url without starting a fetch.
// Use this for rendering when fetch timing is controlled externally via Warm.
// Returns nil if the URL is not yet in the cache.
// Must be called from the SDL main thread.
func (c *ImageCache) Peek(r *Renderer, url string) *sdl.Texture {
	c.mu.Lock()
	el, ok := c.items[url]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	c.lru.MoveToFront(el)
	entry := el.Value.(*cacheEntry)
	advanceGIFFrame(entry)
	tex := entry.texture
	c.mu.Unlock()
	return tex
}

// Warm schedules a background fetch for url if it is not already cached or
// in-flight. Returns immediately with no texture. Use alongside Peek to control
// exactly when fetches are initiated.
func (c *ImageCache) Warm(url string) {
	c.mu.Lock()
	_, cached := c.items[url]
	_, fetching := c.fetching[url]
	_, failed := c.failed[url]
	if !cached && !fetching && !failed {
		c.fetching[url] = struct{}{}
		logger.Debug("image cache: warming %s", url)
		c.mu.Unlock()
		go c.fetchInBackground(url)
		return
	}
	c.mu.Unlock()
}

// Failed reports whether the given URL permanently failed to load.
func (c *ImageCache) Failed(url string) bool {
	c.mu.Lock()
	_, bad := c.failed[url]
	c.mu.Unlock()
	return bad
}

// ProcessPending uploads any decoded images that background goroutines have
// finished fetching. Must be called from the SDL main thread once per frame.
// Returns true if any images were uploaded.
func (c *ImageCache) ProcessPending(r *Renderer) bool {
	uploaded := false
	for {
		select {
		case raw := <-c.readyCh:
			c.uploadTexture(r, raw)
			uploaded = true
		default:
			return uploaded
		}
	}
}

func (c *ImageCache) uploadTexture(r *Renderer, raw rawImage) {
	if raw.w <= 0 || raw.h <= 0 {
		return
	}

	var tex *sdl.Texture
	var textures []*sdl.Texture

	if raw.anim != nil && len(raw.anim.frames) > 0 {
		// Animated GIF: pre-upload every frame as a GPU texture in one shot.
		// After upload, raw.anim.frames is set to nil — GPU VRAM holds all frames,
		// eliminating the 272 MB live heap and the per-tick surface/texture churn.
		textures = make([]*sdl.Texture, len(raw.anim.frames))
		for i, framePix := range raw.anim.frames {
			if len(framePix) == 0 {
				continue
			}
			surface, surfErr := sdl.CreateRGBSurfaceFrom(
				unsafe.Pointer(&framePix[0]),
				raw.anim.w, raw.anim.h,
				32, raw.anim.pitch,
				0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
			)
			if surfErr != nil {
				logger.Warn("image cache: GIF frame %d surface: %v", i, surfErr)
				continue
			}
			t, texErr := r.Renderer.CreateTextureFromSurface(surface)
			surface.Free()
			runtime.KeepAlive(framePix)
			if texErr != nil {
				logger.Warn("image cache: GIF frame %d texture: %v", i, texErr)
				continue
			}
			textures[i] = t
		}
		raw.anim.frames = nil // free pixel slices — all frames live on GPU now
		tex = textures[0]
		raw.anim.nextAt = time.Now().Add(raw.anim.delays[0])
		logger.Debug("image cache: uploaded animated %s (%d frames, %dx%d)", raw.url, len(textures), raw.anim.w, raw.anim.h)
	} else {
		if len(raw.pix) == 0 {
			return
		}
		surface, surfErr := sdl.CreateRGBSurfaceFrom(
			unsafe.Pointer(&raw.pix[0]),
			raw.w, raw.h,
			32, raw.pitch,
			0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
		)
		if surfErr != nil {
			logger.Warn("image cache: create surface: %v", surfErr)
			return
		}
		var err error
		tex, err = r.Renderer.CreateTextureFromSurface(surface)
		surface.Free()
		runtime.KeepAlive(raw.pix)
		if err != nil {
			logger.Warn("image cache: create texture: %v", err)
			return
		}
		logger.Debug("image cache: uploaded static %s (%dx%d)", raw.url, raw.w, raw.h)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[raw.url]; exists {
		// Race: another goroutine uploaded this URL concurrently; discard ours.
		if len(textures) > 0 {
			for _, t := range textures {
				if t != nil {
					t.Destroy()
				}
			}
		} else if tex != nil {
			tex.Destroy()
		}
		return
	}
	entry := &cacheEntry{key: raw.url, texture: tex, textures: textures, anim: raw.anim}
	el := c.lru.PushFront(entry)
	c.items[raw.url] = el
	for c.lru.Len() > c.max {
		back := c.lru.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*cacheEntry)
		evicted.destroyTextures()
		delete(c.items, evicted.key)
		c.lru.Remove(back)
	}
}

func (c *ImageCache) fetchInBackground(url string) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	raw, err := c.fetchRaw(url)

	c.mu.Lock()
	delete(c.fetching, url)
	if err != nil {
		errMsg := err.Error()
		if isDecodingError(errMsg) {
			logger.Warn("image cache (permanent): %s: %v", url, err)
			c.failed[url] = struct{}{}
		} else {
			logger.Warn("image cache (transient): %s: %v", url, err)
		}
	}
	c.mu.Unlock()

	if err == nil {
		c.readyCh <- raw
		if c.notify != nil {
			c.notify()
		}
	}
}

// isDecodingError returns true for errors that won't succeed on retry
// (unsupported format, corrupt data). Network/timeout errors return false.
func isDecodingError(msg string) bool {
	return strings.Contains(msg, "decode image") ||
		strings.Contains(msg, "unknown format")
}

func (c *ImageCache) fetchRaw(url string) (rawImage, error) {
	logger.Debug("image cache: fetch %s", url)
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
	logger.Debug("image cache: decoded %s as %s (%d bytes)", url, format, len(data))

	if format == "gif" {
		if g, err2 := gif.DecodeAll(bytes.NewReader(data)); err2 == nil && len(g.Image) > 1 {
			logger.Debug("image cache: animated GIF %s (%d frames)", url, len(g.Image))
			anim := renderGIFFrames(g)
			if len(anim.frames) > 0 {
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
	}

	logger.Debug("image cache: static %s", url)
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

// Clear destroys all cached textures. Must be called from the SDL main thread.
func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, el := range c.items {
		el.Value.(*cacheEntry).destroyTextures()
	}
	c.lru.Init()
	c.items = make(map[string]*list.Element)
	c.failed = make(map[string]struct{})
}

func resizeMax(img image.Image, maxWidth int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxWidth {
		return img
	}
	newH := h * maxWidth / w
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
