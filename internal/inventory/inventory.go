package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

type DownloadedFile struct {
	Filename     string    `json:"filename"`
	DestPath     string    `json:"dest_path"`
	DownloadedAt time.Time `json:"downloaded_at"`
	UnifiedName  bool      `json:"unified_name,omitempty"`
}

type UpstreamFile struct {
	Filename string    `json:"filename"`
	UploadID string    `json:"upload_id"`
	SeenAt   time.Time `json:"seen_at"`
	IsNew    bool      `json:"is_new,omitempty"`
}

type Entry struct {
	GameURL            string         `json:"game_url"`
	Title              string         `json:"title"`
	Author             string         `json:"author"`
	CoverURL           string         `json:"cover_url"`
	Files              []DownloadedFile `json:"files"`
	VerifiedAt         time.Time      `json:"verified_at,omitempty"`
	IsFree             bool           `json:"is_free,omitempty"`
	KnownUpstreamFiles []UpstreamFile `json:"known_upstream_files,omitempty"`
	UpdateCheckedAt    time.Time      `json:"update_checked_at,omitempty"`
	UpdateDismissedAt  time.Time      `json:"update_dismissed_at,omitempty"`
	GameRemovedAt      time.Time      `json:"game_removed_at,omitempty"`
	RemovalDismissedAt time.Time      `json:"removal_dismissed_at,omitempty"`
	UnifiedNamingDisabled bool           `json:"unified_naming_disabled,omitempty"`
}

type Inventory struct {
	mu      sync.Mutex
	Entries map[string]*Entry `json:"entries"`
}

// Load reads the inventory from path. Returns an empty inventory if the file
// is missing or unparseable — never returns an error for those cases.
func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("inventory: no file at %s, starting empty", path)
		} else {
			logger.Warn("inventory: read error at %s: %v, starting empty", path, err)
		}
		return &Inventory{Entries: make(map[string]*Entry)}, nil
	}
	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		logger.Warn("inventory: corrupt file at %s: %v, starting empty", path, err)
		return &Inventory{Entries: make(map[string]*Entry)}, nil
	}
	if inv.Entries == nil {
		inv.Entries = make(map[string]*Entry)
	}
	logger.Debug("inventory: loaded %d entries from %s", len(inv.Entries), path)
	return &inv, nil
}

// Save writes the inventory to path atomically (write to .tmp then rename).
func (inv *Inventory) Save(path string) error {
	inv.mu.Lock()
	data, err := json.MarshalIndent(inv, "", "  ")
	count := len(inv.Entries)
	inv.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write inventory tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename inventory: %w", err)
	}
	logger.Debug("inventory: saved %d entries to %s", count, path)
	return nil
}

// Add upserts an entry and appends a file, deduplicating by DestPath.
func (inv *Inventory) Add(gameURL string, e Entry, file DownloadedFile) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	existing, ok := inv.Entries[gameURL]
	if !ok {
		entry := &Entry{
			GameURL:  gameURL,
			Title:    e.Title,
			Author:   e.Author,
			CoverURL: e.CoverURL,
			IsFree:   e.IsFree,
		}
		inv.Entries[gameURL] = entry
		existing = entry
	} else {
		existing.Title = e.Title
		existing.Author = e.Author
		existing.CoverURL = e.CoverURL
	}
	for i, f := range existing.Files {
		if f.DestPath == file.DestPath || f.Filename == file.Filename {
			existing.Files[i] = file // overwrite in place (re-download or path change)
			return
		}
	}
	existing.Files = append(existing.Files, file)
}

// Remove deletes the entry for gameURL.
func (inv *Inventory) Remove(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	delete(inv.Entries, gameURL)
}

// Lookup returns a deep copy of the entry for gameURL.
func (inv *Inventory) Lookup(gameURL string) (Entry, bool) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return Entry{}, false
	}
	snap := *e
	snap.Files = append([]DownloadedFile(nil), e.Files...)
	return snap, true
}

// ExistingDestPath returns the dest_path of an already-downloaded file matching
// the given upload filename, or "" if not found. Used to overwrite an existing
// download rather than creating a duplicate.
func (inv *Inventory) ExistingDestPath(gameURL, uploadFilename string) string {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return ""
	}
	for _, f := range e.Files {
		if f.Filename == uploadFilename {
			return f.DestPath
		}
	}
	return ""
}

