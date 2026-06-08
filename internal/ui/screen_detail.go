//go:build !headless

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	pathScrollDelay = time.Second
	pathScrollSpeed = int32(50)
)

type modalKind int

const (
	modalKindInfo          modalKind = iota
	modalKindDeleteConfirm           // A: confirm delete, B: cancel
)

type detailModal struct {
	active    bool
	kind      modalKind
	title     string
	body      string
	onConfirm func()
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
	qrTex         *sdl.Texture // cached QR texture; generated once, destroyed on back

	// Held-button auto-repeat state for scrolling
	heldDir       int       // -1 = up, +1 = down, 0 = none
	heldSince     time.Time
	lastRepeat    time.Time

	// Horizontal scroll for the on-device file path in the status card
	pathScrollX   int32
	pathScrollAt  time.Time

	// Pre-formatted strings cached once when detail/game data is available.
	screenshotLabels []string // "Image N/M  (←→)" for each screenshot index
	formattedPrice   string   // "$X.XX" for s.game.Price

	prev          Screen
	inv           *inventory.Inventory
	inventoryPath string
	updateSvc     UpdateServicer

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	onThemeToggle  func(bool)
}

// ShowModal displays a dismissable overlay message on the detail screen.
// Called by FetchUploadsScreen when it detects a "not owned" condition so the
// error appears as a popup on top of the game page rather than a separate screen.
func (s *DetailScreen) ShowModal(title, body string) {
	s.modal = detailModal{active: true, kind: modalKindInfo, title: title, body: body}
}

func NewDetailScreen(
	client *itchio.Client,
	cfg *settings.Config,
	cfgPath string,
	cache *renderer.ImageCache,
	game itchio.Game,
	inv *inventory.Inventory,
	inventoryPath string,
	prev Screen,
	updateSvc UpdateServicer,
	nextUITheme theme.Theme,
	defaultTheme theme.Theme,
	themeAvailable bool,
	onThemeToggle func(bool),
) *DetailScreen {
	s := &DetailScreen{
		client:         client,
		cfg:            cfg,
		cfgPath:        cfgPath,
		cache:          cache,
		game:           game,
		prev:           prev,
		loading:        true,
		inv:            inv,
		inventoryPath:  inventoryPath,
		updateSvc:      updateSvc,
		pathScrollAt:   time.Now(),
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		onThemeToggle:  onThemeToggle,
		formattedPrice: "$" + strconv.FormatFloat(game.Price, 'f', 2, 64),
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("detail: PANIC in FetchGameDetail goroutine: %v", r)
				s.err = fmt.Errorf("internal error: %v", r)
				s.loading = false
				sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
			}
		}()
		logger.Debug("detail: fetching %s", game.URL)
		d, err := client.FetchGameDetail(game.URL)
		if err != nil {
			logger.Error("detail: FetchGameDetail: %v", err)
		} else {
			logger.Info("detail: %d screenshots", len(d.ScreenshotURLs))
			// Prepend cover art as the first image so it's shown by default
			if game.CoverURL != "" {
				d.ScreenshotURLs = append([]string{game.CoverURL}, d.ScreenshotURLs...)
			}
			// Warm all non-GIF screenshots immediately. When there are multiple
			// GIF screenshots, only warm the first one — concurrent large animated
			// GIF decodes exhaust GPU memory on constrained hardware. The rest are
			// warmed lazily during Draw() as the user navigates.
			gifCount := 0
			for _, u := range d.ScreenshotURLs {
				if strings.EqualFold(filepath.Ext(u), ".gif") {
					gifCount++
				}
			}
			gifsWarmed := 0
			for _, u := range d.ScreenshotURLs {
				if strings.EqualFold(filepath.Ext(u), ".gif") {
					if gifCount > 1 && gifsWarmed > 0 {
						continue // deferred to Draw()
					}
					gifsWarmed++
				}
				cache.Warm(u)
			}
			logger.Debug("detail: warming %d screenshot URLs (%d GIF(s) deferred)",
				len(d.ScreenshotURLs)-max(0, gifCount-1), max(0, gifCount-1))
		}
		s.detail = d
		s.err = err
		if d != nil {
			total := strconv.Itoa(len(d.ScreenshotURLs))
			labels := make([]string, len(d.ScreenshotURLs))
			for i := range d.ScreenshotURLs {
				labels[i] = "Image " + strconv.Itoa(i+1) + "/" + total + "  (←→)"
			}
			s.screenshotLabels = labels
		}
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
		s.loading = false // publish last — renderer sees consistent state
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

