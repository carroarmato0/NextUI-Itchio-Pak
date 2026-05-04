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
	fetchNeedsPurchasePick
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
	ownedKeys     []itchio.OwnedKey // populated when fetchNeedsPurchasePick
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
			// Paid game — find all purchase keys for this game.
			ownedKeys, keysErr := client.FetchOwnedKeys(cfg.APIKey, detail.GameID)
			if keysErr != nil {
				s.err = keysErr
				s.state = fetchError
				s.isNotOwned = strings.Contains(keysErr.Error(), "not owned")
				sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
				return
			}

			// Annotate bundle keys with human-readable bundle names from the game page.
			if detail != nil && len(detail.BundleNames) > 0 {
				ownedKeys = itchio.AnnotateBundleNames(ownedKeys, detail.BundleNames)
			}

			if len(ownedKeys) == 1 {
				// Only one purchase — fetch uploads immediately, no picker needed.
				s.applyUploadsForKey(ownedKeys[0])
			} else {
				// Multiple purchases (e.g. individual + bundle) — let user choose.
				s.ownedKeys = ownedKeys
				s.state = fetchNeedsPurchasePick
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
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

// applyUploadsForKey fetches the upload list for a specific owned key and
// populates s.uploads. Called either directly (single key) or after the user
// picks a purchase from PurchasePickerScreen.
func (s *FetchUploadsScreen) applyUploadsForKey(key itchio.OwnedKey) {
	downloadKeyID := fmt.Sprintf("%d", key.ID)
	authUploads, authErr := s.client.FetchUploadsForKey(s.cfg.APIKey, s.detail.GameID, downloadKeyID)
	if authErr != nil {
		s.err = authErr
		s.state = fetchError
		s.isNotOwned = strings.Contains(authErr.Error(), "not owned")
		return
	}
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
		return
	}
	s.state = fetchDone
}

func (s *FetchUploadsScreen) NeedsRedraw() bool {
	return true
}
func (s *FetchUploadsScreen) HasPendingAnimation() bool { return false }

func (s *FetchUploadsScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(40)
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 6
	contentH := r.H - headerH - footerH
	mid := contentTop + contentH/2

	switch s.state {
	case fetchLoading:
		r.DrawTextCentered("Finding available files...", 0, mid-mainFH/2, r.W, mt[0], mt[1], mt[2])

	case fetchError:
		r.DrawText("Could not fetch files:", 20, mid-mainFH-smallFH-8, 200, 60, 60)
		msg := s.err.Error()
		r.DrawWrappedText(msg, 20, mid-smallFH, r.W-40, smallFH+4, 200, 100, 100)

	case fetchDone, fetchNeedsPurchasePick:
		// Handled by transitioning in HandleEvent / next Draw cycle below
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.state {
	case fetchLoading:
		r.DrawSmallText("Please wait...", 10, ftrY, ht[0], ht[1], ht[2])
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *FetchUploadsScreen) HandleEvent(e sdl.Event) Screen {
	// Transition immediately when fetch completes (triggered by UserEvent).
	if s.state == fetchDone {
		return s.nextScreen()
	}
	if s.state == fetchNeedsPurchasePick {
		return NewPurchasePickerScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, s.ownedKeys, s.inv, s.inventoryPath, s.prev)
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
			if existing := s.inv.ExistingDestPath(s.game.URL, upload.Filename); existing != "" {
				dest = existing
			}
			return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
		}
		return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, known, s.inv, s.inventoryPath, s.prev)
	}
	return NewFormatPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, unknown, s.inv, s.inventoryPath, s.prev)
}
