//go:build !headless

package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	colorBG        = uint8(20)
	colorHighlight = uint8(60)
	colorText      = uint8(220)
)

type ListScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cache   *renderer.ImageCache
	games   []itchio.Game
	cursor  int
	page    int
	loading bool
	err     error
	cfgPath string
}

func NewListScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache) *ListScreen {
	s := &ListScreen{client: client, cfg: cfg, cache: cache, page: 1, cfgPath: cfgPath}
	go s.loadPage(1, "")
	return s
}

func (s *ListScreen) loadPage(page int, query string) {
	s.loading = true
	s.err = nil
	games, err := s.client.FetchGames(page, query)
	s.games = games
	s.err = err
	s.cursor = 0
	s.loading = false
}

func (s *ListScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	if s.loading {
		r.DrawText("Loading...", 20, r.H/2, colorText, colorText, colorText)
		r.Present()
		return
	}
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), 20, r.H/2, 200, 50, 50)
		r.Present()
		return
	}

	leftW := r.W * 55 / 100
	rightX := leftW + 10
	rightW := r.W - rightX - 10

	rowH := int32(32)
	visibleRows := (r.H - 60) / rowH

	startIdx := 0
	if s.cursor >= int(visibleRows) {
		startIdx = s.cursor - int(visibleRows) + 1
	}

	for i, g := range s.games {
		if i < startIdx {
			continue
		}
		rowIdx := i - startIdx
		if int32(rowIdx) >= visibleRows {
			break
		}
		y := int32(20) + int32(rowIdx)*rowH
		if i == s.cursor {
			r.DrawRect(0, y-2, leftW, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}
		label := g.Title
		if len(label) > 40 {
			label = label[:37] + "..."
		}
		r.DrawText(label, 10, y, colorText, colorText, colorText)
		if g.IsFree {
			r.DrawText("free", leftW-60, y, 80, 200, 80)
		} else {
			r.DrawText(fmt.Sprintf("$%.2f", g.Price), leftW-70, y, 220, 180, 60)
		}
	}

	// Cover art in right panel
	if s.cursor < len(s.games) && s.games[s.cursor].CoverURL != "" {
		tex := s.cache.Get(r, s.games[s.cursor].CoverURL)
		if tex != nil {
			_, _, tw, th, _ := tex.Query()
			scale := float32(rightW) / float32(tw)
			dh := int32(float32(th) * scale)
			r.DrawTextureAt(tex, rightX, 20, rightW, dh)
		}
	}

	footer := fmt.Sprintf("Page %d · %d games  |  D-pad navigate · A select · Y search · Start settings",
		s.page, len(s.games))
	r.DrawText(footer, 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *ListScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.games)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_PAGEDOWN:
			s.page++
			go s.loadPage(s.page, "")
		case sdl.K_PAGEUP:
			if s.page > 1 {
				s.page--
				go s.loadPage(s.page, "")
			}
		case sdl.K_RETURN:
			if s.cursor < len(s.games) {
				return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.games[s.cursor], s)
			}
		case sdl.K_s:
			return NewSettingsScreen(s.cfg, s.cfgPath, s)
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.games)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			s.page++
			go s.loadPage(s.page, "")
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			if s.page > 1 {
				s.page--
				go s.loadPage(s.page, "")
			}
		case sdl.CONTROLLER_BUTTON_A:
			if s.cursor < len(s.games) {
				return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.games[s.cursor], s)
			}
		case sdl.CONTROLLER_BUTTON_START:
			return NewSettingsScreen(s.cfg, s.cfgPath, s)
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}