func (s *DetailScreen) processAutoScroll() {
	if s.heldDir == 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(s.heldSince)
	if elapsed < repeatDelay {
		return
	}
	if now.Sub(s.lastRepeat) < currentRepeatInterval(elapsed-repeatDelay) {
		return
	}
	s.scrollY += int32(s.heldDir) * s.scrollStep()
	s.clampScroll(s.viewportH)
	s.lastRepeat = now
}

func (s *DetailScreen) NeedsRedraw() bool {
	if s.heldDir != 0 {
		return true
	}
	// Resume rendering 500ms before pathScrollDelay expires so the first
	// animation frame is not missed when the cursor has been stationary.
	return !s.pathScrollAt.IsZero() &&
		time.Since(s.pathScrollAt) > pathScrollDelay/2
}

func (s *DetailScreen) HasPendingAnimation() bool { return false }

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

	// Advance horizontal scroll for the on-device file path (1s pause then 50px/s).
	if !s.pathScrollAt.IsZero() {
		elapsed := time.Since(s.pathScrollAt)
		if elapsed > pathScrollDelay {
			s.pathScrollX = int32((elapsed - pathScrollDelay).Seconds() * float64(pathScrollSpeed))
		}
	}

	// ── Parental advisory overlay ────────────────────────────
	if !s.loading && s.err == nil && s.advisoryTriggered {
		s.drawAdvisoryOverlay(r)
		return
	}

	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	// ── Header ──────────────────────────────────────────────
	// Two-line header: main title (large font) + "by author" (small font).
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16 // 8px top + 4px gap + 4px bottom
	hBG := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])

	mt := r.Theme.MainText
	ht := r.Theme.HintText

	// Right-side badges — platform and download status.
	// Compute badge positions right-to-left so they never overlap with title.
	badgeRightEdge := r.W - 10
	badgePillH := smallFH + 4

	if s.game.Platform != "" {
		platLabel := s.game.Platform
		pw, _ := r.SmallTextSize(platLabel)
		platPillW := pw + 12
		platPillX := badgeRightEdge - platPillW
		platPillY := (headerH - badgePillH) / 2
		r.DrawPill(platPillX, platPillY, platPillW, badgePillH, 35, 45, 65)
		r.DrawSmallTextCenteredInRect(platLabel, platPillX, platPillY, platPillW, badgePillH, 130, 170, 210)
		badgeRightEdge = platPillX - 6
	}

	if s.detail != nil && s.inv.IsPresent(s.game.URL) {
		dlLabel := "● Downloaded"
		if r.W <= narrowScreenW {
			dlLabel = "● DL"
		}
		dw, _ := r.SmallTextSize(dlLabel)
		dlPillW := dw + 12
		dlPillX := badgeRightEdge - dlPillW
		dlPillY := (headerH - badgePillH) / 2
		r.DrawPill(dlPillX, dlPillY, dlPillW, badgePillH, 25, 50, 30)
		r.DrawSmallTextCenteredInRect(dlLabel, dlPillX, dlPillY, dlPillW, badgePillH, 70, 190, 90)
		badgeRightEdge = dlPillX - 6
	}

	// Title truncated to stay clear of badges.
	maxTitleW := badgeRightEdge - 16
	title := truncateToWidth(r, s.game.Title, maxTitleW)
	blockH := mainFH + 4 + smallFH
	titleY := (headerH - blockH) / 2
	r.DrawText(title, 12, titleY, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, titleY+mainFH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 6
	footerH := int32(52)
	contentH := r.H - contentTop - footerH
	s.viewportH = contentH
	margin := int32(20)
	usableW := r.W - margin*2

	// ── Loading state ───────────────────────────────────────
	// Use the same column geometry as the loaded layout so the image
	// doesn't jump in size once the QR code and detail data arrive.
	// QR column narrower than before — screenshot gets more horizontal space.
	qrColW := r.W / 5
	if r.W <= narrowScreenW {
		qrColW = r.W / 6
	}
	imgAreaW := r.W - qrColW - margin - 10
	imgBoxW := imgAreaW - margin
	imgBoxH := contentH * 2 / 3

	if s.loading {
		r.DrawRect(margin, contentTop, imgBoxW, imgBoxH, bg[0], bg[1], bg[2])
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
		r.DrawRect(margin+imgBoxW+10, contentTop, qrColW, imgBoxH, bg[0], bg[1], bg[2])
		lt := r.Theme.ListText
		r.DrawText("Loading...", margin, contentTop+imgBoxH+16, lt[0], lt[1], lt[2])
		ftrY := r.DrawFooterBar(footerH)
		r.DrawFooterHints(backHints(r.W), ftrY)
		return
	}
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), margin, contentTop+20, 200, 50, 50)
		ftrY := r.DrawFooterBar(footerH)
		r.DrawFooterHints(backHints(r.W), ftrY)
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
		// Proactively warm the next screenshot so navigation feels instant.
		if s.screenshotIdx+1 < len(s.detail.ScreenshotURLs) {
			s.cache.Warm(s.detail.ScreenshotURLs[s.screenshotIdx+1])
		}

		// Background box for screenshot
		r.DrawRect(margin, y, imgBoxW, imgBoxH, bg[0], bg[1], bg[2])

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
		r.DrawText(s.screenshotLabels[s.screenshotIdx], margin, y, 140, 140, 140)
		y += fontH + 6
	} else {
		// No screenshots — show QR full-width at the same pixel size as the
		// right-column version (qrColW-20).  Back-calculate the box height so
		// drawQR produces that exact size instead of being constrained by h.
		_, smallFH := r.SmallTextSize("Ag")
		const qrVMargin = int32(8) // must match vMargin inside drawQR
		captionH := int32(4) + smallFH + int32(2) + smallFH
		qrBoxH := (qrColW - 20) + captionH + qrVMargin*2
		s.drawQR(r, margin, y, usableW, qrBoxH)
		y += qrBoxH + 10
	}

	// ── Action area (full width) ────────────────────────────
	isPresent := s.inv.IsPresent(s.game.URL)

	// drawActionRow renders: circle badge | label text [price pill right-aligned].
	// Captures y, fontH, smallFH, margin, ac, r by reference/closure.
	drawActionRow := func(btn, label string, labelR, labelG, labelB, badgeR, badgeG, badgeB uint8, price float64) {
		d := fontH + 4
		cx, cy := margin+d/2, y+d/2
		aT := r.Theme.AccentText
		r.DrawCircleBadge(cx, cy, d, badgeR, badgeG, badgeB)
		r.DrawSmallTextCenteredInRect(btn, margin, y, d, d, aT[0], aT[1], aT[2])
		r.DrawText(label, margin+d+8, y, labelR, labelG, labelB)
		if price > 0 {
			priceStr := s.formattedPrice
			pw, _ := r.SmallTextSize(priceStr)
			const pp = int32(8) // padding to match tag list
			pillW := pw + pp*2
			pillH := smallFH + 4
			pillX := r.W - margin - pillW
			pillY := y + (fontH+4-pillH)/2
			r.DrawPill(pillX, pillY, pillW, pillH, 80, 60, 10)
			r.DrawSmallTextCenteredInRect(priceStr, pillX, pillY, pillW, pillH, 220, 180, 60)
		}
		y += fontH + 14
	}

	if isPresent {
		// Row 2: action button LEFT + status card RIGHT on the same line.
		rowY := y
		rowH := fontH + 14
		aT := r.Theme.AccentText

		// Determine action label.
		var actionLabel string
		var actionR, actionG, actionB uint8
		if s.game.IsFree {
			actionLabel, actionR, actionG, actionB = "Download again", 80, 200, 80
		} else if s.cfg.APIKey == "" {
			actionLabel, actionR, actionG, actionB = "Purchase required", 220, 180, 60
		} else {
			actionLabel, actionR, actionG, actionB = "Download again", 80, 200, 80
		}

		// Draw action badge + label.
		d := fontH + 4
		r.DrawCircleBadge(margin+d/2, rowY+d/2, d, ac[0], ac[1], ac[2])
		r.DrawSmallTextCenteredInRect("A", margin, rowY, d, d, aT[0], aT[1], aT[2])
		r.DrawText(actionLabel, margin+d+8, rowY, actionR, actionG, actionB)
		alW, _ := r.TextSize(actionLabel)
		actionEndX := margin + d + 8 + alW

		// Status card occupies the remaining width on the same row.
		if entry, ok := s.inv.Lookup(s.game.URL); ok && len(entry.Files) > 0 {
			f := entry.Files[0]
			pathText := f.Filename + " → " + filepath.Dir(f.DestPath) + "/"
			if len(entry.Files) > 1 {
				pathText += " (+" + strconv.Itoa(len(entry.Files)-1) + " more)"
			}

			const cp = int32(8)
			const dlBp = int32(5)

			dlW, _ := r.SmallTextSize("DL")
			dlPillW := dlW + dlBp*2
			dlPillH := smallFH + 4

			delCircleD := smallFH + 4
			delLabelW, _ := r.SmallTextSize("Delete")
			delBlockW := delCircleD + 6 + delLabelW

			cardX := actionEndX + 12
			cardW := r.W - margin - cardX
			cardH := dlPillH + cp*2

			// Align card vertically with the action button.
			cardY := rowY + (rowH-cardH)/2
			if cardY < rowY {
				cardY = rowY
			}

			r.DrawRect(cardX, cardY, cardW, cardH, 42, 42, 58)
			r.DrawRect(cardX+1, cardY+1, cardW-2, cardH-2, 10, 10, 18)

			r.DrawPill(cardX+cp, cardY+cp, dlPillW, dlPillH, 80, 200, 220)
			r.DrawSmallTextCenteredInRect("DL", cardX+cp, cardY+cp, dlPillW, dlPillH, 20, 20, 20)

			delCircleX := cardX + cardW - cp - delBlockW
			delCircleCX := delCircleX + delCircleD/2
			delCircleCY := cardY + cardH/2
			r.DrawCircleBadge(delCircleCX, delCircleCY, delCircleD, 160, 50, 50)
			r.DrawSmallTextCenteredInRect("X", delCircleX, cardY+cp, delCircleD, delCircleD, aT[0], aT[1], aT[2])
			r.DrawSmallText("Delete", delCircleX+delCircleD+6, cardY+cp+2, 200, 80, 80)

			textX := cardX + cp + dlPillW + 6
			textMaxW := delCircleX - textX - 4
			pathW, _ := r.SmallTextSize(pathText)

			r.SetClipRect(textX, cardY, textMaxW, cardH)
			if pathW <= textMaxW {
				s.pathScrollX = 0
				r.DrawSmallText(pathText, textX, cardY+cp+2, 100, 100, 120)
			} else if r.W <= narrowScreenW {
				// On small screens truncate the path rather than scrolling.
				truncated := truncateSmallToWidth(r, pathText, textMaxW)
				r.DrawSmallText(truncated, textX, cardY+cp+2, 100, 100, 120)
			} else {
				maxScrollX := pathW - textMaxW
				scrollX := s.pathScrollX
				if scrollX > maxScrollX {
					scrollX = maxScrollX
				}
				r.DrawSmallText(pathText, textX-scrollX, cardY+cp+2, 100, 100, 120)
				if s.pathScrollX >= maxScrollX {
					totalDur := pathScrollDelay +
						time.Duration(maxScrollX)*time.Second/time.Duration(pathScrollSpeed) +
						time.Second
					if time.Since(s.pathScrollAt) > totalDur {
						s.pathScrollX = 0
						s.pathScrollAt = time.Now()
					}
				}
			}
			r.SetClipRect(0, contentTop, r.W, contentH)

			// Unified naming toggle (below the combined row, if applicable)
			y = rowY + rowH + 4
			if s.cfg.UnifiedNaming {
				toggleLabel := "Disable title filename"
				if entry.UnifiedNamingDisabled {
					toggleLabel = "Enable title filename"
				}
				drawActionRow("Y", toggleLabel, mt[0], mt[1], mt[2], ac[0], ac[1], ac[2], 0)
			}
		} else {
			y = rowY + rowH + 4
		}
	} else {
		if s.detail != nil && s.detail.BrowserOnly {
			drawActionRow("A", "Browser-only", 140, 140, 140, ac[0], ac[1], ac[2], 0)
		} else if s.game.IsFree {
			drawActionRow("A", "Download", 80, 200, 80, ac[0], ac[1], ac[2], 0)
		} else if s.cfg.APIKey == "" {
			drawActionRow("A", "Purchase required", 220, 180, 60, 100, 80, 20, s.game.Price)
		} else {
			drawActionRow("A", "Download", 80, 200, 80, ac[0], ac[1], ac[2], s.game.Price)
		}
		y += 4
	}

	// ── Tags ────────────────────────────────────────────────
	if s.detail != nil && len(s.detail.PageTags) > 0 {
		r.DrawRect(margin, y, usableW, 1, 50, 50, 50)
		y += 11
		ac2 := r.Theme.Accent
		aT2 := r.Theme.AccentText
		// Blend accent toward gray-35 at 50% so the pill is clearly visible against
		// the black background while keeping the accent hue.
		bgPill := [3]uint8{
			uint8((int(ac2[0]) + 35) / 2),
			uint8((int(ac2[1]) + 35) / 2),
			uint8((int(ac2[2]) + 35) / 2),
		}
		// Use a fixed line height for tag pills (consistent with list screen).
		tagsH := r.DrawTagPills(s.detail.PageTags, margin, y, usableW, fontH+6,
			aT2[0], aT2[1], aT2[2], bgPill[0], bgPill[1], bgPill[2])
		y += tagsH + 10
	}

	// ── Description (full width, HTML-formatted) ─────────────
	if s.detail != nil && s.detail.Description != "" {
		r.DrawRect(margin, y, usableW, 1, 50, 50, 50)
		y += 11
		descH := r.DrawFormattedText(s.detail.Description, margin, y, usableW, fontH+4, 180, 180, 180)
		y += descH
	}

	// Track total content height for scroll clamping
	s.contentHeight = y - (contentTop - s.scrollY)

	r.ClearClipRect()

	// Redraw the header bar on top of the clipped content so it is never
	// obscured by content that scrolled toward the top of the screen.
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	if s.game.Platform != "" {
		platLabel := s.game.Platform
		pw, _ := r.SmallTextSize(platLabel)
		platPillW := pw + 12
		platPillX := r.W - 10 - platPillW
		platPillY := (headerH - badgePillH) / 2
		r.DrawPill(platPillX, platPillY, platPillW, badgePillH, 35, 45, 65)
		r.DrawSmallTextCenteredInRect(platLabel, platPillX, platPillY, platPillW, badgePillH, 130, 170, 210)
	}
	if s.detail != nil && s.inv.IsPresent(s.game.URL) {
		dlLabel := "● Downloaded"
		if r.W <= narrowScreenW {
			dlLabel = "● DL"
		}
		dw, _ := r.SmallTextSize(dlLabel)
		dlPillW := dw + 12
		redrawBadgeRight := r.W - 10
		if s.game.Platform != "" {
			pw, _ := r.SmallTextSize(s.game.Platform)
			redrawBadgeRight = redrawBadgeRight - (pw + 12) - 6
		}
		dlPillX := redrawBadgeRight - dlPillW
		dlPillY := (headerH - badgePillH) / 2
		r.DrawPill(dlPillX, dlPillY, dlPillW, badgePillH, 25, 50, 30)
		r.DrawSmallTextCenteredInRect(dlLabel, dlPillX, dlPillY, dlPillW, badgePillH, 70, 190, 90)
	}
	r.DrawText(title, 12, titleY, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, titleY+mainFH+4, ht[0], ht[1], ht[2])

	// ── Footer ──────────────────────────────────────────────
	ftrY := r.DrawFooterBar(footerH)
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
		{Kind: renderer.BadgePill, Label: "←→", Text: "Screenshots"},
	}
	if r.W > narrowScreenW {
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Settings"})
	}
	if s.contentHeight > contentH {
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "↕", Text: "Scroll"})
	}
	r.DrawFooterHints(hints, ftrY)
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

	// Border (accent-tinted) + box background
	ac := r.Theme.Accent
	r.DrawRect(boxX-1, boxY-1, boxW+2, boxH+2, ac[0]/2, ac[1]/2, ac[2]/2)
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

	if s.modal.kind == modalKindDeleteConfirm {
		r.DrawSmallTextCentered("A: confirm  B: cancel", boxX, y, boxW, 200, 100, 100)
	} else {
		r.DrawSmallTextCentered("Press any button to dismiss", boxX, y, boxW, 120, 120, 120)
	}
}

