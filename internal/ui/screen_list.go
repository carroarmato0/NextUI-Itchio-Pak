//go:build !headless

package ui

import (
	"fmt"
	"log"
	"time"

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

// Auto-repeat timing for held D-pad buttons
const (
	repeatDelay    = 400 * time.Millisecond // initial delay before repeating
	repeatInterval = 80 * time.Millisecond  // interval between repeats
)

type ListScreen struct {
	client     *itchio.Client
	cfg        *settings.Config
	cache      *renderer.ImageCache
	games      []itchio.Game
	cursor     int
	page       int
	loading    bool
	err        error
	cfgPath    string
	totalGames int // 0 = not yet known
	totalPages int // 0 = not yet known

	// Held-button auto-repeat state
	heldDir    int       // -1 = up, +1 = down, 0 = none
	heldSince  time.Time // when the button was first pressed
	lastRepeat time.Time // when we last advanced the cursor
}

func NewListScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache) *ListScreen {
	s := &ListScreen{client: client, cfg: cfg, cache: cache, page: 1, cfgPath: cfgPath}
	go s.loadPage(1, "")
	go func() {
		total, err := client.FetchTotalGames()
		if err != nil {
			log.Printf("FetchTotalGames: %v", err)
			return
		}
		log.Printf("total games: %d", total)
		s.totalGames = total
		s.totalPages = (total + itchio.PerPage - 1) / itchio.PerPage
	}()
	return s
}

func (s *ListScreen) loadPage(page int, query string) {
	s.loading = true
	s.err = nil
	log.Printf("loadPage: fetching page %d query=%q", page, query)
	games, err := s.client.FetchGames(page, query)
	if err != nil {
		log.Printf("loadPage: page %d error: %v", page, err)
	} else {
		log.Printf("loadPage: page %d returned %d games", page, len(games))
	}
	s.games = games
	s.err = err
	s.cursor = 0
	s.loading = false
}

func (s *ListScreen) processAutoRepeat() {
	if s.heldDir == 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(s.heldSince)
	if elapsed < repeatDelay {
		return
	}
	if now.Sub(s.lastRepeat) < repeatInterval {
		return
	}
	s.moveCursor(s.heldDir)
	s.lastRepeat = now
}

func (s *ListScreen) moveCursor(dir int) {
	if dir > 0 && s.cursor < len(s.games)-1 {
		s.cursor++
	} else if dir < 0 && s.cursor > 0 {
		s.cursor--
	}
}

func (s *ListScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	r.Clear(colorBG, colorBG, colorBG)

	// Header
	headerH := int32(56)
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawText("Itch.io — GB Studio Games", 12, 14, colorText, colorText, colorText)
	// Thin separator line below header
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)

	contentTop := headerH + 4

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

	leftW := r.W * 52 / 100
	rightX := leftW + 24
	rightW := r.W - rightX - 10

	rowH := int32(32)
	footerH := int32(28)
	visibleRows := (r.H - contentTop - footerH) / rowH

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
		y := contentTop + int32(rowIdx)*rowH
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

	// Right panel: cover art (or placeholder) + metadata
	if s.cursor < len(s.games) {
		g := s.games[s.cursor]
		metaY := contentTop
		boxW := rightW
		boxH := rightW * 3 / 4 // 4:3 aspect ratio box

		// Draw the box background for all states
		r.DrawRect(rightX, metaY, boxW, boxH, 30, 30, 30)

		if g.CoverURL != "" {
			tex := s.cache.Get(r, g.CoverURL)
			if tex != nil {
				_, _, tw, th, _ := tex.Query()
				// Fit image within box, maintaining aspect ratio
				scaleW := float32(boxW) / float32(tw)
				scaleH := float32(boxH) / float32(th)
				scale := scaleW
				if scaleH < scaleW {
					scale = scaleH
				}
				dw := int32(float32(tw) * scale)
				dh := int32(float32(th) * scale)
				// Center within box
				imgX := rightX + (boxW-dw)/2
				imgY := metaY + (boxH-dh)/2
				r.DrawTextureAt(tex, imgX, imgY, dw, dh)
			} else if s.cache.Failed(g.CoverURL) {
				r.DrawText("No Image", rightX+boxW/2-40, metaY+boxH/2-10, 80, 80, 80)
			} else {
				r.DrawText("Loading...", rightX+boxW/2-40, metaY+boxH/2-10, 80, 80, 80)
			}
		} else {
			// No cover URL — wireframe border
			r.DrawRect(rightX+2, metaY+2, boxW-4, boxH-4, colorBG, colorBG, colorBG)
			r.DrawRect(rightX+3, metaY+3, boxW-6, boxH-6, 35, 35, 35)
			r.DrawText("No Image", rightX+boxW/2-40, metaY+boxH/2-10, 80, 80, 80)
		}
		metaY += boxH + 12

		if g.Author != "" {
			r.DrawText("by "+g.Author, rightX, metaY, 160, 160, 160)
			metaY += 26
		}
		for _, tag := range g.Tags {
			r.DrawText(tag, rightX, metaY, 120, 180, 220)
			metaY += 22
		}
	}

	// Footer with pagination info
	var pageInfo string
	if s.totalPages > 0 {
		pageInfo = fmt.Sprintf("Page %d/%d", s.page, s.totalPages)
	} else {
		pageInfo = fmt.Sprintf("Page %d", s.page)
	}
	var countInfo string
	if s.totalGames > 0 {
		countInfo = fmt.Sprintf("%d/%d games", len(s.games), s.totalGames)
	} else {
		countInfo = fmt.Sprintf("%d games", len(s.games))
	}
	footer := fmt.Sprintf("%s · %s  |  A:select  L/R:page  B:exit  Start:settings", pageInfo, countInfo)
	r.DrawText(footer, 10, r.H-24, 140, 140, 140)
	r.Present()
}

// drawPlaceholder renders a bordered rectangle with centered text.
func (s *ListScreen) drawPlaceholder(r *renderer.Renderer, x, y, w, h int32, label string) {
	r.DrawRect(x, y, w, h, 45, 45, 45)
	r.DrawRect(x+2, y+2, w-4, h-4, colorBG, colorBG, colorBG)
	r.DrawText(label, x+w/2-40, y+h/2-10, 80, 80, 80)
}

func (s *ListScreen) startHold(dir int) {
	s.moveCursor(dir)
	s.heldDir = dir
	s.heldSince = time.Now()
	s.lastRepeat = s.heldSince
}

func (s *ListScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *ListScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if ev.Type == sdl.KEYDOWN {
				s.startHold(1)
			} else {
				s.stopHold(1)
			}
			return s
		case sdl.K_UP:
			if ev.Type == sdl.KEYDOWN {
				s.startHold(-1)
			} else {
				s.stopHold(-1)
			}
			return s
		}
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_ESCAPE:
			return nil
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
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(1)
			} else {
				s.stopHold(1)
			}
			return s
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(-1)
			} else {
				s.stopHold(-1)
			}
			return s
		}
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			s.page++
			go s.loadPage(s.page, "")
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			if s.page > 1 {
				s.page--
				go s.loadPage(s.page, "")
			}
		case sdl.CONTROLLER_BUTTON_B:
			if s.cursor < len(s.games) {
				return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.games[s.cursor], s)
			}
		case sdl.CONTROLLER_BUTTON_A:
			return nil
		case sdl.CONTROLLER_BUTTON_START:
			return NewSettingsScreen(s.cfg, s.cfgPath, s)
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}
