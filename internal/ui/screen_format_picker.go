//go:build !headless

package ui

import (
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

type formatChoice int

const (
	formatGB  formatChoice = iota // .gb
	formatGBC                     // .gbc
	formatZIP                     // .zip
)

func (f formatChoice) ext() string {
	switch f {
	case formatGB:
		return ".gb"
	case formatGBC:
		return ".gbc"
	case formatZIP:
		return ".zip"
	}
	return ".gbc"
}

func (f formatChoice) label() string {
	switch f {
	case formatGB:
		return "[GB]"
	case formatGBC:
		return "[GBC]"
	case formatZIP:
		return "[ZIP]"
	}
	return "[GBC]"
}

func (f formatChoice) next() formatChoice { return (f + 1) % 3 }
func (f formatChoice) prev() formatChoice { return (f + 2) % 3 }

// defaultFormatChoice returns ZIP when the filename already ends in .zip,
// GBC for everything else (most GB Studio games target Game Boy Color).
func defaultFormatChoice(filename string) formatChoice {
	if strings.ToLower(filepath.Ext(filename)) == ".zip" {
		return formatZIP
	}
	return formatGBC
}

// FormatPickerScreen is shown when a game's uploads have no recognized .gb/.gbc
// extension. The user selects a file and chooses GB, GBC, or ZIP before the
// download proceeds. The chosen extension is appended to the filename so all
// downstream routing (DestinationDir, LastROMDirs, cover-art) works correctly.
type FormatPickerScreen struct {
	client        *itchio.Client
	cfg           *settings.Config
	cfgPath       string
	game          itchio.Game
	detail        *itchio.GameDetail
	uploads       []roms.Upload
	formats       []formatChoice // parallel to uploads
	cursor        int
	prev          Screen
	inv           *inventory.Inventory
	inventoryPath string
}

func NewFormatPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail,
	uploads []roms.Upload, inv *inventory.Inventory, inventoryPath string, prev Screen,
) *FormatPickerScreen {
	formats := make([]formatChoice, len(uploads))
	for i, u := range uploads {
		formats[i] = defaultFormatChoice(u.Filename)
	}
	return &FormatPickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath,
		game: game, detail: detail,
		uploads: uploads, formats: formats,
		prev: prev,
		inv: inv, inventoryPath: inventoryPath,
	}
}

func (s *FormatPickerScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16

	hBG := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 8
	r.DrawSmallText("No .gb/.gbc detected — choose file and format:", 12, contentTop, 180, 160, 100)
	contentTop += smallFH + 10

	rowH := fontH + 20
	const tagMargin = int32(8)
	const badgePad = int32(4)
	maxTagW, _ := r.SmallTextSize("[GBC]") // widest label — used for a stable filename budget
	badgeW := maxTagW + badgePad*2
	badgeH := smallFH + badgePad

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
		name := truncateToWidth(r, u.Filename, r.W-badgeW-tagMargin-20-12)
		r.DrawText(name, 20, y, tr, tg, tb)

		f := s.formats[i]
		lbl := f.label()
		tagX := r.W - badgeW - tagMargin
		badgeY := y + (fontH-badgeH)/2
		var fR, fG, fB uint8
		switch f {
		case formatGB:
			fR, fG, fB = 120, 220, 120
		case formatGBC:
			fR, fG, fB = 80, 180, 255
		case formatZIP:
			fR, fG, fB = 220, 180, 80
		}
		r.DrawPill(tagX, badgeY, badgeW, badgeH, fR, fG, fB)
		r.DrawSmallText(lbl, tagX+badgePad, badgeY+badgePad/2, 20, 20, 20)
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgePill, Label: "↕", Text: "Select"},
		{Kind: renderer.BadgePill, Label: "L/R", Text: "Format"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Download"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}, ftrY)
	r.Present()
}

func (s *FormatPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.K_LEFT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].prev()
			}
		case sdl.K_RIGHT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].next()
			}
		case sdl.K_RETURN:
			return s.confirm()
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].prev()
			}
		case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].next()
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.confirm()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	}
	return s
}

func (s *FormatPickerScreen) confirm() Screen {
	if s.cursor >= len(s.uploads) {
		return s
	}
	original := s.uploads[s.cursor]
	chosenExt := s.formats[s.cursor].ext()

	upload := original
	// Only append the extension if the file does not already carry it,
	// to avoid producing names like "game.zip.zip".
	if strings.ToLower(filepath.Ext(original.Filename)) != chosenExt {
		upload.Filename = original.Filename + chosenExt
	}
	logger.Info("format-picker: %q → %s", original.Filename,
		strings.ToUpper(strings.TrimPrefix(chosenExt, ".")))

	if s.cfg.ROMLocation == "ask" {
		return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
	}
	dest := roms.DestinationDir(chosenExt) + upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
}
