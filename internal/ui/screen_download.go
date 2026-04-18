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

		itchUpload := itchio.Upload{Filename: upload.Filename, URL: upload.URL}
		var err error

		if cfg.APIKey != "" && detail != nil && detail.GameID != "" {
			owns, oErr := client.CheckOwnership(cfg.APIKey, detail.GameID)
			if oErr == nil && owns {
				err = client.DownloadAuth(cfg.APIKey, itchUpload, dest, progress)
			} else {
				err = client.DownloadFree(game.URL, itchUpload, dest, progress)
			}
		} else {
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
	r.DrawText("Downloading: "+s.game.Title, 20, 30, colorText, colorText, colorText)
	r.DrawText(s.upload.Filename, 20, 65, 160, 160, 160)

	switch s.state {
	case dlDownloading:
		dl := atomic.LoadInt64(&s.downloaded)
		tot := atomic.LoadInt64(&s.total)
		barW := r.W - 80
		r.DrawRect(40, r.H/2-10, barW, 20, 60, 60, 60)
		if tot > 0 {
			filled := int32(float64(barW) * float64(dl) / float64(tot))
			r.DrawRect(40, r.H/2-10, filled, 20, 80, 200, 80)
			r.DrawText(fmt.Sprintf("%d%%  (%s / %s)", dl*100/tot, humanBytes(dl), humanBytes(tot)),
				40, r.H/2+20, colorText, colorText, colorText)
		} else {
			r.DrawRect(40, r.H/2-10, barW/3, 20, 80, 200, 80)
			r.DrawText(humanBytes(dl)+" downloaded", 40, r.H/2+20, colorText, colorText, colorText)
		}

	case dlDone:
		r.DrawText("Download complete!", 20, r.H/2-20, 80, 200, 80)
		r.DrawText("Saved to: "+s.dest, 20, r.H/2+10, 160, 160, 160)
		r.DrawText("A or B: return to game list", 20, r.H/2+50, 140, 140, 140)

	case dlError:
		r.DrawText("Download failed:", 20, r.H/2-40, 200, 60, 60)
		msg := s.err.Error()
		if len(msg) > 80 {
			msg = msg[:77] + "..."
		}
		r.DrawText(msg, 20, r.H/2-10, 200, 100, 100)
		r.DrawText("Scan QR to visit game page:", 20, r.H/2+30, 160, 160, 160)
		tex, err := r.QRTexture(s.game.URL, 128)
		if err == nil && tex != nil {
			r.DrawTextureAt(tex, r.W/2-64, r.H/2+60, 128, 128)
			tex.Destroy()
		}
		r.DrawText("B: back", 20, r.H-24, 140, 140, 140)
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
