//go:build !headless

package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type dlState int

const (
	dlDownloading dlState = iota
	dlDone
	dlError
)

type DownloadScreen struct {
	client     *itchio.Client
	cfg        *settings.Config
	game       itchio.Game
	detail     *itchio.GameDetail
	upload     roms.Upload
	prev       Screen
	state      dlState
	downloaded int64
	total      int64
	dest       string
	err        error
}

func NewDownloadScreen(client *itchio.Client, cfg *settings.Config, game itchio.Game, detail *itchio.GameDetail, upload roms.Upload, prev Screen) *DownloadScreen {
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename

	s := &DownloadScreen{
		client: client, cfg: cfg, game: game, detail: detail,
		upload: upload, prev: prev, dest: dest, state: dlDownloading,
	}

	go func() {
		progress := func(dl, total int64) {
			atomic.StoreInt64(&s.downloaded, dl)
			atomic.StoreInt64(&s.total, total)
		}

		var err error

		if upload.DownloadKeyID != "" {
			// Paid game: use the API-based auth download path.
			err = client.DownloadAuthUpload(cfg.APIKey, upload.UploadID, upload.DownloadKeyID, dest, progress)
		} else {
			// Free game: use the CSRF scraping download path.
			itchUpload := itchio.Upload{Filename: upload.Filename, URL: upload.URL}
			err = client.DownloadFree(game.URL, itchUpload, dest, progress)
		}

		if err != nil {
			s.err = err
			s.state = dlError
		} else {
			s.state = dlDone
		}
	}()

	return s
}

func (s *DownloadScreen) Draw(r *renderer.Renderer) {
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

	contentTop := headerH + 10
	contentH := r.H - headerH - footerH

	switch s.state {
	case dlDownloading:
		dl := atomic.LoadInt64(&s.downloaded)
		tot := atomic.LoadInt64(&s.total)
		mid := headerH + contentH/2
		r.DrawSmallText(s.upload.Filename, 20, contentTop+4, 160, 160, 160)
		barW := r.W - 80
		r.DrawRect(40, mid-10, barW, 20, 60, 60, 60)
		if tot > 0 {
			filled := int32(float64(barW) * float64(dl) / float64(tot))
			r.DrawRect(40, mid-10, filled, 20, 80, 200, 80)
			r.DrawText(fmt.Sprintf("%d%%  (%s / %s)", dl*100/tot, humanBytes(dl), humanBytes(tot)),
				40, mid+18, colorText, colorText, colorText)
		} else {
			r.DrawRect(40, mid-10, barW/3, 20, 80, 200, 80)
			r.DrawText(humanBytes(dl)+" downloaded", 40, mid+18, colorText, colorText, colorText)
		}

	case dlDone:
		mid := headerH + contentH/2
		r.DrawTextCentered("Download complete!", 0, mid-fontH-8, r.W, 80, 200, 80)
		r.DrawSmallTextCentered(s.upload.Filename, 0, mid+4, r.W, 160, 160, 160)
		r.DrawSmallTextCentered("Saved to: "+s.dest, 0, mid+4+smallFH+4, r.W, 120, 120, 120)

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
			r.DrawSmallTextCentered("Scan to visit game page", 0, y+qrSize+4, r.W, 160, 160, 160)
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.state {
	case dlDownloading:
		r.DrawSmallText("Please wait...", 10, ftrY, 140, 140, 140)
	default:
		r.DrawSmallText("A / B: back", 10, ftrY, 140, 140, 140)
	}
	r.Present()
}

func (s *DownloadScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		if s.state != dlDownloading {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE, sdl.K_RETURN:
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		if s.state != dlDownloading {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A:
				return s.prev
			}
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
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
