//go:build !headless

package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// locationRoot is the highest directory the user can navigate to.
const locationRoot = "/mnt/SDCARD"

type rowKind int

const (
	rowSaveHere rowKind = iota
	rowUp
	rowEntry
)

// pickerRow represents one navigable item in the directory browser.
type pickerRow struct {
	kind rowKind
	name string // only set for rowEntry; holds the subdirectory name
}

// LocationPickerScreen lets the user navigate the SD card filesystem and
// choose a destination directory for a ROM download.
type LocationPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	game    itchio.Game
	detail  *itchio.GameDetail
	upload  roms.Upload
	prev    Screen

	ext          string      // lowercase extension e.g. ".gbc"
	currentDir   string      // always ends with "/"
	rows         []pickerRow // [rowSaveHere, optional rowUp, zero or more rowEntry]
	cursor       int         // index into rows
	scrollOffset int         // index into rows[] of the first displayed row (for rows[1:])

	inv           *inventory.Inventory
	inventoryPath string
}

// NewLocationPickerScreen creates a directory browser that opens at the
// remembered path for this file extension (or the default destination if no
// remembered path exists or the remembered path no longer exists on disk).
func NewLocationPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail, upload roms.Upload,
	inv *inventory.Inventory, inventoryPath string,
	prev Screen,
) *LocationPickerScreen {
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	startDir := resolveStartDir(cfg, ext, cfgPath)
	s := &LocationPickerScreen{
		client:        client,
		cfg:           cfg,
		cfgPath:       cfgPath,
		game:          game,
		detail:        detail,
		upload:        upload,
		prev:          prev,
		ext:           ext,
		inv:           inv,
		inventoryPath: inventoryPath,
	}
	s.loadDir(startDir)
	return s
}

// resolveStartDir returns the directory the browser should open at.
// If the remembered path for ext no longer exists on disk it is removed from
// cfg and cfg is saved before returning the default destination.
func resolveStartDir(cfg *settings.Config, ext, cfgPath string) string {
	if cfg.LastROMDirs != nil {
		if dir, ok := cfg.LastROMDirs[ext]; ok && dir != "" {
			if _, err := os.Stat(dir); err == nil {
				return dir // remembered path is valid — use it
			}
			// Stale path — forget it and fall through to default
			delete(cfg.LastROMDirs, ext)
			if len(cfg.LastROMDirs) == 0 {
				cfg.LastROMDirs = nil
			}
			cfg.Save(cfgPath) //nolint:errcheck — best-effort cleanup
		}
	}
	return roms.DestinationDir(ext)
}

// loadDir switches the browser to dir, rebuilds the row list, and resets the
// cursor to "Save here" (index 0).
func (s *LocationPickerScreen) loadDir(dir string) {
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	s.currentDir = dir
	s.rows = buildRows(dir)
	s.cursor = 0
	s.scrollOffset = 0
}

