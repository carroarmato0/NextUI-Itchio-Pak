//go:build !headless

package renderer

import "github.com/veandco/go-sdl2/sdl"

func (r *Renderer) QRTexture(url string, size int) (*sdl.Texture, error) { return nil, nil }
