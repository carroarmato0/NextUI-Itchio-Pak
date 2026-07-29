//go:build !headless

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type dlState int32

const (
	dlDownloading dlState = iota
	dlDone
	dlError
)

func (s *DownloadScreen) loadState() dlState {
	return dlState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *DownloadScreen) storeState(st dlState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}

type DownloadScreen struct {
	client        *itchio.Client
	cfg           *settings.Config
	game          itchio.Game
	detail        *itchio.GameDetail
	upload        roms.Upload
	prev          Screen
	state         dlState
	downloaded    int64
	total         int64
	dest          string
	err           error
	inv           *inventory.Inventory
	inventoryPath string
}

func NewDownloadScreen(client *itchio.Client, cfg *settings.Config, game itchio.Game, detail *itchio.GameDetail, upload roms.Upload, dest string, inv *inventory.Inventory, inventoryPath string, prev Screen) *DownloadScreen {
	s := &DownloadScreen{
		client: client, cfg: cfg, game: game, detail: detail,
		upload: upload, prev: prev, dest: dest, state: dlDownloading,
		inv: inv, inventoryPath: inventoryPath,
	}

	go func() {
		progress := func(dl, total int64) {
			atomic.StoreInt64(&s.downloaded, dl)
			atomic.StoreInt64(&s.total, total)
		}

		isAuth := upload.DownloadKeyID != ""
		logger.Info("download: starting %q file=%s dest=%s auth=%v",
			game.Title, upload.Filename, dest, isAuth)

		var err error
		if isAuth {
			err = client.DownloadAuthUpload(cfg.APIKey, upload.UploadID, upload.DownloadKeyID, dest, progress)
		} else {
			itchUpload := itchio.Upload{Filename: upload.Filename, URL: upload.URL}
			err = client.DownloadFree(itchUpload, dest, progress)
		}

		if err != nil {
			logger.Error("download: failed file=%s: %v", upload.Filename, err)
			s.err = err
			s.storeState(dlError)
		} else {
			logger.Info("download: complete file=%s", upload.Filename)

			// Apply unified naming if enabled for this game.
			finalDest := dest
			unifiedName := false
			if cfg.UnifiedNaming {
				entry, entryExists := inv.Lookup(game.URL)
				disabled := entryExists && entry.UnifiedNamingDisabled
				if !disabled {
					newDest, didRename := roms.ResolveUnifiedDest(dest, game.Title, true)
					if didRename {
						if renameErr := os.Rename(dest, newDest); renameErr != nil {
							logger.Warn("unified-naming: rename failed: %v", renameErr)
						} else {
							logger.Info("unified-naming: renamed %q → %q", filepath.Base(dest), filepath.Base(newDest))
							finalDest = newDest
							unifiedName = true
						}
					} else {
						unifiedName = true // name already correct
					}
				}
			}

			if roms.ROMExt(upload.Filename) == ".p8.png" {
				if artErr := itchio.CopyCoverArt(finalDest); artErr != nil {
					logger.Warn("cover-art: game=%q: %v", game.Title, artErr)
				}
			} else if artErr := client.DownloadCoverArt(game.CoverURL, finalDest); artErr != nil {
				logger.Warn("cover-art: game=%q url=%s: %v", game.Title, game.CoverURL, artErr)
			}
			s.inv.Add(game.URL, inventory.Entry{
				GameURL:  game.URL,
				Title:    game.Title,
				Author:   game.Author,
				CoverURL: game.CoverURL,
				IsFree:   game.IsFree,
			}, inventory.DownloadedFile{
				Filename:     upload.Filename,
				DestPath:     finalDest,
				DownloadedAt: time.Now(),
				UnifiedName:  unifiedName,
			})
			if saveErr := s.inv.Save(s.inventoryPath); saveErr != nil {
				logger.Warn("inventory: save failed: %v", saveErr)
			} else {
				logger.Info("inventory: recorded game=%q file=%s unified=%v", game.Title, filepath.Base(finalDest), unifiedName)
			}
			s.dest = finalDest
			s.storeState(dlDone)
		}
	}()

	return s
}

func (s *DownloadScreen) NeedsRedraw() bool {
	return true
}
func (s *DownloadScreen) HasPendingAnimation() bool { return false }

