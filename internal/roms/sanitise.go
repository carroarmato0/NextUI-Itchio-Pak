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

// UnifiedCollisions returns the unified destination paths that more than one of
// paths would resolve to, given a single game title.
//
// Unified naming derives a filename from the game title, so every ROM belonging
// to one game maps to the same name. That is the whole point when a game ships
// one ROM, and data loss when it ships several: downloading Capybara Village
// renamed both "Capybara-Village-Update1.gb" and
// "Game-Jam-Submission-Version.gb" to "Capybara Village.gb", and the second
// silently replaced the first — leaving the older build installed.
//
// Only genuine clashes are reported. Uploads that differ by extension, such as
// a .gb and a .gbc build, resolve to different names and are left alone.
func UnifiedCollisions(paths []string, gameTitle string) map[string]bool {
	seen := make(map[string]int, len(paths))
	for _, p := range paths {
		if t := unifiedTarget(p, gameTitle); t != "" {
			seen[t]++
		}
	}
	out := make(map[string]bool)
	for t, n := range seen {
		if n > 1 {
			out[t] = true
		}
	}
	return out
}

// unifiedTarget is the path unified naming would derive for one file, ignoring
// what already exists on disk. Empty when no rename would apply.
func unifiedTarget(path, gameTitle string) string {
	ext := ROMExt(filepath.Base(path))
	candidate := SanitiseFilename(gameTitle, ext)
	if candidate == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(path), candidate)
}

// UnifiedTargetFor exposes unifiedTarget for callers deciding whether a planned
// download participates in a collision.
func UnifiedTargetFor(path, gameTitle string) string { return unifiedTarget(path, gameTitle) }
