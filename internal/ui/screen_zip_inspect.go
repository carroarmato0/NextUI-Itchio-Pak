//go:build !headless

package ui

import (
	"path/filepath"
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

// ZIPPlan carries the result of ZIP inspection and user choices through the screen flow.
type ZIPPlan struct {
	Upload   roms.Upload
	CDNURL   string
	Manifest roms.ZIPManifest

	DownloadROMs  bool
	DownloadMusic bool
	// SelectedROMs maps lowercase extension → chosen entry Name.
	// Empty map means all ROMs in the manifest are selected.
	SelectedROMs map[string]string
	MusicDir     string
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

	manifest, err := roms.InspectRemoteZIP(s.client.HTTPClient(), cdnURL)
	if err != nil {
		logger.Error("zip-inspect: inspect ZIP: %v", err)
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
		r.DrawTextCentered("Inspecting ZIP…", 0, mid-mainFH/2, r.W, mt[0], mt[1], mt[2])
	case zipInspectError:
		r.DrawText("Inspection failed:", 20, mid-mainFH-smallFH-8, 200, 60, 60)
		r.DrawWrappedText(s.err.Error(), 20, mid-smallFH, r.W-40, smallFH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.loadState() {
	case zipInspectLoading:
		r.DrawSmallText("Please wait…", 10, ftrY, ht[0], ht[1], ht[2])
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
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
		if ev.Type == sdl.KEYDOWN && s.loadState() == zipInspectError {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE, sdl.K_RETURN:
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type == sdl.CONTROLLERBUTTONDOWN && s.loadState() == zipInspectError {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
				return s.prev
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

	// Single ROM, no music → keep ZIP, use DownloadScreen unchanged.
	if m.IsSingleROMOnly() {
		ext := strings.ToLower(filepath.Ext(s.upload.Filename))
		dest := roms.DestinationDir(ext) + s.upload.Filename
		if existing := s.inv.ExistingDestPath(s.game.URL, s.upload.Filename); existing != "" {
			dest = existing
		}
		patched := s.upload
		patched.URL = s.plan.CDNURL
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, patched, dest, s.inv, s.invPath, s.prev)
	}

	// Multiple ROMs of same extension always require a version picker.
	if m.HasDuplicateROMExt() || s.cfg.MusicDownload == "ask" {
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