// buildRows constructs the navigable row list for dir:
//   - index 0:   rowSaveHere  (always)
//   - index 1:   rowUp        (omitted when dir == locationRoot)
//   - index 1+:  rowEntry     (one per visible subdirectory, sorted case-insensitively)
func buildRows(dir string) []pickerRow {
	rows := []pickerRow{{kind: rowSaveHere}}

	atRoot := strings.TrimRight(dir, "/") == locationRoot
	if !atRoot {
		rows = append(rows, pickerRow{kind: rowUp})
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return rows // e.g. permission denied — graceful degradation
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	for _, name := range names {
		rows = append(rows, pickerRow{kind: rowEntry, name: name})
	}
	return rows
}

// atRoot reports whether the browser is already at locationRoot.
func (s *LocationPickerScreen) atRoot() bool {
	return strings.TrimRight(s.currentDir, "/") == locationRoot
}

// clampScroll adjusts scrollOffset so that cursor is always visible.
// visibleCount is the number of non-header rows that fit on screen.
func (s *LocationPickerScreen) clampScroll(visibleCount int) {
	// rows[0] is always the confirm row (drawn separately), so we work in
	// terms of the sub-list rows[1:]. cursor==0 is always visible.
	if s.cursor == 0 {
		return
	}
	// sub-list index: cursor-1 (since rows[0] is drawn outside the list)
	idx := s.cursor - 1
	if idx < s.scrollOffset {
		s.scrollOffset = idx
	}
	if idx >= s.scrollOffset+visibleCount {
		s.scrollOffset = idx - visibleCount + 1
	}
}

func (s *LocationPickerScreen) NeedsRedraw() bool { return false }
func (s *LocationPickerScreen) HasPendingAnimation() bool { return false }

func (s *LocationPickerScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	footerH := int32(52)
	ac := r.Theme.Accent
	hBG := r.Theme.HeaderBG

	// ── Header ──────────────────────────────────────────────────────────────
	headerH := mainFH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	// ── Path bar ────────────────────────────────────────────────────────────
	pathBarY := headerH + 2
	pathBarH := smallFH + 10
	r.DrawRect(0, pathBarY, r.W, pathBarH, hBG[0], hBG[1], hBG[2])
	pathText := leftTruncatePath(r, s.currentDir, r.W-24)
	r.DrawSmallText(pathText, 12, pathBarY+5, 120, 160, 200)

	// ── Confirm row (pinned first, distinct green tint) ──────────────────────
	confirmY := pathBarY + pathBarH
	confirmH := mainFH + 10
	if s.cursor == 0 {
		r.DrawRect(0, confirmY, r.W, confirmH, 26, 58, 34)
	} else {
		r.DrawRect(0, confirmY, r.W, confirmH, 15, 32, 22)
	}
	r.DrawText("[ \u2713  Save here ]", 12, confirmY+5, 80, 200, 120)
	r.DrawRect(0, confirmY+confirmH, r.W, 1, 28, 58, 28)

	// ── Directory list (rows[1:]) ────────────────────────────────────────────
	listTop := confirmY + confirmH + 2
	rowH := mainFH + 14

	visibleCount := int((r.H - footerH - listTop) / rowH)
	if visibleCount < 1 {
		visibleCount = 1
	}
	s.clampScroll(visibleCount)

	hasEntries := false
	for _, row := range s.rows {
		if row.kind == rowEntry {
			hasEntries = true
			break
		}
	}

	listRowsDrawn := int32(0)
	for i := 1 + s.scrollOffset; i < len(s.rows); i++ {
		row := s.rows[i]
		y := listTop + listRowsDrawn*rowH
		if y+rowH > r.H-footerH {
			break
		}
		selected := s.cursor == i
		aT := r.Theme.AccentText
		lt := r.Theme.ListText
		switch row.kind {
		case rowUp:
			if selected {
				r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			var tr, tg, tb uint8
			if selected {
				tr, tg, tb = aT[0], aT[1], aT[2]
			} else {
				tr, tg, tb = 100, 140, 180
			}
			r.DrawSmallText("\u2191  .. (go up)", 20, y+(rowH-smallFH)/2, tr, tg, tb)
		case rowEntry:
			if selected {
				r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			var tr, tg, tb uint8
			if selected {
				tr, tg, tb = aT[0], aT[1], aT[2]
			} else {
				tr, tg, tb = lt[0], lt[1], lt[2]
			}
			r.DrawText("\u25b8 "+row.name, 20, y, tr, tg, tb)
		}
		listRowsDrawn++
	}

	// Show placeholder when no subdirectories exist in this folder.
	if !hasEntries {
		y := listTop + listRowsDrawn*rowH
		r.DrawSmallText("  (no subfolders)", 20, y, 80, 80, 80)
	}

	// ── Footer ───────────────────────────────────────────────────────────────
	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints(s.footerHints(), ftrY)
	r.Present()
}

// footerHints returns context-sensitive FooterHint items for DrawFooterHints.
func (s *LocationPickerScreen) footerHints() []renderer.FooterHint {
	hints := []renderer.FooterHint{}
	switch {
	case s.cursor == 0 && s.atRoot():
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Confirm"})
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Cancel"})
	case s.cursor == 0:
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Confirm"})
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "A", Text: "go up"})
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Cancel"})
	case s.atRoot():
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "enter dir"})
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Cancel"})
	default:
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "confirm/enter"})
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "A", Text: "go up"})
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgePill, Label: "START", Text: "Cancel"})
	}
	return hints
}

func (s *LocationPickerScreen) HandleEvent(e sdl.Event) Screen {
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
		case sdl.K_RETURN: // B button
			return s.activate()
		case sdl.K_ESCAPE: // A button
			return s.goUp()
		case sdl.K_s: // Start button
			return s.prev // cancel, no download
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
			return s.prev // cancel, no download
		}
	}
	return s
}

// activate handles a B-button press based on what the cursor points to.
func (s *LocationPickerScreen) activate() Screen {
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

// goUp navigates to the parent directory. Does nothing when already at root.
func (s *LocationPickerScreen) goUp() Screen {
	if s.atRoot() {
		return s
	}
	parent := filepath.Dir(strings.TrimRight(s.currentDir, "/"))
	s.loadDir(parent)
	return s
}

// confirm saves the chosen directory to config and proceeds to download.
func (s *LocationPickerScreen) confirm() Screen {
	if s.cfg.LastROMDirs == nil {
		s.cfg.LastROMDirs = make(map[string]string)
	}
	s.cfg.LastROMDirs[s.ext] = s.currentDir
	s.cfg.Save(s.cfgPath) //nolint:errcheck — best-effort persistence
	dest := s.currentDir + s.upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.upload, dest, s.inv, s.inventoryPath, s.prev)
}

// leftTruncatePath shortens text from the left with a "…" prefix so it fits
// within maxW pixels when rendered with the small font. Used for long paths.
func leftTruncatePath(r *renderer.Renderer, text string, maxW int32) string {
	w, _ := r.SmallTextSize(text)
	if int32(w) <= maxW {
		return text
	}
	const ellipsis = "\u2026"
	runes := []rune(text)
	for len(runes) > 1 {
		runes = runes[1:]
		w, _ = r.SmallTextSize(ellipsis + string(runes))
		if int32(w) <= maxW {
			return ellipsis + string(runes)
		}
	}
	return ellipsis
}
