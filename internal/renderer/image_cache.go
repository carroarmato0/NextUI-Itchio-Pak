//go:build !headless

package renderer

import (
	"container/list"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"sync"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/draw"
)

type cacheEntry struct {
	key     string
	texture *sdl.Texture
}

type ImageCache struct {
	mu     sync.Mutex
	lru    *list.List
	items  map[string]*list.Element
	max    int
	client *http.Client
}

func NewImageCache(maxEntries int) *ImageCache {
	return &ImageCache{
		lru:    list.New(),
		items:  make(map[string]*list.Element),
		max:    maxEntries,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Get returns a cached SDL2 texture for the given URL, fetching it if needed.
// Returns nil on fetch/decode failure (caller should show a placeholder).
func (c *ImageCache) Get(r *Renderer, url string) *sdl.Texture {
	c.mu.Lock()
	if el, ok := c.items[url]; ok {
		c.lru.MoveToFront(el)
		tex := el.Value.(*cacheEntry).texture
		c.mu.Unlock()
		return tex
	}
	c.mu.Unlock()

	tex, err := c.fetchAndDecode(r, url)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &cacheEntry{key: url, texture: tex}
	el := c.lru.PushFront(entry)
	c.items[url] = el

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
	return tex
}

// Clear destroys all cached textures.
func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, el := range c.items {
		el.Value.(*cacheEntry).texture.Destroy()
	}
	c.lru.Init()
	c.items = make(map[string]*list.Element)
}

func (c *ImageCache) fetchAndDecode(r *Renderer, url string) (*sdl.Texture, error) {
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	img = resizeMax(img, 640)

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	surface, err := sdl.CreateRGBSurfaceFrom(
		rgba.Pix,
		int32(bounds.Dx()), int32(bounds.Dy()),
		32, int32(bounds.Dx()*4),
		0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
	)
	if err != nil {
		return nil, fmt.Errorf("create surface: %w", err)
	}
	defer surface.Free()

	tex, err := r.Renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return nil, fmt.Errorf("create texture: %w", err)
	}
	return tex, nil
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
