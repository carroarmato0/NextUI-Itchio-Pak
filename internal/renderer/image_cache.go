//go:build !headless

package renderer

import (
	"container/list"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/draw"
)

type cacheEntry struct {
	key     string
	texture *sdl.Texture
}

// rawImage holds decoded pixel data ready to be uploaded to a GPU texture.
type rawImage struct {
	url   string
	pix   []uint8
	w, h  int32
	pitch int
}

type ImageCache struct {
	mu       sync.Mutex
	lru      *list.List
	items    map[string]*list.Element
	fetching map[string]struct{} // URLs currently being fetched in background
	failed   map[string]struct{} // URLs that permanently failed (bad format)
	max      int
	client   *http.Client
	readyCh  chan rawImage      // pixel data ready for main-thread texture upload
	sem      chan struct{}      // concurrency limiter for background fetches
}

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

// Get returns the cached SDL2 texture for url, or nil if not yet loaded.
// When nil is returned a background fetch is started; call Get again on the
// next Draw cycle to pick up the result once it arrives.
// Must be called from the SDL main thread.
func (c *ImageCache) Get(_ *Renderer, url string) *sdl.Texture {
	c.mu.Lock()
	if el, ok := c.items[url]; ok {
		c.lru.MoveToFront(el)
		tex := el.Value.(*cacheEntry).texture
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

// Failed reports whether the given URL permanently failed to load.
func (c *ImageCache) Failed(url string) bool {
	c.mu.Lock()
	_, bad := c.failed[url]
	c.mu.Unlock()
	return bad
}

// ProcessPending uploads any decoded images that background goroutines have
// finished fetching. Must be called from the SDL main thread once per frame.
func (c *ImageCache) ProcessPending(r *Renderer) {
	for {
		select {
		case raw := <-c.readyCh:
			c.uploadTexture(r, raw)
		default:
			return
		}
	}
}

func (c *ImageCache) uploadTexture(r *Renderer, raw rawImage) {
	if len(raw.pix) == 0 || raw.w <= 0 || raw.h <= 0 {
		return
	}
	surface, err := sdl.CreateRGBSurfaceFrom(
		unsafe.Pointer(&raw.pix[0]),
		raw.w, raw.h,
		32, raw.pitch,
		0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
	)
	if err != nil {
		return
	}
	tex, err := r.Renderer.CreateTextureFromSurface(surface)
	surface.Free()
	runtime.KeepAlive(raw.pix)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[raw.url]; exists {
		tex.Destroy()
		return
	}
	entry := &cacheEntry{key: raw.url, texture: tex}
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

func (c *ImageCache) fetchInBackground(url string) {
	// Acquire semaphore slot — blocks if maxConcurrentFetches are already running.
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	raw, err := c.fetchRaw(url)

	c.mu.Lock()
	delete(c.fetching, url)
	if err != nil {
		errMsg := err.Error()
		// Permanent failure: image format not supported (won't succeed on retry).
		// Transient failure (timeout, network): allow retry on next Get call.
		if isDecodingError(errMsg) {
			log.Printf("image cache (permanent): %s: %v", url, err)
			c.failed[url] = struct{}{}
		} else {
			log.Printf("image cache (transient): %s: %v", url, err)
		}
	}
	c.mu.Unlock()

	if err == nil {
		c.readyCh <- raw
	}
}

// isDecodingError returns true for errors that won't succeed on retry
// (unsupported format, corrupt data). Network/timeout errors return false.
func isDecodingError(msg string) bool {
	return strings.Contains(msg, "decode image") ||
		strings.Contains(msg, "unknown format")
}

func (c *ImageCache) fetchRaw(url string) (rawImage, error) {
	resp, err := c.client.Get(url)
	if err != nil {
		return rawImage{}, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rawImage{}, fmt.Errorf("fetch image: HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return rawImage{}, fmt.Errorf("decode image: %w", err)
	}

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
		el.Value.(*cacheEntry).texture.Destroy()
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
