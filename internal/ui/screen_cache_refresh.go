//go:build !headless

package ui

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

type refreshCacheState int32

const (
	refreshCacheLoading refreshCacheState = iota
	refreshCacheDone
	refreshCacheError
	refreshCacheCancelled
)

// CacheRefreshScreen is a blocking progress screen shown while the full game
// list is being re-fetched. It pushes a UserEvent when the goroutine finishes
// so the SDL event loop wakes up immediately.
type CacheRefreshScreen struct {
	client         *itchio.Client
	cachePath      string
	prev           Screen
	onCacheUpdated func([]itchio.Game) // called from background goroutine; must be concurrency-safe
	cancel         context.CancelFunc

	state   refreshCacheState
	fetched int64 // written atomically from goroutine, read in Draw
	total   int   // number of games saved (success only)
	err     error // set on failure
}

// NewCacheRefreshScreen creates the screen and immediately starts the
// background fetch. onCacheUpdated is called on success from the background
// goroutine; the caller is responsible for ensuring its implementation handles
// concurrent access appropriately. It may be nil.
func NewCacheRefreshScreen(
	client *itchio.Client,
	cachePath string,
	prev Screen,
	onCacheUpdated func([]itchio.Game),
) *CacheRefreshScreen {
	ctx, cancel := context.WithCancel(context.Background())
	s := &CacheRefreshScreen{
		client:         client,
		cachePath:      cachePath,
		prev:           prev,
		onCacheUpdated: onCacheUpdated,
		cancel:         cancel,
		state:          refreshCacheLoading,
	}
	go func() {
		games, err := client.FetchAllGames(ctx, func(partial []itchio.Game) {
			atomic.StoreInt64(&s.fetched, int64(len(partial)))
			// Wake the SDL event loop so the counter redraws immediately.
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Info("cache refresh: cancelled by user after %d games", len(games))
				atomic.StoreInt32((*int32)(&s.state), int32(refreshCacheCancelled))
				sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
				return
			}
			logger.Error("cache refresh: failed after %d games: %v", len(games), err)
			s.err = err
			atomic.StoreInt32((*int32)(&s.state), int32(refreshCacheError))
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
			return
		}
		if err := itchio.SaveGamesCache(cachePath, games); err != nil {
			logger.Error("cache refresh: save failed: %v", err)
			s.err = err
			atomic.StoreInt32((*int32)(&s.state), int32(refreshCacheError))
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
			return
		}
		logger.Info("cache refresh: saved %d games", len(games))
		s.total = len(games)
		if onCacheUpdated != nil {
			onCacheUpdated(games)
		}
		atomic.StoreInt32((*int32)(&s.state), int32(refreshCacheDone))
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

func (s *CacheRefreshScreen) NeedsRedraw() bool {
	return refreshCacheState(atomic.LoadInt32((*int32)(&s.state))) == refreshCacheLoading
}
func (s *CacheRefreshScreen) HasPendingAnimation() bool { return false }

func (s *CacheRefreshScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Refreshing Game List", 20, textY, mt[0], mt[1], mt[2])

	fontH := mainFH
	contentTop := headerH + 4
	contentH := r.H - headerH - footerH
	mid := contentTop + contentH/2

	state := refreshCacheState(atomic.LoadInt32((*int32)(&s.state)))
	switch state {
	case refreshCacheLoading:
		fetched := atomic.LoadInt64(&s.fetched)
		r.DrawTextCentered("Fetching games", 0, mid-fontH-smallFH-14, r.W, mt[0], mt[1], mt[2])
		r.DrawSmallTextCentered(fmt.Sprintf("%d fetched", fetched), 0, mid-smallFH/2, r.W, mt[0], mt[1], mt[2])
		drawLoadingDots(r, mid+smallFH+8)

	case refreshCacheDone:
		r.DrawTextCentered("Done!", 0, mid-fontH-4, r.W, 80, 200, 80)
		r.DrawTextCentered(fmt.Sprintf("%d games cached.", s.total), 0, mid+4, r.W, mt[0], mt[1], mt[2])

	case refreshCacheError:
		if errors.Is(s.err, itchio.ErrCloudflareBlocked) {
			r.DrawTextCentered("Cloudflare blocked the request (HTTP 403)", 0, mid-fontH-8, r.W, 200, 100, 50)
			r.DrawWrappedText("Visit itch.io in a browser on the same WiFi, then retry the refresh.", 20, mid, r.W-40, fontH+4, 200, 160, 100)
		} else {
			r.DrawTextCentered("Refresh failed:", 0, mid-fontH-8, r.W, 200, 60, 60)
			r.DrawWrappedText(s.err.Error(), 20, mid, r.W-40, fontH+4, 200, 100, 100)
		}

	case refreshCacheCancelled:
		r.DrawTextCentered("Cancelled.", 0, mid, r.W, 160, 160, 160)
	}

	ftrY := r.DrawFooterBar(footerH)
	switch state {
	case refreshCacheLoading:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		}, ftrY)
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *CacheRefreshScreen) HandleEvent(e sdl.Event) Screen {
	state := refreshCacheState(atomic.LoadInt32((*int32)(&s.state)))
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		if state == refreshCacheLoading {
			if ev.Keysym.Sym == sdl.K_ESCAPE {
				s.cancel()
			}
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_ESCAPE, sdl.K_RETURN:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		if state == refreshCacheLoading {
			if ev.Button == sdl.CONTROLLER_BUTTON_A { // physical B = back/cancel
				s.cancel()
			}
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
			return s.prev
		}
	}
	return s
}

// IsBusy implements BusyChecker. Returns true while the cache fetch is running.
func (s *CacheRefreshScreen) IsBusy() bool {
	return refreshCacheState(atomic.LoadInt32((*int32)(&s.state))) == refreshCacheLoading
}
