//go:build !headless

package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type zipContentRowKind int

const (
	zipRowROM        zipContentRowKind = iota // selectable ROM entry for version picker
	zipRowMusicToggle                         // music download yes/no toggle
)

type zipContentRow struct {
	kind    zipContentRowKind
	entry   roms.ZIPEntry
	ext     string // lowercase extension group, for zipRowROM
	toggled bool   // for zipRowMusicToggle
}

// ZIPContentsScreen shows the ZIP manifest to the user for two cases:
//  1. Multiple ROMs of same extension → version picker (user selects one per ext).
//  2. MusicDownload="ask" → music toggle (user chooses whether to download music).
type ZIPContentsScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	plan    ZIPPlan
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	rows         []zipContentRow
	cursor       int
	selectedROMs map[string]string // ext → selected Name
}

func NewZIPContentsScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache,
	game itchio.Game, detail *itchio.GameDetail, plan ZIPPlan,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *ZIPContentsScreen {
	s := &ZIPContentsScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, plan: plan,
		inv: inv, invPath: invPath, prev: prev,
		selectedROMs: make(map[string]string),
	}
	s.buildRows()
	return s
}

func (s *ZIPContentsScreen) buildRows() {
	s.rows = nil
	byExt := s.plan.Manifest.ROMsByExt()

	for ext, entries := range byExt {
		if len(entries) < 2 {
			s.selectedROMs[ext] = entries[0].Name
			continue
		}
		if _, ok := s.selectedROMs[ext]; !ok {
			s.selectedROMs[ext] = entries[0].Name
		}
		for _, e := range entries {
			s.rows = append(s.rows, zipContentRow{kind: zipRowROM, entry: e, ext: ext})
		}
	}

	if s.cfg.MusicDownload == "ask" && s.plan.Manifest.HasMusic() {
		s.rows = append(s.rows, zipContentRow{kind: zipRowMusicToggle, toggled: false})
	}
}

func (s *ZIPContentsScreen) NeedsRedraw() bool        { return false }
func (s *ZIPContentsScreen) HasPendingAnimation() bool { return false }

func (s *ZIPContentsScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	at := r.Theme.AccentText
	lt := r.Theme.ListText
	mt := r.Theme.MainText
	ht := r.Theme.HintText

	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	y := headerH + 10
	m := s.plan.Manifest
	summary := fmt.Sprintf("ZIP contains: %d ROM(s)", m.ROMCount())
	if m.HasMusic() {
		summary += fmt.Sprintf("  ·  %d music track(s)", m.MusicCount())
	}
	r.DrawSmallText(summary, 20, y, 140, 140, 140)
	y += smallFH + 12

	rowH := mainFH + 14
	for i, row := range s.rows {
		selected := i == s.cursor
		if selected {
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		var tr, tg, tb uint8
		if selected {
			tr, tg, tb = at[0], at[1], at[2]
		} else {
			tr, tg, tb = lt[0], lt[1], lt[2]
		}

		switch row.kind {
		case zipRowROM:
			marker := "  "
			if s.selectedROMs[row.ext] == row.entry.Name {
				marker = "● "
			}
			r.DrawText(marker+row.entry.Name, 20, y, tr, tg, tb)
		case zipRowMusicToggle:
			val := "NO"
			if row.toggled {
				val = "YES"
			}
			label := "Download soundtrack: "
			r.DrawText(label+val, 20, y, tr, tg, tb)
		}
		y += rowH
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Select/Toggle"},
		{Kind: renderer.BadgePill, Label: "START", Text: "Confirm"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Back"},
	}, ftrY)
	r.Present()
}

func (s *ZIPContentsScreen) HandleEvent(e sdl.Event) Screen {
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
			s.activate()
		case sdl.K_s:
			return s.confirm()
		case sdl.K_ESCAPE:
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
			s.activate()
		case sdl.CONTROLLER_BUTTON_START:
			return s.confirm()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	}
	return s
}

func (s *ZIPContentsScreen) activate() {
	if s.cursor >= len(s.rows) {
		return
	}
	row := s.rows[s.cursor]
	switch row.kind {
	case zipRowROM:
		s.selectedROMs[row.ext] = row.entry.Name
	case zipRowMusicToggle:
		s.rows[s.cursor].toggled = !s.rows[s.cursor].toggled
	}
}

func (s *ZIPContentsScreen) confirm() Screen {
	plan := s.plan
	plan.SelectedROMs = s.selectedROMs
	plan.DownloadROMs = true
	plan.DownloadMusic = false

	for _, row := range s.rows {
		if row.kind == zipRowMusicToggle {
			plan.DownloadMusic = row.toggled
			break
		}
	}
	if s.cfg.MusicDownload == "auto" && s.plan.Manifest.HasMusic() {
		plan.DownloadMusic = true
	}

	if plan.DownloadMusic {
		if s.cfg.MusicLocation == "ask" {
			return NewMusicLocationPickerScreen(s.client, s.cfg, s.cfgPath,
				s.game, s.detail, plan, s.inv, s.invPath, s.prev)
		}
		plan.MusicDir = roms.MusicDestinationDir(s.game.Title)
	}

	logger.Info("zip-contents: confirmed ROMs=%v music=%v musicDir=%s", plan.DownloadROMs, plan.DownloadMusic, plan.MusicDir)
	return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
}
