//go:build !headless

package ui

import (
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type ROMPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	uploads       []roms.Upload
	cursor        int
	prev          Screen
	inv           *inventory.Inventory
	inventoryPath string
}

func NewROMPickerScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail, uploads []roms.Upload, inv *inventory.Inventory, inventoryPath string, prev Screen) *ROMPickerScreen {
	return &ROMPickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, uploads: uploads, prev: prev,
		inv: inv, inventoryPath: inventoryPath,
	}
}

func (s *ROMPickerScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	footerH := int32(40)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16

	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, 140, 140, 140)

	// Subheader label
	contentTop := headerH + 10
	r.DrawSmallText("Select file to download:", 20, contentTop, 180, 180, 180)
	contentTop += smallFH + 10

	rowH := fontH + 14
	for i, u := range s.uploads {
		y := contentTop + int32(i)*rowH
		if i == s.cursor {
			r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(u.Filename, 20, y, colorText, colorText, colorText)
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText("B: select  |  A: back", 10, ftrY, 140, 140, 140)
	r.Present()
}

func (s *ROMPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			if s.cursor < len(s.uploads) {
				return s.chooseUpload(s.uploads[s.cursor])
			}
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			if s.cursor < len(s.uploads) {
				return s.chooseUpload(s.uploads[s.cursor])
			}
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *ROMPickerScreen) chooseUpload(upload roms.Upload) Screen {
	if s.cfg.ROMLocation == "ask" {
		return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
	}
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
}