// drawAdvisoryOverlay renders the full-screen content warning cover.
func (s *DetailScreen) drawAdvisoryOverlay(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

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

	footerH := int32(52)
	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints(backHints(r.W), ftrY)
}

// drawQR renders the QR code centered within the given box.
// The texture is generated once and cached in s.qrTex for the lifetime of the
// screen — destroyed in goBack() when the user navigates away.
func (s *DetailScreen) drawQR(r *renderer.Renderer, x, y, w, h int32) {
	// Measure caption first so the QR size accounts for it.
	_, smallFH := r.SmallTextSize("Ag")
	captionH := int32(4) + smallFH + int32(2) + smallFH // gap + "Scan to open" + gap + "in browser"

	// Reserve equal top/bottom margins plus the caption below the QR.
	const vMargin = int32(8)
	maxH := h - captionH - vMargin*2
	if maxH < 80 {
		maxH = 80
	}
	qrSize := int(w - 20)
	if qrSize > int(maxH) {
		qrSize = int(maxH)
	}
	if qrSize > 512 {
		qrSize = 512
	}
	qrS := int32(qrSize)

	if s.qrTex == nil {
		tex, err := r.QRTexture(s.game.URL, qrSize)
		if err != nil {
			logger.Warn("detail: QR generate: %v", err)
			return
		}
		s.qrTex = tex
		logger.Debug("detail: QR texture generated for %s (%dpx)", s.game.URL, qrSize)
	}

	// Center the [QR + caption] block vertically within the box.
	blockH := qrS + captionH
	qrX := x + (w-qrS)/2
	qrY := y + (h-blockH)/2
	if qrY < y+vMargin {
		qrY = y + vMargin
	}
	r.DrawTextureAt(s.qrTex, qrX, qrY, qrS, qrS)
	r.DrawSmallTextCentered("Scan to open", x, qrY+qrS+4, w, 120, 120, 120)
	r.DrawSmallTextCentered("in browser", x, qrY+qrS+4+smallFH+2, w, 120, 120, 120)
}

