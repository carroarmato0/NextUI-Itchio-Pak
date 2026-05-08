//go:build !headless

package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
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

const (
	scrollDelay = time.Second
	scrollSpeed = int32(50)
)

// Auto-repeat timing for held D-pad buttons
const (
	repeatDelay           = 300 * time.Millisecond  // initial delay before repeating
	accelStart            = 180 * time.Millisecond  // repeat interval when acceleration begins
	accelMin              = 30 * time.Millisecond   // repeat interval at full speed
	accelRamp             = 1500 * time.Millisecond // time to reach accelMin from accelStart
	shoulderAccelMin = 15 * time.Millisecond // minimum repeat interval for D-pad page-scroll
	cacheTTL              = 24 * time.Hour

	// coverSettleDelay is how long the cursor must be stationary before cover
	// art fetches are initiated. Below accelStart (180 ms) so normal browsing
	// feels instant; well above accelMin (30 ms) so fast scrolling stays silent.
	coverSettleDelay = 100 * time.Millisecond

	// preloadRadius is the number of neighbours on each side of the cursor
	// that are warmed after the cursor settles.
	preloadRadius = 5
)

// currentRepeatInterval returns the repeat interval for a held button given how
// long the repeat phase has been running (elapsed = hold time minus repeatDelay).
// Uses the ease-out curve 1−(1−t)³ so the cursor accelerates quickly then
// plateaus smoothly at accelMin, mirroring the easing used in NextUI transitions.
func currentRepeatInterval(elapsed time.Duration) time.Duration {
	t := float64(elapsed) / float64(accelRamp)
	if t > 1 {
		t = 1
	}
	eased := 1.0 - math.Pow(1.0-t, 3)
	return accelStart - time.Duration(float64(accelStart-accelMin)*eased)
}

// currentShoulderRepeatInterval is like currentRepeatInterval but uses
// shoulderAccelMin as the floor, allowing L1/R1 to reach a faster rate than D-pad.
func currentShoulderRepeatInterval(elapsed time.Duration) time.Duration {
	t := float64(elapsed) / float64(accelRamp)
	if t > 1 {
		t = 1
	}
	eased := 1.0 - math.Pow(1.0-t, 3)
	return accelStart - time.Duration(float64(accelStart-shoulderAccelMin)*eased)
}

type pageResult struct {
	games []itchio.Game
	err   error
}

