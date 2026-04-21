//go:build !headless

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
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
	titleScrollX   int32     // current pixel offset (increases over time)
	titleScrollAt  time.Time // when the cursor last moved (scroll starts after a delay)

	// Cache fields — populated once the on-disk game cache is loaded.
	// cachedGames is nil until the cache is available.
	cachedGames []itchio.Game
	cacheReady  bool
	cachePath   string
}

func NewListScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, cachePath string) *ListScreen {
	s := &ListScreen{
		client:    client,
		cfg:       cfg,
		cache:     cache,
		page:      1,
		cfgPath:   cfgPath,
		cachePath: cachePath,
	}

	gameCache, err := itchio.LoadGamesCache(cachePath)
	if err == nil && len(gameCache.Games) > 0 {
		// Cache hit: populate list instantly from disk.
		logger.Info("cache: loaded %d games from %s (age=%v)",
			len(gameCache.Games), cachePath, time.Since(gameCache.Meta.FetchedAt).Round(time.Second))
		s.cachedGames = gameCache.Games
		s.cacheReady = true
		s.totalGames = len(gameCache.Games)
		s.totalPages = (s.totalGames + itchio.PerPage - 1) / itchio.PerPage
		s.games = pageSlice(gameCache.Games, 1)
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
		logger.Debug("cache: serving page %d from cache (%d games)", page, len(s.cachedGames))
		s.games = pageSlice(s.cachedGames, page)
		s.cursor = 0
		s.titleScrollX = 0
		s.titleScrollAt = time.Now()
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
	s.cursor = 0
	s.titleScrollX = 0
	s.titleScrollAt = time.Now()
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
	moved := false
	if dir > 0 && s.cursor < len(s.games)-1 {
		s.cursor++
		moved = true
	} else if dir < 0 && s.cursor > 0 {
		s.cursor--
		moved = true
	}
	if moved {
		s.titleScrollX = 0
		s.titleScrollAt = time.Now()
	}
}

func (s *ListScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	r.Clear(colorBG, colorBG, colorBG)

	// Header
	headerH := int32(72)
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	_, fontH := r.TextSize("Ag")
	r.DrawText("Itch.io — GB Studio Games", 12, (headerH-fontH)/2, colorText, colorText, colorText)
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

	rowH := fontH + 12 // measured font height + padding
	footerH := int32(40)
	visibleRows := (r.H - contentTop - footerH) / rowH

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
			r.DrawRect(0, rowTop, leftW, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}

		// Price badge — measure width to anchor it at the right edge.
		// Draw it last so it always renders on top of any scrolling title.
		var priceLabel string
		var priceR, priceG, priceB uint8
		if g.IsFree {
			priceLabel = "Free"
			priceR, priceG, priceB = 80, 200, 80
		} else {
			priceLabel = fmt.Sprintf("$%.2f", g.Price)
			priceR, priceG, priceB = 220, 180, 60
		}
		priceW, _ := r.TextSize(priceLabel)
		priceX := leftW - priceW - 8

		// Title — clip strictly to the area left of the price badge
		titleAreaW := priceX - 14 // available width before the price badge
		if i == s.cursor {
			titleW, _ := r.TextSize(g.Title)
			if titleW <= titleAreaW {
				// Fits — draw normally and reset any scroll
				s.titleScrollX = 0
				r.DrawText(g.Title, 10, y, colorText, colorText, colorText)
			} else {
				// Clamp scroll so the end of the title lands flush at the right edge
				maxScroll := titleW - titleAreaW
				scrollX := s.titleScrollX
				if scrollX > maxScroll {
					scrollX = maxScroll
				}
				// Clip to the title area only — price badge stays outside this rect
				r.SetClipRect(10, rowTop, titleAreaW, rowH)
				r.DrawText(g.Title, 10-scrollX, y, colorText, colorText, colorText)
				r.ClearClipRect()
				// Reset cycle: once we've held at the end for 1s, restart
				if scrollX == maxScroll && time.Since(s.titleScrollAt) > scrollDelay+time.Duration(maxScroll)*time.Second/time.Duration(scrollSpeed)+time.Second {
					s.titleScrollX = 0
					s.titleScrollAt = time.Now()
				}
			}
		} else {
			r.DrawText(truncateToWidth(r, g.Title, titleAreaW), 10, y, colorText, colorText, colorText)
		}

		// Draw price badge after title so it always appears on top
		r.DrawText(priceLabel, priceX, y, priceR, priceG, priceB)
	}

	// Right panel: cover art (or placeholder) + metadata
	if s.cursor < len(s.games) {
		g := s.games[s.cursor]
		metaY := contentTop
		boxW := rightW
		boxH := rightW * 3 / 4 // 4:3 aspect ratio box

		// Draw the box background for all states
		r.DrawRect(rightX, metaY, boxW, boxH, colorBG, colorBG, colorBG)

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

		lineGap := fontH + 5

		if g.Author != "" {
			r.DrawText("by "+g.Author, rightX, metaY, 160, 160, 160)
			metaY += lineGap
		}
		// Price — same colours as the list column
		if g.IsFree {
			r.DrawText("Free", rightX, metaY, 80, 200, 80)
		} else {
			r.DrawText(fmt.Sprintf("$%.2f", g.Price), rightX, metaY, 220, 180, 60)
		}
		metaY += lineGap
		// Tags — skip duplicates of the price already shown:
		// "Free" bracket tags and any bracket price tags like "$12.00"
		for _, tag := range g.Tags {
			if strings.EqualFold(tag, "free") {
				continue
			}
			if len(tag) > 0 && strings.ContainsRune("$€£¥", rune(tag[0])) {
				continue
			}
			r.DrawText(tag, rightX, metaY, 120, 180, 220)
			metaY += lineGap
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
	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText(footer, 10, ftrY, 140, 140, 140)
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
			return NewSettingsScreen(s.cfg, s.cfgPath, s, s.triggerCacheRefresh)
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
			return NewSettingsScreen(s.cfg, s.cfgPath, s, s.triggerCacheRefresh)
		}
	case *sdl.QuitEvent:
		return nil
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
	s.totalGames = len(games)
	s.totalPages = (len(games) + itchio.PerPage - 1) / itchio.PerPage
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

// triggerCacheRefresh is the callback handed to SettingsScreen for the
// "Refresh Game List" menu item.
func (s *ListScreen) triggerCacheRefresh() {
	logger.Info("cache: manual refresh triggered from settings")
	go s.buildCache()
}
