//go:build !headless

package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

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

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time

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

func (s *ManageDownloadsScreen) rowCount() int {
	entry, ok := s.inv.Lookup(s.gameURL)
	if !ok {
		return 0
	}
	extra := 1
	if hasFileType(entry.Files, inventory.FileTypeROM) && hasFileType(entry.Files, inventory.FileTypeMusic) {
		extra = 3
	}
	return len(entry.Files) + extra + 1
}

func (s *ManageDownloadsScreen) startHold(dir int) {
	if s.heldDir == dir {
		return
	}
	s.heldDir = dir
	s.heldSince = time.Now()
	s.lastRepeat = s.heldSince
	s.moveCursor(dir)
}

func (s *ManageDownloadsScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *ManageDownloadsScreen) moveCursor(dir int) {
	n := s.rowCount()
	if dir > 0 && s.cursor < n-1 {
		s.cursor++
	} else if dir < 0 && s.cursor > 0 {
		s.cursor--
	}
}

func (s *ManageDownloadsScreen) processAutoRepeat() {
	if s.heldDir == 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(s.heldSince)
	if elapsed < repeatDelay {
		return
	}
	if now.Sub(s.lastRepeat) < currentRepeatInterval(elapsed-repeatDelay) {
		return
	}
	s.moveCursor(s.heldDir)
	s.lastRepeat = now
}

