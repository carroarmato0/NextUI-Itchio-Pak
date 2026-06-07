//go:build !headless

package ui

import (
	"fmt"
	"path/filepath"
	"strings"
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

// ZIPPlan carries the result of ZIP inspection and user choices through the screen flow.
type ZIPPlan struct {
	Upload   roms.Upload
	CDNURL   string
	Manifest roms.ZIPManifest

	DownloadROMs bool
	DownloadMusic bool
	// Pico8GameDir, when non-empty, triggers path-preserving extraction of all
	// .p8/.p8.png/.lua files from the ZIP into this directory.
	Pico8GameDir string
	// SelectedROMs maps lowercase extension → chosen entry Name.
	// Empty map means all ROMs in the manifest are selected.
	SelectedROMs map[string]string
	// ROMDirs maps lowercase extension → chosen destination directory.
	// Overrides DestinationDir when set (used for user-chosen GBA folder).
	ROMDirs  map[string]string
	MusicDir string
}

type zipInspectState int32

const (
	zipInspectLoading zipInspectState = iota
	zipInspectDone
	zipInspectError
)

// ZIPInspectScreen resolves the CDN URL and reads the ZIP central directory
// before routing to the appropriate next screen.
type ZIPInspectScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	upload  roms.Upload
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	state zipInspectState
	plan  ZIPPlan
	err   error

	// Progress tracking for the loading animation (ZIP range-read path).
	inspectFetched int64     // bytes fetched so far (atomic via sync/atomic)
	inspectTotal   int64     // total file size in bytes (atomic)
	inspectStart   time.Time // when inspection started
}

func (s *ZIPInspectScreen) loadState() zipInspectState {
	return zipInspectState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *ZIPInspectScreen) storeState(st zipInspectState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}

func NewZIPInspectScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache,
	game itchio.Game, detail *itchio.GameDetail, upload roms.Upload,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *ZIPInspectScreen {
	s := &ZIPInspectScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, upload: upload,
		inv: inv, invPath: invPath, prev: prev,
	}
	go s.runInspect()
	return s
}

func (s *ZIPInspectScreen) runInspect() {
	defer func() { sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT}) }()
	logger.Info("zip-inspect: starting for %s", s.upload.Filename)
	s.inspectStart = time.Now()

	var cdnURL string
	var err error
	if s.upload.DownloadKeyID != "" {
		cdnURL, err = s.client.ResolveAuthURL(s.cfg.APIKey, s.upload.UploadID, s.upload.DownloadKeyID)
	} else {
		itchUpload := itchio.Upload{Filename: s.upload.Filename, URL: s.upload.URL}
		cdnURL, err = s.client.ResolveFreeURL(itchUpload)
	}
	if err != nil {
		logger.Error("zip-inspect: resolve CDN URL: %v", err)
		s.err = err
		s.storeState(zipInspectError)
		return
	}

	progress := func(fetched, total int64) {
		atomic.StoreInt64(&s.inspectFetched, fetched)
		atomic.StoreInt64(&s.inspectTotal, total)
	}

	var manifest roms.ZIPManifest
	if strings.ToLower(filepath.Ext(s.upload.Filename)) == ".7z" {
		manifest, err = roms.InspectRemote7z(s.client.HTTPClient(), cdnURL)
	} else {
		manifest, err = roms.InspectRemoteZIP(s.client.HTTPClient(), cdnURL, progress)
	}
	if err != nil {
		logger.Error("zip-inspect: inspect archive: %v", err)
		s.err = err
		s.storeState(zipInspectError)
		return
	}
	logger.Info("zip-inspect: manifest ROMs=%d music=%d", manifest.ROMCount(), manifest.MusicCount())

	s.plan = ZIPPlan{Upload: s.upload, CDNURL: cdnURL, Manifest: manifest}
	s.storeState(zipInspectDone)
}

func (s *ZIPInspectScreen) NeedsRedraw() bool        { return true }
func (s *ZIPInspectScreen) HasPendingAnimation() bool { return false }

func (s *ZIPInspectScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(40)
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 6
	contentH := r.H - headerH - footerH
	mid := contentTop + contentH/2

	switch s.loadState() {
	case zipInspectLoading:
		r.DrawTextCentered("Inspecting archive", 0, mid-mainFH-smallFH-14, r.W, mt[0], mt[1], mt[2])
		// Show bytes fetched and throughput so the user can see progress is happening.
		fetched := atomic.LoadInt64(&s.inspectFetched)
		total := atomic.LoadInt64(&s.inspectTotal)
		if fetched > 0 && !s.inspectStart.IsZero() {
			elapsed := time.Since(s.inspectStart).Seconds()
			var info string
			if elapsed > 0.5 {
				kbps := float64(fetched) / 1024 / elapsed
				if total > 0 {
					info = fmt.Sprintf("%s fetched at %.0f KB/s  (file: %s)",
						formatKB(fetched), kbps, formatKB(total))
				} else {
					info = fmt.Sprintf("%s fetched at %.0f KB/s", formatKB(fetched), kbps)
				}
			} else {
				info = fmt.Sprintf("%s fetched", formatKB(fetched))
			}
			r.DrawSmallTextCentered(info, 0, mid-smallFH-4, r.W, ht[0], ht[1], ht[2])
		}
		drawLoadingDots(r, mid+8)
	case zipInspectError:
		r.DrawText("Inspection failed:", 20, mid-mainFH-smallFH-8, 200, 60, 60)
		r.DrawWrappedText(s.err.Error(), 20, mid-smallFH, r.W-40, smallFH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
	}, ftrY)
	r.Present()
}

