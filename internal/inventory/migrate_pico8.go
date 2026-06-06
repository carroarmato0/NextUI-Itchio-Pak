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
//
// Multi-file games whose files all share a single immediate subdirectory of
// oldDir (e.g. POOM/) are migrated by renaming the entire subdirectory at once.
// This also moves untracked files that the inventory does not know about, such
// as the .m3u launcher generated at download time, preventing a situation where
// the game appears in both the old and new core directories simultaneously.
//
// Individual failures are logged as warnings but do not abort the migration.
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

		// Collect files for this game that live under oldDir.
		var relevant []DownloadedFile
		for _, f := range entry.Files {
			if strings.HasPrefix(f.DestPath, oldDir) {
				relevant = append(relevant, f)
			}
		}
		if len(relevant) == 0 {
			continue
		}

		// If all files share a single immediate subdirectory, rename the whole
		// directory atomically so untracked files (e.g. .m3u) move with it.
		if subdir := immediateSubdir(oldDir, relevant); subdir != "" {
			migrateSubdir(inv, gameURL, oldDir, newDir, subdir, relevant)
		} else {
			migrateFlatFiles(inv, gameURL, entry, oldDir, newDir, relevant)
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

// immediateSubdir returns the name of the single immediate child directory of
// parentDir that all files live under, or "" if they are flat in parentDir or
// spread across multiple subdirectories.
func immediateSubdir(parentDir string, files []DownloadedFile) string {
	var subdir string
	for _, f := range files {
		rel := strings.TrimPrefix(f.DestPath, parentDir)
		slash := strings.Index(rel, "/")
		if slash < 0 {
			return "" // file is directly in parentDir, not in a subdir
		}
		name := rel[:slash]
		if subdir == "" {
			subdir = name
		} else if subdir != name {
			return "" // files are spread across multiple subdirectories
		}
	}
	return subdir
}

// migrateSubdir renames an entire game subdirectory atomically (so untracked
// files such as .m3u launchers move with it) and moves the directory-level
// cover art stored at parentDir/.media/SubdirName.png.
func migrateSubdir(inv *Inventory, gameURL, oldDir, newDir, subdir string, files []DownloadedFile) {
	oldSubDir := oldDir + subdir
	newSubDir := newDir + subdir

	if err := os.MkdirAll(filepath.Dir(newSubDir), 0755); err != nil {
		logger.Warn("migrate pico8: mkdir parent for subdir %s: %v", newSubDir, err)
		return
	}
	if err := os.Rename(oldSubDir, newSubDir); err != nil {
		logger.Warn("migrate pico8: rename subdir %s → %s: %v", oldSubDir, newSubDir, err)
		return
	}
	logger.Info("migrate pico8: moved subdir %s → %s", oldSubDir, newSubDir)

	// Directory-level cover art lives at parentDir/.media/SubdirName.png —
	// different from the per-file CoverArtPath which puts art inside the subdir.
	oldArt := oldDir + ".media/" + subdir + ".png"
	if _, err := os.Stat(oldArt); err == nil {
		newArt := newDir + ".media/" + subdir + ".png"
		if err := os.MkdirAll(filepath.Dir(newArt), 0755); err == nil {
			if err := os.Rename(oldArt, newArt); err != nil {
				logger.Warn("migrate pico8: dir cover art %s: %v", oldArt, err)
			}
		}
	}

	// Update inventory DestPaths for all files in the renamed directory.
	for _, f := range files {
		rel := strings.TrimPrefix(f.DestPath, oldDir)
		updated := f
		updated.DestPath = newDir + rel
		inv.UpdateFile(gameURL, f.DestPath, updated)
	}
}

// migrateFlatFiles moves individual files that live directly in parentDir
// (single-file games). Cover art is handled via CoverArtPath.
func migrateFlatFiles(inv *Inventory, gameURL string, entry Entry, oldDir, newDir string, files []DownloadedFile) {
	for _, f := range files {
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

		oldArt := CoverArtPath(entry.CoverURL, f.DestPath)
		if oldArt != "" {
			if _, err := os.Stat(oldArt); err == nil {
				newArt := CoverArtPath(entry.CoverURL, newPath)
				if err := os.MkdirAll(filepath.Dir(newArt), 0755); err == nil {
					if err := os.Rename(oldArt, newArt); err != nil {
						logger.Warn("migrate pico8: cover art %s: %v", oldArt, err)
					}
				}
			}
		}

		updated := f
		updated.DestPath = newPath
		inv.UpdateFile(gameURL, f.DestPath, updated)
	}
}