// goBack destroys any cached SDL resources and returns the previous screen.
func (s *DetailScreen) goBack() Screen {
	if s.qrTex != nil {
		s.qrTex.Destroy()
		s.qrTex = nil
	}
	return s.prev
}

// scrollStep returns ~5 % of the visible content area height per tick so that
// hold-scroll speed feels consistent across devices with different resolutions
// (640×480 Miyoo Flip, 1024×768 TrimUI Brick, 1280×720 Smart Pro).
func (s *DetailScreen) scrollStep() int32 {
	if s.viewportH <= 0 {
		return 40 // safe default before first Draw
	}
	return s.viewportH / 20
}

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
	s.scrollY += int32(dir) * s.scrollStep()
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
				if s.modal.kind == modalKindDeleteConfirm {
					switch ev.Keysym.Sym {
					case sdl.K_RETURN: // physical A = confirm
						if s.modal.onConfirm != nil {
							s.modal.onConfirm()
						}
						s.modal.active = false
					case sdl.K_ESCAPE: // physical B = cancel
						s.modal.active = false
					}
				} else {
					s.modal.active = false
				}
			}
		case *sdl.ControllerButtonEvent:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				if s.modal.kind == modalKindDeleteConfirm {
					switch ev.Button {
					case sdl.CONTROLLER_BUTTON_B: // physical A = confirm
						if s.modal.onConfirm != nil {
							s.modal.onConfirm()
						}
						s.modal.active = false
					case sdl.CONTROLLER_BUTTON_A: // physical B = cancel
						s.modal.active = false
					}
				} else {
					s.modal.active = false
				}
			}
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
			return s.goBack()
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
		case sdl.K_x:
			return s.triggerDelete()
		case sdl.K_y:
			if s.inv.IsPresent(s.game.URL) && s.cfg.UnifiedNaming {
				return s.startUnifiedNamingToggle()
			}
		case sdl.K_s:
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s.inv, s.inventoryPath, s.cache, s, nil, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle, nil)

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
			return s.goBack()
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER, sdl.CONTROLLER_BUTTON_DPAD_LEFT:
			if s.detail != nil && s.screenshotIdx > 0 {
				s.screenshotIdx--
			}
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER, sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
			if s.detail != nil && s.screenshotIdx < len(s.detail.ScreenshotURLs)-1 {
				s.screenshotIdx++
			}
		case sdl.CONTROLLER_BUTTON_B:
			if !s.advisoryTriggered {
				return s.startDownload()
			}
		case sdl.CONTROLLER_BUTTON_X: // physical Y = unified naming toggle
			if s.inv.IsPresent(s.game.URL) && s.cfg.UnifiedNaming {
				return s.startUnifiedNamingToggle()
			}
		case sdl.CONTROLLER_BUTTON_Y: // physical X = delete
			return s.triggerDelete()
		case sdl.CONTROLLER_BUTTON_START:
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s.inv, s.inventoryPath, s.cache, s, nil, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle, nil)

		}
	}
	return s
}

