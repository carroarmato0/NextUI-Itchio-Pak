//go:build !headless

package ui

import (
	"archive/zip"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bodgit/sevenzip"

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

	// 7z archives are extracted via sevenzip; everything else uses archive/zip.
	if strings.ToLower(filepath.Ext(s.plan.Upload.Filename)) == ".7z" {
		s.run7z(tmpPath)
		return
	}

	r, err := zip.OpenReader(tmpPath)
	if err != nil {
		logger.Error("zip-download: open ZIP: %v", err)
		s.err = fmt.Errorf("open ZIP: %w", err)
		s.storeState(zipDLError)
		return
	}
	defer r.Close()

	// Pico-8 multi-file: path-preserving extraction to game subdirectory.
	if s.plan.Pico8GameDir != "" {
		now := time.Now()
		s.extractPico8ZIP(&r.Reader, now)
		if err := s.inv.Save(s.invPath); err != nil {
			logger.Warn("zip-download: save inventory: %v", err)
		}
		// Cover art and .m3u launcher for multi-file Pico-8 games.
		if len(s.extracted) > 0 {
			gameDir := strings.TrimSuffix(s.plan.Pico8GameDir, "/")

			// Cover art: artRef is <gameDir>.p8 so CoverArtPath places the image
			// in the PARENT directory's .media/ — where NextUI looks for directory art.
			artRef := gameDir + ".p8"
			if artErr := s.client.DownloadCoverArt(s.game.CoverURL, artRef); artErr != nil {
				logger.Warn("zip-download: pico8 cover art: %v", artErr)
			}

			// .m3u launcher: collect .p8/.p8.png files, sort naturally, write
			// <safe>.m3u inside the game directory so the emulator loads all carts.
			safe := roms.SanitiseFilename(s.game.Title, "")
			if safe == "" {
				safe = "Unknown"
			}
			var p8Files []string
			for _, dest := range s.extracted {
				ext := strings.ToLower(roms.ROMExt(filepath.Base(dest)))
				if ext == ".p8" || ext == ".p8.png" {
					p8Files = append(p8Files, filepath.Base(dest))
				}
			}
			if len(p8Files) > 1 {
				sort.Slice(p8Files, func(i, j int) bool { return naturalLess(p8Files[i], p8Files[j]) })
				m3uPath := filepath.Join(gameDir, safe+".m3u")
				if err := os.WriteFile(m3uPath, []byte(strings.Join(p8Files, "\n")+"\n"), 0644); err != nil {
					logger.Warn("zip-download: pico8 m3u write: %v", err)
				} else {
					logger.Info("zip-download: pico8 m3u written %s (%d carts)", m3uPath, len(p8Files))
					s.inv.Add(s.game.URL, inventory.Entry{
						GameURL: s.game.URL, Title: s.game.Title,
						Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
					}, inventory.DownloadedFile{
						Filename:      filepath.Base(m3uPath),
						DestPath:      m3uPath,
						DownloadedAt:  now,
						FileType:      inventory.FileTypeM3U,
						SourceArchive: s.plan.Upload.Filename,
					})
				}
			}
		}
		if len(s.extracted) == 0 {
			logger.Error("zip-download: pico8: no files extracted (skipped=%d)", len(s.skipped))
			s.err = fmt.Errorf("no Pico-8 files could be extracted from ZIP")
			s.storeState(zipDLError)
			return
		}
		logger.Info("zip-download: pico8 done, extracted %d file(s)", len(s.extracted))
		s.storeState(zipDLDone)
		return
	}

	now := time.Now()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if roms.IsInMacOSMetaDir(f.Name) {
			continue
		}
		baseName := filepath.Base(f.Name)
		// macOS resource-fork stubs start with "._"; skip them.
		if strings.HasPrefix(baseName, "._") {
			continue
		}
		kind, baseName := classifyWithMagic(baseName, f.Open)

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

// run7z handles extraction for 7z archives using the same plan logic as run().
func (s *ZIPDownloadScreen) run7z(tmpPath string) {
	r, err := sevenzip.OpenReader(tmpPath)
	if err != nil {
		logger.Error("7z-download: open archive: %v", err)
		s.err = fmt.Errorf("open 7z: %w", err)
		s.storeState(zipDLError)
		return
	}
	defer r.Close()

	if s.plan.Pico8GameDir != "" {
		now := time.Now()
		s.extractPico8_7z(r, now)
		if err := s.inv.Save(s.invPath); err != nil {
			logger.Warn("7z-download: save inventory: %v", err)
		}
		if len(s.extracted) == 0 {
			logger.Error("7z-download: pico8: no files extracted (skipped=%d)", len(s.skipped))
			s.err = fmt.Errorf("no Pico-8 files could be extracted from 7z")
			s.storeState(zipDLError)
			return
		}
		logger.Info("7z-download: pico8 done, extracted %d file(s)", len(s.extracted))
		s.storeState(zipDLDone)
		return
	}

	now := time.Now()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if roms.IsInMacOSMetaDir(f.Name) {
			continue
		}
		baseName := filepath.Base(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.HasPrefix(baseName, "._") {
			continue
		}
		kind, baseName := classifyWithMagic(baseName, f.Open)
		switch kind {
		case roms.KindROM:
			if !s.shouldExtractROM(baseName) {
				continue
			}
			dest, err := s.extractROMFromOpener(f.Open, f.FileInfo().Size(), baseName, now)
			if err != nil {
				logger.Warn("7z-download: ROM %s: %v", baseName, err)
				s.skipped = append(s.skipped, baseName)
				continue
			}
			s.extracted = append(s.extracted, dest)
		case roms.KindMusic:
			if !s.plan.DownloadMusic || s.plan.MusicDir == "" {
				continue
			}
			dest, err := s.extractMusicFromOpener(f.Open, baseName, now)
			if err != nil {
				logger.Warn("7z-download: music %s: %v", baseName, err)
				s.skipped = append(s.skipped, baseName)
				continue
			}
			s.extracted = append(s.extracted, dest)
		}
	}

	if err := s.inv.Save(s.invPath); err != nil {
		logger.Warn("7z-download: save inventory: %v", err)
	}
	if len(s.extracted) == 0 {
		logger.Error("7z-download: no files extracted (skipped=%d)", len(s.skipped))
		s.err = fmt.Errorf("no files could be extracted from 7z")
		s.storeState(zipDLError)
		return
	}
	logger.Info("7z-download: done, extracted %d file(s)", len(s.extracted))
	s.storeState(zipDLDone)
}

// extractPico8_7z extracts .p8, .p8.png, and .lua files from a 7z archive,
// preserving relative paths into s.plan.Pico8GameDir.
func (s *ZIPDownloadScreen) extractPico8_7z(r *sevenzip.ReadCloser, now time.Time) {
	gameDir := strings.TrimSuffix(s.plan.Pico8GameDir, "/")
	var relevantPaths []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if roms.IsInMacOSMetaDir(f.Name) {
			continue
		}
		name := filepath.ToSlash(strings.ReplaceAll(f.Name, "\\", "/"))
		base := filepath.Base(name)
		if strings.HasPrefix(base, "._") {
			continue
		}
		lower := strings.ToLower(base)
		ext := strings.ToLower(roms.ROMExt(base))
		if ext == ".p8" || ext == ".p8.png" || strings.HasSuffix(lower, ".lua") {
			relevantPaths = append(relevantPaths, name)
		}
	}
	prefix := commonPathPrefix(relevantPaths)

	p8PNGCount := 0
	for _, p := range relevantPaths {
		if strings.ToLower(roms.ROMExt(filepath.Base(p))) == ".p8.png" {
			p8PNGCount++
		}
	}
	unifyP8PNG := p8PNGCount == 1 && s.cfg.UnifiedNaming
	if unifyP8PNG {
		if inv, ok := s.inv.Lookup(s.game.URL); ok && inv.UnifiedNamingDisabled {
			unifyP8PNG = false
		}
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if roms.IsInMacOSMetaDir(f.Name) {
			continue
		}
		name := filepath.ToSlash(strings.ReplaceAll(f.Name, "\\", "/"))
		base := filepath.Base(name)
		if strings.HasPrefix(base, "._") {
			continue
		}
		lower := strings.ToLower(base)
		ext := strings.ToLower(roms.ROMExt(base))
		if ext != ".p8" && ext != ".p8.png" && !strings.HasSuffix(lower, ".lua") {
			continue
		}
		relPath := strings.TrimPrefix(name, prefix)
		dest := filepath.Join(gameDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			s.skipped = append(s.skipped, base)
			continue
		}
		if err := extractEntry(f.Open, dest); err != nil {
			logger.Warn("7z-download: pico8 extract %s: %v", base, err)
			s.skipped = append(s.skipped, base)
			continue
		}

		finalDest := dest
		unifiedName := false
		if ext == ".p8.png" && unifyP8PNG {
			if newDest, didRename := roms.ResolveUnifiedDest(dest, s.game.Title, true); didRename {
				if err := os.Rename(dest, newDest); err != nil {
					logger.Warn("7z-download: pico8 unified rename: %v", err)
				} else {
					finalDest = newDest
					unifiedName = true
				}
			} else {
				unifiedName = true
			}
		}

		logger.Info("7z-download: pico8 extracted %s → %s", base, finalDest)
		s.extracted = append(s.extracted, finalDest)
		s.inv.Add(s.game.URL, inventory.Entry{
			GameURL: s.game.URL, Title: s.game.Title,
			Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
		}, inventory.DownloadedFile{
			Filename:      filepath.Base(finalDest),
			DestPath:      finalDest,
			DownloadedAt:  now,
			FileType:      inventory.FileTypeROM,
			UnifiedName:   unifiedName,
			SourceArchive: s.plan.Upload.Filename,
		})
	}
}

