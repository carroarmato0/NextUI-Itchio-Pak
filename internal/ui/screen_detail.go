//go:build !headless

package ui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
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
	scrollY       int32 // vertical scroll offset for content area
	contentHeight int32 // total content height (computed during Draw)
	viewportH     int32 // visible content area height (set during Draw)
	advisoryTriggered bool // true when a filter match is found after loading

	// Held-button auto-repeat state for scrolling
	heldDir    int       // -1 = up, +1 = down, 0 = none
	heldSince  time.Time
	lastRepeat time.Time

	prev Screen
}

func NewDetailScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, prev Screen) *DetailScreen {
	s := &DetailScreen{client: client, cfg: cfg, cfgPath: cfgPath, cache: cache, game: game, prev: prev, loading: true}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in FetchGameDetail goroutine: %v", r)
				s.err = fmt.Errorf("internal error: %v", r)
				s.loading = false
			}
		}()
		log.Printf("fetching detail for: %s", game.URL)
		d, err := client.FetchGameDetail(game.URL)
		if err != nil {
			log.Printf("FetchGameDetail error: %v", err)
		} else {
			log.Printf("FetchGameDetail ok: %d screenshots", len(d.ScreenshotURLs))
			// Prepend cover art as the first image so it's shown by default
			if game.CoverURL != "" {
				d.ScreenshotURLs = append([]string{game.CoverURL}, d.ScreenshotURLs...)
			}
		}
		s.detail = d
		s.err = err
		if d != nil && err == nil {
			s.advisoryTriggered = itchio.IsAdvisoryTriggered(
				d.PageTags,
				itchio.FilterConfig{
					AdultContent: itchio.CategoryFilter{
						Enabled:  cfg.Filter.AdultContent.Enabled,
						Disabled: cfg.Filter.AdultContent.Disabled,
					},
					QueerContent: itchio.CategoryFilter{
						Enabled:  cfg.Filter.QueerContent.Enabled,
						Disabled: cfg.Filter.QueerContent.Disabled,
					},
					HeavyThemes: itchio.CategoryFilter{
						Enabled:  cfg.Filter.HeavyThemes.Enabled,
						Disabled: cfg.Filter.HeavyThemes.Disabled,
					},
					SubstanceUse: itchio.CategoryFilter{
						Enabled:  cfg.Filter.SubstanceUse.Enabled,
						Disabled: cfg.Filter.SubstanceUse.Disabled,
					},
				},
			)
		}
		s.loading = false  // publish last — renderer sees consistent state
	}()
	return s
}

func (s *DetailScreen) processAutoScroll() {
	if s.heldDir == 0 {
		return
	}
	now := time.Now()
	if now.Sub(s.heldSince) < repeatDelay {
		return
	}
	if now.Sub(s.lastRepeat) < repeatInterval {
		return
	}
	s.scrollY += int32(s.heldDir) * scrollStep
	s.clampScroll(s.viewportH)
	s.lastRepeat = now
}

