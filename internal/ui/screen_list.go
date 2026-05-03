//go:build !headless

package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
)

// narrowScreenW is the display width of the Miyoo Flip (my355). Footer hints
// are abbreviated at or below this width to prevent overflow.
const narrowScreenW = int32(640)

// Auto-repeat timing for held D-pad buttons
const (
	repeatDelay    = 300 * time.Millisecond // initial delay before repeating
	repeatInterval = 40 * time.Millisecond  // interval between repeats
	cacheTTL       = 24 * time.Hour
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

	// Horizontal title scroll for selected row
	titleScrollX  int32     // current pixel offset (increases over time)
	titleScrollAt time.Time // when the cursor last moved (scroll starts after a delay)

	// Vertical tag scroll for selected game's right-panel tag line
	tagScrollY  int32
	tagScrollAt time.Time

	// Cache fields — populated once the on-disk game cache is loaded.
	// cachedGames is nil until the cache is available.
	cachedGames []itchio.Game
	cacheReady  bool
	cachePath   string

	inv           *inventory.Inventory
	inventoryPath string
	updateSvc     UpdateServicer

	// cacheBuilding is set while buildCache / refreshCacheIfStale runs.
	cacheBuilding atomic.Bool

	// jumpToEnd signals that the next loadPage call should place the cursor on
	// the last item rather than the first. Set when navigating to a previous page.
	jumpToEnd bool

	// Sort/filter state
	sortMode     itchio.SortMode
	viewGames    []itchio.Game // sorted/filtered view; paging operates on this
	needsRebuild bool         // set by ScheduleRebuild; consumed at next Draw

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	onThemeToggle  func(bool)
}

func NewListScreen(
	client *itchio.Client,
	cfg *settings.Config,
	cfgPath string,
	cache *renderer.ImageCache,
	cachePath string,
	inv *inventory.Inventory,
	inventoryPath string,
	updateSvc UpdateServicer,
	nextUITheme theme.Theme,
	defaultTheme theme.Theme,
	themeAvailable bool,
	onThemeToggle func(bool),
) *ListScreen {
	s := &ListScreen{
		client:         client,
		cfg:            cfg,
		cache:          cache,
		page:           1,
		cfgPath:        cfgPath,
		cachePath:      cachePath,
		inv:            inv,
		inventoryPath:  inventoryPath,
		updateSvc:      updateSvc,
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		onThemeToggle:  onThemeToggle,
	}
	s.sortMode = itchio.SortMode(cfg.SortMode)

	gameCache, err := itchio.LoadGamesCache(cachePath)
	if err == nil && len(gameCache.Games) > 0 {
		// Cache hit: populate list instantly from disk.
		logger.Info("cache: loaded %d games from %s (age=%v)",
			len(gameCache.Games), cachePath, time.Since(gameCache.Meta.FetchedAt).Round(time.Second))
		s.cachedGames = gameCache.Games
		s.cacheReady = true
		s.rebuildView()
		// Refresh in background if stale.
		go s.refreshCacheIfStale(gameCache.Meta.FetchedAt)
	} else {
		// No cache: live fetch page 1 (existing behaviour) + build cache in background.
		if err != nil {
			logger.Debug("cache: no cache found (%v), using live feed", err)
		} else {
			logger.Debug("cache: file exists but contains no games, using live feed")
		}
		go s.loadPage(1, "")
		go func() {
			total, err := client.FetchTotalGames()
			if err != nil {
				logger.Error("feed: total games: %v", err)
				return
			}
			logger.Info("feed: total games=%d", total)
			s.totalGames = total
			s.totalPages = (total + itchio.PerPage - 1) / itchio.PerPage
		}()
		go s.buildCache()
	}
	return s
}

