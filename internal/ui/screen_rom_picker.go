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
	client        *itchio.Client
	cfg           *settings.Config
	cfgPath       string
	cache         *renderer.ImageCache
	game          itchio.Game
	detail        *itchio.GameDetail
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

func (s *ROMPickerScreen) NeedsRedraw() bool { return false }
func (s *ROMPickerScreen) HasPendingAnimation() bool { return false }

func (s *ROMPickerScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16

	hBG := r.Theme.Surface()
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, ht[0], ht[1], ht[2])

	// Subheader label
	contentTop := headerH + 10
	r.DrawSmallText("Select file to download:", 20, contentTop, 180, 180, 180)
	contentTop += smallFH + 10

	rowH := fontH + 14
	for i, u := range s.uploads {
		y := contentTop + int32(i)*rowH
		if i == s.cursor {
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		var tr, tg, tb uint8
		if i == s.cursor {
			c := r.Theme.AccentText
			tr, tg, tb = c[0], c[1], c[2]
		} else {
			c := r.Theme.ListText
			tr, tg, tb = c[0], c[1], c[2]
		}
		r.DrawText(u.Filename, 20, y, tr, tg, tb)
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}, ftrY)
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
	}
	return s
}

func (s *ROMPickerScreen) chooseUpload(upload roms.Upload) Screen {
	// ZIP and 7z uploads go through ZIPInspectScreen for smart content handling.
	if ext := strings.ToLower(filepath.Ext(upload.Filename)); ext == ".zip" || ext == ".7z" {
		return NewZIPInspectScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
	}
	if s.cfg.ROMLocation == "ask" {
		return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
	}
	ext := strings.ToLower(roms.ROMExt(upload.Filename))
	dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + upload.Filename
	if existing := s.inv.ExistingDestPath(s.game.URL, upload.Filename); existing != "" {
		dest = existing
	}
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
}
