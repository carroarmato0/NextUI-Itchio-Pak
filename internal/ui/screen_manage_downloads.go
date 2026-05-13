//go:build !headless

package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	deleteIdxAll        = -1 // delete entire game entry
	deleteIdxROMs       = -2 // delete only ROM-typed files
	deleteIdxSoundtrack = -3 // delete only music-typed files
)

type ManageDownloadsScreen struct {
	inv           *inventory.Inventory
	inventoryPath string
	gameURL       string
	cursor        int // 0..len(files)-1 = file rows; len(files) = "Delete all"

	confirmActive  bool
	confirmFileIdx int // deleteIdxAll/deleteIdxROMs/deleteIdxSoundtrack or index into entry.Files

	cfg  *settings.Config
	prev Screen
}

func NewManageDownloadsScreen(inv *inventory.Inventory, inventoryPath string, gameURL string, cfg *settings.Config, prev Screen) *ManageDownloadsScreen {
	return &ManageDownloadsScreen{
		inv:            inv,
		inventoryPath:  inventoryPath,
		gameURL:        gameURL,
		cfg:            cfg,
		prev:           prev,
		confirmFileIdx: -1,
	}
}

func (s *ManageDownloadsScreen) NeedsRedraw() bool { return true }
func (s *ManageDownloadsScreen) HasPendingAnimation() bool { return false }

func hasFileType(files []inventory.DownloadedFile, ft string) bool {
	for _, f := range files {
		if f.FileType == ft || (ft == inventory.FileTypeROM && f.FileType == "") {
			return true
		}
	}
	return false
}

