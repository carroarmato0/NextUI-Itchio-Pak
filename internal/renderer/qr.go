//go:build !headless

package renderer

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"github.com/skip2/go-qrcode"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/draw"
)

// QRTexture generates a QR code PNG for the given URL and returns an SDL2 texture.
// size is the pixel dimensions of the QR code (e.g. 128).
// The caller must call texture.Destroy() when done.
func (r *Renderer) QRTexture(url string, size int) (*sdl.Texture, error) {
	pngBytes, err := qrcode.Encode(url, qrcode.Medium, size)
	if err != nil {
		return nil, fmt.Errorf("qr encode: %w", err)
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("qr png decode: %w", err)
	}

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
		return nil, fmt.Errorf("qr surface: %w", err)
	}
	defer surface.Free()

	tex, err := r.Renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return nil, fmt.Errorf("qr texture: %w", err)
	}
	return tex, nil
}
