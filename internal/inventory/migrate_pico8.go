package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// MigratePico8Files moves all Pico-8 ROM files (and their cover art) whose
// DestPath is under oldDir to newDir, updates the inventory, and saves it.
// Individual file failures are logged as warnings but do not abort the migration.
func MigratePico8Files(inv *Inventory, invPath, oldDir, newDir string) error {
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("migrate pico8: create dest dir: %w", err)
	}

	urls := inv.AllURLs()
	for _, gameURL := range urls {
		entry, ok := inv.Lookup(gameURL)
		if !ok {
			continue
		}
		for _, f := range entry.Files {
			if !strings.HasPrefix(f.DestPath, oldDir) {
				continue
			}
			rel := strings.TrimPrefix(f.DestPath, oldDir)
			newPath := filepath.Join(newDir, rel)

			if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
				logger.Warn("migrate pico8: mkdir %s: %v", filepath.Dir(newPath), err)
				continue
			}
			if err := os.Rename(f.DestPath, newPath); err != nil {
				logger.Warn("migrate pico8: rename %s → %s: %v", f.DestPath, newPath, err)
				continue
			}
			logger.Info("migrate pico8: moved %s → %s", f.DestPath, newPath)

			// Move cover art (best-effort).
			oldArt := CoverArtPath(entry.CoverURL, f.DestPath)
			if oldArt != "" {
				if _, err := os.Stat(oldArt); err == nil {
					newArt := CoverArtPath(entry.CoverURL, newPath)
					if err := os.MkdirAll(filepath.Dir(newArt), 0755); err == nil {
						if err := os.Rename(oldArt, newArt); err != nil {
							logger.Warn("migrate pico8: cover art rename %s: %v", oldArt, err)
						}
					}
				}
			}

			updated := f
			updated.DestPath = newPath
			inv.UpdateFile(gameURL, f.DestPath, updated)
		}
	}

	if err := inv.Save(invPath); err != nil {
		return fmt.Errorf("migrate pico8: save inventory: %w", err)
	}

	// Best-effort cleanup of old .media dir and old root dir.
	_ = os.Remove(filepath.Join(strings.TrimSuffix(oldDir, "/"), ".media"))
	_ = os.Remove(strings.TrimSuffix(oldDir, "/"))

	return nil
}