// extractROMFromOpener is like extractROM but takes an opener func instead of *zip.File.
// Used by run7z so the same inventory/naming logic applies to 7z entries.
func (s *ZIPDownloadScreen) extractROMFromOpener(open func() (io.ReadCloser, error), size int64, baseName string, now time.Time) (string, error) {
	ext := strings.ToLower(roms.ROMExt(baseName))
	destDir := s.plan.ROMDirs[ext]
	if destDir == "" {
		destDir = roms.DestinationDir(ext, s.cfg.Pico8Core)
	}
	stem := strings.TrimSuffix(baseName, roms.ROMExt(baseName))
	safeName := roms.SanitiseFilename(stem, ext)
	if safeName == "" {
		safeName = baseName
	}
	dest := destDir + safeName

	// Skip when an identical ROM already exists.
	if existing := s.findIdenticalFromOpener(open, size, ext); existing != "" {
		logger.Info("7z-download: ROM %s: identical file at %s, skipping", baseName, existing)
		s.backfillSourceArchive(existing)
		return existing, nil
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("mkdirall %s: %w", destDir, err)
	}
	if err := extractEntry(open, dest); err != nil {
		return "", err
	}

	finalDest := dest
	unifiedName := false
	if s.cfg.UnifiedNaming {
		entry, entryExists := s.inv.Lookup(s.game.URL)
		disabled := entryExists && entry.UnifiedNamingDisabled
		if !disabled {
			newDest, didRename := roms.ResolveUnifiedDest(dest, s.game.Title, true)
			if didRename {
				if err := os.Rename(dest, newDest); err != nil {
					logger.Warn("7z-download: unified rename: %v", err)
				} else {
					finalDest = newDest
					unifiedName = true
				}
			} else {
				unifiedName = true
			}
		}
	}
	logger.Info("7z-download: ROM extracted → %s (unified=%v)", finalDest, unifiedName)
	s.inv.Add(s.game.URL, inventory.Entry{
		GameURL: s.game.URL, Title: s.game.Title,
		Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
	}, inventory.DownloadedFile{
		Filename:      filepath.Base(finalDest),
		DestPath:      finalDest,
		DownloadedAt:  now,
		FileType:      inventory.FileTypeROM,
		UnifiedName:   unifiedName,
		SourceArchive: s.plan.Upload.Filename,
	})
	if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
		logger.Warn("7z-download: cover art: %v", artErr)
	}
	return finalDest, nil
}

