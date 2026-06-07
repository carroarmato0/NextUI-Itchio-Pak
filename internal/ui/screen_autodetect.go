//go:build !headless

package ui

import (
	"strings"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type autoDetectState int32

const (
	autoDetectLoading autoDetectState = iota
	autoDetectDone
	autoDetectError
)

// AutoDetectScreen resolves the CDN URL for an upload whose file type is
// unknown, fetches the first DetectBufSize bytes, and uses magic-byte
// detection to determine the correct destination without asking the user to
// pick a format. On success it transitions immediately to the appropriate
// screen (ZIPInspectScreen, DownloadScreen, or FormatPickerScreen as a
// last-resort fallback). On failure it shows an error and lets the user go back.
type AutoDetectScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	upload  roms.Upload
	inv     *inventory.Inventory
	invPath string
	prev    Screen

	state int32 // autoDetectState, accessed atomically
	next  Screen
	err   error
}

func NewAutoDetectScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache,
	game itchio.Game, detail *itchio.GameDetail, upload roms.Upload,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *AutoDetectScreen {
	s := &AutoDetectScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, upload: upload,
		inv: inv, invPath: invPath, prev: prev,
	}
	go s.run()
	return s
}

func (s *AutoDetectScreen) loadState() autoDetectState {
	return autoDetectState(atomic.LoadInt32(&s.state))
}

func (s *AutoDetectScreen) run() {
	defer func() { sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT}) }()

	// Step 1: resolve the CDN URL (same logic as ZIPInspectScreen).
	var cdnURL string
	var err error
	if s.upload.DownloadKeyID != "" {
		cdnURL, err = s.client.ResolveAuthURL(s.cfg.APIKey, s.upload.UploadID, s.upload.DownloadKeyID)
	} else {
		itchUpload := itchio.Upload{Filename: s.upload.Filename, URL: s.upload.URL}
		cdnURL, err = s.client.ResolveFreeURL(itchUpload)
	}
	if err != nil {
		logger.Error("auto-detect: resolve CDN URL: %v", err)
		s.err = err
		atomic.StoreInt32(&s.state, int32(autoDetectError))
		return
	}

	// Step 2: fetch the file header.
	header, err := s.client.FetchFileHeader(cdnURL, roms.DetectBufSize)
	if err != nil {
		logger.Error("auto-detect: fetch header: %v", err)
		s.err = err
		atomic.StoreInt32(&s.state, int32(autoDetectError))
		return
	}

	// Step 3: detect format from magic bytes.
	ext := roms.DetectROMExt(header)

	// In the direct-download context any PNG is almost certainly a Pico-8
	// .p8.png cartridge — the strict 128px-width check in DetectROMExt is
	// designed to filter artwork images inside ZIPs (where a cover.png might
	// sit next to the cart), not standalone game files. If the strict check
	// returned "" but the file is a PNG, treat it as .p8.png.
	if ext == "" && len(header) >= 4 &&
		header[0] == 0x89 && header[1] == 'P' && header[2] == 'N' && header[3] == 'G' {
		ext = ".p8.png"
		logger.Debug("auto-detect: PNG detected without strict width match, treating as .p8.png")
	}

	logger.Info("auto-detect: %q → detected ext=%q", s.upload.Filename, ext)

	switch ext {
	case ".zip":
		// Route through ZIPInspectScreen. Pass the original upload (resolver URL)
		// so ZIPInspectScreen can resolve a fresh CDN URL for the inspection.
		s.next = NewZIPInspectScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, s.upload, s.inv, s.invPath, s.prev)

	case "":
		// Unrecognised — fall back to the manual format picker for this one file.
		logger.Warn("auto-detect: no signature matched for %q, falling back to format picker", s.upload.Filename)
		s.next = NewFormatPickerScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, []roms.Upload{s.upload}, s.inv, s.invPath, s.prev)

	default:
		// Known ROM type: append the detected extension (if not already present)
		// and route directly to the download screen.
		upload := s.upload
		upload.NeedsFormat = false
		if strings.ToLower(roms.ROMExt(upload.Filename)) != ext {
			upload.Filename = upload.Filename + ext
		}
		dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + upload.Filename
		if existing := s.inv.ExistingDestPath(s.game.URL, upload.Filename); existing != "" {
			dest = existing
		}
		logger.Info("auto-detect: routing %q → %s", upload.Filename, dest)
		s.next = NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest,
			s.inv, s.invPath, s.prev)
	}

	atomic.StoreInt32(&s.state, int32(autoDetectDone))
}

func (s *AutoDetectScreen) NeedsRedraw() bool        { return s.loadState() == autoDetectLoading }
func (s *AutoDetectScreen) HasPendingAnimation() bool { return false }

func (s *AutoDetectScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	mt := r.Theme.MainText
	ht := r.Theme.HintText
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, ht[0], ht[1], ht[2])

	contentH := r.H - headerH - footerH
	mid := headerH + contentH/2

	switch s.loadState() {
	case autoDetectLoading:
		r.DrawTextCentered("Detecting file type", 0, mid-fontH-10, r.W, mt[0], mt[1], mt[2])
		drawLoadingDots(r, mid+8)
	case autoDetectError:
		r.DrawText("Detection failed:", 20, mid-fontH-4, 200, 60, 60)
		r.DrawWrappedText(s.err.Error(), 20, mid+4, r.W-40, fontH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.loadState() {
	case autoDetectLoading:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		}, ftrY)
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *AutoDetectScreen) HandleEvent(e sdl.Event) Screen {
	if s.loadState() == autoDetectDone {
		return s.next
	}
	switch ev := e.(type) {
	case *sdl.UserEvent:
		_ = ev
		if s.loadState() == autoDetectDone {
			return s.next
		}
	case *sdl.KeyboardEvent:
		if ev.Type == sdl.KEYDOWN {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE: // B — cancel at any time
				return s.prev
			case sdl.K_RETURN: // A — dismiss error
				if s.loadState() == autoDetectError {
					return s.prev
				}
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type == sdl.CONTROLLERBUTTONDOWN {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_A: // physical B — cancel at any time
				return s.prev
			case sdl.CONTROLLER_BUTTON_B: // physical A — dismiss error
				if s.loadState() == autoDetectError {
					return s.prev
				}
			}
		}
	}
	return s
}
