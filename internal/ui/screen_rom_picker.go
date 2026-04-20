//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type ROMPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	uploads []roms.Upload
	cursor  int
	prev    Screen
}

func NewROMPickerScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail, uploads []roms.Upload, prev Screen) *ROMPickerScreen {
	return &ROMPickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, uploads: uploads, prev: prev,
	}
}

func (s *ROMPickerScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Select ROM file to download:", 20, 20, colorText, colorText, colorText)

	for i, u := range s.uploads {
		y := int32(70 + i*40)
		if i == s.cursor {
			r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(u.Filename, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("A: select · B: back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *ROMPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			if s.cursor < len(s.uploads) {
				return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.uploads[s.cursor], s.prev)
			}
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			if s.cursor < len(s.uploads) {
				return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.uploads[s.cursor], s.prev)
			}
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}