func (s *DownloadScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16
	hdr := r.Theme.Surface()
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 10
	contentH := r.H - headerH - footerH

	switch s.loadState() {
	case dlDownloading:
		dl := atomic.LoadInt64(&s.downloaded)
		tot := atomic.LoadInt64(&s.total)
		mid := headerH + contentH/2
		r.DrawSmallText(s.upload.Filename, 20, contentTop+4, ht[0], ht[1], ht[2])
		barW := r.W - 80
		r.DrawRect(40, mid-10, barW, 20, 60, 60, 60)
		if tot > 0 {
			filled := int32(float64(barW) * float64(dl) / float64(tot))
			r.DrawRect(40, mid-10, filled, 20, 80, 200, 80)
			r.DrawText(fmt.Sprintf("%d%%  (%s / %s)", dl*100/tot, humanBytes(dl), humanBytes(tot)),
				40, mid+18, mt[0], mt[1], mt[2])
		} else {
			if dl > 0 {
				r.DrawRect(40, mid-10, barW/3, 20, 80, 200, 80)
			}
			r.DrawText(humanBytes(dl)+" downloaded", 40, mid+18, mt[0], mt[1], mt[2])
		}

	case dlDone:
		mid := headerH + contentH/2
		const pathMargin = int32(20)
		maxPathW := r.W - pathMargin*2
		const rowGap = int32(6)
		// Total height: title + filename + "Saved to:" + dir + file
		totalH := fontH + rowGap + smallFH + rowGap*2 + smallFH + rowGap + smallFH + rowGap + smallFH
		y := mid - totalH/2
		r.DrawTextCentered("Download complete!", 0, y, r.W, 80, 200, 80)
		y += fontH + rowGap
		r.DrawSmallTextCentered(s.upload.Filename, 0, y, r.W, ht[0], ht[1], ht[2])
		y += smallFH + rowGap*2
		r.DrawSmallTextCentered("Saved to:", 0, y, r.W, 120, 120, 120)
		y += smallFH + rowGap
		dir := truncateSmallToWidth(r, filepath.Dir(s.dest)+"/", maxPathW)
		r.DrawSmallTextCentered(dir, 0, y, r.W, 80, 80, 80)
		y += smallFH + rowGap
		file := truncateSmallToWidth(r, filepath.Base(s.dest), maxPathW)
		r.DrawSmallTextCentered(file, 0, y, r.W, 120, 120, 120)

	case dlError:
		// Layout from top of content area: error title, message, then QR centered
		// in the remaining space with label below it.
		y := contentTop + 8
		r.DrawText("Download failed:", 20, y, 200, 60, 60)
		y += fontH + 6
		msg := s.err.Error()
		msgH := r.DrawWrappedText(msg, 20, y, r.W-40, fontH+4, 200, 100, 100)
		y += msgH + 16

		// QR code: fill the remaining content area generously.
		qrAreaBottom := r.H - footerH - 4
		qrAreaH := qrAreaBottom - y
		qrSize := qrAreaH - smallFH - 10 // leave room for label below
		if qrSize > r.W*2/3 {
			qrSize = r.W * 2 / 3
		}
		if qrSize < 128 {
			qrSize = 128
		}
		tex, err := r.QRTexture(s.game.URL, int(qrSize))
		if err == nil && tex != nil {
			qrX := (r.W - qrSize) / 2
			r.DrawTextureAt(tex, qrX, y, qrSize, qrSize)
			tex.Destroy()
			r.DrawSmallTextCentered("Scan to visit game page", 0, y+qrSize+4, r.W, ht[0], ht[1], ht[2])
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.loadState() {
	case dlDownloading:
		r.DrawSmallText("Please wait...", 10, ftrY, ht[0], ht[1], ht[2])
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *DownloadScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		if s.loadState() != dlDownloading {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE, sdl.K_RETURN:
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		if s.loadState() != dlDownloading {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A:
				return s.prev
			}
		}
	}
	return s
}

// IsBusy implements BusyChecker. Returns true while a download is in flight.
func (s *DownloadScreen) IsBusy() bool {
	return s.loadState() == dlDownloading
}

func humanBytes(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