func (s *ListScreen) loadPage(page int, query string) {
	if s.cacheReady && query == "" {
		// Serve from local cache — no network, instant.
		logger.Debug("cache: serving page %d from view (%d games)", page, len(s.viewGames))
		s.games = pageSlice(s.viewGames, page)
		s.placeCursor()
		s.titleScrollX = 0
		s.titleScrollAt = time.Now()
		s.tagScrollY = 0
		s.tagScrollAt = time.Now()
		return
	}
	// Live network fetch (existing behaviour).
	s.loading = true
	s.err = nil
	logger.Debug("feed: loading page %d query=%q", page, query)
	games, err := s.client.FetchGames(page, query)
	if err != nil {
		logger.Error("feed: page %d error: %v", page, err)
	} else {
		logger.Info("feed: page %d returned %d games", page, len(games))
	}
	s.games = games
	s.err = err
	s.placeCursor()
	s.titleScrollX = 0
	s.titleScrollAt = time.Now()
	s.tagScrollY = 0
	s.tagScrollAt = time.Now()
	s.loading = false
}

// placeCursor sets the cursor position after a page load.
// If jumpToEnd is set the cursor lands on the last item (used when navigating
// to a previous page); otherwise it lands on the first item.
func (s *ListScreen) placeCursor() {
	if s.jumpToEnd && len(s.games) > 0 {
		s.cursor = len(s.games) - 1
	} else {
		s.cursor = 0
	}
	s.jumpToEnd = false
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
	if dir > 0 {
		if s.cursor < len(s.games)-1 {
			s.cursor++
			s.titleScrollX = 0
			s.titleScrollAt = time.Now()
			s.tagScrollY = 0
			s.tagScrollAt = time.Now()
		} else if s.totalPages == 0 || s.page < s.totalPages {
			// At the last item on the page — advance to the next page.
			s.page++
			go s.loadPage(s.page, "")
		}
	} else if dir < 0 {
		if s.cursor > 0 {
			s.cursor--
			s.titleScrollX = 0
			s.titleScrollAt = time.Now()
			s.tagScrollY = 0
			s.tagScrollAt = time.Now()
		} else if s.page > 1 {
			// At the first item on the page — go back to the previous page,
			// landing on its last item.
			s.page--
			s.jumpToEnd = true
			go s.loadPage(s.page, "")
		}
	}
}

