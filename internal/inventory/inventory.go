package inventory

import (
	"encoding/json"
	"fmt"
	"net/url"
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
}

type Entry struct {
	GameURL    string           `json:"game_url"`
	Title      string           `json:"title"`
	Author     string           `json:"author"`
	CoverURL   string           `json:"cover_url"`
	Files      []DownloadedFile `json:"files"`
	VerifiedAt time.Time        `json:"verified_at,omitempty"`
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
		}
		inv.Entries[gameURL] = entry
		existing = entry
	} else {
		existing.Title = e.Title
		existing.Author = e.Author
		existing.CoverURL = e.CoverURL
	}
	for _, f := range existing.Files {
		if f.DestPath == file.DestPath {
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
// longer exists on disk, removes Entry values with no remaining files, saves if
// any changes were made, and returns the count of removed DownloadedFile rows.
func (inv *Inventory) VerifyAndClean(path string) int {
	removed := 0
	inv.mu.Lock()
	for gameURL, entry := range inv.Entries {
		var kept []DownloadedFile
		for _, f := range entry.Files {
			if _, err := os.Stat(f.DestPath); err == nil {
				kept = append(kept, f)
			} else {
				logger.Debug("inventory: removing stale file=%s", f.DestPath)
				removed++
			}
		}
		if len(kept) == 0 {
			logger.Debug("inventory: removing empty entry game=%q", entry.Title)
			delete(inv.Entries, gameURL)
		} else if len(kept) < len(entry.Files) {
			entry.Files = kept
			entry.VerifiedAt = time.Now()
		}
		// No else — VerifiedAt is only updated when files are actually removed
	}
	inv.mu.Unlock()
	logger.Info("inventory: cleaned %d stale file(s)", removed)
	if removed > 0 {
		if err := inv.Save(path); err != nil {
			logger.Error("inventory: failed to save after clean: %v", err)
		}
	}
	return removed
}

// CoverArtPath returns the filesystem path for the cover art of a downloaded ROM,
// mirroring the naming convention used by itchio.DownloadCoverArt.
// Returns "" if either argument is empty.
func CoverArtPath(coverURL, romDestPath string) string {
	if coverURL == "" || romDestPath == "" {
		return ""
	}
	ext := ".png"
	if u, err := url.Parse(coverURL); err == nil {
		if e := filepath.Ext(u.Path); e != "" {
			ext = e
		}
	}
	dir := filepath.Dir(romDestPath)
	base := strings.TrimSuffix(filepath.Base(romDestPath), filepath.Ext(romDestPath))
	return filepath.Join(dir, ".media", base+ext)
}