func (s *DetailScreen) Draw(r *renderer.Renderer) {
	s.processAutoScroll()

	// ── Parental advisory overlay ────────────────────────────
	if !s.loading && s.err == nil && s.advisoryTriggered {
		s.drawAdvisoryOverlay(r)
		r.Present()
		return
	}

	r.Clear(colorBG, colorBG, colorBG)

	// ── Header ──────────────────────────────────────────────
	headerH := int32(56)
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)

	title := s.game.Title
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawText("by "+s.game.Author, 12, 32, 140, 140, 140)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)

	contentTop := headerH + 6
	footerH := int32(28)
	contentH := r.H - contentTop - footerH
	s.viewportH = contentH
	margin := int32(20)
	usableW := r.W - margin*2

	// ── Loading state ───────────────────────────────────────
	if s.loading {
		r.DrawText("Loading game details...", margin, contentTop+contentH/2-10, colorText, colorText, colorText)
		if s.game.CoverURL != "" {
			tex := s.cache.Get(r, s.game.CoverURL)
			if tex != nil {
				_, _, tw, th, _ := tex.Query()
				maxH := contentH - 40
				scaleW := float32(usableW) / float32(tw)
				scaleH := float32(maxH) / float32(th)
				scale := scaleW
				if scaleH < scaleW {
					scale = scaleH
				}
				dw := int32(float32(tw) * scale)
				dh := int32(float32(th) * scale)
				r.DrawTextureAt(tex, margin, contentTop+10, dw, dh)
			}
		}
		r.DrawText("B:back  |  Start:settings", 10, r.H-24, 140, 140, 140)
		r.Present()
		return
	}
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), margin, contentTop+20, 200, 50, 50)
		r.DrawText("B:back  |  Start:settings", 10, r.H-24, 140, 140, 140)
		r.Present()
		return
	}

	// ── Scrollable content ──────────────────────────────────
	r.SetClipRect(0, contentTop, r.W, contentH)

	// Virtual Y tracks layout position; actual drawing offset by scrollY
	y := contentTop - s.scrollY

	// ── Top row: screenshot (left) + QR code (right) ────────
	qrColW := r.W / 4
	imgAreaW := r.W - qrColW - margin - 10 // 10px gap between image and QR

	if s.detail != nil && len(s.detail.ScreenshotURLs) > 0 {
		ssURL := s.detail.ScreenshotURLs[s.screenshotIdx]
		tex := s.cache.Get(r, ssURL)

		imgBoxH := contentH * 2 / 3
		imgBoxW := imgAreaW - margin

		// Background box for screenshot
		r.DrawRect(margin, y, imgBoxW, imgBoxH, 30, 30, 30)

		if tex != nil {
			_, _, tw, th, _ := tex.Query()
			scaleW := float32(imgBoxW) / float32(tw)
			scaleH := float32(imgBoxH) / float32(th)
			scale := scaleW
			if scaleH < scaleW {
				scale = scaleH
			}
			dw := int32(float32(tw) * scale)
			dh := int32(float32(th) * scale)
			imgX := margin + (imgBoxW-dw)/2
			imgY := y + (imgBoxH-dh)/2
			r.DrawTextureAt(tex, imgX, imgY, dw, dh)
		} else if !s.cache.Failed(ssURL) {
			r.DrawText("Loading...", margin+imgBoxW/2-40, y+imgBoxH/2-10, 80, 80, 80)
		} else {
			r.DrawText("No Image", margin+imgBoxW/2-40, y+imgBoxH/2-10, 80, 80, 80)
		}

		// QR code in the right portion of the top row
		qrX := margin + imgBoxW + 10
		s.drawQR(r, qrX, y, qrColW, imgBoxH)

		y += imgBoxH + 6
		r.DrawText(fmt.Sprintf("Image %d/%d  (L/R)", s.screenshotIdx+1, len(s.detail.ScreenshotURLs)),
			margin, y, 140, 140, 140)
		y += 26
	} else {
		// No screenshots — just QR code
		qrBoxH := contentH / 3
		s.drawQR(r, margin, y, usableW, qrBoxH)
		y += qrBoxH + 10
	}

	// ── Action area (full width) ────────────────────────────
	if s.game.IsFree {
		r.DrawText("[ A: Download ]", margin, y, 80, 200, 80)
	} else if s.cfg.APIKey == "" {
		r.DrawText(fmt.Sprintf("$%.2f  Purchase required", s.game.Price), margin, y, 220, 180, 60)
	} else {
		r.DrawText(fmt.Sprintf("[ A: Download ]  $%.2f", s.game.Price), margin, y, 80, 200, 80)
	}
	y += 30

	// ── Tags ────────────────────────────────────────────────
	if s.detail != nil && len(s.detail.PageTags) > 0 {
		tagLine := "Tags: " + strings.Join(s.detail.PageTags, ", ")
		_, lineH := r.TextSize("Ag")
		if lineH < 20 {
			lineH = 20
		}
		tagsH := r.DrawWrappedText(tagLine, margin, y, usableW, lineH+2, 120, 180, 220)
		y += tagsH + 8
	}

	// ── Description (full width) ────────────────────────────
	if s.detail != nil && s.detail.Description != "" {
		y += 10
		r.DrawRect(margin, y, usableW, 1, 50, 50, 50) // separator
		y += 10

		_, lineH := r.TextSize("Ag")
		if lineH < 20 {
			lineH = 20
		}
		descH := r.DrawWrappedText(s.detail.Description, margin, y, usableW, lineH+4, 180, 180, 180)
		y += descH
	}

	// Track total content height for scroll clamping
	s.contentHeight = y - (contentTop - s.scrollY)

	r.ClearClipRect()

	// ── Footer ──────────────────────────────────────────────
	scrollHint := ""
	if s.contentHeight > contentH {
		scrollHint = "  |  Up/Down:scroll"
	}
	r.DrawText("B:back  |  L/R:screenshots  |  Start:settings"+scrollHint, 10, r.H-24, 140, 140, 140)
	r.Present()
}