func (s *ListScreen) Draw(r *renderer.Renderer) {
	if s.needsRebuild {
		s.needsRebuild = false
		s.rebuildView()
	}
	s.processAutoRepeat()
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	// Header
	headerH := int32(72)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerTextY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Itch.io — GB Studio Games", 12, headerTextY, mt[0], mt[1], mt[2])
	if s.cacheReady {
		badge := itchio.SortModeBadge(s.sortMode)
		bw, bh := r.TextSize(badge)
		const hPad = int32(8)
		pillW := bw + hPad*2
		pillH := bh + 4
		pillX := r.W - pillW - 12
		pillY := headerTextY - 2
		ac := r.Theme.Accent
		aT := r.Theme.AccentText
		r.DrawPill(pillX, pillY, pillW, pillH, ac[0], ac[1], ac[2])
		r.DrawTextCenteredInRect(badge, pillX, pillY, pillW, pillH, aT[0], aT[1], aT[2])
	}

	contentTop := headerH + 4

	if s.loading {
		lt := r.Theme.ListText
		r.DrawText("Loading...", 20, r.H/2, lt[0], lt[1], lt[2])
		r.Present()
		return
	}
	if s.err != nil {
		_, fontH := r.TextSize("Ag")
		mid := r.H / 2
		if errors.Is(s.err, itchio.ErrCloudflareBlocked) {
			r.DrawTextCentered("Cloudflare blocked the request (HTTP 403)", 0, mid-fontH-4, r.W, 200, 100, 50)
			r.DrawWrappedText("Visit itch.io in a browser on the same WiFi, then press A to retry.", 20, mid+4, r.W-40, fontH+4, 200, 160, 100)
		} else {
			r.DrawText("Error: "+s.err.Error(), 20, mid, 200, 50, 50)
		}
		ftrY := r.DrawFooterBar(52)
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "A", Text: "Retry"},
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"},
		}, ftrY)
		r.Present()
		return
	}

	leftW := r.W * 52 / 100
	rightX := leftW + 24
	rightW := r.W - rightX - 10

	rowH := fontH + 12 // measured font height + padding
	footerH := int32(52)
	visibleRows := (r.H - contentTop - footerH) / rowH

	if len(s.viewGames) == 0 && s.cacheReady {
		ht := r.Theme.HintText
		r.DrawTextCentered("No games match this filter.", 0, r.H/2-fontH, leftW, ht[0], ht[1], ht[2])
		r.DrawTextCentered("Press SELECT to change sort.", 0, r.H/2+4, leftW, 80, 160, 180)
		ftrY := r.DrawFooterBar(footerH)
		if r.W <= narrowScreenW {
			r.DrawFooterHints([]renderer.FooterHint{
				{Kind: renderer.BadgePill, Label: "SEL", Text: "Sort"},
				{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"},
				{Kind: renderer.BadgePill, Label: "⚙", Text: ""},
			}, ftrY)
		} else {
			r.DrawFooterHints([]renderer.FooterHint{
				{Kind: renderer.BadgePill, Label: "SELECT", Text: "Sort"},
				{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"},
				{Kind: renderer.BadgePill, Label: "START", Text: "Settings"},
			}, ftrY)
		}
		r.Present()
		return
	}

	startIdx := 0
	if s.cursor >= int(visibleRows) {
		startIdx = s.cursor - int(visibleRows) + 1
	}

	// Advance horizontal scroll for the selected title (1s pause, then 50px/s).
	const scrollDelay = time.Second
	const scrollSpeed = int32(50) // pixels per second
	if !s.titleScrollAt.IsZero() {
		elapsed := time.Since(s.titleScrollAt)
		if elapsed > scrollDelay {
			s.titleScrollX = int32((elapsed - scrollDelay).Seconds() * float64(scrollSpeed))
		}
	}

	const tagScrollSpeed = int32(30) // pixels per second
	if !s.tagScrollAt.IsZero() {
		elapsed := time.Since(s.tagScrollAt)
		if elapsed > scrollDelay {
			s.tagScrollY = int32((elapsed - scrollDelay).Seconds() * float64(tagScrollSpeed))
		}
	}

	// In [DL] mode, compute where group transitions occur for separator lines.
	dlSepAfterUpdates := -1
	if s.sortMode == itchio.SortModeDL && len(s.games) > 0 {
		lastUpdateIdx := -1
		for i, g := range s.games {
			if s.inv.HasPendingUpdates(g.URL) || s.inv.IsRemoved(g.URL) {
				lastUpdateIdx = i
			}
		}
		if lastUpdateIdx >= 0 && lastUpdateIdx < len(s.games)-1 {
			dlSepAfterUpdates = lastUpdateIdx
		}
	}

	for i, g := range s.games {
		if i < startIdx {
			continue
		}
		rowIdx := i - startIdx
		if int32(rowIdx) >= visibleRows {
			break
		}
		y := contentTop + int32(rowIdx)*rowH + (rowH-fontH)/2 // vertically centre text in row
		rowTop := contentTop + int32(rowIdx)*rowH
		if i == s.cursor {
			ac := r.Theme.Accent
			r.DrawPill(4, rowTop+2, leftW-8, rowH-4, ac[0], ac[1], ac[2])
		}

		// Determine download/update status for this row.
		isPendingUpdate := s.inv.HasPendingUpdates(g.URL)
		isRemovedGame := s.inv.IsRemoved(g.URL)
		isPresent := s.inv.IsPresent(g.URL)

		// Badge: update/removed/downloaded state or price.
		var badgeLabel string
		var badgeR, badgeG, badgeB uint8
		switch {
		case isPendingUpdate:
			badgeLabel = "UP"
			badgeR, badgeG, badgeB = 240, 160, 40
		case isRemovedGame:
			badgeLabel = "!"
			badgeR, badgeG, badgeB = 200, 60, 60
		case isPresent:
			badgeLabel = "DL"
			badgeR, badgeG, badgeB = 80, 200, 220
		case g.IsFree:
			badgeLabel = "Free"
			badgeR, badgeG, badgeB = 80, 200, 80
		default:
			badgeLabel = fmt.Sprintf("$%.2f", g.Price)
			badgeR, badgeG, badgeB = 220, 180, 60
		}
		badgeW, _ := r.SmallTextSize(badgeLabel)
		pillW := badgeW + 10 // 5px horizontal padding per side
		// Increased margin from the right edge of the list (leftW) from 8 to 16.
		badgeX := leftW - pillW - 16

		// Title area is between the left margin (16) and the badge gap.
		titleAreaW := badgeX - 16 - 14

		isDownloaded := isPresent || isPendingUpdate || isRemovedGame
		const titleX = int32(16)
		if i == s.cursor {
			aT := r.Theme.AccentText
			if isDownloaded {
				titleW, _ := r.BoldTextSize(g.Title)
				if titleW <= titleAreaW {
					s.titleScrollX = 0
					r.DrawBoldText(g.Title, titleX, y, aT[0], aT[1], aT[2])
				} else {
					maxScroll := titleW - titleAreaW
					scrollX := s.titleScrollX
					if scrollX > maxScroll {
						scrollX = maxScroll
					}
					r.SetClipRect(titleX, rowTop, titleAreaW, rowH)
					r.DrawBoldText(g.Title, titleX-scrollX, y, aT[0], aT[1], aT[2])
					r.ClearClipRect()
					if scrollX == maxScroll && time.Since(s.titleScrollAt) > scrollDelay+time.Duration(maxScroll)*time.Second/time.Duration(scrollSpeed)+time.Second {
						s.titleScrollX = 0
						s.titleScrollAt = time.Now()
					}
				}
			} else {
				titleW, _ := r.TextSize(g.Title)
				if titleW <= titleAreaW {
					s.titleScrollX = 0
					r.DrawText(g.Title, titleX, y, aT[0], aT[1], aT[2])
				} else {
					maxScroll := titleW - titleAreaW
					scrollX := s.titleScrollX
					if scrollX > maxScroll {
						scrollX = maxScroll
					}
					r.SetClipRect(titleX, rowTop, titleAreaW, rowH)
					r.DrawText(g.Title, titleX-scrollX, y, aT[0], aT[1], aT[2])
					r.ClearClipRect()
					if scrollX == maxScroll && time.Since(s.titleScrollAt) > scrollDelay+time.Duration(maxScroll)*time.Second/time.Duration(scrollSpeed)+time.Second {
						s.titleScrollX = 0
						s.titleScrollAt = time.Now()
					}
				}
			}
		} else {
			lt := r.Theme.ListText
			if isDownloaded {
				r.DrawBoldText(truncateBoldToWidth(r, g.Title, titleAreaW), titleX, y, lt[0], lt[1], lt[2])
			} else {
				r.DrawText(truncateToWidth(r, g.Title, titleAreaW), titleX, y, lt[0], lt[1], lt[2])
			}
		}

		// Badge always rendered on top of title.
		pillH := smallFH + 4
		pillY := y + (fontH-pillH)/2
		r.DrawPill(badgeX, pillY, pillW, pillH, badgeR, badgeG, badgeB)
		r.DrawSmallText(badgeLabel, badgeX+5, pillY+2, 20, 20, 20)
	}

	// Draw the DL-mode group separator AFTER all row content so the label
	// renders on top instead of being covered by the next row's title.
	// The label is vertically centred on the separator line so it sits at
	// the row boundary without overlapping either neighbour's text.
	if dlSepAfterUpdates >= 0 {
		sepRowIdx := dlSepAfterUpdates - startIdx
		if sepRowIdx >= 0 && sepRowIdx < int(visibleRows) {
			sepRowTop := contentTop + int32(sepRowIdx)*rowH
			sepY := sepRowTop + rowH
			r.DrawRect(0, sepY, leftW, 1, 50, 50, 50)
			r.DrawSmallText("— downloaded —", titleX, sepY-smallFH/2, 80, 80, 80)
		}
	}

	// Right panel: cover art (or placeholder) + metadata
	if s.cursor < len(s.games) {
		g := s.games[s.cursor]
		metaY := contentTop
		boxW := rightW
		boxH := rightW * 3 / 4 // 4:3 aspect ratio box

		// Draw the box background for all states
		r.DrawRect(rightX, metaY, boxW, boxH, bg[0], bg[1], bg[2])

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
				// Pill badge overlay — drawn after texture so it appears above animated GIFs.
				if s.inv.HasPendingUpdates(g.URL) || s.inv.IsRemoved(g.URL) || s.inv.IsPresent(g.URL) {
					var pillLabel string
					var pillR, pillG, pillB uint8
					var shadowR, shadowG, shadowB uint8
					var textR, textG, textB uint8
					if s.inv.HasPendingUpdates(g.URL) {
						pillLabel = "UPDATE"
						pillR, pillG, pillB = 240, 160, 40
						shadowR, shadowG, shadowB = 160, 96, 16
						textR, textG, textB = 20, 20, 20
					} else if s.inv.IsRemoved(g.URL) {
						pillLabel = "REMOVED"
						pillR, pillG, pillB = 200, 60, 60
						shadowR, shadowG, shadowB = 122, 16, 16
						textR, textG, textB = 255, 255, 255
					} else {
						pillLabel = "DL"
						pillR, pillG, pillB = 80, 200, 220
						shadowR, shadowG, shadowB = 30, 130, 150
						textR, textG, textB = 20, 20, 20
					}
					lw, lh := r.SmallTextSize(pillLabel)
					const pad = int32(5)
					pillW := lw + pad*2
					pillH := lh + 4
					pillX := imgX + dw - pillW - 6
					pillY := imgY + 6
					// Draw a subtle shadow/border for the overlay badge
					r.DrawPill(pillX+1, pillY+1, pillW, pillH, shadowR, shadowG, shadowB)
					r.DrawPill(pillX, pillY, pillW, pillH, pillR, pillG, pillB)
					r.DrawSmallTextCenteredInRect(pillLabel, pillX, pillY, pillW, pillH, textR, textG, textB)
				}
			} else if s.cache.Failed(g.CoverURL) {
				r.DrawText("No Image", rightX+boxW/2-40, metaY+boxH/2-10, 80, 80, 80)
			} else {
				r.DrawText("Loading...", rightX+boxW/2-40, metaY+boxH/2-10, 80, 80, 80)
			}
		} else {
			// No cover URL — wireframe border
			r.DrawRect(rightX+2, metaY+2, boxW-4, boxH-4, bg[0], bg[1], bg[2])
			r.DrawRect(rightX+3, metaY+3, boxW-6, boxH-6, 35, 35, 35)
			r.DrawText("No Image", rightX+boxW/2-40, metaY+boxH/2-10, 80, 80, 80)
		}
		metaY += boxH + 12

		lineGap := fontH + 5

		if g.Author != "" {
			mt2 := r.Theme.MainText
			r.DrawText("by "+g.Author, rightX, metaY, mt2[0], mt2[1], mt2[2])
			metaY += lineGap
		}
		// Tags: filter and render as pill badges with vertical-scroll if overflow.
		var filteredTags []string
		for _, tag := range g.Tags {
			if strings.EqualFold(tag, "free") {
				continue
			}
			if len(tag) > 0 && strings.ContainsRune("$€£¥", rune(tag[0])) {
				continue
			}
			filteredTags = append(filteredTags, tag)
		}
		if len(filteredTags) > 0 {
			ac := r.Theme.Accent
			aT := r.Theme.AccentText
			// Measure total pill height to know whether scroll is needed.
			totalTagH := r.MeasureTagPills(filteredTags, rightX, rightW, lineGap)
			availH := r.H - footerH - metaY
			if availH <= 0 {
				availH = 0
			}
			if totalTagH <= availH {
				s.tagScrollY = 0
				// Blend accent toward gray-35 at 50% so the pill is clearly visible against
				// the black background while keeping the accent hue.
				bgPill := [3]uint8{
					uint8((int(ac[0]) + 35) / 2),
					uint8((int(ac[1]) + 35) / 2),
					uint8((int(ac[2]) + 35) / 2),
				}
				r.DrawTagPills(filteredTags, rightX, metaY, rightW, lineGap,
					aT[0], aT[1], aT[2], bgPill[0], bgPill[1], bgPill[2])
				metaY += totalTagH
			} else {
				maxTagScroll := totalTagH - availH
				if s.tagScrollY > maxTagScroll {
					s.tagScrollY = maxTagScroll
				}
				if s.tagScrollY == maxTagScroll &&
					time.Since(s.tagScrollAt) > scrollDelay+time.Duration(maxTagScroll)*time.Second/time.Duration(tagScrollSpeed)+time.Second {
					s.tagScrollY = 0
					s.tagScrollAt = time.Now()
				}
				r.SetClipRect(rightX, metaY, rightW, availH)
				bgPill := [3]uint8{
					uint8((int(ac[0]) + 35) / 2),
					uint8((int(ac[1]) + 35) / 2),
					uint8((int(ac[2]) + 35) / 2),
				}
				r.DrawTagPills(filteredTags, rightX, metaY-s.tagScrollY, rightW, lineGap,
					aT[0], aT[1], aT[2], bgPill[0], bgPill[1], bgPill[2])
				r.ClearClipRect()
			}
		}
	}

	// Build footer hints.
	ftrY := r.DrawFooterBar(footerH)

	var footerHints []renderer.FooterHint
	footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"})
	footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "L/R", Text: "Page"})
	if s.cacheReady {
		if r.W <= narrowScreenW {
			footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "SEL", Text: "Sort"})
		} else {
			footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "SELECT", Text: "Sort"})
		}
	}
	footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"})
	if r.W <= narrowScreenW {
		footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "⚙", Text: ""})
	} else {
		footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Settings"})
	}
	r.DrawFooterHints(footerHints, ftrY)

	// Pagination info right-aligned.
	pageInfo := fmt.Sprintf("Page %d", s.page)
	if s.totalPages > 0 {
		pageInfo = fmt.Sprintf("Page %d/%d", s.page, s.totalPages)
	}
	ht := r.Theme.HintText
	piW, _ := r.SmallTextSize(pageInfo)
	r.DrawSmallText(pageInfo, r.W-piW-10, ftrY, ht[0], ht[1], ht[2])

	r.Present()
}

