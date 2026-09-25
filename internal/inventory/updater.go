package inventory

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// UpdateService checks each inventory entry for missing cover art, removed
// games, and new upstream files. It runs once at startup and re-runs each time
// TriggerNow is called.
type UpdateService struct {
	inv           *Inventory
	inventoryPath string
	client        *itchio.Client
	notify        func()
	triggerCh     chan struct{} // buffered(1): absorbs duplicate triggers
	stopCh        chan struct{}
	stopOnce      sync.Once
	running       atomic.Bool
}

// NewUpdateService constructs an UpdateService. notify (may be nil) is called
// after each runCheck completes; use it to push an SDL UserEvent from the
// caller without importing SDL here.
func NewUpdateService(inv *Inventory, inventoryPath string, client *itchio.Client, notify func()) *UpdateService {
	return &UpdateService{
		inv:           inv,
		inventoryPath: inventoryPath,
		client:        client,
		notify:        notify,
		triggerCh:     make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
	}
}

// Start launches the background goroutine and runs the first check immediately.
// onDone is called after the first check completes (for tests; may be nil).
func (s *UpdateService) Start(onDone func()) {
	go func() {
		s.running.Store(true)
		s.runCheck()
		s.running.Store(false)
		if onDone != nil {
			onDone()
		}
		if s.notify != nil {
			s.notify()
		}
		for {
			select {
			case <-s.triggerCh:
				s.running.Store(true)
				s.runCheck()
				s.running.Store(false)
				if onDone != nil {
					onDone()
				}
				if s.notify != nil {
					s.notify()
				}
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop signals the goroutine to exit. Idempotent — safe to call multiple times.
func (s *UpdateService) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// TriggerNow queues a re-check. Non-blocking; a pending check absorbs the signal.
func (s *UpdateService) TriggerNow() {
	select {
	case s.triggerCh <- struct{}{}:
		logger.Info("update-svc: manual check triggered")
	default:
		logger.Debug("update-svc: trigger ignored (check already queued)")
	}
}

// IsRunning reports whether runCheck is currently executing.
func (s *UpdateService) IsRunning() bool {
	return s.running.Load()
}

// LatestCheckedAt delegates to the inventory's LatestCheckedAt.
func (s *UpdateService) LatestCheckedAt() time.Time {
	return s.inv.LatestCheckedAt()
}

func (s *UpdateService) runCheck() {
	s.inv.VerifyAndClean(s.inventoryPath)

	s.inv.mu.Lock()
	urls := make([]string, 0, len(s.inv.Entries))
	for url := range s.inv.Entries {
		urls = append(urls, url)
	}
	s.inv.mu.Unlock()

	token := s.client.AuthToken()
	logger.Info("update-svc: checking %d inventory entries (signed in: %v)", len(urls), token != "")

	// Pass 1: data.json for every game — public, and it triggers nothing on
	// itch.io's side. It says whether the game still exists, its ID, and
	// whether it is paid.
	games := make(map[string]*itchio.GameData, len(urls))
	var paidIDs []string
	for _, gameURL := range urls {
		s.repairCoverArt(gameURL)
		d, err := s.client.FetchGameData(gameURL)
		switch {
		case isGameRemoved(err):
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: game removed (404) %s", gameURL)
			continue
		case err != nil:
			logger.Warn("update-svc: transient error for %s: %v", gameURL, err)
			continue
		}
		games[gameURL] = d
		if token != "" && d.Pricing() == itchio.PricingPaid && d.ID != 0 {
			paidIDs = append(paidIDs, strconv.FormatInt(d.ID, 10))
		}
	}

	// Pass 2: one owned-keys request for every installed paid game.
	var keys map[string]string
	if len(paidIDs) > 0 {
		var err error
		if keys, err = s.client.OwnedKeysForGames(token, paidIDs); err != nil {
			logger.Warn("update-svc: owned keys unavailable, paid games use the page: %v", err)
		}
	}

	// Pass 3: the file list of each game. Collected here and applied all at
	// once, so the UI never sees badges change before the sort order does.
	type result struct {
		source string
		files  []UpstreamFile
	}
	pending := make(map[string]result)
	for gameURL, d := range games {
		if source, files := s.checkGame(gameURL, d, token, keys); files != nil {
			pending[gameURL] = result{source, files}
		}
	}

	for gameURL, r := range pending {
		s.inv.SetUpstreamFilesFrom(gameURL, r.source, r.files)
	}
	if err := s.inv.Save(s.inventoryPath); err != nil {
		logger.Error("update-svc: save: %v", err)
	}

	logger.Info("update-svc: check complete")
}

// repairCoverArt downloads cover art that has gone missing from disk.
func (s *UpdateService) repairCoverArt(gameURL string) {
	s.inv.mu.Lock()
	entry, ok := s.inv.Entries[gameURL]
	if !ok {
		s.inv.mu.Unlock()
		return
	}
	coverURL := entry.CoverURL
	files := append([]DownloadedFile(nil), entry.Files...)
	s.inv.mu.Unlock()

	for _, f := range files {
		artPath := CoverArtPath(coverURL, f.DestPath)
		if artPath == "" {
			continue
		}
		if _, err := os.Stat(artPath); err == nil {
			logger.Debug("update-svc: cover art present for %s", f.Filename)
			continue
		}
		logger.Info("update-svc: repairing cover art for %s", f.Filename)
		if err := s.client.DownloadCoverArt(coverURL, f.DestPath); err != nil {
			logger.Error("update-svc: cover art repair failed for %s: %v", f.Filename, err)
		}
	}
}

// isGameRemoved reports whether err indicates a 404 or 410 HTTP response.
func isGameRemoved(err error) bool {
	return errors.Is(err, itchio.ErrGameRemoved)
}

// checkGame reads one game's upstream files and returns them with their
// source, or nil when nothing should be recorded.
//
// Signed in, the API lists them — for a paid game with the owned download
// key. That is what itch.io asked for: no browser-style requests, and the
// API says when a file's content changes even if its name does not. Signed
// out, or for a paid game not owned, the public page's list is read. Neither
// path ever starts the web download flow.
func (s *UpdateService) checkGame(gameURL string, d *itchio.GameData, token string, keys map[string]string) (string, []UpstreamFile) {
	if token != "" && d.ID != 0 {
		id := strconv.FormatInt(d.ID, 10)
		key, owned := keys[id]
		if d.Pricing() != itchio.PricingPaid || owned {
			files, err := s.apiFiles(gameURL, id, token, key)
			if err == nil {
				return s.recordFiles(gameURL, SourceAPI, files)
			}
			logger.Warn("update-svc: API listing failed for %s, using the page: %v", gameURL, err)
		} else {
			logger.Debug("update-svc: %s is paid and not owned; using the page", gameURL)
		}
	}
	names, err := s.client.FetchPageUploadNames(gameURL)
	if err != nil {
		if isGameRemoved(err) {
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: game removed (404) %s", gameURL)
		} else {
			logger.Warn("update-svc: transient error for %s: %v", gameURL, err)
		}
		return "", nil
	}
	files := make([]UpstreamFile, 0, len(names))
	for _, n := range names {
		files = append(files, UpstreamFile{Filename: n, SeenAt: time.Now()})
	}
	return s.recordFiles(gameURL, SourcePage, files)
}

// apiFiles lists a game's files through the API. The name recorded is the
// one itch.io shows (the display name when set), the same the page shows, so
// both sources describe a file the same way. Browser-played builds are not
// something a handheld downloads, so they are left out.
func (s *UpdateService) apiFiles(gameURL, gameID, token, keyID string) ([]UpstreamFile, error) {
	uploads, err := s.client.FetchUploadsForKey(token, gameID, keyID)
	if err != nil {
		return nil, err
	}
	files := make([]UpstreamFile, 0, len(uploads))
	for _, u := range uploads {
		if u.Type == "html" {
			continue
		}
		name := u.DisplayName
		if name == "" {
			name = u.Filename
		}
		files = append(files, UpstreamFile{
			Filename: name, UploadID: u.UploadID, SeenAt: time.Now(), Fingerprint: fingerprint(u),
		})
	}
	logger.Debug("update-svc: %s — %d file(s) via the API", gameURL, len(files))
	return files, nil
}

// fingerprint identifies an upload's content: its butler build when it has
// one, else its checksum, else its modification time and size.
func fingerprint(u itchio.Upload) string {
	switch {
	case u.BuildID != 0:
		return "build:" + strconv.FormatInt(u.BuildID, 10)
	case u.MD5 != "":
		return "md5:" + u.MD5
	case !u.UpdatedAt.IsZero():
		return "upd:" + u.UpdatedAt.UTC().Format(time.RFC3339) + "/" + strconv.FormatInt(u.Size, 10)
	}
	return ""
}

// recordFiles applies the removal rules to a file list and returns it for
// the batched write.
func (s *UpdateService) recordFiles(gameURL, source string, files []UpstreamFile) (string, []UpstreamFile) {
	// A reachable game that offers zero downloads is effectively gone — the
	// only non-404 case that counts as a removal.
	if len(files) == 0 {
		s.inv.MarkRemoved(gameURL)
		logger.Warn("update-svc: game offers no downloads %s", gameURL)
		return "", nil
	}
	s.logSuperseded(gameURL, files)
	// Reachable and offering downloads — clear any stale removal state.
	s.inv.MarkReachable(gameURL)
	logger.Debug("update-svc: %s — %d upstream file(s) recorded from %s", gameURL, len(files), source)
	return source, files
}

// logSuperseded notes (informationally) downloaded files no longer offered.
// This is NOT a removal: a version bump replaces the old upload with a new one
// (e.g. "Moss Moss 1.3.zip" → "Moss Moss 1.4.zip"), which surfaces to the user
// as a pending update via HasPendingUpdates. Music tracks extracted from ZIPs
// are never listed as upload filenames, so they are skipped.
func (s *UpdateService) logSuperseded(gameURL string, upstream []UpstreamFile) {
	names := make(map[string]bool, len(upstream)*2)
	for _, u := range upstream {
		names[u.Filename] = true
		if stem := strings.TrimSuffix(u.Filename, romFileExt(u.Filename)); stem != u.Filename {
			names[stem] = true
		}
	}
	s.inv.mu.Lock()
	var downloaded []DownloadedFile
	if e, ok := s.inv.Entries[gameURL]; ok {
		downloaded = append(downloaded, e.Files...)
	}
	s.inv.mu.Unlock()
	for _, f := range downloaded {
		if f.FileType == FileTypeMusic {
			continue
		}
		// For files extracted from archives (ZIP/7z), check the source archive
		// name rather than the extracted ROM's (possibly renamed) filename.
		checkName := f.Filename
		if f.SourceArchive != "" {
			checkName = f.SourceArchive
		}
		stem := strings.TrimSuffix(checkName, romFileExt(checkName))
		if !names[checkName] && !names[stem] {
			logger.Info("update-svc: downloaded file %q superseded upstream for %s (update available)", checkName, gameURL)
		}
	}
}