// drawAdvisoryOverlay renders the full-screen content warning cover.
func (s *DetailScreen) drawAdvisoryOverlay(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	cy := r.H / 2

	r.DrawTextCentered("[!]", 0, cy-90, r.W, 240, 180, 60)
	r.DrawTextCentered("Content Warning", 0, cy-54, r.W, 240, 180, 60)

	r.DrawRect(r.W/4, cy-28, r.W/2, 1, 60, 60, 60)

	_, lh := r.TextSize("Ag")
	if lh < 20 {
		lh = 20
	}
	r.DrawWrappedText(
		"This game contains content matched by one of your active filters.",
		r.W/8, cy-16, r.W*3/4, lh+2, 180, 180, 180,
	)
	r.DrawWrappedText(
		"You can adjust your filters in Settings.",
		r.W/8, cy-16+lh+6, r.W*3/4, lh+2, 180, 180, 180,
	)

	r.DrawRect(r.W/4, cy+60, r.W/2, 1, 60, 60, 60)
	r.DrawTextCentered("B  Go back", 0, cy+72, r.W, 180, 80, 80)
}

// drawQR renders the QR code centered within the given box.
func (s *DetailScreen) drawQR(r *renderer.Renderer, x, y, w, h int32) {
	qrSize := int(w - 20)
	if qrSize > int(h-40) {
		qrSize = int(h - 40)
	}
	if qrSize < 80 {
		qrSize = 80
	}
	if qrSize > 512 {
		qrSize = 512
	}
	qrS := int32(qrSize)

	tex, err := r.QRTexture(s.game.URL, qrSize)
	if err == nil && tex != nil {
		qrX := x + (w-qrS)/2
		qrY := y + (h-qrS)/2 - 16
		r.DrawTextureAt(tex, qrX, qrY, qrS, qrS)
		tex.Destroy()
		r.DrawTextCentered("Scan to open", x, qrY+qrS+4, w, 120, 120, 120)
		r.DrawTextCentered("in browser", x, qrY+qrS+22, w, 120, 120, 120)
	}
}

const scrollStep = 30

func (s *DetailScreen) clampScroll(contentH int32) {
	maxScroll := s.contentHeight - contentH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if s.scrollY > maxScroll {
		s.scrollY = maxScroll
	}
	if s.scrollY < 0 {
		s.scrollY = 0
	}
}

func (s *DetailScreen) startScrollHold(dir int) {
	s.scrollY += int32(dir) * scrollStep
	s.clampScroll(s.viewportH)
	s.heldDir = dir
	s.heldSince = time.Now()
	s.lastRepeat = s.heldSince
}

func (s *DetailScreen) stopScrollHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *DetailScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		switch ev.Keysym.Sym {
		case sdl.K_UP:
			if ev.Type == sdl.KEYDOWN {
				s.startScrollHold(-1)
			} else {
				s.stopScrollHold(-1)
			}
			return s
		case sdl.K_DOWN:
			if ev.Type == sdl.KEYDOWN {
				s.startScrollHold(1)
			} else {
				s.stopScrollHold(1)
			}
			return s
		}
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
			if !s.advisoryTriggered {
				return s.startDownload()
			}
		case sdl.K_s:
			return NewSettingsScreen(s.cfg, s.cfgPath, s)
		}
	case *sdl.ControllerButtonEvent:
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startScrollHold(-1)
			} else {
				s.stopScrollHold(-1)
			}
			return s
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startScrollHold(1)
			} else {
				s.stopScrollHold(1)
			}
			return s
		}
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			if s.detail != nil && s.screenshotIdx > 0 {
				s.screenshotIdx--
			}
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			if s.detail != nil && s.screenshotIdx < len(s.detail.ScreenshotURLs)-1 {
				s.screenshotIdx++
			}
		case sdl.CONTROLLER_BUTTON_B:
			if !s.advisoryTriggered {
				return s.startDownload()
			}
		case sdl.CONTROLLER_BUTTON_START:
			return NewSettingsScreen(s.cfg, s.cfgPath, s)
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}


func (s *DetailScreen) startDownload() Screen {
	if s.loading {
		return s
	}
	if !s.game.IsFree && s.cfg.APIKey == "" {
		return s
	}
	return NewFetchUploadsScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s)
}
