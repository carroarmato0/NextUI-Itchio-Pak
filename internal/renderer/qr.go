//go:build !headless

package renderer

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"

	"github.com/skip2/go-qrcode"
	"github.com/veandco/go-sdl2/sdl"
	stdraw "image/draw"
)

// QRTexture generates a QR code for the given URL and returns an SDL2 texture.
// size is the pixel dimensions of the QR code (e.g. 128).
// The caller must call texture.Destroy() when done.
// Uses qrcode.Image() directly to avoid the encode→PNG→decode round-trip.
func (r *Renderer) QRTexture(url string, size int) (*sdl.Texture, error) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("qr encode: %w", err)
	}
	img := qr.Image(size)

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	stdraw.Draw(rgba, bounds, img, bounds.Min, stdraw.Src)

	surface, err := sdl.CreateRGBSurfaceFrom(
		unsafe.Pointer(&rgba.Pix[0]),
		int32(bounds.Dx()), int32(bounds.Dy()),
		32, bounds.Dx()*4,
		0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
	)
	if err != nil {
		return nil, fmt.Errorf("qr surface: %w", err)
	}
	tex, err := r.Renderer.CreateTextureFromSurface(surface)
	surface.Free()
	runtime.KeepAlive(rgba)
	if err != nil {
		return nil, fmt.Errorf("qr texture: %w", err)
	}
	return tex, nil
}

// QRModules returns how many modules wide the QR code for url is, including
// the four-module quiet zone QRTexture draws. A texture of exactly
// QRModules(url)*n pixels gives n-pixel modules with no blurred edges.
func QRModules(url string) (int, error) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return 0, fmt.Errorf("qr encode: %w", err)
	}
	return len(qr.Bitmap()), nil
}