type ListScreen struct {
	client     *itchio.Client
	cfg        *settings.Config
	cache      *renderer.ImageCache
	cursor      int
	loading     atomic.Bool
	err         error
	cfgPath     string
	totalGames  atomic.Int32 // 0 = not yet known
	totalPages  atomic.Int32 // 0 = not yet known
	pageUpdateCh chan pageResult

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

	// cacheUpdateCh carries game-slice updates from background goroutines to the SDL thread.
	// Capacity 1: old unseen updates are overwritten rather than queued.
	cacheUpdateCh chan []itchio.Game

	// ownedUpdateCh carries owned-URL map updates from the background goroutine
	// (post-API-key-validation) to the SDL thread. Capacity 1: stale updates
	// are silently discarded, same as cacheUpdateCh.
	ownedUpdateCh  chan map[string]bool
	ownedURLs      map[string]bool
	ownedCachePath string

	// onOwnedReady is called by KeyTestScreen after a successful key validation.
	// It saves the owned cache to disk and sends the new map to ownedUpdateCh.
	onOwnedReady func([]itchio.OwnedGame)

	// Cover-art fetch throttling: fetches are deferred until the cursor has
	// been stationary for coverSettleDelay, then the current game plus
	// preloadRadius neighbours are warmed.
	lastCursorMove time.Time
	warmedGameURL  string

	// Shoulder button (D-pad L/R / PgDn/PgUp) auto-repeat state for page-scroll
	heldShoulderDir    int
	heldShoulderSince  time.Time
	lastShoulderRepeat time.Time

	// lastVisibleRows is set each Draw so the initial Left/Right press (startShoulderHold)
	// can jump by one full screen. Not used in the hold-repeat path.
	lastVisibleRows int

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
	ownedCachePath string,
) *ListScreen {
	s := &ListScreen{
		client:          client,
		cfg:             cfg,
		cache:           cache,
		cfgPath:         cfgPath,
		cachePath:       cachePath,
		inv:             inv,
		inventoryPath:   inventoryPath,
		updateSvc:       updateSvc,
		nextUITheme:     nextUITheme,
		defaultTheme:    defaultTheme,
		themeAvailable:  themeAvailable,
		onThemeToggle:   onThemeToggle,
		lastVisibleRows: 10,
		cacheUpdateCh:   make(chan []itchio.Game, 1),
		pageUpdateCh:    make(chan pageResult, 1),
	}
	s.ownedCachePath = ownedCachePath
	s.ownedUpdateCh = make(chan map[string]bool, 1)
	s.ownedURLs = make(map[string]bool)

	if urls, err := itchio.LoadOwnedCache(ownedCachePath); err == nil && len(urls) > 0 {
		for _, u := range urls {
			s.ownedURLs[u] = true
		}
		logger.Info("owned: loaded %d owned game URL(s) from cache", len(s.ownedURLs))
	} else if err != nil {
		logger.Warn("owned: failed to load owned cache: %v", err)
	}

	s.onOwnedReady = func(owned []itchio.OwnedGame) {
		urls := make([]string, len(owned))
		for i, g := range owned {
			urls[i] = g.URL
		}
		if err := itchio.SaveOwnedCache(s.ownedCachePath, urls); err != nil {
			logger.Warn("owned: failed to save owned cache: %v", err)
		}
		m := make(map[string]bool, len(urls))
		for _, u := range urls {
			m[u] = true
		}
		select {
		case <-s.ownedUpdateCh:
		default:
		}
		s.ownedUpdateCh <- m
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
		logger.Info("owned: %d owned game URL(s) received from key validation", len(m))
	}

	// If an API key is already configured, validate it in the background so that
	// owned game data is refreshed without requiring the user to open Settings.
	if cfg.APIKey != "" {
		go func() {
			_, owned, err := client.ValidateAPIKey(cfg.APIKey)
			if err != nil {
				logger.Warn("owned: startup key validation failed: %v", err)
				return
			}
			s.onOwnedReady(owned)
		}()
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
			s.totalGames.Store(int32(total))
			s.totalPages.Store(int32((total + itchio.PerPage - 1) / itchio.PerPage))
		}()
		go s.buildCache()
	}
	return s
}

func (s *ListScreen) loadPage(page int, query string) {
	s.loading.Store(true)
	logger.Debug("feed: loading page %d query=%q", page, query)
	games, err := s.client.FetchGames(page, query)
	if err != nil {
		logger.Error("feed: page %d error: %v", page, err)
	} else {
		logger.Info("feed: page %d returned %d games", page, len(games))
	}
	select {
	case <-s.pageUpdateCh:
	default:
	}
	s.pageUpdateCh <- pageResult{games: games, err: err}
	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
}

func (s *ListScreen) processAutoRepeat() {
	now := time.Now()
	if s.heldDir != 0 {
		elapsed := now.Sub(s.heldSince)
		if elapsed >= repeatDelay && now.Sub(s.lastRepeat) >= currentRepeatInterval(elapsed-repeatDelay) {
			s.moveCursor(s.heldDir)
			s.lastRepeat = now
		}
	}
	if s.heldShoulderDir != 0 {
		elapsed := now.Sub(s.heldShoulderSince)
		if elapsed >= repeatDelay && now.Sub(s.lastShoulderRepeat) >= currentShoulderRepeatInterval(elapsed-repeatDelay) {
			s.jumpCursor(s.heldShoulderDir)
			s.lastShoulderRepeat = now
		}
	}
}

