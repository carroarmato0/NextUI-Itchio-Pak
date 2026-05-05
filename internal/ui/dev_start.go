//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
)

// DevNavEventCode is the SDL user-event code used to trigger automatic screen
// navigation in DEV_START_SCREEN mode. Must not clash with the power event codes
// defined in cmd/itchio-pak/main_sdl.go (0=inventory, 1=sleep, 2=shutdown).
const DevNavEventCode = int32(3)

// devAutoDetailScreen wraps ListScreen and navigates to the detail screen of
// the first available game once the list cache is populated. Used exclusively
// when DEV_START_SCREEN=detail is set; never instantiated in production paths.
type devAutoDetailScreen struct {
	list          *ListScreen
	client        *itchio.Client
	cfg           *settings.Config
	cfgPath       string
	cache         *renderer.ImageCache
	inv           *inventory.Inventory
	inventoryPath string
	stopPoll      chan struct{}

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	onThemeToggle  func(bool)
}

func newDevAutoDetailScreen(
	list *ListScreen, client *itchio.Client, cfg *settings.Config,
	cfgPath string, cache *renderer.ImageCache,
	inv *inventory.Inventory, inventoryPath string,
	nextUITheme theme.Theme, defaultTheme theme.Theme,
	themeAvailable bool, onThemeToggle func(bool),
) *devAutoDetailScreen {
	s := &devAutoDetailScreen{
		list:           list,
		client:         client,
		cfg:            cfg,
		cfgPath:        cfgPath,
		cache:          cache,
		inv:            inv,
		inventoryPath:  inventoryPath,
		stopPoll:       make(chan struct{}),
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		onThemeToggle:  onThemeToggle,
	}
	go s.pollForGames()
	return s
}

// pollForGames pokes the SDL event loop every 100 ms until the list cache is
// populated, then fires a user event to trigger the detail navigation.
func (s *devAutoDetailScreen) pollForGames() {
	for {
		select {
		case <-s.stopPoll:
			return
		case <-time.After(100 * time.Millisecond):
			if len(s.list.viewGames) > 0 {
				sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: DevNavEventCode})
				return
			}
		}
	}
}

func (s *devAutoDetailScreen) NeedsRedraw() bool {
	return s.list.NeedsRedraw()
}

func (s *devAutoDetailScreen) HasPendingAnimation() bool {
	return s.list.HasPendingAnimation()
}

func (s *devAutoDetailScreen) Draw(r *renderer.Renderer) {
	s.list.Draw(r)
}

func (s *devAutoDetailScreen) HandleEvent(e sdl.Event) Screen {
	if uev, ok := e.(*sdl.UserEvent); ok && uev.Code == DevNavEventCode {
		select {
		case <-s.stopPoll: // already closed — goroutine already exited
		default:
			close(s.stopPoll)
		}
		if len(s.list.viewGames) > 0 {
			logger.Info("dev:detail-ready %q", s.list.viewGames[0].Title)
			return NewDetailScreen(
				s.client, s.cfg, s.cfgPath, s.cache,
				s.list.viewGames[0], s.inv, s.inventoryPath, s.list,
				s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle,
			)
		}
		return s.list
	}
	// Let the list handle the event (for scrolling, etc.), but keep this
	// wrapper as current unless the list navigated away to a different screen.
	next := s.list.HandleEvent(e)
	if next == Screen(s.list) {
		return s
	}
	return next
}

// NewDevStartScreen returns the initial Screen for the given DEV_START_SCREEN
// value. Falls back to listScreen for unrecognised values.
func NewDevStartScreen(
	name string,
	list *ListScreen,
	client *itchio.Client,
	cfg *settings.Config,
	cfgPath string,
	cache *renderer.ImageCache,
	inv *inventory.Inventory,
	inventoryPath string,
	updateSvc UpdateServicer,
	nextUITheme theme.Theme,
	defaultTheme theme.Theme,
	themeAvailable bool,
	onThemeToggle func(bool),
) Screen {
	switch name {
	case "settings":
		// onRefreshGames is nil-safe inside SettingsScreen.
		return NewSettingsScreen(client, cfg, cfgPath, list, nil, updateSvc, nextUITheme, defaultTheme, themeAvailable, onThemeToggle)
	case "detail":
		return newDevAutoDetailScreen(list, client, cfg, cfgPath, cache, inv, inventoryPath, nextUITheme, defaultTheme, themeAvailable, onThemeToggle)
	default:
		return list
	}
}
