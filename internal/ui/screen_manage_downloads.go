//go:build !headless

package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

type ManageDownloadsScreen struct {
	inv           *inventory.Inventory
	inventoryPath string
	gameURL       string
	cursor        int // 0..len(files)-1 = file rows; len(files) = "Delete all"

	confirmActive  bool
	confirmFileIdx int // -1 = delete all, otherwise index into entry.Files

	prev Screen
}

func NewManageDownloadsScreen(inv *inventory.Inventory, inventoryPath string, gameURL string, prev Screen) *ManageDownloadsScreen {
	return &ManageDownloadsScreen{
		inv:           inv,
		inventoryPath: inventoryPath,
		gameURL:       gameURL,
		prev:          prev,
	}
}

func (s *ManageDownloadsScreen) Draw(r *renderer.Renderer) {
	entry, ok := s.inv.Lookup(s.gameURL)
	if !ok {
		r.Present()
		return
	}

	r.Clear(colorBG, colorBG, colorBG)
	footerH := int32(40)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	headerH := fontH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
	title := truncateToWidth(r, "Manage Downloads — "+entry.Title, r.W-24)
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawSmallText("by "+entry.Author, 12, 8+fontH+4, 140, 140, 140)

	contentTop := headerH + 10
	rowH := fontH + 14
	margin := int32(20)

	for i, f := range entry.Files {
		y := contentTop + int32(i)*rowH
		if i == s.cursor && !s.confirmActive {
			r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}
		nameW, _ := r.TextSize(f.Filename)
		r.DrawText(f.Filename, margin, y, colorText, colorText, colorText)
		dirLabel := "→  " + f.DestPath
		r.DrawSmallText(dirLabel, margin+nameW+12, y+(fontH-smallFH)/2, 120, 120, 120)
	}

	sepY := contentTop + int32(len(entry.Files))*rowH
	r.DrawRect(margin, sepY, r.W-margin*2, 1, 50, 50, 50)
	deleteAllY := sepY + 8
	deleteAllIdx := len(entry.Files)
	if s.cursor == deleteAllIdx && !s.confirmActive {
		r.DrawRect(0, deleteAllY-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
	}
	r.DrawText("Delete all", margin, deleteAllY, 200, 80, 80)

	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText("B: select  |  A: back", 10, ftrY, 140, 140, 140)

	if s.confirmActive {
		s.drawConfirmOverlay(r, entry)
	}

	r.Present()
}

func (s *ManageDownloadsScreen) drawConfirmOverlay(r *renderer.Renderer, entry *inventory.Entry) {
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	pad := int32(16)

	var title, body string
	if s.confirmFileIdx == -1 {
		title = fmt.Sprintf("Delete all %d file(s)?", len(entry.Files))
		var names []string
		for _, f := range entry.Files {
			names = append(names, f.Filename)
		}
		body = strings.Join(names, "\n")
	} else {
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

	y := boxY + pad
	r.DrawTextCentered(title, boxX, y, boxW, 240, 180, 60)
	y += fontH + pad
	r.DrawRect(boxX+pad, y, boxW-pad*2, 1, 60, 60, 60)
	y += 1 + pad

	for _, line := range strings.Split(body, "\n") {
		r.DrawSmallText(line, boxX+pad, y, 200, 200, 200)
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
	rowCount := len(entry.Files) + 1

	if s.confirmActive {
		switch ev := e.(type) {
		case *sdl.ControllerButtonEvent:
			if ev.Type != sdl.CONTROLLERBUTTONDOWN {
				return s
			}
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_B: // physical A = confirm
				allGone := s.performDelete(entry, s.confirmFileIdx)
				s.confirmActive = false
				s.confirmFileIdx = -1
				if allGone {
					return s.prev
				}
				if s.cursor >= len(entry.Files) {
					s.cursor = len(entry.Files)
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
				allGone := s.performDelete(entry, s.confirmFileIdx)
				s.confirmActive = false
				s.confirmFileIdx = -1
				if allGone {
					return s.prev
				}
				if s.cursor >= len(entry.Files) {
					s.cursor = len(entry.Files)
				}
			case sdl.K_ESCAPE:
				s.confirmActive = false
				s.confirmFileIdx = -1
			}
		case *sdl.QuitEvent:
			return nil
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
			s.confirmActive = true
			if s.cursor == len(entry.Files) {
				s.confirmFileIdx = -1
			} else {
				s.confirmFileIdx = s.cursor
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
			s.confirmActive = true
			if s.cursor == len(entry.Files) {
				s.confirmFileIdx = -1
			} else {
				s.confirmFileIdx = s.cursor
			}
		case sdl.CONTROLLER_BUTTON_A: // physical B = back
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *ManageDownloadsScreen) performDelete(entry *inventory.Entry, fileIdx int) bool {
	var toDelete []inventory.DownloadedFile
	if fileIdx == -1 {
		toDelete = make([]inventory.DownloadedFile, len(entry.Files))
		copy(toDelete, entry.Files)
		entry.Files = nil
	} else {
		toDelete = []inventory.DownloadedFile{entry.Files[fileIdx]}
		var remaining []inventory.DownloadedFile
		for i, f := range entry.Files {
			if i != fileIdx {
				remaining = append(remaining, f)
			}
		}
		entry.Files = remaining
	}

	for _, f := range toDelete {
		if err := os.Remove(f.DestPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("inventory: delete file=%s: %v", f.DestPath, err)
		} else {
			logger.Debug("inventory: deleted file=%s", f.DestPath)
		}
		if artPath := inventory.CoverArtPath(entry.CoverURL, f.DestPath); artPath != "" {
			if err := os.Remove(artPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("inventory: delete cover-art=%s: %v", artPath, err)
			} else {
				logger.Debug("inventory: deleted cover-art=%s", artPath)
			}
		}
	}
	logger.Info("inventory: deleted game=%q files=%d", entry.Title, len(toDelete))

	if len(entry.Files) == 0 {
		s.inv.Remove(entry.GameURL)
	}
	if err := s.inv.Save(s.inventoryPath); err != nil {
		logger.Warn("inventory: save after delete failed: %v", err)
	}
	return len(entry.Files) == 0
}
