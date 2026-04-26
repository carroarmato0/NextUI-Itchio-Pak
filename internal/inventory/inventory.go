package inventory

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	Entries map[string]*Entry `json:"entries"`
}

// Load reads the inventory from path. Returns an empty inventory if the file
// is missing or unparseable — never returns an error for those cases.
func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return &Inventory{Entries: make(map[string]*Entry)}, nil
	}
	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return &Inventory{Entries: make(map[string]*Entry)}, nil
	}
	if inv.Entries == nil {
		inv.Entries = make(map[string]*Entry)
	}
	return &inv, nil
}

// Save writes the inventory to path atomically (write to .tmp then rename).
func (inv *Inventory) Save(path string) error {
	data, err := json.MarshalIndent(inv, "", "  ")
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
	return nil
}

// Add upserts an entry and appends a file, deduplicating by DestPath.
func (inv *Inventory) Add(gameURL string, e Entry, file DownloadedFile) {
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
	delete(inv.Entries, gameURL)
}

// Lookup returns the entry for gameURL.
func (inv *Inventory) Lookup(gameURL string) (*Entry, bool) {
	e, ok := inv.Entries[gameURL]
	return e, ok
}

// IsPresent reports whether gameURL has an inventory entry with at least one file.
// Assumes VerifyAndClean has already removed entries whose files are gone from disk.
func (inv *Inventory) IsPresent(gameURL string) bool {
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	return len(e.Files) > 0
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
