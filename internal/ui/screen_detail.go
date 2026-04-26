//go:build !headless

package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type detailModal struct {
	active bool
	title  string
	body   string
}

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
	modal         detailModal

	// Held-button auto-repeat state for scrolling
	heldDir    int       // -1 = up, +1 = down, 0 = none
	heldSince  time.Time
	lastRepeat time.Time

	prev Screen
}

// ShowModal displays a dismissable overlay message on the detail screen.
// Called by FetchUploadsScreen when it detects a "not owned" condition so the
// error appears as a popup on top of the game page rather than a separate screen.
func (s *DetailScreen) ShowModal(title, body string) {
	s.modal = detailModal{active: true, title: title, body: body}
}

func NewDetailScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, prev Screen) *DetailScreen {
	s := &DetailScreen{client: client, cfg: cfg, cfgPath: cfgPath, cache: cache, game: game, prev: prev, loading: true}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("detail: PANIC in FetchGameDetail goroutine: %v", r)
				s.err = fmt.Errorf("internal error: %v", r)
				s.loading = false
			}
		}()
		logger.Debug("detail: fetching %s", game.URL)
		d, err := client.FetchGameDetail(game.URL)
		if err != nil {
			logger.Error("detail: FetchGameDetail: %v", err)
		} else {
			logger.Debug("detail: %d screenshots", len(d.ScreenshotURLs))
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
	// Draw the modal overlay (if active) and call Present exactly once,
	// regardless of which branch below renders the underlying content.
	defer func() {
		if s.modal.active {
			s.drawModal(r)
		}
		r.Present()
	}()

	s.processAutoScroll()

	// ── Parental advisory overlay ────────────────────────────
	if !s.loading && s.err == nil && s.advisoryTriggered {
		s.drawAdvisoryOverlay(r)
		return
	}

	r.Clear(colorBG, colorBG, colorBG)

	// ── Header ──────────────────────────────────────────────
	// Two-line header: main title (large font) + "by author" (small font).
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16 // 8px top + 4px gap + 4px bottom
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)

	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, 140, 140, 140)

	contentTop := headerH + 6
	footerH := int32(40)
	contentH := r.H - contentTop - footerH
	s.viewportH = contentH
	margin := int32(20)
	usableW := r.W - margin*2

	// ── Loading state ───────────────────────────────────────
	// Use the same column geometry as the loaded layout so the image
	// doesn't jump in size once the QR code and detail data arrive.
	qrColW := r.W / 4
	imgAreaW := r.W - qrColW - margin - 10
	imgBoxW := imgAreaW - margin
	imgBoxH := contentH * 2 / 3

	if s.loading {
		r.DrawRect(margin, contentTop, imgBoxW, imgBoxH, colorBG, colorBG, colorBG)
		if s.game.CoverURL != "" {
			tex := s.cache.Get(r, s.game.CoverURL)
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
				imgY := contentTop + (imgBoxH-dh)/2
				r.DrawTextureAt(tex, imgX, imgY, dw, dh)
			}
		}
		// QR placeholder so the right column is visible while loading
		r.DrawRect(margin+imgBoxW+10, contentTop, qrColW, imgBoxH, colorBG, colorBG, colorBG)
		r.DrawText("Loading...", margin, contentTop+imgBoxH+16, colorText, colorText, colorText)
		ftrY := r.DrawFooterBar(footerH)
		r.DrawSmallText("B:back  |  Start:settings", 10, ftrY, 140, 140, 140)
		return
	}
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), margin, contentTop+20, 200, 50, 50)
		ftrY := r.DrawFooterBar(footerH)
		r.DrawSmallText("B:back  |  Start:settings", 10, ftrY, 140, 140, 140)
		return
	}

	// ── Scrollable content ──────────────────────────────────
	r.SetClipRect(0, contentTop, r.W, contentH)

	// Virtual Y tracks layout position; actual drawing offset by scrollY
	y := contentTop - s.scrollY
	_, fontH := r.TextSize("Ag")

	// ── Top row: screenshot (left) + QR code (right) ────────
	// qrColW, imgAreaW, imgBoxW, imgBoxH already declared above (shared with loading state)

	if s.detail != nil && len(s.detail.ScreenshotURLs) > 0 {
		ssURL := s.detail.ScreenshotURLs[s.screenshotIdx]
		tex := s.cache.Get(r, ssURL)

		// Background box for screenshot
		r.DrawRect(margin, y, imgBoxW, imgBoxH, colorBG, colorBG, colorBG)

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
		y += fontH + 6
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
	y += fontH + 8

	// ── Tags ────────────────────────────────────────────────
	if s.detail != nil && len(s.detail.PageTags) > 0 {
		tagLine := "Tags: " + strings.Join(s.detail.PageTags, ", ")
		tagsH := r.DrawWrappedText(tagLine, margin, y, usableW, fontH+4, 120, 180, 220)
		y += tagsH + 8
	}

	// ── Description (full width) ────────────────────────────
	if s.detail != nil && s.detail.Description != "" {
		y += 10
		r.DrawRect(margin, y, usableW, 1, 50, 50, 50) // separator
		y += 10
		descH := r.DrawWrappedText(s.detail.Description, margin, y, usableW, fontH+4, 180, 180, 180)
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
	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText("B:back  |  L/R:screenshots  |  Start:settings"+scrollHint, 10, ftrY, 140, 140, 140)
}

// drawModal renders a centered popup overlay with a title, body, and dismiss hint.
func (s *DetailScreen) drawModal(r *renderer.Renderer) {
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	pad := int32(16)
	lineH := smallFH + 4
	bodyLines := int32(4) // generous for typical "not owned" messages
	boxW := r.W * 2 / 3
	boxH := pad + fontH + pad + 2 + pad + lineH*bodyLines + pad + 2 + pad + smallFH + pad
	boxX := (r.W - boxW) / 2
	boxY := (r.H - boxH) / 2

	// Dark background covers underlying content
	r.DrawRect(0, 0, r.W, r.H, 10, 10, 15)

	// Border + box background
	r.DrawRect(boxX-1, boxY-1, boxW+2, boxH+2, 70, 70, 70)
	r.DrawRect(boxX, boxY, boxW, boxH, 25, 25, 35)

	y := boxY + pad

	r.DrawTextCentered(s.modal.title, boxX, y, boxW, 240, 180, 60)
	y += fontH + pad

	r.DrawRect(boxX+pad, y, boxW-pad*2, 1, 60, 60, 60)
	y += 1 + pad

	r.DrawWrappedText(s.modal.body, boxX+pad, y, boxW-pad*2, lineH, 200, 200, 200)
	y += lineH*bodyLines + pad

	r.DrawRect(boxX+pad, y, boxW-pad*2, 1, 60, 60, 60)
	y += 1 + pad

	r.DrawSmallTextCentered("Press any button to dismiss", boxX, y, boxW, 120, 120, 120)
}

// drawAdvisoryOverlay renders the full-screen content warning cover.
func (s *DetailScreen) drawAdvisoryOverlay(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	const pad = 8
	// Block: [!] + gap + "Content Warning" + gap + divider + gap + body1 + gap + body2 + gap + divider + gap + "B Go back"
	blockH := fontH + pad + fontH + pad + 1 + pad + smallFH + pad + smallFH + pad + 1 + pad + fontH
	y := (r.H - blockH) / 2

	r.DrawTextCentered("[!]", 0, y, r.W, 240, 180, 60)
	y += fontH + pad

	r.DrawTextCentered("Content Warning", 0, y, r.W, 240, 180, 60)
	y += fontH + pad

	r.DrawRect(r.W/4, y, r.W/2, 1, 60, 60, 60)
	y += 1 + pad

	r.DrawSmallTextCentered("This game contains content matched by one of your active filters.", 0, y, r.W, 180, 180, 180)
	y += smallFH + pad

	r.DrawSmallTextCentered("You can adjust your filters in Settings.", 0, y, r.W, 180, 180, 180)
	y += smallFH + pad

	r.DrawRect(r.W/4, y, r.W/2, 1, 60, 60, 60)
	y += 1 + pad

	r.DrawTextCentered("B  Go back", 0, y, r.W, 180, 80, 80)

	footerH := int32(40)
	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText("B:back  |  Start:settings", 10, ftrY, 140, 140, 140)
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
		_, smallFH := r.SmallTextSize("Ag")
		qrX := x + (w-qrS)/2
		qrY := y + (h-qrS)/2 - smallFH - 4
		r.DrawTextureAt(tex, qrX, qrY, qrS, qrS)
		tex.Destroy()
		r.DrawSmallTextCentered("Scan to open", x, qrY+qrS+4, w, 120, 120, 120)
		r.DrawSmallTextCentered("in browser", x, qrY+qrS+4+smallFH+2, w, 120, 120, 120)
	}
}

const scrollStep = 15

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
	// While the modal is visible, consume all input and dismiss on any button press.
	if s.modal.active {
		switch ev := e.(type) {
		case *sdl.KeyboardEvent:
			if ev.Type == sdl.KEYDOWN {
				s.modal.active = false
			}
		case *sdl.ControllerButtonEvent:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.modal.active = false
			}
		case *sdl.QuitEvent:
			return nil
		}
		return s
	}

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
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, nil)
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
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, nil)
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
