//go:build !headless

package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type fetchState int

const (
	fetchLoading fetchState = iota
	fetchDone
	fetchError
)

// FetchUploadsScreen is a transitional screen shown while the app fetches
// the list of available ROM files from the itch.io signed download page.
// Once done, it proceeds to the ROM picker (multiple files) or directly to
// the download screen (single file).
type FetchUploadsScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	prev    Screen

	state         fetchState
	uploads       []roms.Upload
	err           error
	isNotOwned    bool // true when error is "game not owned" — triggers auto-modal on prev screen
	inv           *inventory.Inventory
	inventoryPath string
}

func NewFetchUploadsScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail,
	inv *inventory.Inventory, inventoryPath string,
	prev Screen,
) *FetchUploadsScreen {
	s := &FetchUploadsScreen{
		client: client, cfg: cfg, cfgPath: cfgPath,
		cache: cache, game: game, detail: detail, prev: prev,
		state: fetchLoading,
		inv: inv, inventoryPath: inventoryPath,
	}
	go func() {
		var err error

		useAuthPath := !game.IsFree && cfg.APIKey != "" &&
			detail != nil && detail.GameID != ""
		logger.Debug("fetch: isFree=%v apiKey=%v detailNil=%v gameID=%q useAuthPath=%v",
			game.IsFree, cfg.APIKey != "", detail == nil, func() string {
				if detail != nil {
					return detail.GameID
				}
				return "<nil>"
			}(), useAuthPath)

		if useAuthPath {
			// Paid game owned by the user — use the itch.io API.
			authUploads, downloadKeyID, authErr := client.FetchAuthUploads(cfg.APIKey, detail.GameID)
			if authErr != nil {
				s.err = authErr
				s.state = fetchError
				s.isNotOwned = strings.Contains(authErr.Error(), "not owned")
			} else {
				for _, u := range authUploads {
					s.uploads = append(s.uploads, roms.Upload{
						Filename:      u.Filename,
						UploadID:      u.UploadID,
						DownloadKeyID: downloadKeyID,
						NeedsFormat:   u.NeedsFormat,
					})
				}
				if len(s.uploads) == 0 {
					logger.Warn("fetch: no downloadable uploads found for game (auth path)")
					s.err = fmt.Errorf("no downloadable files found for this game")
					s.state = fetchError
				} else {
					s.state = fetchDone
				}
			}
		} else {
			// Free game — use the CSRF scraping path.
			itchUploads, freeErr := client.FetchUploads(game.URL)
			err = freeErr
			if err != nil {
				s.err = err
				s.state = fetchError
			} else {
				for _, u := range itchUploads {
					s.uploads = append(s.uploads, roms.Upload{
						Filename:    u.Filename,
						URL:         u.URL,
						NeedsFormat: u.NeedsFormat,
					})
				}
				if len(s.uploads) == 0 {
					logger.Warn("fetch: no downloadable uploads found for game (free path)")
					s.err = fmt.Errorf("no downloadable files found for this game")
					s.state = fetchError
				} else {
					s.state = fetchDone
				}
			}
		}
		// Wake up the SDL event loop so HandleEvent fires immediately.
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

func (s *FetchUploadsScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	footerH := int32(40)
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16

	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, 140, 140, 140)

	contentTop := headerH + 6
	contentH := r.H - headerH - footerH
	mid := contentTop + contentH/2

	switch s.state {
	case fetchLoading:
		r.DrawTextCentered("Finding available files...", 0, mid-mainFH/2, r.W, colorText, colorText, colorText)

	case fetchError:
		r.DrawText("Could not fetch files:", 20, mid-mainFH-smallFH-8, 200, 60, 60)
		msg := s.err.Error()
		r.DrawWrappedText(msg, 20, mid-smallFH, r.W-40, smallFH+4, 200, 100, 100)

	case fetchDone:
		// Handled by transitioning in HandleEvent / next Draw cycle below
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.state {
	case fetchLoading:
		r.DrawSmallText("Please wait...", 10, ftrY, 140, 140, 140)
	default:
		r.DrawSmallText("A / B: back", 10, ftrY, 140, 140, 140)
	}
	r.Present()
}

func (s *FetchUploadsScreen) HandleEvent(e sdl.Event) Screen {
	// Transition immediately when fetch completes (triggered by UserEvent).
	if s.state == fetchDone {
		return s.nextScreen()
	}

	switch ev := e.(type) {
	case *sdl.UserEvent:
		_ = ev
		// "Not owned" error: go back to the detail screen and show a modal there
		// instead of showing a standalone error screen.
		if s.isNotOwned {
			if ds, ok := s.prev.(*DetailScreen); ok {
				ds.ShowModal("Cannot Download", s.err.Error())
			}
			return s.prev
		}
		// Other errors: stay on this screen to show the error.
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_ESCAPE, sdl.K_RETURN:
			if s.state == fetchError {
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
			if s.state == fetchError {
				return s.prev
			}
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *FetchUploadsScreen) nextScreen() Screen {
	var known, unknown []roms.Upload
	for _, u := range s.uploads {
		if u.NeedsFormat {
			unknown = append(unknown, u)
		} else {
			known = append(known, u)
		}
	}

	if len(known) > 0 {
		if len(known) == 1 {
			upload := known[0]
			if s.cfg.ROMLocation == "ask" {
				return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
			}
			ext := strings.ToLower(filepath.Ext(upload.Filename))
			dest := roms.DestinationDir(ext) + upload.Filename
			return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
		}
		return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, known, s.inv, s.inventoryPath, s.prev)
	}
	return NewFormatPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, unknown, s.inv, s.inventoryPath, s.prev)
}
