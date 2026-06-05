package roms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/text"
)

// SanitiseFilename builds a safe filename from a game title and extension.
// Strips emoji, then strips / : ? * " < > | from the title, trims and collapses whitespace.
// Returns "" when title is empty or reduces to empty after stripping (caller should use the upstream filename instead).
func SanitiseFilename(title, ext string) string {
	if title == "" {
		return ""
	}
	title = text.StripEmoji(title)
	const strip = `/:?*"<>|`
	var b strings.Builder
	for _, r := range title {
		if strings.ContainsRune(strip, r) {
			continue
		}
		b.WriteRune(r)
	}
	s := strings.Join(strings.Fields(b.String()), " ")
	if s == "" {
		return ""
	}
	return s + ext
}

// ResolveUnifiedDest returns the desired on-disk path for a ROM after applying
// unified naming. currentPath is where the file was written. gameTitle is the
// itch.io game title (used to derive the target filename).
//
// Returns (currentPath, false) when no rename is needed (name already correct,
// or title is empty). Returns (targetPath, true) when a rename is required.
//
// If allowOverwrite is true (download context), the returned path may already
// exist on disk — the caller's os.Rename will atomically replace it. Exception:
// if currentPath is already a numbered slot for this game (e.g. "Title (2).gb"),
// the slot is preserved and (currentPath, false) is returned so a re-download
// does not overwrite a different game occupying the primary name.
//
// If allowOverwrite is false (migration context), appends " (2)", " (3)" etc.
// to avoid colliding with any pre-existing file.
func ResolveUnifiedDest(currentPath, gameTitle string, allowOverwrite bool) (string, bool) {
	ext := ROMExt(filepath.Base(currentPath))
	candidate := SanitiseFilename(gameTitle, ext)
	if candidate == "" || candidate == filepath.Base(currentPath) {
		return currentPath, false
	}
	dir := filepath.Dir(currentPath)
	target := filepath.Join(dir, candidate)
	if _, err := os.Stat(target); err == nil && target != currentPath {
		if allowOverwrite {
			stem := strings.TrimSuffix(candidate, ext)
			if isNumberedSlot(filepath.Base(currentPath), stem, ext) {
				return currentPath, false
			}
			// Allow overwriting: os.Rename will atomically replace the target.
		} else {
			stem := strings.TrimSuffix(candidate, ext)
			for n := 2; ; n++ {
				candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
				target = filepath.Join(dir, candidate)
				if _, err := os.Stat(target); os.IsNotExist(err) {
					break
				}
				if target == currentPath {
					return currentPath, false
				}
			}
		}
	}
	return target, target != currentPath
}

// isNumberedSlot reports whether base matches the pattern "stem (N)ext" for
// some non-empty digit sequence N. Used to detect that a file was deliberately
// placed in a collision slot and should not be moved to the primary name.
func isNumberedSlot(base, stem, ext string) bool {
	prefix := stem + " ("
	suffix := ")" + ext
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, suffix) {
		return false
	}
	mid := base[len(prefix) : len(base)-len(suffix)]
	if mid == "" {
		return false
	}
	for _, c := range mid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
