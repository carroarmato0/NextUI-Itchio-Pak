//go:build !headless

package renderer

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// Offscreen rendering: draw real screens to a PNG on a development host with no
// display, no device and no window.
//
// This exists so the screen × palette matrix is cheap. NextUI ships eighteen
// palettes and the app has two dozen screens; capturing that on hardware means
// a relaunch per shot and the better part of an hour. Here it is a few hundred
// milliseconds, which makes it something a test suite can do on every change.
//
// The construction path is deliberately shared with New via newWithTarget: same
// fonts, same sizes, same caches. Only the SDL video driver and the renderer
// backend differ, so what is captured is what the device draws.

// NewOffscreen creates a Renderer that draws into an off-screen software target.
//
// SDL still needs a window to hang a renderer off, so one is created hidden
// under the "dummy" video driver, which needs no display server. The renderer
// is a software rasteriser, which is what makes ReadPixels possible — an
// accelerated renderer may not support reading back, and on a headless host
// there is no GPU context to accelerate against anyway.
func NewOffscreen(w, h int, th theme.Theme) (*Renderer, error) {
	// Must be set before sdl.Init; SDL reads it when the video subsystem starts.
	if err := os.Setenv("SDL_VIDEODRIVER", "dummy"); err != nil {
		return nil, fmt.Errorf("set SDL_VIDEODRIVER: %w", err)
	}
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return nil, fmt.Errorf("sdl init (offscreen): %w", err)
	}
	if err := ttf.Init(); err != nil {
		return nil, fmt.Errorf("ttf init: %w", err)
	}

	win, err := sdl.CreateWindow("itchio-offscreen",
		sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(w), int32(h), sdl.WINDOW_HIDDEN)
	if err != nil {
		return nil, fmt.Errorf("create hidden window: %w", err)
	}
	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_SOFTWARE)
	if err != nil {
		return nil, fmt.Errorf("create software renderer: %w", err)
	}
	logger.Debug("renderer: offscreen %dx%d (dummy driver, software rasteriser)", w, h)
	return newWithTarget(win, ren, w, h, th)
}

// Pixels reads the current target back as an RGBA image.
//
// Call after the screen has drawn and before the next Clear. Present is not
// required — there is no display to present to — but calling it is harmless.
func (r *Renderer) Pixels() (*image.RGBA, error) {
	return r.pixelsRect(0, 0, r.W, r.H)
}

// pixelsRect reads an arbitrary sub-rectangle of the target.
func (r *Renderer) pixelsRect(x, y, w, h int32) (*image.RGBA, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("pixels: empty rect %dx%d", w, h)
	}
	pitch := int(w) * 4
	buf := make([]byte, pitch*int(h))
	rect := sdl.Rect{X: x, Y: y, W: w, H: h}
	if err := r.Renderer.ReadPixels(&rect, uint32(sdl.PIXELFORMAT_ARGB8888),
		unsafe.Pointer(&buf[0]), pitch); err != nil {
		return nil, fmt.Errorf("read pixels: %w", err)
	}

	// SDL's ARGB8888 is a packed little-endian uint32, so the bytes land as
	// B,G,R,A. image.RGBA wants R,G,B,A.
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i+0] = buf[i+2] // R
		img.Pix[i+1] = buf[i+1] // G
		img.Pix[i+2] = buf[i+0] // B
		img.Pix[i+3] = 0xFF     // target is always opaque
	}
	return img, nil
}

// CapturePNG reads the target back and writes it to path, creating parent
// directories as needed.
func (r *Renderer) CapturePNG(path string) error {
	img, err := r.Pixels()
	if err != nil {
		return err
	}
	return WritePNG(path, img)
}

// WritePNG encodes an image to disk, creating parent directories.
func WritePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	logger.Debug("renderer: wrote %s", path)
	return nil
}
