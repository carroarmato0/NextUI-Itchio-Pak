//go:build !headless

package renderer

import "github.com/veandco/go-sdl2/sdl"

// ImageCache is an LRU cache of SDL2 textures keyed by URL.
type ImageCache struct{}

func NewImageCache(maxEntries int) *ImageCache                  { return nil }
func (c *ImageCache) Get(r *Renderer, url string) *sdl.Texture  { return nil }
func (c *ImageCache) Clear()                                     {}
