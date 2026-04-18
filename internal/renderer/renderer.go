//go:build !headless

package renderer

import (
	"fmt"

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

	font, err := ttf.OpenFont("assets/font.ttf", 18)
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

func (r *Renderer) DrawTextureAt(tex *sdl.Texture, x, y, w, h int32) {
	r.Renderer.Copy(tex, nil, &sdl.Rect{X: x, Y: y, W: w, H: h})
}