func (s *DetailScreen) startDownload() Screen {
	if s.loading {
		return s
	}
	if s.detail != nil && s.detail.BrowserOnly {
		logger.Info("detail: browser-only game, blocking download for %s", s.game.URL)
		s.ShowModal("Browser-only game",
			"This game has no downloadable files and can only be played in a web browser. "+
				"Press any button to dismiss, then scan the QR code to open the game page.")
		return s
	}
	if !s.game.IsFree && s.cfg.APIKey == "" {
		return s
	}
	return NewFetchUploadsScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s.inv, s.inventoryPath, s)
}

func (s *DetailScreen) startUnifiedNamingToggle() Screen {
	entry, ok := s.inv.Lookup(s.game.URL)
	if !ok || len(entry.Files) == 0 {
		return s
	}
	newDisabled := !entry.UnifiedNamingDisabled
	s.inv.SetUnifiedNamingDisabled(s.game.URL, newDisabled)
	formats := inventory.ReadMigrateFormats(inventory.NXSettingsPath)
	return NewMigrateFlowScreen(s.inv, s.inventoryPath, s.game.URL, s.game.Title,
		entry.Files[0], !newDisabled, formats, s)
}

// ScheduleRebuild implements Rebuildable by propagating up the prev chain so
// ListScreen.needsRebuild is set after any inventory mutation on a sub-screen.
func (s *DetailScreen) ScheduleRebuild() {
	if r, ok := s.prev.(Rebuildable); ok {
		r.ScheduleRebuild()
	}
}