// drawPlaceholder renders a bordered rectangle with centered text.
func (s *ListScreen) drawPlaceholder(r *renderer.Renderer, x, y, w, h int32, label string) {
	bg := r.Theme.Background
	r.DrawRect(x, y, w, h, 45, 45, 45)
	r.DrawRect(x+2, y+2, w-4, h-4, bg[0], bg[1], bg[2])
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
			if len(s.viewGames) == 0 {
				return s
			}
			if s.totalPages == 0 || s.page < s.totalPages {
				s.page++
				go s.loadPage(s.page, "")
			}
		case sdl.K_PAGEUP:
			if len(s.viewGames) == 0 {
				return s
			}
			if s.page > 1 {
				s.page--
				go s.loadPage(s.page, "")
			}
		case sdl.K_RETURN:
			if s.cursor < len(s.games) {
				return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.games[s.cursor], s.inv, s.inventoryPath, s, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle)
			}
		case sdl.K_s:
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle)
		case sdl.K_x:
			if s.cursor < len(s.games) {
				g := s.games[s.cursor]
				if s.inv.HasPendingUpdates(g.URL) {
					s.inv.DismissUpdate(g.URL)
					if err := s.inv.Save(s.inventoryPath); err != nil {
						logger.Warn("inventory: save after dismiss: %v", err)
					}
					s.rebuildView()
				} else if s.inv.IsRemoved(g.URL) {
					s.inv.DismissRemoval(g.URL)
					if err := s.inv.Save(s.inventoryPath); err != nil {
						logger.Warn("inventory: save after dismiss: %v", err)
					}
					s.rebuildView()
				}
			}
			return s
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
		// Allow retrying when the feed is blocked (physical A = confirm button = sdl B).
		// CONTROLLER_BUTTON_A (physical B = back/exit) is intentionally left unhandled
		// here so it falls through to the exit case below.
		if s.err != nil && ev.Button == sdl.CONTROLLER_BUTTON_B {
			go s.loadPage(s.page, "")
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			if len(s.viewGames) == 0 {
				return s
			}
			if s.totalPages == 0 || s.page < s.totalPages {
				s.page++
				go s.loadPage(s.page, "")
			}
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			if len(s.viewGames) == 0 {
				return s
			}
			if s.page > 1 {
				s.page--
				go s.loadPage(s.page, "")
			}
		case sdl.CONTROLLER_BUTTON_B:
			if s.cursor < len(s.games) {
				return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.games[s.cursor], s.inv, s.inventoryPath, s, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle)
			}
		case sdl.CONTROLLER_BUTTON_A:
			return nil
		case sdl.CONTROLLER_BUTTON_START:
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle)
		case sdl.CONTROLLER_BUTTON_BACK:
			if !s.cacheReady {
				return s
			}
			s.sortMode = itchio.NextSortMode(s.sortMode)
			logger.Info("sort: mode changed to %q (%s)", s.sortMode, itchio.SortModeBadge(s.sortMode))
			s.rebuildView()
			s.cfg.SortMode = string(s.sortMode)
			go s.cfg.Save(s.cfgPath)
			return s
		case sdl.CONTROLLER_BUTTON_X:
			if s.cursor < len(s.games) {
				g := s.games[s.cursor]
				if s.inv.HasPendingUpdates(g.URL) {
					s.inv.DismissUpdate(g.URL)
					logger.Info("update-svc: update dismissed for game=%q", g.Title)
					if err := s.inv.Save(s.inventoryPath); err != nil {
						logger.Warn("inventory: save after dismiss: %v", err)
					}
					s.rebuildView()
				} else if s.inv.IsRemoved(g.URL) {
					s.inv.DismissRemoval(g.URL)
					logger.Info("update-svc: removal dismissed for game=%q", g.Title)
					if err := s.inv.Save(s.inventoryPath); err != nil {
						logger.Warn("inventory: save after dismiss: %v", err)
					}
					s.rebuildView()
				}
			}
			return s
		}
	}
	return s
}

