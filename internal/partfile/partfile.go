// Package partfile writes downloads to a hidden partial file beside their
// destination and renames it into place only when complete, so a dropped
// connection never damages the file already installed. Every partial file is
// recorded in a journal first, so one left behind by a crash or a power cut is
// deleted on the next launch.
package partfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Suffix marks files this app created. Nothing without it is ever deleted.
const Suffix = ".itchio-part"

var (
	mu      sync.Mutex
	journal string              // partials.json; "" disables the journal
	active  = map[string]bool{} // partial files being written by this process
)

// PathFor is the partial file for dest: hidden (dot-prefixed, so neither
// launcher lists it) and in the same folder, so the final rename never
// crosses filesystems.
func PathFor(dest string) string {
	dir, base := filepath.Split(dest)
	return filepath.Join(dir, "."+base+Suffix)
}

// SetJournal sets where partial files are recorded. Call once at startup.
func SetJournal(path string) {
	mu.Lock()
	journal = path
	mu.Unlock()
}

// saveLocked writes the active set as the journal. Caller holds mu.
func saveLocked() error {
	if journal == "" {
		return nil
	}
	paths := make([]string, 0, len(active))
	for p := range active {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	data, _ := json.Marshal(paths)
	tmp := journal + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, journal)
}

func forget(part string) {
	mu.Lock()
	delete(active, part)
	err := saveLocked()
	mu.Unlock()
	if err != nil {
		logger.Warn("partfile: journal update failed: %v", err)
	}
}

// File is a partial download. Write to it, then Commit or Abort exactly once;
// Abort after Commit is a no-op, so `defer f.Abort()` is safe.
type File struct {
	*os.File
	dest, part string
	done       bool
}

// Create journals and opens the partial file for dest.
func Create(dest string) (*File, error) {
	part := PathFor(dest)
	mu.Lock()
	active[part] = true
	jerr := saveLocked()
	mu.Unlock()
	if jerr != nil {
		// The sweep still finds it in default folders; not worth failing for.
		logger.Warn("partfile: could not journal %s: %v", part, jerr)
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		forget(part)
		return nil, err
	}
	logger.Debug("partfile: writing %s", part)
	return &File{File: f, dest: dest, part: part}, nil
}

// Commit closes the partial file and renames it over dest in one step.
func (f *File) Commit() error {
	if f.done {
		return nil
	}
	f.done = true
	if err := f.File.Close(); err != nil {
		os.Remove(f.part)
		forget(f.part)
		logger.Error("partfile: close %s: %v", f.part, err)
		return err
	}
	if err := os.Rename(f.part, f.dest); err != nil {
		os.Remove(f.part)
		forget(f.part)
		logger.Error("partfile: rename %s -> %s: %v", f.part, f.dest, err)
		return err
	}
	forget(f.part)
	logger.Debug("partfile: committed %s", f.dest)
	return nil
}

// Abort closes and deletes the partial file, leaving dest untouched.
func (f *File) Abort() {
	if f.done {
		return
	}
	f.done = true
	f.File.Close()
	if err := os.Remove(f.part); err != nil && !os.IsNotExist(err) {
		logger.Warn("partfile: remove %s: %v", f.part, err)
	}
	forget(f.part)
	logger.Info("partfile: discarded %s", f.part)
}

func removeLeftover(path, how string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if err := os.Remove(path); err != nil {
		logger.Warn("partfile: could not remove leftover %s: %v", path, err)
		return false
	}
	logger.Info("partfile: removed leftover %s (%d bytes, found by %s)", path, info.Size(), how)
	return true
}

// Recover deletes every journalled partial file that is not being written now,
// then rewrites the journal. Call once at startup, before any download.
func Recover() int {
	mu.Lock()
	defer mu.Unlock()
	if journal == "" {
		return 0
	}
	var paths []string
	if data, err := os.ReadFile(journal); err == nil {
		if err := json.Unmarshal(data, &paths); err != nil {
			logger.Warn("partfile: journal %s unreadable, relying on the sweep: %v", journal, err)
			paths = nil
		}
	}
	n := 0
	for _, p := range paths {
		if strings.HasSuffix(p, Suffix) && !active[p] && removeLeftover(p, "journal") {
			n++
		}
	}
	if err := saveLocked(); err != nil {
		logger.Warn("partfile: journal rewrite failed: %v", err)
	}
	return n
}

// Sweep deletes, non-recursively, suffixed files in dirs that this process is
// not writing. It catches leftovers whose journal was lost.
func Sweep(dirs []string) int {
	n := 0
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, Suffix) {
				continue
			}
			p := filepath.Join(dir, name)
			mu.Lock()
			busy := active[p]
			mu.Unlock()
			if !busy && removeLeftover(p, "sweep") {
				n++
			}
		}
	}
	logger.Debug("partfile: sweep of %d folder(s) removed %d file(s)", len(dirs), n)
	return n
}