func (s *ManageDownloadsScreen) Draw(r *renderer.Renderer) {
	entry, ok := s.inv.Lookup(s.gameURL)
	if !ok {
		r.Present()
		return
	}

	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])
	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	headerH := fontH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	title := truncateToWidth(r, "Manage Downloads — "+entry.Title, r.W-24)
	r.DrawText(title, 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+entry.Author, 12, 8+fontH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 10
	rowH := fontH + 14
	margin := int32(20)

	lt := r.Theme.ListText
	at := r.Theme.AccentText
	arrowLabel := "→  "
	arrowW, _ := r.SmallTextSize(arrowLabel)
	for i, f := range entry.Files {
		y := contentTop + int32(i)*rowH
		selected := i == s.cursor && !s.confirmActive
		if selected {
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		var nr, ng, nb uint8
		if selected {
			nr, ng, nb = at[0], at[1], at[2]
		} else {
			nr, ng, nb = lt[0], lt[1], lt[2]
		}
		nameMaxW := r.W/2 - margin
		r.DrawScrollingText(f.Filename, margin, y, nameMaxW, nr, ng, nb)
		nameW, _ := r.TextSize(f.Filename)
		if nameW > nameMaxW {
			nameW = nameMaxW
		}
		pathX := margin + nameW + 12
		pathMaxW := r.W - pathX - margin
		if pathMaxW > arrowW {
			r.DrawSmallText(arrowLabel, pathX, y+(fontH-smallFH)/2, 120, 120, 120)
			r.DrawSmallScrollingText(f.DestPath, pathX+arrowW, y+(fontH-smallFH)/2, pathMaxW-arrowW, 120, 120, 120)
		}
	}

	sepY := contentTop + int32(len(entry.Files))*rowH
	r.DrawRect(margin, sepY, r.W-margin*2, 1, 50, 50, 50)
	actionY := sepY + 8
	deleteAllIdx := len(entry.Files)

	hasROM := hasFileType(entry.Files, inventory.FileTypeROM)
	hasMusic := hasFileType(entry.Files, inventory.FileTypeMusic)

	if hasROM && hasMusic {
		if s.cursor == deleteAllIdx && !s.confirmActive {
			r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		r.DrawText("Delete ROM", margin, actionY, 200, 120, 80)
		actionY += rowH

		if s.cursor == deleteAllIdx+1 && !s.confirmActive {
			r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		r.DrawText("Delete Soundtrack", margin, actionY, 200, 120, 80)
		actionY += rowH

		if s.cursor == deleteAllIdx+2 && !s.confirmActive {
			r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		r.DrawText("Delete all", margin, actionY, 200, 80, 80)
		actionY += rowH
	} else {
		if s.cursor == deleteAllIdx && !s.confirmActive {
			r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		r.DrawText("Delete all", margin, actionY, 200, 80, 80)
		actionY += rowH
	}

	sep2Y := actionY + 4
	r.DrawRect(margin, sep2Y, r.W-margin*2, 1, 50, 50, 50)
	toggleY := sep2Y + 8
	toggleIdx := len(entry.Files) + 1
	if hasROM && hasMusic {
		toggleIdx = len(entry.Files) + 3
	}
	unifiedDisabled := entry.UnifiedNamingDisabled
	toggleLabel := "Use game title as filename"
	toggleVal := "ON"
	if unifiedDisabled {
		toggleVal = "OFF"
	}
	if s.cursor == toggleIdx && !s.confirmActive {
		r.DrawPill(4, toggleY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		r.DrawText(toggleLabel, margin, toggleY, at[0], at[1], at[2])
		tw, _ := r.TextSize(toggleLabel)
		r.DrawText(toggleVal, margin+tw+16, toggleY, at[0], at[1], at[2])
	} else {
		textColor := [3]uint8{lt[0], lt[1], lt[2]}
		if !s.cfg.UnifiedNaming {
			textColor = [3]uint8{80, 80, 80}
		}
		r.DrawText(toggleLabel, margin, toggleY, textColor[0], textColor[1], textColor[2])
		tw, _ := r.TextSize(toggleLabel)
		r.DrawText(toggleVal, margin+tw+16, toggleY, textColor[0], textColor[1], textColor[2])
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Back"},
	}, ftrY)

	if s.confirmActive {
		s.drawConfirmOverlay(r, entry)
	}

	r.Present()
}

func (s *ManageDownloadsScreen) drawConfirmOverlay(r *renderer.Renderer, entry inventory.Entry) {
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	pad := int32(16)

	var title, body string
	switch s.confirmFileIdx {
	case deleteIdxAll:
		title = fmt.Sprintf("Delete all %d file(s)?", len(entry.Files))
		var names []string
		for _, f := range entry.Files {
			names = append(names, f.Filename)
		}
		body = strings.Join(names, "\n")
	case deleteIdxROMs:
		var names []string
		count := 0
		for _, f := range entry.Files {
			if f.FileType == inventory.FileTypeROM || f.FileType == "" {
				names = append(names, f.Filename)
				count++
			}
		}
		title = fmt.Sprintf("Delete %d ROM file(s)?", count)
		body = strings.Join(names, "\n")
	case deleteIdxSoundtrack:
		var names []string
		count := 0
		for _, f := range entry.Files {
			if f.FileType == inventory.FileTypeMusic {
				names = append(names, f.Filename)
				count++
			}
		}
		title = fmt.Sprintf("Delete %d soundtrack file(s)?", count)
		body = strings.Join(names, "\n")
	default:
		title = "Delete this file?"
		f := entry.Files[s.confirmFileIdx]
		body = f.Filename + "\n" + f.DestPath
	}

	lineH := smallFH + 4
	bodyLineCount := int32(strings.Count(body, "\n") + 1)
	boxW := r.W * 2 / 3
	boxH := pad + fontH + pad + 2 + pad + lineH*bodyLineCount + pad + 2 + pad + smallFH + pad
	boxX := (r.W - boxW) / 2
	boxY := (r.H - boxH) / 2

	r.DrawRect(0, 0, r.W, r.H, 10, 10, 15)
	r.DrawRect(boxX-1, boxY-1, boxW+2, boxH+2, 70, 70, 70)
	r.DrawRect(boxX, boxY, boxW, boxH, 25, 25, 35)

	innerW := boxW - pad*2
	y := boxY + pad
	r.DrawTextCentered(truncateToWidth(r, title, innerW), boxX, y, boxW, 240, 180, 60)
	y += fontH + pad
	r.DrawRect(boxX+pad, y, innerW, 1, 60, 60, 60)
	y += 1 + pad

	for _, line := range strings.Split(body, "\n") {
		r.DrawSmallText(truncateSmallToWidth(r, line, innerW), boxX+pad, y, 200, 200, 200)
		y += lineH
	}
	y += pad
	r.DrawRect(boxX+pad, y, boxW-pad*2, 1, 60, 60, 60)
	y += 1 + pad
	r.DrawSmallTextCentered("A: confirm  B: cancel", boxX, y, boxW, 200, 100, 100)
}

func (s *ManageDownloadsScreen) HandleEvent(e sdl.Event) Screen {
	entry, ok := s.inv.Lookup(s.gameURL)
	if !ok {
		return s.prev
	}

	hasROM := hasFileType(entry.Files, inventory.FileTypeROM)
	hasMusic := hasFileType(entry.Files, inventory.FileTypeMusic)
	extraActions := 1
	if hasROM && hasMusic {
		extraActions = 3
	}
	rowCount := len(entry.Files) + extraActions + 1 // +1 for unified naming toggle
	deleteAllIdx := len(entry.Files)

	if s.confirmActive {
		switch ev := e.(type) {
		case *sdl.ControllerButtonEvent:
			if ev.Type != sdl.CONTROLLERBUTTONDOWN {
				return s
			}
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_B: // physical A = confirm
				allGone, newFileCount := s.performDelete(s.gameURL, s.confirmFileIdx)
				s.confirmActive = false
				s.confirmFileIdx = -1
				if r, ok := s.prev.(Rebuildable); ok {
					r.ScheduleRebuild()
				}
				if allGone {
					return s.prev
				}
				if s.cursor >= newFileCount {
					s.cursor = newFileCount
				}
			case sdl.CONTROLLER_BUTTON_A: // physical B = cancel
				s.confirmActive = false
				s.confirmFileIdx = -1
			}
		case *sdl.KeyboardEvent:
			if ev.Type != sdl.KEYDOWN {
				return s
			}
			switch ev.Keysym.Sym {
			case sdl.K_RETURN:
				allGone, newFileCount := s.performDelete(s.gameURL, s.confirmFileIdx)
				s.confirmActive = false
				s.confirmFileIdx = -1
				if r, ok := s.prev.(Rebuildable); ok {
					r.ScheduleRebuild()
				}
				if allGone {
					return s.prev
				}
				if s.cursor >= newFileCount {
					s.cursor = newFileCount
				}
			case sdl.K_ESCAPE:
				s.confirmActive = false
				s.confirmFileIdx = -1
			}
		}
		return s
	}

	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < rowCount-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			switch {
			case s.cursor < len(entry.Files):
				s.confirmActive = true
				s.confirmFileIdx = s.cursor
			case hasROM && hasMusic && s.cursor == deleteAllIdx:
				s.confirmActive = true
				s.confirmFileIdx = deleteIdxROMs
			case hasROM && hasMusic && s.cursor == deleteAllIdx+1:
				s.confirmActive = true
				s.confirmFileIdx = deleteIdxSoundtrack
			case s.cursor == deleteAllIdx+extraActions-1:
				s.confirmActive = true
				s.confirmFileIdx = deleteIdxAll
			case s.cursor == len(entry.Files)+extraActions:
				if s.cfg.UnifiedNaming {
					return s.startUnifiedNamingMigration(entry)
				}
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
			if s.cursor < rowCount-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B: // physical A = select
			switch {
			case s.cursor < len(entry.Files):
				s.confirmActive = true
				s.confirmFileIdx = s.cursor
			case hasROM && hasMusic && s.cursor == deleteAllIdx:
				s.confirmActive = true
				s.confirmFileIdx = deleteIdxROMs
			case hasROM && hasMusic && s.cursor == deleteAllIdx+1:
				s.confirmActive = true
				s.confirmFileIdx = deleteIdxSoundtrack
			case s.cursor == deleteAllIdx+extraActions-1:
				s.confirmActive = true
				s.confirmFileIdx = deleteIdxAll
			case s.cursor == len(entry.Files)+extraActions:
				if s.cfg.UnifiedNaming {
					return s.startUnifiedNamingMigration(entry)
				}
			}
		case sdl.CONTROLLER_BUTTON_A: // physical B = back
			return s.prev
		}
	}
	return s
}

func (s *ManageDownloadsScreen) performDelete(gameURL string, fileIdx int) (bool, int) {
	entry, ok := s.inv.Lookup(gameURL)
	if !ok {
		return true, 0
	}

	var toDelete []inventory.DownloadedFile
	switch fileIdx {
	case deleteIdxAll:
		toDelete = make([]inventory.DownloadedFile, len(entry.Files))
		copy(toDelete, entry.Files)
	case deleteIdxROMs:
		for _, f := range entry.Files {
			if f.FileType == inventory.FileTypeROM || f.FileType == "" {
				toDelete = append(toDelete, f)
			}
		}
	case deleteIdxSoundtrack:
		for _, f := range entry.Files {
			if f.FileType == inventory.FileTypeMusic {
				toDelete = append(toDelete, f)
			}
		}
	default:
		if fileIdx >= 0 && fileIdx < len(entry.Files) {
			toDelete = []inventory.DownloadedFile{entry.Files[fileIdx]}
		}
	}

	if len(toDelete) == 0 {
		logger.Warn("inventory: performDelete no matching files game=%q fileIdx=%d", entry.Title, fileIdx)
		return false, len(entry.Files)
	}

	for _, f := range toDelete {
		if err := os.Remove(f.DestPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("inventory: delete file=%s: %v", f.DestPath, err)
		} else {
			logger.Debug("inventory: deleted file=%s", f.DestPath)
		}
		// Only delete cover art for ROM files, not music.
		if f.FileType != inventory.FileTypeMusic {
			if artPath := inventory.CoverArtPath(entry.CoverURL, f.DestPath); artPath != "" {
				if err := os.Remove(artPath); err != nil && !os.IsNotExist(err) {
					logger.Warn("inventory: delete cover-art=%s: %v", artPath, err)
				}
			}
		}
	}
	logger.Info("inventory: deleted game=%q files=%d", entry.Title, len(toDelete))

	if fileIdx == deleteIdxAll {
		s.inv.Remove(gameURL)
		if err := s.inv.Save(s.inventoryPath); err != nil {
			logger.Warn("inventory: save after delete failed: %v", err)
		}
		return true, 0
	}

	allGone := false
	for _, f := range toDelete {
		if s.inv.RemoveFile(gameURL, f.DestPath) {
			allGone = true
		}
	}
	remaining := 0
	if !allGone {
		if e, ok := s.inv.Lookup(gameURL); ok {
			remaining = len(e.Files)
		}
	}
	if err := s.inv.Save(s.inventoryPath); err != nil {
		logger.Warn("inventory: save after delete failed: %v", err)
	}
	return allGone, remaining
}

func (s *ManageDownloadsScreen) startUnifiedNamingMigration(entry inventory.Entry) Screen {
	if len(entry.Files) == 0 {
		return s
	}
	newDisabled := !entry.UnifiedNamingDisabled
	s.inv.SetUnifiedNamingDisabled(s.gameURL, newDisabled)
	formats := inventory.ReadMigrateFormats(inventory.NXSettingsPath)
	return NewMigrateFlowScreen(s.inv, s.inventoryPath, s.gameURL, entry.Title,
		entry.Files[0], !newDisabled, formats, s)
}