// extractMusicFromOpener is like extractMusic but takes an opener func.
func (s *ZIPDownloadScreen) extractMusicFromOpener(open func() (io.ReadCloser, error), baseName string, now time.Time) (string, error) {
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
	if err := extractEntry(open, dest); err != nil {
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

// backfillSourceArchive patches SourceArchive into an existing inventory entry
// whose DestPath matches. Called when extraction is skipped because an identical
// file already exists — pre-fix entries have SourceArchive="" which causes the
// update service to incorrectly mark the game as removed.
func (s *ZIPDownloadScreen) backfillSourceArchive(destPath string) {
	if s.plan.Upload.Filename == "" {
		return
	}
	entry, ok := s.inv.Lookup(s.game.URL)
	if !ok {
		return
	}
	for _, f := range entry.Files {
		if f.DestPath == destPath && f.SourceArchive == "" {
			f.SourceArchive = s.plan.Upload.Filename
			s.inv.UpdateFile(s.game.URL, destPath, f)
			logger.Debug("zip-download: backfilled SourceArchive=%q for %s",
				s.plan.Upload.Filename, filepath.Base(destPath))
			return
		}
	}
}

// findIdenticalFromOpener checks the inventory for a ROM matching the given
// opener's content. Used by the 7z extraction path.
func (s *ZIPDownloadScreen) findIdenticalFromOpener(open func() (io.ReadCloser, error), size int64, ext string) string {
	entry, ok := s.inv.Lookup(s.game.URL)
	if !ok {
		return ""
	}
	wantHash, err := entryMD5(open)
	if err != nil {
		return ""
	}
	for _, df := range entry.Files {
		if df.FileType != inventory.FileTypeROM {
			continue
		}
		if strings.ToLower(roms.ROMExt(df.DestPath)) != ext {
			continue
		}
		fi, err := os.Stat(df.DestPath)
		if err != nil || fi.Size() != size {
			continue
		}
		if hash, err := fileMD5(df.DestPath); err == nil && hash == wantHash {
			return df.DestPath
		}
	}
	return ""
}

// classifyWithMagic calls ClassifyEntry on baseName; if the extension is
// unrecognised (KindOther) it reads the first roms.DetectBufSize bytes via
// open() and retries with magic-byte detection. Returns the resolved
// FileKind and the (possibly extension-corrected) baseName.
// This handles archive entries whose filename uses a generic extension such
// as ".bin" for a Sega Genesis ROM — extension lookup fails but the ROM
// header signature at 0x100 reliably identifies the format.
func classifyWithMagic(baseName string, open func() (io.ReadCloser, error)) (roms.FileKind, string) {
	kind := roms.ClassifyEntry(baseName)
	if kind != roms.KindOther {
		return kind, baseName
	}
	// Don't promote image files to ROMs via magic-byte detection: a .png
	// named .png is artwork even if it is 128 px wide.
	if roms.IsImageExt(strings.ToLower(filepath.Ext(baseName))) {
		return kind, baseName
	}
	rc, err := open()
	if err != nil {
		return kind, baseName
	}
	buf := make([]byte, roms.DetectBufSize)
	n, _ := io.ReadFull(rc, buf)
	rc.Close()
	detected := roms.DetectROMExt(buf[:n])
	if detected == "" {
		return kind, baseName
	}
	stem := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	return roms.KindROM, stem + detected
}

func (s *ZIPDownloadScreen) shouldExtractROM(name string) bool {
	if !s.plan.DownloadROMs {
		return false
	}
	if len(s.plan.SelectedROMs) == 0 {
		return true
	}
	ext := strings.ToLower(roms.ROMExt(name))
	chosen, ok := s.plan.SelectedROMs[ext]
	if !ok {
		return true
	}
	return chosen == name
}

func (s *ZIPDownloadScreen) extractROM(f *zip.File, baseName string, now time.Time) (string, error) {
	ext := strings.ToLower(roms.ROMExt(baseName))
	destDir := s.plan.ROMDirs[ext]
	if destDir == "" {
		destDir = roms.DestinationDir(ext, s.cfg.Pico8Core)
	}
	stem := strings.TrimSuffix(baseName, roms.ROMExt(baseName))
	safeName := roms.SanitiseFilename(stem, ext)
	if safeName == "" {
		safeName = baseName
	}
	dest := destDir + safeName

	// Skip extraction when the game already has an identical ROM on disk.
	if existing := s.findIdenticalROMInInventory(f, ext); existing != "" {
		logger.Info("zip-download: ROM %s: identical file already at %s, skipping", baseName, existing)
		s.backfillSourceArchive(existing)
		return existing, nil
	}

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
			newDest, didRename := roms.ResolveUnifiedDest(dest, s.game.Title, true)
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

	if ext == ".p8.png" {
		if artErr := itchio.CopyCoverArt(finalDest); artErr != nil {
			logger.Warn("zip-download: cover art copy: %v", artErr)
		}
	} else if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
		logger.Warn("zip-download: cover art: %v", artErr)
	}
	s.inv.Add(s.game.URL, inventory.Entry{
		GameURL: s.game.URL, Title: s.game.Title,
		Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
	}, inventory.DownloadedFile{
		Filename:      filepath.Base(finalDest),
		DestPath:      finalDest,
		DownloadedAt:  now,
		UnifiedName:   unifiedName,
		FileType:      inventory.FileTypeROM,
		SourceArchive: s.plan.Upload.Filename,
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

// extractPico8ZIP extracts all .p8, .p8.png, and .lua files from r into
// s.plan.Pico8GameDir, preserving relative paths from the ZIP after stripping
// any common top-level wrapper directory. Support files (.lua) required by
// Pico-8 carts are extracted alongside the cartridges.
func (s *ZIPDownloadScreen) extractPico8ZIP(r *zip.Reader, now time.Time) {
	gameDir := strings.TrimSuffix(s.plan.Pico8GameDir, "/")

	// Collect all relevant file paths to determine the common prefix to strip.
	var relevantPaths []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if roms.IsInMacOSMetaDir(f.Name) {
			continue
		}
		name := filepath.ToSlash(f.Name)
		base := filepath.Base(name)
		if strings.HasPrefix(base, "._") {
			continue
		}
		lower := strings.ToLower(base)
		ext := strings.ToLower(roms.ROMExt(base))
		if ext == ".p8" || ext == ".p8.png" || strings.HasSuffix(lower, ".lua") {
			relevantPaths = append(relevantPaths, name)
		}
	}
	prefix := commonPathPrefix(relevantPaths)
	logger.Debug("zip-download: pico8 strip-prefix=%q game-dir=%s", prefix, gameDir)

	// Apply unified naming to the .p8.png only when it is the sole compiled
	// cart in the ZIP. Multiple .p8.png files indicate a genuine multi-cart
	// game where per-file names are meaningful and must not be collapsed.
	p8PNGCount := 0
	for _, p := range relevantPaths {
		if strings.ToLower(roms.ROMExt(filepath.Base(p))) == ".p8.png" {
			p8PNGCount++
		}
	}
	unifyP8PNG := p8PNGCount == 1 && s.cfg.UnifiedNaming
	if unifyP8PNG {
		if inv, ok := s.inv.Lookup(s.game.URL); ok && inv.UnifiedNamingDisabled {
			unifyP8PNG = false
		}
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if roms.IsInMacOSMetaDir(f.Name) {
			continue
		}
		name := filepath.ToSlash(f.Name)
		base := filepath.Base(name)
		if strings.HasPrefix(base, "._") {
			continue
		}
		lower := strings.ToLower(base)
		ext := strings.ToLower(roms.ROMExt(base))
		isP8 := ext == ".p8" || ext == ".p8.png"
		isLua := strings.HasSuffix(lower, ".lua")
		if !isP8 && !isLua {
			continue
		}

		relPath := strings.TrimPrefix(name, prefix)
		dest := filepath.Join(gameDir, filepath.FromSlash(relPath))

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			logger.Warn("zip-download: pico8 mkdir %s: %v", filepath.Dir(dest), err)
			s.skipped = append(s.skipped, base)
			continue
		}
		if err := extractZIPEntry(f, dest); err != nil {
			logger.Warn("zip-download: pico8 extract %s: %v", base, err)
			s.skipped = append(s.skipped, base)
			continue
		}

		finalDest := dest
		unifiedName := false
		if ext == ".p8.png" && unifyP8PNG {
			if newDest, didRename := roms.ResolveUnifiedDest(dest, s.game.Title, true); didRename {
				if err := os.Rename(dest, newDest); err != nil {
					logger.Warn("zip-download: pico8 unified rename: %v", err)
				} else {
					finalDest = newDest
					unifiedName = true
				}
			} else {
				unifiedName = true
			}
		}

		logger.Info("zip-download: pico8 extracted %s → %s", base, finalDest)
		s.extracted = append(s.extracted, finalDest)

		s.inv.Add(s.game.URL, inventory.Entry{
			GameURL: s.game.URL, Title: s.game.Title,
			Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
		}, inventory.DownloadedFile{
			Filename:      filepath.Base(finalDest),
			DestPath:      finalDest,
			DownloadedAt:  now,
			FileType:      inventory.FileTypeROM,
			UnifiedName:   unifiedName,
			SourceArchive: s.plan.Upload.Filename,
		})
	}
}

// findIdenticalROMInInventory returns the DestPath of an already-downloaded ROM
// for this game that has byte-for-byte identical content to the ZIP entry f.
// Size is checked first (cheap); MD5 is computed only when sizes match.
// Returns "" when no identical file is found or the check cannot be performed.
func (s *ZIPDownloadScreen) findIdenticalROMInInventory(f *zip.File, ext string) string {
	entry, ok := s.inv.Lookup(s.game.URL)
	if !ok {
		return ""
	}
	wantSize := f.FileInfo().Size()
	wantHash, err := zipEntryMD5(f)
	if err != nil {
		logger.Warn("zip-download: dedup hash failed for %s: %v", f.Name, err)
		return ""
	}
	for _, df := range entry.Files {
		if df.FileType != inventory.FileTypeROM {
			continue
		}
		if strings.ToLower(roms.ROMExt(df.DestPath)) != ext {
			continue
		}
		fi, err := os.Stat(df.DestPath)
		if err != nil || fi.Size() != wantSize {
			continue
		}
		hash, err := fileMD5(df.DestPath)
		if err != nil {
			continue
		}
		if hash == wantHash {
			return df.DestPath
		}
	}
	return ""
}

// entryMD5 reads the uncompressed content via open() and returns its MD5 hex digest.
func entryMD5(open func() (io.ReadCloser, error)) (string, error) {
	rc, err := open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	h := md5.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// zipEntryMD5 is a convenience wrapper around entryMD5 for zip.File.
func zipEntryMD5(f *zip.File) (string, error) { return entryMD5(f.Open) }

// fileMD5 returns the MD5 hex digest of the file at path.
func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// extractEntry copies the content returned by open() to dest on disk.
// Used for both ZIP and 7z entries.
func extractEntry(open func() (io.ReadCloser, error), dest string) error {
	rc, err := open()
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

// extractZIPEntry is a convenience wrapper around extractEntry for zip.File.
func extractZIPEntry(f *zip.File, dest string) error {
	return extractEntry(f.Open, dest)
}

// commonPathPrefix returns the longest common directory path shared by all
// paths, including the trailing slash. Returns "" when paths share no common
// parent directory (files at the ZIP root, or in entirely different subtrees).
//
// Unlike the former commonPathPrefix which stopped after the first component,
// this strips ALL shared leading directory levels so that packaging structures
// like "GameName/pico8/cart.p8" are fully unwrapped:
//
//	["game/pico8/a.p8", "game/pico8/b.p8"]  → "game/pico8/"
//	["game/cart_a/a.p8", "game/cart_b/b.p8"] → "game/"
//	["a.p8", "b.p8"]                         → ""
func commonPathPrefix(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	dirParts := func(p string) []string {
		p = filepath.ToSlash(p)
		idx := strings.LastIndex(p, "/")
		if idx < 0 {
			return nil
		}
		return strings.Split(p[:idx], "/")
	}
	parts := dirParts(paths[0])
	if len(parts) == 0 {
		return ""
	}
	for _, p := range paths[1:] {
		other := dirParts(p)
		n := len(parts)
		if len(other) < n {
			n = len(other)
		}
		for i := 0; i < n; i++ {
			if parts[i] != other[i] {
				n = i
				break
			}
		}
		parts = parts[:n]
		if len(parts) == 0 {
			return ""
		}
	}
	return strings.Join(parts, "/") + "/"
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
		r.DrawTextCentered("Extracting", 0, mid-fontH-10, r.W, mt[0], mt[1], mt[2])
		drawLoadingDots(r, mid+8)

	case zipDLDone:
		// Centre the title + count block, then list filenames below.
		const doneGap = int32(8)
		blockH := fontH + doneGap + smallFH
		blockY := mid - blockH/2
		r.DrawTextCentered("Extraction complete!", 0, blockY, r.W, 80, 200, 80)
		count := fmt.Sprintf("%d file(s) extracted", len(s.extracted))
		r.DrawSmallTextCentered(count, 0, blockY+fontH+doneGap, r.W, ht[0], ht[1], ht[2])

		// List filenames, capped to available space so they never overflow the footer.
		rowH := smallFH + 4
		y := blockY + blockH + 12
		bottomLimit := r.H - footerH - 8
		if s.musicFailed {
			bottomLimit -= rowH // reserve a row for the warning
		}
		shown := 0
		for i, p := range s.extracted {
			remaining := len(s.extracted) - i
			// Stop one row early when more items follow so the "…and N more"
			// summary line fits within bottomLimit.
			if y+rowH > bottomLimit || (remaining > 1 && y+rowH*2 > bottomLimit) {
				break
			}
			r.DrawSmallTextCentered(truncateSmallToWidth(r, filepath.Base(p), r.W-40), 0, y, r.W, 120, 120, 120)
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

// naturalLess compares two strings using natural sort order so that numeric
// substrings are compared as integers (poom_9.p8 < poom_10.p8).
func naturalLess(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		// Consume matching non-digit prefix.
		i := 0
		for i < len(a) && i < len(b) && (a[i] < '0' || a[i] > '9') && (b[i] < '0' || b[i] > '9') {
			if a[i] != b[i] {
				return a[i] < b[i]
			}
			i++
		}
		a, b = a[i:], b[i:]
		if len(a) == 0 || len(b) == 0 {
			break
		}
		// One or both strings are at a digit run.
		aIsDigit := a[0] >= '0' && a[0] <= '9'
		bIsDigit := b[0] >= '0' && b[0] <= '9'
		if !aIsDigit || !bIsDigit {
			return a[0] < b[0]
		}
		// Parse both numeric runs.
		ai, bi := 0, 0
		for ai < len(a) && a[ai] >= '0' && a[ai] <= '9' {
			ai++
		}
		for bi < len(b) && b[bi] >= '0' && b[bi] <= '9' {
			bi++
		}
		na, _ := strconv.Atoi(a[:ai])
		nb, _ := strconv.Atoi(b[:bi])
		if na != nb {
			return na < nb
		}
		a, b = a[ai:], b[bi:]
	}
	return len(a) < len(b)
}