func (s *ListScreen) moveCursor(dir int) {
	if dir > 0 {
		if s.cursor < len(s.viewGames)-1 {
			s.cursor++
			s.titleScrollX = 0
			s.titleScrollAt = time.Now()
			s.tagScrollY = 0
			s.tagScrollAt = time.Now()
			s.lastCursorMove = time.Now()
			s.warmedGameURL = ""
		}
	} else if dir < 0 {
		if s.cursor > 0 {
			s.cursor--
			s.titleScrollX = 0
			s.titleScrollAt = time.Now()
			s.tagScrollY = 0
			s.tagScrollAt = time.Now()
			s.lastCursorMove = time.Now()
			s.warmedGameURL = ""
		}
	}
}

// jumpCursor moves the cursor by n items, clamping to the list bounds.
// Used by the initial L1/R1 press (one screen) and by the hold-repeat path (one row).
func (s *ListScreen) jumpCursor(n int) {
	if n == 0 {
		return
	}
	newPos := s.cursor + n
	if newPos < 0 {
		newPos = 0
	}
	if newPos >= len(s.viewGames) {
		newPos = len(s.viewGames) - 1
	}
	if newPos < 0 {
		newPos = 0 // viewGames may be empty
	}
	s.cursor = newPos
	s.titleScrollX = 0
	s.titleScrollAt = time.Now()
	s.tagScrollY = 0
	s.tagScrollAt = time.Now()
	s.lastCursorMove = time.Now()
	s.warmedGameURL = ""
}


func (s *ListScreen) NeedsRedraw() bool {
	if s.heldDir != 0 || s.heldShoulderDir != 0 {
		return true
	}
	// Resume rendering 500ms before scrollDelay expires so the first
	// animation frame is not missed when the cursor has been stationary.
	return !s.titleScrollAt.IsZero() &&
		time.Since(s.titleScrollAt) > scrollDelay/2
}

// HasPendingAnimation returns true while the title-scroll delay is counting
// down but hasn't yet crossed the NeedsRedraw threshold. This tells the main
// loop to use a medium timeout instead of blocking indefinitely, so the
// animation fires on schedule even when no SDL events arrive.
func (s *ListScreen) HasPendingAnimation() bool {
	if s.titleScrollAt.IsZero() {
		return false
	}
	return time.Since(s.titleScrollAt) <= scrollDelay/2
}

// warmPreloadWindow warms cover art for the current game plus preloadRadius
// neighbours on each side, indexed into viewGames so page boundaries are
// handled transparently. Sets warmedGameURL so Draw does not re-warm until
// the cursor moves again.
func (s *ListScreen) warmPreloadWindow() {
	if s.cursor >= len(s.viewGames) {
		return
	}
	absIdx := s.cursor
	for i := absIdx - preloadRadius; i <= absIdx+preloadRadius; i++ {
		if i < 0 || i >= len(s.viewGames) {
			continue
		}
		if url := s.viewGames[i].CoverURL; url != "" {
			s.cache.Warm(url)
		}
	}
	s.warmedGameURL = s.viewGames[s.cursor].CoverURL
	logger.Debug("cover: warmed window abs=%d ±%d (%d games in view)", absIdx, preloadRadius, len(s.viewGames))
}