func (s *ZIPInspectScreen) HandleEvent(e sdl.Event) Screen {
	if s.loadState() == zipInspectDone {
		return s.route()
	}
	switch ev := e.(type) {
	case *sdl.UserEvent:
		_ = ev
		if s.loadState() == zipInspectDone {
			return s.route()
		}
	case *sdl.KeyboardEvent:
		if ev.Type == sdl.KEYDOWN {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE: // B — cancel at any time
				return s.prev
			case sdl.K_RETURN: // A — dismiss error
				if s.loadState() == zipInspectError {
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
				if s.loadState() == zipInspectError {
					return s.prev
				}
			}
		}
	}
	return s
}

func (s *ZIPInspectScreen) route() Screen {
	m := s.plan.Manifest

	if !m.HasROMs() && !m.HasMusic() {
		logger.Warn("zip-inspect: manifest empty, returning to prev")
		return s.prev
	}

	manifestHasGBA := len(m.ROMsByExt()[".gba"]) > 0

	// GBA + "ask": route through contents screen before anything else so the
	// user can choose between Game Boy Advance (GBA) and Game Boy Advance (MGBA).
	if manifestHasGBA && s.cfg.ROMLocation == "ask" {
		return NewZIPContentsScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, s.plan, s.inv, s.invPath, s.prev)
	}

	// Single ROM, no music, no extra files.
	if m.IsSingleROMOnly() && !m.HasOtherFiles() {
		// Use the inner ROM's extension to route to the correct destination directory
		// (e.g., a ZIP containing a single .gba should land in the GBA folder).
		ext := strings.ToLower(roms.ROMExt(s.upload.Filename))
		for _, e := range m.Entries {
			if e.Kind == roms.KindROM {
				if inner := strings.ToLower(roms.ROMExt(e.Name)); roms.DestinationDir(inner, s.cfg.Pico8Core) != "" {
					ext = inner
				}
				break
			}
		}

		if ext == ".p8" || ext == ".p8.png" {
			// Pico-8: always extract — emulators cannot load .p8/.p8.png from a ZIP.
			plan := s.plan
			plan.DownloadROMs = true
			return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
		}

		// 7z archives must always be extracted — emulators do not support 7z natively.
		if strings.ToLower(filepath.Ext(s.upload.Filename)) == ".7z" {
			plan := s.plan
			plan.DownloadROMs = true
			return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
		}

		// Non-Pico-8 ZIP: keep on disk (most emulators support ZIP natively).
		dest := roms.DestinationDir(ext, s.cfg.Pico8Core) + s.upload.Filename
		if existing := s.inv.ExistingDestPath(s.game.URL, s.upload.Filename); existing != "" {
			dest = existing
		}
		patched := s.upload
		patched.URL = s.plan.CDNURL
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, patched, dest, s.inv, s.invPath, s.prev)
	}

	// Multi-file Pico-8: extract all .p8/.p8.png/.lua files to a game
	// subdirectory, preserving the ZIP's relative path structure.
	p8Count := len(m.ROMsByExt()[".p8"]) + len(m.ROMsByExt()[".p8.png"])
	if p8Count > 1 || (p8Count == 1 && m.HasLuaFiles()) {
		gameDir := roms.Pico8GameSubDir(s.cfg.Pico8Core, s.game.Title)
		plan := s.plan
		plan.DownloadROMs = true
		plan.Pico8GameDir = gameDir
		plan.DownloadMusic = m.HasMusic() && s.cfg.MusicDownload == "auto"
		if plan.DownloadMusic {
			if s.cfg.MusicLocation == "ask" {
				return NewMusicLocationPickerScreen(s.client, s.cfg, s.cfgPath,
					s.game, s.detail, plan, s.inv, s.invPath, s.prev)
			}
			plan.MusicDir = roms.MusicDestinationDir(s.game.Title)
		}
		return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
	}

	// Multiple ROMs of same extension, GBA present, or music choice needed → picker.
	if m.HasDuplicateROMExt() || (s.cfg.MusicDownload == "ask" && m.HasMusic()) || manifestHasGBA {
		return NewZIPContentsScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, s.plan, s.inv, s.invPath, s.prev)
	}

	// Auto path.
	plan := s.plan
	plan.DownloadROMs = m.HasROMs()
	plan.DownloadMusic = m.HasMusic() && s.cfg.MusicDownload == "auto"
	if plan.DownloadMusic {
		if s.cfg.MusicLocation == "ask" {
			return NewMusicLocationPickerScreen(s.client, s.cfg, s.cfgPath,
				s.game, s.detail, plan, s.inv, s.invPath, s.prev)
		}
		plan.MusicDir = roms.MusicDestinationDir(s.game.Title)
	}
	return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
}