// truncateToWidth returns text truncated with "…" so it fits within maxW pixels.
// Uses rune-based trimming and accounts for the ellipsis width itself.
func truncateToWidth(r *renderer.Renderer, text string, maxW int32) string {
	tw, _ := r.TextSize(text)
	if tw <= maxW {
		return text
	}
	ellipsisW, _ := r.TextSize("…")
	target := maxW - ellipsisW
	runes := []rune(text)
	for len(runes) > 0 {
		tw, _ = r.TextSize(string(runes))
		if tw <= target {
			break
		}
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), " ") + "…"
}

// truncateBoldToWidth truncates text with "…" so it fits within maxW pixels
// when rendered in bold.
func truncateBoldToWidth(r *renderer.Renderer, text string, maxW int32) string {
	tw, _ := r.BoldTextSize(text)
	if tw <= maxW {
		return text
	}
	ellipsisW, _ := r.BoldTextSize("…")
	target := maxW - ellipsisW
	runes := []rune(text)
	for len(runes) > 0 {
		tw, _ = r.BoldTextSize(string(runes))
		if tw <= target {
			break
		}
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), " ") + "…"
}

// Rebuildable is implemented by screens that hold a sorted/filtered game view.
// Sub-screens (detail, manage-downloads) call ScheduleRebuild on their prev
// chain after any inventory mutation so the list reflects the change immediately
// when control returns to it.
type Rebuildable interface {
	ScheduleRebuild()
}