func (s *DetailScreen) triggerDelete() Screen {
	entry, ok := s.inv.Lookup(s.game.URL)
	if !ok {
		return s
	}
	if len(entry.Files) == 0 {
		return s
	}
	if len(entry.Files) > 1 {
		return NewManageDownloadsScreen(s.inv, s.inventoryPath, s.game.URL, s.cfg, s)
	}
	var bodyLines []string
	if len(entry.Files) == 1 {
		bodyLines = append(bodyLines, entry.Files[0].Filename)
		bodyLines = append(bodyLines, filepath.Dir(entry.Files[0].DestPath)+"/")
	}
	s.modal = detailModal{
		active: true,
		kind:   modalKindDeleteConfirm,
		title:  "Delete downloaded file?",
		body:   strings.Join(bodyLines, "\n"),
		onConfirm: func() {
			s.performSingleFileDelete()
		},
	}
	return s
}

func (s *DetailScreen) performSingleFileDelete() {
	entry, ok := s.inv.Lookup(s.game.URL)
	if !ok {
		return
	}
	for _, f := range entry.Files {
		if err := os.Remove(f.DestPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("inventory: delete file=%s: %v", f.DestPath, err)
		} else {
			logger.Debug("inventory: deleted file=%s", f.DestPath)
		}
		if artPath := inventory.CoverArtPath(entry.CoverURL, f.DestPath); artPath != "" {
			if err := os.Remove(artPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("inventory: delete cover-art=%s: %v", artPath, err)
			} else {
				logger.Debug("inventory: deleted cover-art=%s", artPath)
			}
		}
	}
	logger.Info("inventory: deleted game=%q files=%d", entry.Title, len(entry.Files))
	s.inv.Remove(s.game.URL)
	if err := s.inv.Save(s.inventoryPath); err != nil {
		logger.Warn("inventory: save after delete failed: %v", err)
	}
	s.ScheduleRebuild()
}

// backHints returns standard "back + settings" footer hints scaled to screen width.
func backHints(screenW int32) []renderer.FooterHint {
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}
	if screenW > narrowScreenW {
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Settings"})
	} else {
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "⚙", Text: ""})
	}
	return hints
}
