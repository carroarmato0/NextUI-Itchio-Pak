//go:build !headless

package ui

import (
	"os"
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

const musicLocationRoot = "/mnt/SDCARD/Music"

// MusicLocationPickerScreen lets the user choose where to save a game soundtrack.
// It mirrors LocationPickerScreen but is rooted at /mnt/SDCARD/Music and
// routes to ZIPDownloadScreen on confirmation.
type MusicLocationPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	game    itchio.Game
	detail  *itchio.GameDetail
	plan    ZIPPlan
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	currentDir   string
	rows         []pickerRow
	cursor       int
	scrollOffset int
}

func NewMusicLocationPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail, plan ZIPPlan,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *MusicLocationPickerScreen {
	startDir := roms.MusicDestinationDir(game.Title)
	if _, err := os.Stat(startDir); err != nil {
		startDir = musicLocationRoot + "/"
	}
	s := &MusicLocationPickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath,
		game: game, detail: detail, plan: plan,
		inv: inv, invPath: invPath, prev: prev,
	}
	s.loadDir(startDir)
	return s
}

func (s *MusicLocationPickerScreen) loadDir(dir string) {
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	s.currentDir = dir
	s.rows = buildRows(dir, musicLocationRoot)
	s.cursor = 0
	s.scrollOffset = 0
}

func (s *MusicLocationPickerScreen) atRoot() bool {
	return strings.TrimRight(s.currentDir, "/") == musicLocationRoot
}

func (s *MusicLocationPickerScreen) NeedsRedraw() bool        { return false }
func (s *MusicLocationPickerScreen) HasPendingAnimation() bool { return false }

func (s *MusicLocationPickerScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	footerH := int32(52)
	ac := r.Theme.Accent
	hBG := r.Theme.Surface()
	mt := r.Theme.MainText
	ht := r.Theme.HintText
	at := r.Theme.AccentText
	lt := r.Theme.ListText

	headerH := mainFH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	pathBarY := headerH + 2
	pathBarH := smallFH + 10
	r.DrawRect(0, pathBarY, r.W, pathBarH, hBG[0], hBG[1], hBG[2])
	r.DrawSmallText(leftTruncatePath(r, s.currentDir, r.W-24), 12, pathBarY+5, 120, 160, 200)

	confirmY := pathBarY + pathBarH
	confirmH := mainFH + 10
	if s.cursor == 0 {
		r.DrawRect(0, confirmY, r.W, confirmH, 26, 58, 34)
	} else {
		r.DrawRect(0, confirmY, r.W, confirmH, 15, 32, 22)
	}
	r.DrawText("[ ✓  Save here ]", 12, confirmY+5, 80, 200, 120)
	r.DrawRect(0, confirmY+confirmH, r.W, 1, 28, 58, 28)

	listTop := confirmY + confirmH + 2
	rowH := mainFH + 14
	visibleCount := int((r.H - footerH - listTop) / rowH)
	if visibleCount < 1 {
		visibleCount = 1
	}
	s.clampScroll(visibleCount)

	var listRowsDrawn int32
	for i := 1 + s.scrollOffset; i < len(s.rows); i++ {
		row := s.rows[i]
		y := listTop + listRowsDrawn*rowH
		if y+rowH > r.H-footerH {
			break
		}
		selected := s.cursor == i
		switch row.kind {
		case rowUp:
			if selected {
				r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			var tr, tg, tb uint8
			if selected {
				tr, tg, tb = at[0], at[1], at[2]
			} else {
				tr, tg, tb = 100, 140, 180
			}
			r.DrawSmallText("↑  .. (go up)", 20, y+(rowH-smallFH)/2, tr, tg, tb)
		case rowEntry:
			if selected {
				r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			var tr, tg, tb uint8
			if selected {
				tr, tg, tb = at[0], at[1], at[2]
			} else {
				tr, tg, tb = lt[0], lt[1], lt[2]
			}
			r.DrawText("▸ "+row.name, 20, y, tr, tg, tb)
		}
		listRowsDrawn++
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Confirm/Enter"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Go up"},
		{Kind: renderer.BadgePill, Label: "START", Text: "Cancel"},
	}, ftrY)
	r.Present()
}

func (s *MusicLocationPickerScreen) clampScroll(visibleCount int) {
	if s.cursor == 0 {
		return
	}
	idx := s.cursor - 1
	if idx < s.scrollOffset {
		s.scrollOffset = idx
	}
	if idx >= s.scrollOffset+visibleCount {
		s.scrollOffset = idx - visibleCount + 1
	}
}

func (s *MusicLocationPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			return s.activate()
		case sdl.K_ESCAPE:
			return s.goUp()
		case sdl.K_s:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.activate()
		case sdl.CONTROLLER_BUTTON_A:
			return s.goUp()
		case sdl.CONTROLLER_BUTTON_START:
			return s.prev
		}
	}
	return s
}

func (s *MusicLocationPickerScreen) activate() Screen {
	if s.cursor >= len(s.rows) {
		return s
	}
	row := s.rows[s.cursor]
	switch row.kind {
	case rowSaveHere:
		return s.confirm()
	case rowUp:
		return s.goUp()
	case rowEntry:
		s.loadDir(s.currentDir + row.name + "/")
	}
	return s
}

func (s *MusicLocationPickerScreen) goUp() Screen {
	if s.atRoot() {
		return s
	}
	parent := filepath.Dir(strings.TrimRight(s.currentDir, "/"))
	s.loadDir(parent)
	return s
}

func (s *MusicLocationPickerScreen) confirm() Screen {
	plan := s.plan
	plan.MusicDir = s.currentDir
	logger.Info("music-location: confirmed musicDir=%s", plan.MusicDir)
	return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
}