// ScheduleRebuild marks the list view as stale; Draw will call rebuildView before
// its next render.
func (s *ListScreen) ScheduleRebuild() { s.needsRebuild = true }

// rebuildView regenerates viewGames from cachedGames using the current sortMode,
// then resets paging to page 1.
func (s *ListScreen) rebuildView() {
	downloaded := make(map[string]bool)
	pendingUpdates := make(map[string]bool)
	removed := make(map[string]bool)
	for _, g := range s.cachedGames {
		if s.inv.IsPresent(g.URL) {
			downloaded[g.URL] = true
		}
		if s.inv.HasPendingUpdates(g.URL) {
			pendingUpdates[g.URL] = true
		}
		if s.inv.IsRemoved(g.URL) {
			removed[g.URL] = true
		}
	}
	s.viewGames = itchio.ApplySort(s.cachedGames, s.sortMode, downloaded, pendingUpdates, removed)
	s.totalGames = len(s.viewGames)
	s.totalPages = (s.totalGames + itchio.PerPage - 1) / itchio.PerPage
	s.page = 1
	s.loadPage(1, "")
	logger.Debug("sort: view rebuilt — %d games visible (mode=%s)", len(s.viewGames), itchio.SortModeBadge(s.sortMode))
}

// IsBusy implements BusyChecker. Returns true while the background game-list
// fetch/write goroutine is running.
func (s *ListScreen) IsBusy() bool {
	return s.cacheBuilding.Load()
}

