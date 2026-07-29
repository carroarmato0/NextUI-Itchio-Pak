//go:build !headless

package ui

import (
	"fmt"
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

// PurchasePickerScreen lets the user choose which purchase to download from
// when a game has been bought more than once (e.g. individually and via a
// bundle). Each purchase may expose different downloads.
type PurchasePickerScreen struct {
	client        *itchio.Client
	cfg           *settings.Config
	cfgPath       string
	cache         *renderer.ImageCache
	game          itchio.Game
	detail        *itchio.GameDetail
	ownedKeys     []itchio.OwnedKey
	cursor        int
	prev          Screen
	inv           *inventory.Inventory
	inventoryPath string
}

// purchaseLabel returns a human-readable label for a purchase entry.
func purchaseLabel(key itchio.OwnedKey) string {
	if key.BundleSize <= 1 {
		return "Individual purchase"
	}
	if key.BundleName != "" {
		return "Bundle: " + key.BundleName
	}
	return fmt.Sprintf("Bundle purchase (%d games)", key.BundleSize)
}

func NewPurchasePickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail,
	ownedKeys []itchio.OwnedKey,
	inv *inventory.Inventory, inventoryPath string,
	prev Screen,
) *PurchasePickerScreen {
	return &PurchasePickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, ownedKeys: ownedKeys, prev: prev,
		inv: inv, inventoryPath: inventoryPath,
	}
}

func (s *PurchasePickerScreen) NeedsRedraw() bool { return false }
func (s *PurchasePickerScreen) HasPendingAnimation() bool { return false }

func (s *PurchasePickerScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16

	hBG := r.Theme.Surface()
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 10
	r.DrawSmallText("Multiple purchases found — choose one:", 20, contentTop, 180, 180, 180)
	contentTop += smallFH + 10

	rowH := fontH + smallFH + 18
	for i, k := range s.ownedKeys {
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
		label := purchaseLabel(k)
		r.DrawText(label, 20, y, tr, tg, tb)
		dlStr := fmt.Sprintf("Downloaded %d×", k.Downloads)
		r.DrawSmallText(dlStr, 20, y+fontH+2, ht[0], ht[1], ht[2])
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Back"},
	}, ftrY)
	r.Present()
}

func (s *PurchasePickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.ownedKeys)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			return s.choosePurchase(s.ownedKeys[s.cursor])
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.ownedKeys)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.choosePurchase(s.ownedKeys[s.cursor])
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	}
	return s
}

func (s *PurchasePickerScreen) choosePurchase(key itchio.OwnedKey) Screen {
	downloadKeyID := fmt.Sprintf("%d", key.ID)
	logger.Debug("purchase-picker: selected key id=%d purchase_id=%d", key.ID, key.PurchaseID)

	authUploads, err := s.client.FetchUploadsForKey(s.cfg.APIKey, s.detail.GameID, downloadKeyID)
	if err != nil {
		logger.Error("purchase-picker: fetch uploads for key id=%d: %v", key.ID, err)
		// Show error on the detail screen rather than a dead-end screen.
		if ds, ok := s.prev.(*DetailScreen); ok {
			ds.ShowModal("Download Error", err.Error())
		}
		return s.prev
	}

	var uploads []roms.Upload
	for _, u := range authUploads {
		uploads = append(uploads, roms.Upload{
			Filename:      u.Filename,
			UploadID:      u.UploadID,
			DownloadKeyID: downloadKeyID,
			NeedsFormat:   u.NeedsFormat,
		})
	}

	if len(uploads) == 0 {
		logger.Warn("purchase-picker: no downloadable uploads for key id=%d", key.ID)
		if ds, ok := s.prev.(*DetailScreen); ok {
			ds.ShowModal("No Files Found", "No downloadable ROM files found for this purchase.")
		}
		return s.prev
	}

	var known, unknown []roms.Upload
	for _, u := range uploads {
		if u.NeedsFormat {
			unknown = append(unknown, u)
		} else {
			known = append(known, u)
		}
	}

	if len(known) > 0 {
		if len(known) == 1 {
			upload := known[0]
			if s.cfg.ROMLocation == "ask" {
				return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
			}
			ext := strings.ToLower(filepath.Ext(upload.Filename))
			dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + upload.Filename
			if existing := s.inv.ExistingDestPath(s.game.URL, upload.Filename); existing != "" {
				dest = existing
			}
			return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
		}
		return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, known, s.inv, s.inventoryPath, s.prev)
	}
	return NewFormatPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, unknown, s.inv, s.inventoryPath, s.prev)
}