// IsPresent reports whether gameURL has an inventory entry with at least one file.
// Assumes VerifyAndClean has already removed entries whose files are gone from disk.
func (inv *Inventory) IsPresent(gameURL string) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	return len(e.Files) > 0
}

// RemoveFile removes the DownloadedFile with the given destPath from the entry for gameURL.
// Returns true when no files remain (the entry is also removed from the inventory).
func (inv *Inventory) RemoveFile(gameURL, destPath string) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	entry, ok := inv.Entries[gameURL]
	if !ok {
		return true
	}
	var remaining []DownloadedFile
	for _, f := range entry.Files {
		if f.DestPath != destPath {
			remaining = append(remaining, f)
		}
	}
	entry.Files = remaining
	if len(entry.Files) == 0 {
		delete(inv.Entries, gameURL)
		return true
	}
	return false
}

// VerifyAndClean walks all entries, removes DownloadedFile rows whose DestPath no
// longer exists on disk, deduplicates rows with the same Filename (keeping the
// most recently downloaded), removes Entry values with no remaining files, saves
// if any changes were made, and returns the count of removed DownloadedFile rows.
func (inv *Inventory) VerifyAndClean(path string) int {
	removed := 0
	changed := false
	inv.mu.Lock()
	for gameURL, entry := range inv.Entries {
		// Pass 1: drop files missing from disk.
		var present []DownloadedFile
		for _, f := range entry.Files {
			if _, err := os.Stat(f.DestPath); err == nil {
				present = append(present, f)
			} else {
				logger.Debug("inventory: removing stale file=%s", f.DestPath)
				removed++
				changed = true
			}
		}

		// Pass 2: deduplicate by Filename, keeping the most recently downloaded.
		best := make(map[string]DownloadedFile, len(present))
		for _, f := range present {
			if cur, ok := best[f.Filename]; !ok || f.DownloadedAt.After(cur.DownloadedAt) {
				best[f.Filename] = f
			}
		}
		if len(best) < len(present) {
			dropped := len(present) - len(best)
			logger.Debug("inventory: deduplicating %d file(s) for game=%q", dropped, entry.Title)
			removed += dropped
			changed = true
		}
		var kept []DownloadedFile
		for _, f := range best {
			kept = append(kept, f)
		}

		if len(kept) == 0 {
			logger.Debug("inventory: removing empty entry game=%q", entry.Title)
			delete(inv.Entries, gameURL)
		} else {
			entry.Files = kept
			entry.VerifiedAt = time.Now()
		}
	}
	inv.mu.Unlock()
	logger.Info("inventory: cleaned %d stale/duplicate file(s)", removed)
	if changed {
		if err := inv.Save(path); err != nil {
			logger.Error("inventory: failed to save after clean: %v", err)
		}
	}
	return removed
}

// HasPendingUpdates returns true when any UpstreamFile for gameURL is marked
// as a new upload (appeared after the first check), has a filename not in the
// downloaded set, and was seen after UpdateDismissedAt.
func (inv *Inventory) HasPendingUpdates(gameURL string) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	downloaded := make(map[string]bool, len(e.Files)*2)
	for _, f := range e.Files {
		downloaded[f.Filename] = true
		// Format-picker appends an extension the upload name doesn't carry (e.g.
		// "Game Boy ROM.gbc" stored vs "Game Boy ROM" upstream). Also index the
		// stem so the already-downloaded file isn't treated as a new upload.
		if stem := strings.TrimSuffix(f.Filename, filepath.Ext(f.Filename)); stem != f.Filename {
			downloaded[stem] = true
		}
	}
	for _, u := range e.KnownUpstreamFiles {
		// Only flag genuinely new uploads (IsNew = true means appeared after first check).
		// Files discovered on first check were present when the user downloaded the game.
		if u.IsNew && !downloaded[u.Filename] && u.SeenAt.After(e.UpdateDismissedAt) {
			return true
		}
	}
	return false
}

// IsRemoved returns true when the game was detected as 404 upstream and the
// user has not yet dismissed the warning (or the warning reappeared after a
// subsequent removal).
func (inv *Inventory) IsRemoved(gameURL string) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	return !e.GameRemovedAt.IsZero() &&
		(e.RemovalDismissedAt.IsZero() || e.GameRemovedAt.After(e.RemovalDismissedAt))
}