// pageSlice returns the sub-slice of games for the given 1-based page number,
// using the global PerPage constant. The returned slice shares backing memory
// with games — callers must not mutate it.
func pageSlice(games []itchio.Game, page int) []itchio.Game {
	start := (page - 1) * itchio.PerPage
	if start >= len(games) {
		return nil
	}
	end := start + itchio.PerPage
	if end > len(games) {
		end = len(games)
	}
	return games[start:end]
}

// buildCache fetches the complete game list and writes it to disk.
// Called as a goroutine. On success, future page turns use the local cache.
func (s *ListScreen) buildCache() {
	s.cacheBuilding.Store(true)
	defer s.cacheBuilding.Store(false)
	logger.Info("cache: starting background full fetch")
	// context.Background() is intentional: this goroutine is not cancellable on
	// app exit. A future improvement could thread an app-level context here.
	games, err := s.client.FetchAllGames(context.Background(), func(fetched int) {
		logger.Debug("cache: fetched %d games so far", fetched)
	})
	if err != nil {
		logger.Error("cache: full fetch failed after %d games: %v", len(games), err)
		return
	}
	if err := itchio.SaveGamesCache(s.cachePath, games); err != nil {
		logger.Error("cache: save failed: %v", err)
		return
	}
	logger.Info("cache: saved %d games to %s", len(games), s.cachePath)
	// Flip to cache mode. The current page view is left untouched;
	// the next page navigation will source from the cache.
	s.cachedGames = games
	s.cacheReady = true
	s.rebuildView()
	if s.updateSvc != nil {
		s.updateSvc.TriggerNow()
	}
}

// refreshCacheIfStale triggers a full re-fetch if the cache is older than cacheTTL.
func (s *ListScreen) refreshCacheIfStale(fetchedAt time.Time) {
	age := time.Since(fetchedAt)
	if age < cacheTTL {
		logger.Debug("cache: fresh (age=%v), skipping background refresh", age.Round(time.Second))
		return
	}
	logger.Info("cache: stale (age=%v), refreshing in background", age.Round(time.Second))
	s.buildCache()
}

// newCacheRefreshScreen returns a CacheRefreshScreen that runs a full cache
// rebuild and notifies this ListScreen on completion via onCacheUpdated.
// It is passed to SettingsScreen as the onRefreshGames callback.
func (s *ListScreen) newCacheRefreshScreen(prev Screen) Screen {
	logger.Info("cache: manual refresh triggered from settings")
	return NewCacheRefreshScreen(s.client, s.cachePath, prev, func(games []itchio.Game) {
		s.cachedGames = games
		s.cacheReady = true
		s.rebuildView()
		if s.updateSvc != nil {
			s.updateSvc.TriggerNow()
		}
	})
}
