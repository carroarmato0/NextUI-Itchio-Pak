package roms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SanitiseFilename builds a safe filename from a game title and extension.
// Strips / : ? * " < > | from the title, trims and collapses whitespace.
// Returns "" when title is empty (caller should use the upstream filename instead).
func SanitiseFilename(title, ext string) string {
	if title == "" {
		return ""
	}
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
// Appends " (2)", " (3)" etc. to avoid colliding with existing files.
func ResolveUnifiedDest(currentPath, gameTitle string) (string, bool) {
	ext := filepath.Ext(currentPath)
	candidate := SanitiseFilename(gameTitle, ext)
	if candidate == "" || candidate == filepath.Base(currentPath) {
		return currentPath, false
	}
	dir := filepath.Dir(currentPath)
	target := filepath.Join(dir, candidate)
	if _, err := os.Stat(target); err == nil && target != currentPath {
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
	return target, target != currentPath
}