func (s *ManageDownloadsScreen) NeedsRedraw() bool         { return s.heldDir != 0 }
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
	bad := r.Theme.Error()
	mu := r.Theme.Muted()
	sep := r.Theme.Separator()
	warn := r.Theme.Warning()
	s.processAutoRepeat()
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

	hdr := r.Theme.Surface()
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
	bottomEdge := r.H - footerH

	hasROM := hasFileType(entry.Files, inventory.FileTypeROM)
	hasMusic := hasFileType(entry.Files, inventory.FileTypeMusic)
	extraActions := 1
	if hasROM && hasMusic {
		extraActions = 3
	}
	rowCount := len(entry.Files) + extraActions + 1 // +1 for unified-naming toggle

	// Compute how many rows fit and scroll so the cursor is always in view.
	visibleRows := int((bottomEdge - contentTop) / rowH)
	if visibleRows < 1 {
		visibleRows = 1
	}
	scrollOffset := s.cursor - visibleRows + 1
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if max := rowCount - visibleRows; scrollOffset > max {
		scrollOffset = max
	}
	pixelOffset := int32(scrollOffset) * rowH

	inView := func(y int32) bool { return y < bottomEdge && y+rowH > contentTop }

	lt := r.Theme.ListText
	at := r.Theme.AccentText
	arrowLabel := "→  "
	arrowW, _ := r.SmallTextSize(arrowLabel)
	for i, f := range entry.Files {
		y := contentTop + int32(i)*rowH - pixelOffset
		if !inView(y) {
			continue
		}
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
			r.DrawSmallText(arrowLabel, pathX, y+(fontH-smallFH)/2, mu[0], mu[1], mu[2])
			r.DrawSmallScrollingText(f.DestPath, pathX+arrowW, y+(fontH-smallFH)/2, pathMaxW-arrowW, mu[0], mu[1], mu[2])
		}
	}

	sepY := contentTop + int32(len(entry.Files))*rowH - pixelOffset
	if sepY >= contentTop && sepY < bottomEdge {
		r.DrawRect(margin, sepY, r.W-margin*2, 1, sep[0], sep[1], sep[2])
	}
	actionY := sepY + 8
	deleteAllIdx := len(entry.Files)

	if hasROM && hasMusic {
		if inView(actionY) {
			if s.cursor == deleteAllIdx && !s.confirmActive {
				r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			r.DrawText("Delete ROM", margin, actionY, warn[0], warn[1], warn[2])
		}
		actionY += rowH
		if inView(actionY) {
			if s.cursor == deleteAllIdx+1 && !s.confirmActive {
				r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			r.DrawText("Delete Soundtrack", margin, actionY, warn[0], warn[1], warn[2])
		}
		actionY += rowH
		if inView(actionY) {
			if s.cursor == deleteAllIdx+2 && !s.confirmActive {
				r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			r.DrawText("Delete all", margin, actionY, bad[0], bad[1], bad[2])
		}
		actionY += rowH
	} else {
		if inView(actionY) {
			if s.cursor == deleteAllIdx && !s.confirmActive {
				r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			r.DrawText("Delete all", margin, actionY, bad[0], bad[1], bad[2])
		}
		actionY += rowH
	}

	sep2Y := actionY + 4
	if sep2Y >= contentTop && sep2Y < bottomEdge {
		r.DrawRect(margin, sep2Y, r.W-margin*2, 1, sep[0], sep[1], sep[2])
	}
	toggleY := sep2Y + 8
	toggleIdx := len(entry.Files) + extraActions
	if inView(toggleY) {
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
				textColor = r.Theme.Muted()
			}
			r.DrawText(toggleLabel, margin, toggleY, textColor[0], textColor[1], textColor[2])
			tw, _ := r.TextSize(toggleLabel)
			r.DrawText(toggleVal, margin+tw+16, toggleY, textColor[0], textColor[1], textColor[2])
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}, ftrY)

	if s.confirmActive {
		s.drawConfirmOverlay(r, entry)
	}

	r.Present()
}

func (s *ManageDownloadsScreen) drawConfirmOverlay(r *renderer.Renderer, entry inventory.Entry) {
	panel := r.Theme.ModalPanel()
	// Toned against the panel rather than the background: the panel is a raised
	// surface, so a colour toned only against the background can land too close
	// to it.
	badTx := r.Theme.ToneOn(r.Theme.ErrorText(), panel)
	bd := r.Theme.ModalBorder()
	bodyC := r.Theme.ContrastText(panel)
	scrim := r.Theme.ModalScrim()
	sep := r.Theme.Separator()
	warn := r.Theme.ToneOn(r.Theme.Warning(), panel)
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
	bodyLines := strings.Split(body, "\n")

	// Cap lines so the box never overflows the screen (20 px margin top+bottom).
	// fixedH accounts for title, separators, footer and all surrounding padding.
	fixedH := pad + fontH + pad + 2 + pad + pad + 2 + pad + smallFH + pad
	maxBodyLines := (r.H - 40 - fixedH) / lineH
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	if int32(len(bodyLines)) > maxBodyLines {
		overflow := len(bodyLines) - int(maxBodyLines) + 1
		bodyLines = bodyLines[:maxBodyLines-1]
		bodyLines = append(bodyLines, fmt.Sprintf("(+%d more)", overflow))
	}

	bodyLineCount := int32(len(bodyLines))
	boxW := r.W * 2 / 3
	boxH := pad + fontH + pad + 2 + pad + lineH*bodyLineCount + pad + 2 + pad + smallFH + pad
	boxX := (r.W - boxW) / 2
	boxY := (r.H - boxH) / 2

	r.DrawRect(0, 0, r.W, r.H, scrim[0], scrim[1], scrim[2])
	r.DrawRect(boxX-1, boxY-1, boxW+2, boxH+2, bd[0], bd[1], bd[2])
	r.DrawRect(boxX, boxY, boxW, boxH, panel[0], panel[1], panel[2])

	innerW := boxW - pad*2
	y := boxY + pad
	r.DrawTextCentered(truncateToWidth(r, title, innerW), boxX, y, boxW, warn[0], warn[1], warn[2])
	y += fontH + pad
	r.DrawRect(boxX+pad, y, innerW, 1, sep[0], sep[1], sep[2])
	y += 1 + pad

	for _, line := range bodyLines {
		r.DrawSmallText(truncateSmallToWidth(r, line, innerW), boxX+pad, y, bodyC[0], bodyC[1], bodyC[2])
		y += lineH
	}
	y += pad
	r.DrawRect(boxX+pad, y, boxW-pad*2, 1, sep[0], sep[1], sep[2])
	y += 1 + pad
	r.DrawSmallTextCentered("A: confirm  B: cancel", boxX, y, boxW, badTx[0], badTx[1], badTx[2])
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
	deleteAllIdx := len(entry.Files)

	if s.confirmActive {
		switch ev := e.(type) {
		case *sdl.ControllerButtonEvent:
			if ev.Type != sdl.CONTROLLERBUTTONDOWN {
				return s
			}
			switch ev.Button {
			case btnA: // physical A = confirm
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
			case btnB: // physical B = cancel
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
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if ev.Type == sdl.KEYDOWN {
				s.startHold(1)
			} else {
				s.stopHold(1)
			}
			return s
		case sdl.K_UP:
			if ev.Type == sdl.KEYDOWN {
				s.startHold(-1)
			} else {
				s.stopHold(-1)
			}
			return s
		}
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
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
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(1)
			} else {
				s.stopHold(1)
			}
			return s
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(-1)
			} else {
				s.stopHold(-1)
			}
			return s
		}
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case btnA: // physical A = select
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
		case btnB: // physical B = back
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
				} else {
					pruneMediaDir(artPath)
				}
			}
		}
	}
	pruneDeletedDirs(toDelete, s.cfg.Pico8Core)
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
	formats := inventory.ReadMigrateFormats(inventory.SettingsPath())
	return NewMigrateFlowScreen(s.inv, s.inventoryPath, s.gameURL, entry.Title,
		entry.Files[0], !newDisabled, formats, s)
}
