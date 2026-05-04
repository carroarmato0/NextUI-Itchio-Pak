package inventory

import (
	"os"
	"path/filepath"
	"strings"
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

// Stop signals the goroutine to exit. Idempotent.
func (s *UpdateService) Stop() {
	select {
	case s.stopCh <- struct{}{}:
	default:
	}
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
	s.inv.mu.Lock()
	urls := make([]string, 0, len(s.inv.Entries))
	for url := range s.inv.Entries {
		urls = append(urls, url)
	}
	s.inv.mu.Unlock()

	logger.Info("update-svc: checking %d inventory entries", len(urls))

	// Collect upstream file lists during the check loop but do NOT write them
	// to the inventory yet. Applying them all at once at the end means the UI
	// never sees a state where badges have changed but sort order has not.
	pendingFiles := make(map[string][]UpstreamFile)
	for _, gameURL := range urls {
		if files := s.checkEntry(gameURL); files != nil {
			pendingFiles[gameURL] = files
		}
	}

	for gameURL, files := range pendingFiles {
		s.inv.SetUpstreamFiles(gameURL, files)
	}
	if err := s.inv.Save(s.inventoryPath); err != nil {
		logger.Error("update-svc: save: %v", err)
	}

	logger.Info("update-svc: check complete")
}

// checkEntry runs cover-art repair and the upstream check for one game.
// For free games it returns the upstream file list to apply later (batched);
// for paid games it returns nil.
func (s *UpdateService) checkEntry(gameURL string) []UpstreamFile {
	s.inv.mu.Lock()
	entry, ok := s.inv.Entries[gameURL]
	if !ok {
		s.inv.mu.Unlock()
		return nil
	}
	// Snapshot without holding the lock during I/O.
	coverURL := entry.CoverURL
	isFree := entry.IsFree
	files := append([]DownloadedFile(nil), entry.Files...)
	s.inv.mu.Unlock()

	// 1. Cover art repair.
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

	// 2. Upstream check.
	if isFree {
		return s.checkFreeGame(gameURL, files)
	}
	s.checkPaidGame(gameURL)
	return nil
}

// isGameRemoved reports whether err indicates a 404 or 410 HTTP response.
func isGameRemoved(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 404") || strings.Contains(s, "HTTP 410")
}

// checkFreeGame fetches the current upload list and returns it for the caller
// to apply via SetUpstreamFiles (batched at end-of-check). Returns nil on error.
func (s *UpdateService) checkFreeGame(gameURL string, downloadedFiles []DownloadedFile) []UpstreamFile {
	uploads, err := s.client.FetchUploads(gameURL)
	if err != nil {
		if isGameRemoved(err) {
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: game removed (404) %s", gameURL)
		} else {
			logger.Warn("update-svc: transient error for %s: %v", gameURL, err)
		}
		return nil
	}

	upstreamFiles := make([]UpstreamFile, 0, len(uploads))
	upstreamNames := make(map[string]bool, len(uploads)*2)
	for _, u := range uploads {
		upstreamFiles = append(upstreamFiles, UpstreamFile{
			Filename: u.Filename,
			UploadID: u.UploadID,
			SeenAt:   time.Now(), // preserved for known files by SetUpstreamFiles
		})
		upstreamNames[u.Filename] = true
		if stem := strings.TrimSuffix(u.Filename, filepath.Ext(u.Filename)); stem != u.Filename {
			upstreamNames[stem] = true
		}
	}

	// Warn if any downloaded file is no longer offered upstream.
	for _, f := range downloadedFiles {
		stem := strings.TrimSuffix(f.Filename, filepath.Ext(f.Filename))
		if !upstreamNames[f.Filename] && !upstreamNames[stem] {
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: downloaded file %q no longer available upstream for %s", f.Filename, gameURL)
			return upstreamFiles
		}
	}

	// All downloaded files still available — clear any stale removal state.
	s.inv.MarkReachable(gameURL)
	logger.Debug("update-svc: %s — %d upstream file(s) recorded", gameURL, len(upstreamFiles))
	return upstreamFiles
}

func (s *UpdateService) checkPaidGame(gameURL string) {
	_, err := s.client.FetchGameDetail(gameURL)
	if err != nil {
		if isGameRemoved(err) {
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: paid game removed (404) %s", gameURL)
		} else {
			logger.Warn("update-svc: transient error for paid game %s: %v", gameURL, err)
		}
		return
	}
	s.inv.MarkReachable(gameURL)

	s.inv.mu.Lock()
	if e, ok := s.inv.Entries[gameURL]; ok {
		e.UpdateCheckedAt = time.Now()
	}
	s.inv.mu.Unlock()
	logger.Debug("update-svc: paid game %s reachable, no file diff", gameURL)
}