func (s *ListScreen) Draw(r *renderer.Renderer) {
	select {
	case games := <-s.cacheUpdateCh:
		s.cachedGames = games
		s.cacheReady = true
		s.needsRebuild = false
		s.rebuildView()
	default:
	}
	select {
	case newOwned := <-s.ownedUpdateCh:
		s.ownedURLs = newOwned
		s.rebuildView()
	default:
	}
	select {
	case res := <-s.pageUpdateCh:
		s.loading.Store(false)
		s.viewGames = res.games
		s.err = res.err
		s.cursor = 0
		s.titleScrollX = 0
		s.titleScrollAt = time.Now()
		s.tagScrollY = 0
		s.tagScrollAt = time.Now()
		s.lastCursorMove = time.Now()
		s.warmedGameURL = ""
	default:
	}
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

	if s.loading.Load() {
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
	s.lastVisibleRows = int(visibleRows)

	if len(s.viewGames) == 0 && s.cacheReady {
		ht := r.Theme.HintText
		r.DrawTextCentered("No games match this filter.", 0, r.H/2-fontH, leftW, ht[0], ht[1], ht[2])
		r.DrawTextCentered("Press L1/R1 to change sort.", 0, r.H/2+4, leftW, 80, 160, 180)
		ftrY := r.DrawFooterBar(footerH)
		if r.W <= narrowScreenW {
			r.DrawFooterHints([]renderer.FooterHint{
				{Kind: renderer.BadgePill, Label: "L1/R1", Text: "Sort"},
				{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"},
				{Kind: renderer.BadgePill, Label: "START", Text: "Set"},
			}, ftrY)
		} else {
			r.DrawFooterHints([]renderer.FooterHint{
				{Kind: renderer.BadgePill, Label: "L1/R1", Text: "Sort"},
				{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"},
				{Kind: renderer.BadgePill, Label: "START", Text: "Settings"},
			}, ftrY)
		}
		r.Present()
		return
	}

	// In [DL] mode, compute where the group separator falls. Must be done
	// before startIdx so the scroll calculation can account for the gap.
	dlSepAfterUpdates := -1
	if s.sortMode == itchio.SortModeDL && len(s.viewGames) > 0 {
		lastUpdateIdx := -1
		for i, g := range s.viewGames {
			if s.inv.HasPendingUpdates(g.URL) || s.inv.IsRemoved(g.URL) {
				lastUpdateIdx = i
			}
		}
		if lastUpdateIdx >= 0 && lastUpdateIdx < len(s.viewGames)-1 {
			dlSepAfterUpdates = lastUpdateIdx
		}
	}

	// Height of the separator bar. Rows in the downloaded group are shifted
	// down by this amount so the bar occupies its own dedicated slot.
	dlSepBarH := int32(0)
	if dlSepAfterUpdates >= 0 {
		dlSepBarH = smallFH + 8
	}

	// Compute startIdx to keep the cursor row on screen.
	// When the cursor is in the downloaded group and the separator is still in
	// the viewport, the separator consumes dlSepBarH pixels, so fewer rows fit.
	contentH := r.H - footerH - contentTop
	startIdx := 0
	if dlSepAfterUpdates >= 0 && s.cursor > dlSepAfterUpdates {
		// Assume separator is visible; reduce available height accordingly.
		effVis := int((contentH - dlSepBarH) / rowH)
		if effVis < 1 {
			effVis = 1
		}
		startIdx = s.cursor - effVis + 1
		if startIdx < 0 {
			startIdx = 0
		}
		// If that pushes startIdx past the separator, the separator has
		// scrolled off the top — yOff will be 0, so use the plain formula.
		if startIdx > dlSepAfterUpdates {
			startIdx = s.cursor - int(visibleRows) + 1
			if startIdx < 0 {
				startIdx = 0
			}
		}
	} else {
		if s.cursor >= int(visibleRows) {
			startIdx = s.cursor - int(visibleRows) + 1
		}
	}

	// Advance horizontal scroll for the selected title (1s pause, then 50px/s).
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

	for i, g := range s.viewGames {
		if i < startIdx {
			continue
		}
		rowIdx := i - startIdx
		// Shift rows in the downloaded group down by dlSepBarH, but only
		// while the separator itself is still within the visible range.
		// Once startIdx > dlSepAfterUpdates, the separator has scrolled off
		// the top and the downloaded rows start cleanly from contentTop.
		yOff := int32(0)
		if dlSepAfterUpdates >= 0 && i > dlSepAfterUpdates && startIdx <= dlSepAfterUpdates {
			yOff = dlSepBarH
		}
		rowTop := contentTop + int32(rowIdx)*rowH + yOff
		if rowTop >= r.H-footerH {
			break
		}
		y := rowTop + (rowH-fontH)/2
		if i == s.cursor {
			ac := r.Theme.Accent
			r.DrawPill(4, rowTop+2, leftW-8, rowH-4, ac[0], ac[1], ac[2])
		}

		// Determine download/update status for this row.
		isPendingUpdate := s.inv.HasPendingUpdates(g.URL)
		isRemovedGame := s.inv.IsRemoved(g.URL)
		isPresent := s.inv.IsPresent(g.URL)
		isOwned := s.ownedURLs[g.URL]

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
		case isOwned:
			badgeLabel = "OWNED"
			badgeR, badgeG, badgeB = 60, 200, 120
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

	// Draw the DL-mode group separator AFTER all row content so it renders
	// on top of any row backgrounds. The bar fills the dedicated gap slot
	// between the last update row and the first downloaded row.
	if dlSepAfterUpdates >= 0 && dlSepBarH > 0 {
		sepRowIdx := dlSepAfterUpdates - startIdx
		if sepRowIdx >= 0 {
			sepBarY := contentTop + int32(sepRowIdx+1)*rowH
			if sepBarY < r.H-footerH {
				r.DrawRect(0, sepBarY, leftW, dlSepBarH, 40, 40, 40)
				r.DrawSmallTextCentered("— downloaded —", 0, sepBarY+(dlSepBarH-smallFH)/2, leftW, 100, 100, 100)
			}
		}
	}

	// Settle check: once the cursor has been stationary for coverSettleDelay,
	// warm the current game and its neighbours. Resets when cursor moves.
	if s.cursor < len(s.viewGames) &&
		!s.lastCursorMove.IsZero() &&
		s.heldShoulderDir == 0 &&
		time.Since(s.lastCursorMove) >= coverSettleDelay &&
		s.viewGames[s.cursor].CoverURL != s.warmedGameURL {
		s.warmPreloadWindow()
	}

	// Right panel: cover art (or placeholder) + metadata
	if s.cursor < len(s.viewGames) {
		g := s.viewGames[s.cursor]
		metaY := contentTop
		boxW := rightW
		boxH := rightW * 3 / 4 // 4:3 aspect ratio box

		// Draw the box background for all states
		r.DrawRect(rightX, metaY, boxW, boxH, bg[0], bg[1], bg[2])

		if g.CoverURL != "" {
			tex := s.cache.Peek(r, g.CoverURL)
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
				r.DrawTextCenteredInRect("No Image", rightX, metaY, boxW, boxH, 80, 80, 80)
			} else {
				r.DrawTextCenteredInRect("Loading...", rightX, metaY, boxW, boxH, 80, 80, 80)
			}
		} else {
			// No cover URL — wireframe border
			r.DrawRect(rightX+2, metaY+2, boxW-4, boxH-4, bg[0], bg[1], bg[2])
			r.DrawRect(rightX+3, metaY+3, boxW-6, boxH-6, 35, 35, 35)
			r.DrawTextCenteredInRect("No Image", rightX, metaY, boxW, boxH, 80, 80, 80)
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
	footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "←→", Text: "Page"})
	if s.cacheReady {
		footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "L1/R1", Text: "Sort"})
	}
	footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"})
	if r.W <= narrowScreenW {
		footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Set"})
	} else {
		footerHints = append(footerHints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Settings"})
	}
	r.DrawFooterHints(footerHints, ftrY)

	// Pagination info right-aligned.
	currentPage := s.cursor/itchio.PerPage + 1
	pageInfo := fmt.Sprintf("Page %d", currentPage)
	if tp := s.totalPages.Load(); tp > 0 {
		pageInfo = fmt.Sprintf("Page %d/%d", currentPage, tp)
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

func (s *ListScreen) startShoulderHold(dir int) {
	s.jumpCursor(dir * s.lastVisibleRows)
	s.heldShoulderDir = dir
	s.heldShoulderSince = time.Now()
	s.lastShoulderRepeat = s.heldShoulderSince
}

func (s *ListScreen) stopShoulderHold(dir int) {
	if s.heldShoulderDir == dir {
		s.heldShoulderDir = 0
	}
}

func (s *ListScreen) nextSortMode() itchio.SortMode {
	m := itchio.NextSortMode(s.sortMode)
	if m == itchio.SortModeOwned && len(s.ownedURLs) == 0 {
		m = itchio.NextSortMode(m)
	}
	return m
}

func (s *ListScreen) prevSortMode() itchio.SortMode {
	m := itchio.PrevSortMode(s.sortMode)
	if m == itchio.SortModeOwned && len(s.ownedURLs) == 0 {
		m = itchio.PrevSortMode(m)
	}
	return m
}

// changeSortMode applies a new sort mode, resets the cursor to the top, and
// persists the choice to config.
func (s *ListScreen) changeSortMode(mode itchio.SortMode) {
	s.sortMode = mode
	logger.Info("sort: mode changed to %q (%s)", s.sortMode, itchio.SortModeBadge(s.sortMode))
	s.rebuildView()
	s.cursor = 0
	s.titleScrollX = 0
	s.titleScrollAt = time.Now()
	s.tagScrollY = 0
	s.tagScrollAt = time.Now()
	s.lastCursorMove = time.Now()
	s.warmedGameURL = ""
	s.cfg.SortMode = string(s.sortMode)
	go s.cfg.Save(s.cfgPath)
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
		case sdl.K_RIGHT:
			if ev.Type == sdl.KEYDOWN {
				s.startShoulderHold(1)
			} else {
				s.stopShoulderHold(1)
			}
			return s
		case sdl.K_LEFT:
			if ev.Type == sdl.KEYDOWN {
				s.startShoulderHold(-1)
			} else {
				s.stopShoulderHold(-1)
			}
			return s
		}
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_PAGEDOWN:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.nextSortMode())
			return s
		case sdl.K_PAGEUP:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.prevSortMode())
			return s
		case sdl.K_ESCAPE:
			return nil
		case sdl.K_RETURN:
			if s.cursor < len(s.viewGames) {
				return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.viewGames[s.cursor], s.inv, s.inventoryPath, s, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle)
			}
		case sdl.K_s:
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle, s.onOwnedReady)
		case sdl.K_x:
			if s.cursor < len(s.viewGames) {
				g := s.viewGames[s.cursor]
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
		case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startShoulderHold(1)
			} else {
				s.stopShoulderHold(1)
			}
			return s
		case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startShoulderHold(-1)
			} else {
				s.stopShoulderHold(-1)
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
			go s.loadPage(1, "")
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_B:
			if s.cursor < len(s.viewGames) {
				return NewDetailScreen(s.client, s.cfg, s.cfgPath, s.cache, s.viewGames[s.cursor], s.inv, s.inventoryPath, s, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle)
			}
		case sdl.CONTROLLER_BUTTON_A:
			return nil
		case sdl.CONTROLLER_BUTTON_START:
			return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle, s.onOwnedReady)
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.nextSortMode())
			return s
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			if !s.cacheReady {
				return s
			}
			s.changeSortMode(s.prevSortMode())
			return s
		case sdl.CONTROLLER_BUTTON_X:
			if s.cursor < len(s.viewGames) {
				g := s.viewGames[s.cursor]
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

// truncateSmallToWidth truncates text with "…" so it fits within maxW pixels
// when rendered in the small hint font.
func truncateSmallToWidth(r *renderer.Renderer, text string, maxW int32) string {
	tw, _ := r.SmallTextSize(text)
	if tw <= maxW {
		return text
	}
	ellipsisW, _ := r.SmallTextSize("…")
	target := maxW - ellipsisW
	runes := []rune(text)
	for len(runes) > 0 {
		tw, _ = r.SmallTextSize(string(runes))
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

// rebuildView regenerates viewGames from cachedGames using the current sortMode.
// It preserves the currently selected game's position where possible; falls back
// to cursor 0 if the game is no longer in the view.
func (s *ListScreen) rebuildView() {
	var selectedURL string
	selectedViewIdx := s.cursor
	if s.cursor < len(s.viewGames) {
		selectedURL = s.viewGames[s.cursor].URL
	}

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
	s.viewGames = itchio.ApplySort(s.cachedGames, s.sortMode, downloaded, pendingUpdates, removed, s.ownedURLs)
	n := len(s.viewGames)
	s.totalGames.Store(int32(n))
	s.totalPages.Store(int32((n + itchio.PerPage - 1) / itchio.PerPage))

	if selectedURL != "" {
		for i, g := range s.viewGames {
			if g.URL == selectedURL {
				s.cursor = i
				s.titleScrollX = 0
				s.titleScrollAt = time.Now()
				s.tagScrollY = 0
				s.tagScrollAt = time.Now()
				s.lastCursorMove = time.Now()
				s.warmedGameURL = ""
				logger.Debug("sort: view rebuilt — %d games visible (mode=%s), restored selection to %q (cursor=%d)",
					len(s.viewGames), itchio.SortModeBadge(s.sortMode), selectedURL, i)
				return
			}
		}
	}

	// Selected game gone — land on the nearest position in the new view.
	if len(s.viewGames) > 0 {
		if selectedViewIdx >= len(s.viewGames) {
			selectedViewIdx = len(s.viewGames) - 1
		}
		s.cursor = selectedViewIdx
		s.titleScrollX = 0
		s.titleScrollAt = time.Now()
		s.tagScrollY = 0
		s.tagScrollAt = time.Now()
		s.lastCursorMove = time.Now()
		s.warmedGameURL = ""
		logger.Debug("sort: view rebuilt — %d games visible (mode=%s), selection gone; landing at nearest position (cursor=%d)",
			len(s.viewGames), itchio.SortModeBadge(s.sortMode), selectedViewIdx)
		return
	}
	s.cursor = 0
	if !s.cacheReady {
		go s.loadPage(1, "")
	}
	logger.Debug("sort: view rebuilt — %d games visible (mode=%s)", len(s.viewGames), itchio.SortModeBadge(s.sortMode))
}

// IsBusy implements BusyChecker. Returns true while the background game-list
// fetch/write goroutine is running.
func (s *ListScreen) IsBusy() bool {
	return s.cacheBuilding.Load()
}

// buildCache fetches the complete game list and writes it to disk.
// Called as a goroutine. On success, future page turns use the local cache.
func (s *ListScreen) buildCache() {
	s.cacheBuilding.Store(true)
	defer s.cacheBuilding.Store(false)
	logger.Info("cache: starting background full fetch")
	// context.Background() is intentional: this goroutine is not cancellable on
	// app exit. A future improvement could thread an app-level context here.
	games, err := s.client.FetchAllGames(context.Background(), func(partial []itchio.Game) {
		logger.Debug("cache: fetched %d games so far", len(partial))
		snapshot := make([]itchio.Game, len(partial))
		copy(snapshot, partial)
		select {
		case s.cacheUpdateCh <- snapshot:
		default:
		}
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
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
	// Flip to cache mode. Send to the SDL thread via the channel; the
	// current page view is updated on the next Draw call.
	select {
	case s.cacheUpdateCh <- games:
	default:
	}
	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
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
		select {
		case s.cacheUpdateCh <- games:
		default:
		}
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
		if s.updateSvc != nil {
			s.updateSvc.TriggerNow()
		}
	})
}
