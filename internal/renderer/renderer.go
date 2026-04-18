//go:build !headless

package renderer

import "github.com/veandco/go-sdl2/sdl"

// Renderer wraps SDL2 window, renderer, and font.
type Renderer struct {
	W, H int32
}

func New(title string, w, h int) (*Renderer, error)                                 { return nil, nil }
func (r *Renderer) Close()                                                          {}
func (r *Renderer) Clear(red, green, blue uint8)                                    {}
func (r *Renderer) Present()                                                        {}
func (r *Renderer) DrawRect(x, y, w, h int32, red, green, blue uint8)              {}
func (r *Renderer) DrawText(text string, x, y int32, red, green, blue uint8) error { return nil }
func (r *Renderer) DrawTextureAt(tex *sdl.Texture, x, y, w, h int32)               {}
