//go:build !headless

package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type DetailScreen struct {
	client        *itchio.Client
	cfg           *settings.Config
	cfgPath       string
	cache         *renderer.ImageCache
	game          itchio.Game
	detail        *itchio.GameDetail
	loading       bool
	err           error
	screenshotIdx int
	prev          Screen
}

func NewDetailScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, prev Screen) *DetailScreen {
	s := &DetailScreen{client: client, cfg: cfg, cfgPath: cfgPath, cache: cache, game: game, prev: prev, loading: true}
	go func() {
		d, err := client.FetchGameDetail(game.URL)
		s.detail = d
		s.err = err
		s.loading = false
	}()
	return s
}

func (s *DetailScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText(s.game.Title, 20, 20, colorText, colorText, colorText)
	r.DrawText("by "+s.game.Author, 20, 50, 160, 160, 160)

	if s.loading {
		r.DrawText("Loading...", 20, 100, colorText, colorText, colorText)
		r.Present()
		return
	}
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), 20, 100, 200, 50, 50)
		s.drawQR(r)
		r.Present()
		return
	}

	y := int32(80)

	// Screenshots
	if s.detail != nil && len(s.detail.ScreenshotURLs) > 0 {
		ssURL := s.detail.ScreenshotURLs[s.screenshotIdx]
		tex := s.cache.Get(r, ssURL)
		if tex != nil {
			_, _, tw, th, _ := tex.Query()
			dispW := r.W * 60 / 100
			scale := float32(dispW) / float32(tw)
			dh := int32(float32(th) * scale)
			r.DrawTextureAt(tex, 20, y, dispW, dh)
			y += dh + 10
		}
		r.DrawText(fmt.Sprintf("Screenshot %d/%d  (L/R)", s.screenshotIdx+1, len(s.detail.ScreenshotURLs)),
			20, y, 140, 140, 140)
		y += 30
	}

	// Action area
	if s.game.IsFree {
		r.DrawText("[ A: Download ]", 20, y, 80, 200, 80)
	} else if s.cfg.APIKey == "" {
		r.DrawText(fmt.Sprintf("$%.2f  Purchase required", s.game.Price), 20, y, 220, 180, 60)
		y += 30
		r.DrawText("[ + : Open Settings to add API Key ]", 20, y, 160, 160, 160)
	} else {
		r.DrawText(fmt.Sprintf("[ A: Download ]  $%.2f", s.game.Price), 20, y, 80, 200, 80)
	}
	y += 40

	r.DrawText("Store page QR:", 20, y, 160, 160, 160)
	s.drawQR(r)

	r.DrawText("B: back  |  L/R: screenshots  |  Start: settings", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *DetailScreen) drawQR(r *renderer.Renderer) {
	tex, err := r.QRTexture(s.game.URL, 128)
	if err == nil && tex != nil {
		r.DrawTextureAt(tex, r.W-148, 80, 128, 128)
		tex.Destroy()
	}
}

func (s *DetailScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_ESCAPE:
			return s.prev
		case sdl.K_LEFT:
			if s.detail != nil && s.screenshotIdx > 0 {
				s.screenshotIdx--
			}
		case sdl.K_RIGHT:
			if s.detail != nil && s.screenshotIdx < len(s.detail.ScreenshotURLs)-1 {
				s.screenshotIdx++
			}
		case sdl.K_RETURN:
			return s.startDownload()
		case sdl.K_s:
			return NewSettingsScreen(s.cfg, s.cfgPath, s)
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_B:
			return s.prev
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			if s.detail != nil && s.screenshotIdx > 0 {
				s.screenshotIdx--
			}
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			if s.detail != nil && s.screenshotIdx < len(s.detail.ScreenshotURLs)-1 {
				s.screenshotIdx++
			}
		case sdl.CONTROLLER_BUTTON_A:
			return s.startDownload()
		case sdl.CONTROLLER_BUTTON_START:
			return NewSettingsScreen(s.cfg, s.cfgPath, s)
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *DetailScreen) startDownload() Screen {
	if s.detail == nil || s.loading {
		return s
	}
	if !s.game.IsFree && s.cfg.APIKey == "" {
		return s
	}

	var uploads []roms.Upload
	for _, u := range s.detail.Uploads {
		uploads = append(uploads, roms.Upload{Filename: u.Filename, URL: u.URL})
	}

	if s.cfg.ROMSelection == "ask" && len(uploads) > 1 {
		return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, uploads, s)
	}

	selected := roms.SelectBest(uploads)
	if selected == nil {
		s.err = fmt.Errorf("no .gb or .gbc files found for this game")
		return s
	}
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, *selected, s)
}
