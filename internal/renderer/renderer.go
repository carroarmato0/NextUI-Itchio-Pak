//go:build !headless

package renderer

import (
	"fmt"
	"strings"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

type Renderer struct {
	Window   *sdl.Window
	Renderer *sdl.Renderer
	Font     *ttf.Font
	W, H     int32
}

func New(title string, w, h int) (*Renderer, error) {
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return nil, fmt.Errorf("sdl init: %w", err)
	}
	if err := ttf.Init(); err != nil {
		return nil, fmt.Errorf("ttf init: %w", err)
	}

	win, err := sdl.CreateWindow(title,
		sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(w), int32(h), sdl.WINDOW_SHOWN)
	if err != nil {
		return nil, fmt.Errorf("create window: %w", err)
	}

	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return nil, fmt.Errorf("create renderer: %w", err)
	}

	// Scale font with screen height: h/32 gives ~24pt at 768, ~22pt at 720.
	// Clamp to a minimum of 18 so it stays readable on very small displays.
	fontSize := h / 32
	if fontSize < 18 {
		fontSize = 18
	}
	font, err := ttf.OpenFont("assets/font.ttf", fontSize)
	if err != nil {
		return nil, fmt.Errorf("open font: %w", err)
	}

	return &Renderer{
		Window: win, Renderer: ren, Font: font,
		W: int32(w), H: int32(h),
	}, nil
}

func (r *Renderer) Close() {
	if r.Font != nil {
		r.Font.Close()
	}
	if r.Renderer != nil {
		r.Renderer.Destroy()
	}
	if r.Window != nil {
		r.Window.Destroy()
	}
	ttf.Quit()
	sdl.Quit()
}

func (r *Renderer) Clear(red, green, blue uint8) {
	r.Renderer.SetDrawColor(red, green, blue, 255)
	r.Renderer.Clear()
}

func (r *Renderer) Present() {
	r.Renderer.Present()
}

func (r *Renderer) DrawRect(x, y, w, h int32, red, green, blue uint8) {
	r.Renderer.SetDrawColor(red, green, blue, 255)
	r.Renderer.FillRect(&sdl.Rect{X: x, Y: y, W: w, H: h})
}

func (r *Renderer) DrawText(text string, x, y int32, red, green, blue uint8) error {
	surface, err := r.Font.RenderUTF8Blended(text, sdl.Color{R: red, G: green, B: blue, A: 255})
	if err != nil {
		return err
	}
	defer surface.Free()

	texture, err := r.Renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return err
	}
	defer texture.Destroy()

	_, _, tw, th, _ := texture.Query()
	r.Renderer.Copy(texture, nil, &sdl.Rect{X: x, Y: y, W: tw, H: th})
	return nil
}

// TextSize returns the pixel width and height of text without drawing it.
func (r *Renderer) TextSize(text string) (int32, int32) {
	w, h, err := r.Font.SizeUTF8(text)
	if err != nil {
		return 0, 0
	}
	return int32(w), int32(h)
}

// DrawTextCentered draws text horizontally centered within a region [x, x+w].
func (r *Renderer) DrawTextCentered(text string, x, y, w int32, red, green, blue uint8) {
	tw, _ := r.TextSize(text)
	r.DrawText(text, x+(w-tw)/2, y, red, green, blue)
}

func (r *Renderer) DrawTextureAt(tex *sdl.Texture, x, y, w, h int32) {
	r.Renderer.Copy(tex, nil, &sdl.Rect{X: x, Y: y, W: w, H: h})
}

// SetClipRect sets the clipping rectangle for rendering.
func (r *Renderer) SetClipRect(x, y, w, h int32) {
	rect := sdl.Rect{X: x, Y: y, W: w, H: h}
	r.Renderer.SetClipRect(&rect)
}

// ClearClipRect removes any clipping rectangle.
func (r *Renderer) ClearClipRect() {
	r.Renderer.SetClipRect(nil)
}

// WrapText breaks text into lines that fit within maxWidth pixels.
func (r *Renderer) WrapText(text string, maxWidth int32) []string {
	var lines []string
	for _, paragraph := range splitLines(text) {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := splitWords(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			test := current + " " + word
			tw, _ := r.TextSize(test)
			if tw > maxWidth {
				lines = append(lines, current)
				current = word
			} else {
				current = test
			}
		}
		lines = append(lines, current)
	}
	return lines
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func splitWords(s string) []string {
	var words []string
	for _, w := range strings.Fields(s) {
		if w != "" {
			words = append(words, w)
		}
	}
	return words
}

// DrawWrappedText renders word-wrapped text starting at (x, y) within maxWidth,
// using lineH pixels between lines. Returns the total height used.
func (r *Renderer) DrawWrappedText(text string, x, y, maxWidth, lineH int32, red, green, blue uint8) int32 {
	lines := r.WrapText(text, maxWidth)
	for i, line := range lines {
		if line != "" {
			r.DrawText(line, x, y+int32(i)*lineH, red, green, blue)
		}
	}
	return int32(len(lines)) * lineH
}
