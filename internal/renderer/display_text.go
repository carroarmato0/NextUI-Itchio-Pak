//go:build !headless

package renderer

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// Display text is the primary font at a size of the caller's choosing, drawn
// bold — for the rare label that must read from across a room, such as the
// sign-in code. It covers only what the primary font covers (no fallback
// fonts), so it is for short ASCII strings, not titles from itch.io.

// displayFont returns the primary font opened at size, opening it once.
func (r *Renderer) displayFont(size int) *ttf.Font {
	if f, ok := r.displayFonts[size]; ok {
		return f
	}
	f, err := ttf.OpenFont("assets/font.ttf", size)
	if err != nil {
		logger.Warn("renderer: display font at %dpx: %v", size, err)
		return nil
	}
	f.SetStyle(ttf.STYLE_BOLD)
	if r.displayFonts == nil {
		r.displayFonts = map[int]*ttf.Font{}
	}
	r.displayFonts[size] = f
	logger.Debug("renderer: display font opened at %dpx", size)
	return f
}

// DisplayTextSize measures text at size.
func (r *Renderer) DisplayTextSize(text string, size int) (int32, int32) {
	f := r.displayFont(size)
	if f == nil {
		return r.BoldTextSize(text)
	}
	w, h, err := f.SizeUTF8(text)
	if err != nil {
		return 0, 0
	}
	return int32(w), int32(h)
}

// DrawDisplayText draws text at size, falling back to the main bold font if
// the size cannot be opened.
func (r *Renderer) DrawDisplayText(text string, x, y int32, size int, red, green, blue uint8) {
	f := r.displayFont(size)
	if f == nil {
		r.DrawBoldText(text, x, y, red, green, blue)
		return
	}
	if r.drawLogOn {
		w, h := r.DisplayTextSize(text, size)
		r.logSizedTextDraw(text, x, y, w, h, [3]uint8{red, green, blue}, false)
	}
	surface, err := f.RenderUTF8Blended(text, sdl.Color{R: red, G: green, B: blue, A: 255})
	if err != nil {
		return
	}
	defer surface.Free()
	tex, err := r.Renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return
	}
	defer tex.Destroy()
	r.Renderer.Copy(tex, nil, &sdl.Rect{X: x, Y: y, W: surface.W, H: surface.H})
}