// DismissUpdate sets UpdateDismissedAt to now, suppressing [UP] for all
// upstream files seen before this moment.
func (inv *Inventory) DismissUpdate(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.UpdateDismissedAt = time.Now()
}

// DismissRemoval sets RemovalDismissedAt to now, suppressing [!] until the
// game is re-detected as removed after reappearing upstream.
func (inv *Inventory) DismissRemoval(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.RemovalDismissedAt = time.Now()
}

// MarkRemoved sets GameRemovedAt to now only on the first detection
// (idempotent: does nothing if GameRemovedAt is already set).
func (inv *Inventory) MarkRemoved(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok || !e.GameRemovedAt.IsZero() {
		return
	}
	e.GameRemovedAt = time.Now()
}

// MarkReachable clears GameRemovedAt and RemovalDismissedAt, returning the
// entry to a clean slate when a previously-removed game becomes reachable again.
func (inv *Inventory) MarkReachable(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.GameRemovedAt = time.Time{}
	e.RemovalDismissedAt = time.Time{}
}

// SetUpstreamFiles replaces KnownUpstreamFiles for gameURL and sets
// UpdateCheckedAt to now. Call this after each successful file-list scrape.
//
// SeenAt is PRESERVED for files that were already known so that a dismissed
// update is not re-triggered on the next check cycle. Only genuinely new files
// (not previously in KnownUpstreamFiles) receive SeenAt = now.
func (inv *Inventory) SetUpstreamFiles(gameURL string, files []UpstreamFile) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	// isFirstCheck: no previous update run — files were already present when the
	// user downloaded the game and are not genuine new uploads.
	isFirstCheck := e.UpdateCheckedAt.IsZero()
	type priorInfo struct {
		seenAt time.Time
		isNew  bool
	}
	prior := make(map[string]priorInfo, len(e.KnownUpstreamFiles))
	for _, f := range e.KnownUpstreamFiles {
		prior[f.Filename] = priorInfo{seenAt: f.SeenAt, isNew: f.IsNew}
	}
	for i := range files {
		if p, ok := prior[files[i].Filename]; ok {
			files[i].SeenAt = p.seenAt // preserve original first-seen time
			files[i].IsNew = p.isNew   // preserve new-upload flag
		} else if !isFirstCheck {
			// Genuinely new file appearing after the first check — flag it.
			files[i].IsNew = true
			if files[i].SeenAt.IsZero() {
				files[i].SeenAt = time.Now()
			}
		}
		// if isFirstCheck: IsNew stays false (zero value); file was already present at download time
	}
	e.KnownUpstreamFiles = files
	e.UpdateCheckedAt = time.Now()
}

// LatestCheckedAt returns the most recent UpdateCheckedAt across all entries,
// or the zero time if no checks have run.
func (inv *Inventory) LatestCheckedAt() time.Time {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	var latest time.Time
	for _, e := range inv.Entries {
		if e.UpdateCheckedAt.After(latest) {
			latest = e.UpdateCheckedAt
		}
	}
	return latest
}

// CoverArtPath returns the filesystem path for the cover art of a downloaded ROM,
// mirroring the naming convention used by itchio.DownloadCoverArt.
// Cover art is always stored as .jpg using the exact ROM filename stem so it
// matches NextUI's cover art lookup (which uses the full filename including
// bracket/paren tags like [v1.2]).
// Returns "" if either argument is empty.
func CoverArtPath(coverURL, romDestPath string) string {
	if coverURL == "" || romDestPath == "" {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(romDestPath), filepath.Ext(romDestPath))
	dir := filepath.Dir(romDestPath)
	return filepath.Join(dir, ".media", base+".png")
}

// SetUnifiedNamingDisabled sets the per-game unified-naming opt-out flag.
func (inv *Inventory) SetUnifiedNamingDisabled(gameURL string, disabled bool) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.UnifiedNamingDisabled = disabled
}

// UpdateFile replaces the DownloadedFile whose DestPath matches oldDestPath.
// Returns false if the game URL or file is not found.
func (inv *Inventory) UpdateFile(gameURL, oldDestPath string, file DownloadedFile) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	for i, f := range e.Files {
		if f.DestPath == oldDestPath {
			e.Files[i] = file
			return true
		}
	}
	return false
}
