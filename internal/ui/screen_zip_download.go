//go:build !headless

package ui

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
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

type zipDLState int32

const (
	zipDLDownloading zipDLState = iota
	zipDLExtracting
	zipDLDone
	zipDLError
)

// ZIPDownloadScreen downloads a ZIP to a temp path, extracts ROM and music files
// to their respective destinations, and records all extracted files in inventory.
type ZIPDownloadScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	game    itchio.Game
	detail  *itchio.GameDetail
	plan    ZIPPlan
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	state       zipDLState
	downloaded  int64
	total       int64
	extracted   []string
	skipped     []string
	musicFailed bool
	err         error
}

func (s *ZIPDownloadScreen) loadState() zipDLState {
	return zipDLState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *ZIPDownloadScreen) storeState(st zipDLState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}

func NewZIPDownloadScreen(
	client *itchio.Client, cfg *settings.Config,
	game itchio.Game, detail *itchio.GameDetail, plan ZIPPlan,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *ZIPDownloadScreen {
	s := &ZIPDownloadScreen{
		client: client, cfg: cfg,
		game: game, detail: detail, plan: plan,
		inv: inv, invPath: invPath, prev: prev,
	}
	go s.run()
	return s
}

func (s *ZIPDownloadScreen) run() {
	defer func() { sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT}) }()

	tmp, err := os.CreateTemp("", "itchio-zip-*.zip")
	if err != nil {
		logger.Error("zip-download: create temp file: %v", err)
		s.err = fmt.Errorf("create temp file: %w", err)
		s.storeState(zipDLError)
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	// Re-resolve CDN URL immediately before the download so a stale URL from
	// the inspect step (which may have run minutes ago) does not cause a 403.
	cdnURL := s.plan.CDNURL
	if s.plan.Upload.DownloadKeyID != "" {
		fresh, rerr := s.client.ResolveAuthURL(s.cfg.APIKey, s.plan.Upload.UploadID, s.plan.Upload.DownloadKeyID)
		if rerr != nil {
			logger.Warn("zip-download: re-resolve auth URL failed (%v), using cached URL", rerr)
		} else {
			cdnURL = fresh
		}
	} else {
		itchUpload := itchio.Upload{Filename: s.plan.Upload.Filename, URL: s.plan.Upload.URL}
		fresh, rerr := s.client.ResolveFreeURL(itchUpload)
		if rerr != nil {
			logger.Warn("zip-download: re-resolve free URL failed (%v), using cached URL", rerr)
		} else {
			cdnURL = fresh
		}
	}

	progress := func(dl, total int64) {
		atomic.StoreInt64(&s.downloaded, dl)
		atomic.StoreInt64(&s.total, total)
	}
	logger.Info("zip-download: streaming %s → %s", s.plan.Upload.Filename, tmpPath)
	if err := s.client.DownloadURL(cdnURL, tmpPath, progress); err != nil {
		s.err = fmt.Errorf("download ZIP: %w", err)
		s.storeState(zipDLError)
		return
	}

	s.storeState(zipDLExtracting)
	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})

	r, err := zip.OpenReader(tmpPath)
	if err != nil {
		logger.Error("zip-download: open ZIP: %v", err)
		s.err = fmt.Errorf("open ZIP: %w", err)
		s.storeState(zipDLError)
		return
	}
	defer r.Close()

	now := time.Now()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		baseName := filepath.Base(f.Name)
		// macOS resource-fork stubs start with "._"; skip them.
		if strings.HasPrefix(baseName, "._") {
			continue
		}
		kind := roms.ClassifyEntry(baseName)

		switch kind {
		case roms.KindROM:
			if !s.shouldExtractROM(baseName) {
				continue
			}
			dest, err := s.extractROM(f, baseName, now)
			if err != nil {
				logger.Warn("zip-download: ROM %s: %v", baseName, err)
				s.skipped = append(s.skipped, baseName)
				continue
			}
			s.extracted = append(s.extracted, dest)

		case roms.KindMusic:
			if !s.plan.DownloadMusic || s.plan.MusicDir == "" {
				continue
			}
			dest, err := s.extractMusic(f, baseName, now)
			if err != nil {
				logger.Warn("zip-download: music %s: %v", baseName, err)
				s.skipped = append(s.skipped, baseName)
				continue
			}
			s.extracted = append(s.extracted, dest)
		}
	}

	if err := s.inv.Save(s.invPath); err != nil {
		logger.Warn("zip-download: save inventory: %v", err)
	}

	if len(s.extracted) == 0 {
		logger.Error("zip-download: no files extracted (skipped=%d)", len(s.skipped))
		s.err = fmt.Errorf("no files could be extracted from ZIP")
		s.storeState(zipDLError)
		return
	}
	logger.Info("zip-download: done, extracted %d file(s)", len(s.extracted))
	s.storeState(zipDLDone)
}

func (s *ZIPDownloadScreen) shouldExtractROM(name string) bool {
	if !s.plan.DownloadROMs {
		return false
	}
	if len(s.plan.SelectedROMs) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	chosen, ok := s.plan.SelectedROMs[ext]
	if !ok {
		return true
	}
	return chosen == name
}

