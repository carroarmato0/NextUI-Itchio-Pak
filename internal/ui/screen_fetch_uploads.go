//go:build !headless

package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
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

	state   fetchState
	uploads []roms.Upload
	err     error
}

func NewFetchUploadsScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail,
	prev Screen,
) *FetchUploadsScreen {
	s := &FetchUploadsScreen{
		client: client, cfg: cfg, cfgPath: cfgPath,
		cache: cache, game: game, detail: detail, prev: prev,
		state: fetchLoading,
	}
	go func() {
		itchUploads, err := client.FetchUploads(game.URL)
		if err != nil {
			s.err = err
			s.state = fetchError
		} else {
			for _, u := range itchUploads {
				s.uploads = append(s.uploads, roms.Upload{Filename: u.Filename, URL: u.URL})
			}
			if len(s.uploads) == 0 {
				s.err = fmt.Errorf("no .gb or .gbc files found for this game")
				s.state = fetchError
			} else {
				s.state = fetchDone
			}
		}
		// Wake up the SDL event loop so HandleEvent fires immediately.
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

func (s *FetchUploadsScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	headerH := int32(56)
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	title := s.game.Title
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawText("by "+s.game.Author, 12, 32, 140, 140, 140)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)

	mid := r.H / 2

	switch s.state {
	case fetchLoading:
		r.DrawText("Finding available files...", 20, mid-10, colorText, colorText, colorText)
		r.DrawText("B: cancel", 10, r.H-24, 140, 140, 140)

	case fetchError:
		r.DrawText("Could not fetch files:", 20, mid-30, 200, 60, 60)
		msg := s.err.Error()
		if len(msg) > 70 {
			msg = msg[:67] + "..."
		}
		r.DrawText(msg, 20, mid, 200, 100, 100)
		r.DrawText("B: back", 10, r.H-24, 140, 140, 140)

	case fetchDone:
		// Handled by transitioning in HandleEvent / next Draw cycle below
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
		// Fetch finished (error state) — stay on this screen to show the error.
		_ = ev
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
	if len(s.uploads) == 1 {
		// Single file — go straight to download
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.uploads[0], s.prev)
	}
	// Multiple files — always show picker so the user can choose
	return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s.uploads, s.prev)
}