func (s *ZIPDownloadScreen) extractROM(f *zip.File, baseName string, now time.Time) (string, error) {
	ext := strings.ToLower(filepath.Ext(baseName))
	destDir := s.plan.ROMDirs[ext]
	if destDir == "" {
		destDir = roms.DestinationDir(ext)
	}
	stem := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	safeName := roms.SanitiseFilename(stem, ext)
	if safeName == "" {
		safeName = baseName
	}
	dest := destDir + safeName

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("mkdirall %s: %w", destDir, err)
	}
	if err := extractZIPEntry(f, dest); err != nil {
		return "", err
	}

	finalDest := dest
	unifiedName := false
	if s.cfg.UnifiedNaming {
		entry, entryExists := s.inv.Lookup(s.game.URL)
		disabled := entryExists && entry.UnifiedNamingDisabled
		if !disabled {
			newDest, didRename := roms.ResolveUnifiedDest(dest, s.game.Title)
			if didRename {
				if err := os.Rename(dest, newDest); err != nil {
					logger.Warn("zip-download: unified rename: %v", err)
				} else {
					finalDest = newDest
					unifiedName = true
				}
			} else {
				unifiedName = true
			}
		}
	}

	if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
		logger.Warn("zip-download: cover art: %v", artErr)
	}
	s.inv.Add(s.game.URL, inventory.Entry{
		GameURL: s.game.URL, Title: s.game.Title,
		Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
	}, inventory.DownloadedFile{
		Filename:     filepath.Base(finalDest),
		DestPath:     finalDest,
		DownloadedAt: now,
		UnifiedName:  unifiedName,
		FileType:     inventory.FileTypeROM,
	})
	return finalDest, nil
}

func (s *ZIPDownloadScreen) extractMusic(f *zip.File, baseName string, now time.Time) (string, error) {
	if err := os.MkdirAll(s.plan.MusicDir, 0755); err != nil {
		s.musicFailed = true
		return "", fmt.Errorf("mkdirall music dir %s: %w", s.plan.MusicDir, err)
	}
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	safeName := roms.SanitiseFilename(stem, ext)
	if safeName == "" {
		safeName = baseName
	}
	dest := s.plan.MusicDir + safeName

	if err := extractZIPEntry(f, dest); err != nil {
		return "", err
	}
	s.inv.Add(s.game.URL, inventory.Entry{
		GameURL: s.game.URL, Title: s.game.Title,
		Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
	}, inventory.DownloadedFile{
		Filename:     filepath.Base(dest),
		DestPath:     dest,
		DownloadedAt: now,
		FileType:     inventory.FileTypeMusic,
	})
	return dest, nil
}

// extractZIPEntry copies a single ZIP file entry to dest on disk.
func extractZIPEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func (s *ZIPDownloadScreen) NeedsRedraw() bool        { return true }
func (s *ZIPDownloadScreen) HasPendingAnimation() bool { return false }

func (s *ZIPDownloadScreen) Draw(r *renderer.Renderer) {
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

	contentTop := headerH + 10
	contentH := r.H - headerH - footerH
	mid := headerH + contentH/2

	switch s.loadState() {
	case zipDLDownloading:
		dl := atomic.LoadInt64(&s.downloaded)
		tot := atomic.LoadInt64(&s.total)
		r.DrawSmallText(s.plan.Upload.Filename, 20, contentTop+4, ht[0], ht[1], ht[2])
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

	case zipDLExtracting:
		r.DrawTextCentered("Extracting…", 0, mid-fontH/2, r.W, mt[0], mt[1], mt[2])

	case zipDLDone:
		r.DrawTextCentered("Extraction complete!", 0, mid-fontH-8, r.W, 80, 200, 80)
		count := fmt.Sprintf("%d file(s) extracted", len(s.extracted))
		r.DrawSmallTextCentered(count, 0, mid+4, r.W, ht[0], ht[1], ht[2])

		// List filenames, capped to available space so they never overflow the footer.
		rowH := smallFH + 4
		y := mid + 4 + smallFH + 12
		bottomLimit := r.H - footerH - 8
		if s.musicFailed {
			bottomLimit -= rowH // reserve a row for the warning
		}
		shown := 0
		for _, p := range s.extracted {
			if y+rowH > bottomLimit {
				break
			}
			r.DrawSmallScrollingText(filepath.Base(p), 20, y, r.W-40, 120, 120, 120)
			y += rowH
			shown++
		}
		if shown < len(s.extracted) {
			more := fmt.Sprintf("…and %d more file(s)", len(s.extracted)-shown)
			r.DrawSmallTextCentered(more, 0, y, r.W, 80, 80, 80)
		}
		if s.musicFailed {
			r.DrawSmallTextCentered("Note: music folder could not be created",
				0, r.H-footerH-8-smallFH, r.W, 200, 160, 60)
		}

	case zipDLError:
		y := contentTop + 8
		r.DrawText("Extraction failed:", 20, y, 200, 60, 60)
		y += fontH + 6
		r.DrawWrappedText(s.err.Error(), 20, y, r.W-40, fontH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.loadState() {
	case zipDLDownloading, zipDLExtracting:
		r.DrawSmallText("Please wait…", 10, ftrY, ht[0], ht[1], ht[2])
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *ZIPDownloadScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		if s.loadState() != zipDLDownloading && s.loadState() != zipDLExtracting {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE, sdl.K_RETURN:
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		if s.loadState() != zipDLDownloading && s.loadState() != zipDLExtracting {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
				return s.prev
			}
		}
	}
	return s
}

// IsBusy implements BusyChecker. Returns true while download or extraction is in flight.
func (s *ZIPDownloadScreen) IsBusy() bool {
	st := s.loadState()
	return st == zipDLDownloading || st == zipDLExtracting
}
