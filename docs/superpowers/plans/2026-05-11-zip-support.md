# Smart ZIP Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add remote ZIP inspection via HTTP Range requests, selective ROM/music extraction, and music soundtrack download to the Itch.io NextUI Pak.

**Architecture:** A `ZIPInspectScreen` resolves the CDN URL and reads the ZIP central directory remotely (Range requests, fallback to full download). Based on the manifest and settings it routes to `ZIPContentsScreen` (ask mode / version picker) or directly to `ZIPDownloadScreen` (auto mode). `ZIPDownloadScreen` downloads to a temp path, extracts ROM and music files to their respective folders, and records each file in inventory with a `FileType` tag.

**Tech Stack:** Go 1.22, `archive/zip`, `net/http` Range requests, SDL2 (UI layer only), existing `internal/roms`, `internal/inventory`, `internal/itchio` packages.

---

## File Map

| Action | Path |
|--------|------|
| Create | `internal/roms/zip_classify.go` |
| Create | `internal/roms/zip_classify_test.go` |
| Create | `internal/roms/zip_remote.go` |
| Create | `internal/roms/zip_remote_test.go` |
| Modify | `internal/roms/roms.go` |
| Modify | `internal/roms/roms_test.go` |
| Modify | `internal/settings/settings.go` |
| Modify | `internal/settings/settings_test.go` |
| Modify | `internal/inventory/inventory.go` |
| Modify | `internal/inventory/inventory_test.go` |
| Modify | `internal/itchio/download.go` |
| Modify | `internal/itchio/download_test.go` |
| Create | `internal/ui/screen_zip_inspect.go` |
| Create | `internal/ui/screen_zip_contents.go` |
| Create | `internal/ui/screen_music_location_picker.go` |
| Create | `internal/ui/screen_zip_download.go` |
| Modify | `internal/ui/screen_fetch_uploads.go` |
| Modify | `internal/ui/screen_rom_picker.go` |
| Modify | `internal/ui/screen_settings.go` |
| Modify | `internal/ui/screen_manage_downloads.go` |

---

## Task 1: ZIP content classification (`internal/roms/zip_classify.go`)

**Files:**
- Create: `internal/roms/zip_classify.go`
- Create: `internal/roms/zip_classify_test.go`

- [ ] **Step 1.1: Write failing tests**

```go
// internal/roms/zip_classify_test.go
package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestClassifyEntry(t *testing.T) {
	tests := []struct {
		name string
		want roms.FileKind
	}{
		{"game.gb", roms.KindROM},
		{"game.GB", roms.KindROM},
		{"game.gbc", roms.KindROM},
		{"game.GBC", roms.KindROM},
		{"game.gba", roms.KindROM},
		{"track01.mp3", roms.KindMusic},
		{"track01.MP3", roms.KindMusic},
		{"track01.ogg", roms.KindMusic},
		{"track01.flac", roms.KindMusic},
		{"track01.wav", roms.KindMusic},
		{"track01.opus", roms.KindMusic},
		{"track01.mod", roms.KindMusic},
		{"track01.xm", roms.KindMusic},
		{"track01.s3m", roms.KindMusic},
		{"track01.it", roms.KindMusic},
		{"readme.txt", roms.KindOther},
		{"cover.png", roms.KindOther},
		{"manual.pdf", roms.KindOther},
		{"noext", roms.KindOther},
	}
	for _, tt := range tests {
		got := roms.ClassifyEntry(tt.name)
		if got != tt.want {
			t.Errorf("ClassifyEntry(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestZIPManifestHelpers(t *testing.T) {
	m := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game-v1.gbc", Kind: roms.KindROM},
		{Name: "game-v2.gbc", Kind: roms.KindROM},
		{Name: "game.gb", Kind: roms.KindROM},
		{Name: "track.mp3", Kind: roms.KindMusic},
	}}

	if !m.HasROMs() {
		t.Error("HasROMs() = false, want true")
	}
	if !m.HasMusic() {
		t.Error("HasMusic() = false, want true")
	}
	if m.ROMCount() != 3 {
		t.Errorf("ROMCount() = %d, want 3", m.ROMCount())
	}
	if m.MusicCount() != 1 {
		t.Errorf("MusicCount() = %d, want 1", m.MusicCount())
	}
	if m.IsSingleROMOnly() {
		t.Error("IsSingleROMOnly() = true, want false (has music + multiple ROMs)")
	}
	if !m.HasDuplicateROMExt() {
		t.Error("HasDuplicateROMExt() = false, want true (two .gbc entries)")
	}

	byExt := m.ROMsByExt()
	if len(byExt[".gbc"]) != 2 {
		t.Errorf("ROMsByExt()[.gbc] len = %d, want 2", len(byExt[".gbc"]))
	}
	if len(byExt[".gb"]) != 1 {
		t.Errorf("ROMsByExt()[.gb] len = %d, want 1", len(byExt[".gb"]))
	}
}

func TestIsSingleROMOnly(t *testing.T) {
	m := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game.gbc", Kind: roms.KindROM},
		{Name: "readme.txt", Kind: roms.KindOther},
	}}
	if !m.IsSingleROMOnly() {
		t.Error("IsSingleROMOnly() = false, want true (1 ROM, no music)")
	}
}

func TestEmptyManifest(t *testing.T) {
	m := roms.ZIPManifest{}
	if m.HasROMs() || m.HasMusic() || m.ROMCount() != 0 || m.MusicCount() != 0 {
		t.Error("empty manifest should have no ROMs or music")
	}
	if m.IsSingleROMOnly() {
		t.Error("empty manifest IsSingleROMOnly() should be false")
	}
}
```

- [ ] **Step 1.2: Run tests to confirm they fail**

```bash
./scripts/test.sh 2>&1 | grep -E "zip_classify|FAIL|ok"
```
Expected: compilation error (types not defined yet).

- [ ] **Step 1.3: Implement `zip_classify.go`**

```go
// internal/roms/zip_classify.go
package roms

import (
	"path/filepath"
	"strings"
)

// FileKind categorises a file inside a ZIP archive.
type FileKind int

const (
	KindOther FileKind = iota
	KindROM            // .gb .gbc .gba
	KindMusic          // .mp3 .ogg .flac .wav .opus .mod .xm .s3m .it
)

// ZIPEntry is one file from a ZIP's central directory.
type ZIPEntry struct {
	Name string
	Kind FileKind
	Size uint64 // uncompressed bytes
}

// ZIPManifest is the classified contents of a ZIP file.
type ZIPManifest struct {
	Entries []ZIPEntry
}

var romExts = map[string]bool{
	".gb": true, ".gbc": true, ".gba": true,
}

var musicExts = map[string]bool{
	".mp3": true, ".ogg": true, ".flac": true, ".wav": true, ".opus": true,
	".mod": true, ".xm": true, ".s3m": true, ".it": true,
}

// ClassifyEntry returns the FileKind for a filename based on its extension.
func ClassifyEntry(name string) FileKind {
	ext := strings.ToLower(filepath.Ext(name))
	if romExts[ext] {
		return KindROM
	}
	if musicExts[ext] {
		return KindMusic
	}
	return KindOther
}

func (m ZIPManifest) HasROMs() bool {
	for _, e := range m.Entries {
		if e.Kind == KindROM {
			return true
		}
	}
	return false
}

func (m ZIPManifest) HasMusic() bool {
	for _, e := range m.Entries {
		if e.Kind == KindMusic {
			return true
		}
	}
	return false
}

func (m ZIPManifest) ROMCount() int {
	n := 0
	for _, e := range m.Entries {
		if e.Kind == KindROM {
			n++
		}
	}
	return n
}

func (m ZIPManifest) MusicCount() int {
	n := 0
	for _, e := range m.Entries {
		if e.Kind == KindMusic {
			n++
		}
	}
	return n
}

// IsSingleROMOnly reports whether the manifest contains exactly one ROM and no music.
func (m ZIPManifest) IsSingleROMOnly() bool {
	return m.ROMCount() == 1 && !m.HasMusic()
}

// ROMsByExt groups ROM entries by lowercase extension.
func (m ZIPManifest) ROMsByExt() map[string][]ZIPEntry {
	groups := make(map[string][]ZIPEntry)
	for _, e := range m.Entries {
		if e.Kind == KindROM {
			ext := strings.ToLower(filepath.Ext(e.Name))
			groups[ext] = append(groups[ext], e)
		}
	}
	return groups
}

// HasDuplicateROMExt reports whether any ROM extension appears more than once.
func (m ZIPManifest) HasDuplicateROMExt() bool {
	for _, entries := range m.ROMsByExt() {
		if len(entries) > 1 {
			return true
		}
	}
	return false
}
```

- [ ] **Step 1.4: Run tests — all must pass**

```bash
./scripts/test.sh 2>&1 | grep -E "zip_classify|FAIL|ok"
```
Expected: `ok  github.com/carroarmato0/nextui-itchio-pak/internal/roms`

- [ ] **Step 1.5: Commit**

```bash
git add internal/roms/zip_classify.go internal/roms/zip_classify_test.go
git commit -m "feat(roms): add ZIP content classification types and helpers"
```

---

## Task 2: Remote ZIP inspection (`internal/roms/zip_remote.go`)

**Files:**
- Create: `internal/roms/zip_remote.go`
- Create: `internal/roms/zip_remote_test.go`
- Modify: `internal/itchio/client.go` (add `DownloadURL` method to `*Client`)

- [ ] **Step 2.1: Add `DownloadURL` to `internal/itchio/client.go`**

Append after the existing `HTTPClient()` method:

```go
// DownloadURL streams directly from a pre-resolved CDN URL to dest.
// Use when the CDN URL was already resolved by ResolveFreeURL or ResolveAuthURL.
func (c *Client) DownloadURL(cdnURL, dest string, progress func(int64, int64)) error {
	return c.streamToFile(cdnURL, dest, progress)
}
```

- [ ] **Step 2.2: Write failing tests for `zip_remote.go`**

```go
// internal/roms/zip_remote_test.go
package roms_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

// buildTestZIP creates an in-memory ZIP with the given files (name → content).
func buildTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	w.Close()
	return buf.Bytes()
}

func TestInspectRemoteZIP_RangeSupported(t *testing.T) {
	data := buildTestZIP(t, map[string]string{
		"game.gbc":    "romdata",
		"track01.mp3": "musicdata",
		"readme.txt":  "textdata",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
			return
		}
		rangeHdr := r.Header.Get("Range")
		if rangeHdr == "" {
			http.ServeContent(w, r, "test.zip", testModTime(), bytes.NewReader(data))
			return
		}
		http.ServeContent(w, r, "test.zip", testModTime(), bytes.NewReader(data))
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(srv.Client(), srv.URL+"/test.zip")
	if err != nil {
		t.Fatalf("InspectRemoteZIP error: %v", err)
	}
	if manifest.ROMCount() != 1 {
		t.Errorf("ROMCount = %d, want 1", manifest.ROMCount())
	}
	if manifest.MusicCount() != 1 {
		t.Errorf("MusicCount = %d, want 1", manifest.MusicCount())
	}
	otherCount := 0
	for _, e := range manifest.Entries {
		if e.Kind == roms.KindOther {
			otherCount++
		}
	}
	if otherCount != 1 {
		t.Errorf("KindOther count = %d, want 1", otherCount)
	}
}

func TestInspectRemoteZIP_FallbackOn200(t *testing.T) {
	data := buildTestZIP(t, map[string]string{
		"game.gb": "romdata",
	})

	// Server returns 200 for everything (no range support)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		// No Accept-Ranges header
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(srv.Client(), srv.URL+"/test.zip")
	if err != nil {
		t.Fatalf("InspectRemoteZIP fallback error: %v", err)
	}
	if manifest.ROMCount() != 1 {
		t.Errorf("ROMCount = %d, want 1", manifest.ROMCount())
	}
}

func TestInspectRemoteZIP_MusicOnly(t *testing.T) {
	data := buildTestZIP(t, map[string]string{
		"track01.mp3": "music1",
		"track02.ogg": "music2",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(srv.Client(), srv.URL+"/test.zip")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if manifest.HasROMs() {
		t.Error("HasROMs() = true, want false")
	}
	if manifest.MusicCount() != 2 {
		t.Errorf("MusicCount = %d, want 2", manifest.MusicCount())
	}
}

// testModTime is a helper for http.ServeContent — any fixed time works.
func testModTime() interface{ IsZero() bool } {
	return struct{ isZero bool }{}
}
```

Note: `http.ServeContent` handles Range requests automatically. Replace `testModTime()` with `time.Time{}`.

- [ ] **Step 2.3: Fix the test helper — use `time.Time{}`**

Replace `testModTime()` calls in the test with `time.Time{}` directly:

```go
http.ServeContent(w, r, "test.zip", time.Time{}, bytes.NewReader(data))
```

Add `"time"` to imports.

- [ ] **Step 2.4: Run tests to confirm failure**

```bash
./scripts/test.sh 2>&1 | grep -E "zip_remote|FAIL|ok"
```
Expected: compile error (`InspectRemoteZIP` not defined).

- [ ] **Step 2.5: Implement `zip_remote.go`**

```go
// internal/roms/zip_remote.go
package roms

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// rangeReaderAt implements io.ReaderAt using HTTP Range requests with a small
// chunk cache to coalesce the reads zip.NewReader makes internally.
type rangeReaderAt struct {
	url    string
	client *http.Client
	size   int64
	mu     sync.Mutex
	chunks []rangeChunk
}

type rangeChunk struct {
	start int64
	data  []byte
}

func (r *rangeReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	end := off + int64(len(p)) - 1
	if end >= r.size {
		end = r.size - 1
	}
	need := end - off + 1
	if need <= 0 {
		return 0, io.EOF
	}

	for _, chunk := range r.chunks {
		chunkEnd := chunk.start + int64(len(chunk.data)) - 1
		if off >= chunk.start && end <= chunkEnd {
			src := chunk.data[off-chunk.start : off-chunk.start+need]
			n := copy(p, src)
			if int64(n) < need {
				return n, io.EOF
			}
			return n, nil
		}
	}

	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, end))
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("range: server returned %d (expected 206)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	r.chunks = append(r.chunks, rangeChunk{start: off, data: data})
	n := copy(p, data)
	if int64(n) < need {
		return n, io.EOF
	}
	return n, nil
}

// InspectRemoteZIP reads the ZIP central directory via HTTP Range requests.
// Falls back to a full download when the server does not support Range.
func InspectRemoteZIP(client *http.Client, cdnURL string) (ZIPManifest, error) {
	headResp, err := client.Head(cdnURL)
	if err != nil {
		return inspectViaFullDownload(client, cdnURL)
	}
	headResp.Body.Close()

	size := headResp.ContentLength
	supportsRange := strings.EqualFold(headResp.Header.Get("Accept-Ranges"), "bytes")

	if size > 0 && supportsRange {
		m, err := inspectViaRange(client, cdnURL, size)
		if err == nil {
			return m, nil
		}
	}
	return inspectViaFullDownload(client, cdnURL)
}

func inspectViaRange(client *http.Client, cdnURL string, size int64) (ZIPManifest, error) {
	rra := &rangeReaderAt{url: cdnURL, client: client, size: size}
	r, err := zip.NewReader(rra, size)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("zip.NewReader: %w", err)
	}
	return manifestFromZipReader(r), nil
}

func inspectViaFullDownload(client *http.Client, cdnURL string) (ZIPManifest, error) {
	tmp, err := os.CreateTemp("", "itchio-inspect-*.zip")
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	resp, err := client.Get(cdnURL)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("full download: %w", err)
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("open temp: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return ZIPManifest{}, fmt.Errorf("write temp: %w", err)
	}
	f.Close()

	r, err := zip.OpenReader(tmpPath)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("zip.OpenReader: %w", err)
	}
	defer r.Close()
	return manifestFromZipReader(&r.Reader), nil
}

func manifestFromZipReader(r *zip.Reader) ZIPManifest {
	var m ZIPManifest
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		m.Entries = append(m.Entries, ZIPEntry{
			Name: name,
			Kind: ClassifyEntry(name),
			Size: f.UncompressedSize64,
		})
	}
	return m
}
```

- [ ] **Step 2.6: Run tests — all must pass**

```bash
./scripts/test.sh 2>&1 | grep -E "zip_remote|FAIL|ok"
```

- [ ] **Step 2.7: Commit**

```bash
git add internal/roms/zip_remote.go internal/roms/zip_remote_test.go internal/itchio/client.go
git commit -m "feat(roms): add remote ZIP inspection via HTTP Range with full-download fallback"
```

---

## Task 3: `MusicDestinationDir` in `roms.go`

**Files:**
- Modify: `internal/roms/roms.go`
- Modify: `internal/roms/roms_test.go`

- [ ] **Step 3.1: Add failing test**

Append to `internal/roms/roms_test.go`:

```go
func TestMusicDestinationDir(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Solastra", "/mnt/SDCARD/Music/Solastra/"},
		{"Game: Title?", "/mnt/SDCARD/Music/Game Title/"},
		{"", "/mnt/SDCARD/Music/Unknown/"},
	}
	for _, tt := range tests {
		got := roms.MusicDestinationDir(tt.title)
		if got != tt.want {
			t.Errorf("MusicDestinationDir(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}
```

- [ ] **Step 3.2: Run test to confirm failure**

```bash
./scripts/test.sh 2>&1 | grep -E "MusicDestinationDir|FAIL"
```

- [ ] **Step 3.3: Implement in `roms.go`**

Append to `internal/roms/roms.go`:

```go
// MusicBaseDir is the root directory for all extracted game soundtracks.
const MusicBaseDir = "/mnt/SDCARD/Music/"

// MusicDestinationDir returns the target directory for a game's music files.
// Uses SanitiseFilename to produce a safe subdirectory name from the game title.
func MusicDestinationDir(gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return MusicBaseDir + safe + "/"
}
```

- [ ] **Step 3.4: Run tests — pass**

```bash
./scripts/test.sh 2>&1 | grep -E "roms|FAIL|ok"
```

- [ ] **Step 3.5: Commit**

```bash
git add internal/roms/roms.go internal/roms/roms_test.go
git commit -m "feat(roms): add MusicDestinationDir for soundtrack extraction"
```

---

## Task 4: Settings — `MusicDownload` and `MusicLocation`

**Files:**
- Modify: `internal/settings/settings.go`
- Create (or modify): `internal/settings/settings_test.go`

- [ ] **Step 4.1: Write failing tests**

Check if `settings_test.go` exists first. If not, create it; if it does, append:

```go
// internal/settings/settings_test.go
package settings_test

import (
	"encoding/json"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

func TestMusicDefaults(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MusicDownload != "off" {
		t.Errorf("MusicDownload default = %q, want \"off\"", cfg.MusicDownload)
	}
	if cfg.MusicLocation != "auto" {
		t.Errorf("MusicLocation default = %q, want \"auto\"", cfg.MusicLocation)
	}
}

func TestMusicBackwardCompat(t *testing.T) {
	// Old config JSON without music fields
	raw := `{"api_key":"","rom_selection":"auto","rom_location":"auto","unified_naming":true}`
	var cfg settings.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// omitempty fields unmarshal to zero value (""), apply defaults manually in Load()
	// so after Load() the values are "off" and "auto". Direct unmarshal gives "".
	// We only test that Load() applies defaults correctly.
	cfg2, _ := settings.Load("/nonexistent/path.json")
	if cfg2.MusicDownload != "off" {
		t.Errorf("backward-compat default MusicDownload = %q, want \"off\"", cfg2.MusicDownload)
	}
}
```

- [ ] **Step 4.2: Run to confirm failure**

```bash
./scripts/test.sh 2>&1 | grep -E "settings|FAIL"
```

- [ ] **Step 4.3: Update `settings.go`**

In `internal/settings/settings.go`, add two fields to `Config`:

```go
type Config struct {
	APIKey        string            `json:"api_key"`
	ROMSelection  string            `json:"rom_selection"`
	ROMLocation   string            `json:"rom_location"`
	LastROMDirs   map[string]string `json:"last_rom_dirs,omitempty"`
	Filter        ContentFilter     `json:"content_filter"`
	LogLevel      string            `json:"log_level,omitempty"`
	SortMode      string            `json:"sort_mode,omitempty"`
	NextUITheme   bool              `json:"nextui_theme"`
	UnifiedNaming bool              `json:"unified_naming"`
	MusicDownload string            `json:"music_download,omitempty"` // "auto"|"ask"|"off"
	MusicLocation string            `json:"music_location,omitempty"` // "auto"|"ask"
}
```

Update `defaults()`:

```go
func defaults() *Config {
	return &Config{
		APIKey:        "",
		ROMSelection:  "auto",
		ROMLocation:   "auto",
		UnifiedNaming: true,
		MusicDownload: "off",
		MusicLocation: "auto",
		Filter: ContentFilter{
			AdultContent: CategoryFilter{Enabled: true},
			HeavyThemes:  CategoryFilter{Enabled: true},
			SubstanceUse: CategoryFilter{Enabled: true},
		},
	}
}
```

- [ ] **Step 4.4: Run tests — pass**

```bash
./scripts/test.sh 2>&1 | grep -E "settings|FAIL|ok"
```

- [ ] **Step 4.5: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat(settings): add MusicDownload and MusicLocation settings (default off/auto)"
```

---

## Task 5: Inventory `FileType` field

**Files:**
- Modify: `internal/inventory/inventory.go`
- Modify: `internal/inventory/inventory_test.go`

- [ ] **Step 5.1: Write failing tests**

Append to `internal/inventory/inventory_test.go`:

```go
func TestDownloadedFileFileType_RoundTrip(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	// Add a music file
	inv.Add("http://example.com/game", inventory.Entry{
		GameURL: "http://example.com/game", Title: "Game",
	}, inventory.DownloadedFile{
		Filename:  "ost.zip",
		DestPath:  "/mnt/SDCARD/Music/Game/track.mp3",
		FileType:  inventory.FileTypeMusic,
	})
	// Add a ROM file
	inv.Add("http://example.com/game", inventory.Entry{
		GameURL: "http://example.com/game", Title: "Game",
	}, inventory.DownloadedFile{
		Filename: "ost.zip",
		DestPath: "/mnt/SDCARD/Roms/Game Boy Color (GBC)/Game.gbc",
		FileType: inventory.FileTypeROM,
	})

	dir := t.TempDir()
	path := dir + "/inv.json"
	if err := inv.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := inventory.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entry, ok := loaded.Lookup("http://example.com/game")
	if !ok {
		t.Fatal("entry not found after load")
	}
	types := map[string]string{}
	for _, f := range entry.Files {
		types[f.DestPath] = f.FileType
	}
	if types["/mnt/SDCARD/Music/Game/track.mp3"] != inventory.FileTypeMusic {
		t.Errorf("music file type = %q, want %q", types["/mnt/SDCARD/Music/Game/track.mp3"], inventory.FileTypeMusic)
	}
	if types["/mnt/SDCARD/Roms/Game Boy Color (GBC)/Game.gbc"] != inventory.FileTypeROM {
		t.Errorf("ROM file type = %q, want %q", types["/mnt/SDCARD/Roms/Game Boy Color (GBC)/Game.gbc"], inventory.FileTypeROM)
	}
}

func TestDownloadedFileFileType_BackwardCompat(t *testing.T) {
	// Old JSON without file_type field
	raw := `{"entries":{"http://example.com/game":{"game_url":"http://example.com/game","title":"Game","author":"","cover_url":"","files":[{"filename":"game.gbc","dest_path":"/mnt/SDCARD/Roms/Game Boy Color (GBC)/Game.gbc","downloaded_at":"2024-01-01T00:00:00Z"}]}}}`
	dir := t.TempDir()
	path := dir + "/inv.json"
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	inv, err := inventory.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entry, ok := inv.Lookup("http://example.com/game")
	if !ok {
		t.Fatal("entry not found")
	}
	if len(entry.Files) != 1 {
		t.Fatalf("files len = %d, want 1", len(entry.Files))
	}
	// Empty FileType is valid ("" == rom for display purposes)
	if entry.Files[0].FileType != "" {
		t.Errorf("old entry FileType = %q, want \"\" (backward compat)", entry.Files[0].FileType)
	}
}

func TestVerifyAndClean_MixedFileTypes(t *testing.T) {
	dir := t.TempDir()

	romPath := dir + "/game.gbc"
	musicPath := dir + "/track.mp3"
	os.WriteFile(romPath, []byte("rom"), 0644)
	os.WriteFile(musicPath, []byte("music"), 0644)

	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("http://g", inventory.Entry{GameURL: "http://g", Title: "G"}, inventory.DownloadedFile{
		Filename: "z.zip", DestPath: romPath, FileType: inventory.FileTypeROM,
	})
	inv.Add("http://g", inventory.Entry{GameURL: "http://g", Title: "G"}, inventory.DownloadedFile{
		Filename: "z.zip", DestPath: musicPath, FileType: inventory.FileTypeMusic,
	})

	invPath := dir + "/inv.json"
	inv.Save(invPath)

	// Delete the music file
	os.Remove(musicPath)
	inv.VerifyAndClean(invPath)

	entry, ok := inv.Lookup("http://g")
	if !ok {
		t.Fatal("game entry should still exist (ROM is present)")
	}
	for _, f := range entry.Files {
		if f.FileType == inventory.FileTypeMusic {
			t.Error("music entry should have been pruned")
		}
	}
	if len(entry.Files) != 1 || entry.Files[0].FileType != inventory.FileTypeROM {
		t.Errorf("only ROM entry should remain, got %+v", entry.Files)
	}
}
```

Add `"os"` to the test file imports if not present.

- [ ] **Step 5.2: Run to confirm failure**

```bash
./scripts/test.sh 2>&1 | grep -E "inventory|FAIL"
```

- [ ] **Step 5.3: Update `inventory.go`**

Add constants and field. In `internal/inventory/inventory.go`:

After the package declaration and imports, add:

```go
const (
	FileTypeROM   = "rom"
	FileTypeMusic = "music"
)
```

Update `DownloadedFile`:

```go
type DownloadedFile struct {
	Filename     string    `json:"filename"`
	DestPath     string    `json:"dest_path"`
	DownloadedAt time.Time `json:"downloaded_at"`
	UnifiedName  bool      `json:"unified_name,omitempty"`
	FileType     string    `json:"file_type,omitempty"` // "rom" | "music"; empty == "rom"
}
```

No other changes — `VerifyAndClean` already checks `os.Stat(f.DestPath)` for every file regardless of type.

- [ ] **Step 5.4: Run tests — pass**

```bash
./scripts/test.sh 2>&1 | grep -E "inventory|FAIL|ok"
```

- [ ] **Step 5.5: Commit**

```bash
git add internal/inventory/inventory.go internal/inventory/inventory_test.go
git commit -m "feat(inventory): add FileType field to DownloadedFile for ROM/music distinction"
```

---

## Task 6: Split CDN URL resolution from download functions

**Files:**
- Modify: `internal/itchio/download.go`
- Modify: `internal/itchio/download_test.go`

- [ ] **Step 6.1: Read the current `DownloadFree` and `DownloadAuthUpload` in `download.go` lines 186–296**

The body of `DownloadFree` (lines 191–240) does: parse URL, POST resolver, parse JSON CDN URL, then call `streamToFile`.
`DownloadAuthUpload` (lines 261–296) does: build API URL, GET, parse CDN URL, call `streamToFile`.

- [ ] **Step 6.2: Write failing tests for the new resolve methods**

Append to `internal/itchio/download_test.go`:

```go
func TestResolveFreeURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/download_url"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"url":"%s/download/TESTKEY"}`, "http://"+r.Host)
		case strings.Contains(r.URL.Path, "/download/"):
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<meta name="csrf_token" content="CSRF123">`)
		case strings.Contains(r.URL.Path, "/file/"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"url":"https://cdn.example.com/file.zip"}`)
		}
	}))
	defer srv.Close()

	client := itchio.NewClientWithBase(srv.URL)
	upload := itchio.Upload{
		Filename: "game.zip",
		UploadID: "999",
		URL:      srv.URL + "/author/game/file/999?key=TESTKEY&csrf=CSRF123",
	}
	cdnURL, err := client.ResolveFreeURL(upload)
	if err != nil {
		t.Fatalf("ResolveFreeURL: %v", err)
	}
	if cdnURL != "https://cdn.example.com/file.zip" {
		t.Errorf("cdnURL = %q, want %q", cdnURL, "https://cdn.example.com/file.zip")
	}
}

func TestResolveAuthURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"url":"https://cdn.example.com/auth-file.zip"}`)
	}))
	defer srv.Close()

	client := itchio.NewClientWithBase(srv.URL)
	cdnURL, err := client.ResolveAuthURL("apikey", "555", "777")
	if err != nil {
		t.Fatalf("ResolveAuthURL: %v", err)
	}
	if cdnURL != "https://cdn.example.com/auth-file.zip" {
		t.Errorf("cdnURL = %q, want %q", cdnURL, "https://cdn.example.com/auth-file.zip")
	}
}
```

- [ ] **Step 6.3: Run to confirm failure**

```bash
./scripts/test.sh 2>&1 | grep -E "download_test|FAIL"
```

- [ ] **Step 6.4: Refactor `download.go`**

Replace the `DownloadFree` function body with a call to the new `ResolveFreeURL`:

```go
// ResolveFreeURL resolves the CDN download URL for a free game upload.
// It performs the CSRF/key POST but does NOT stream the file.
// The returned URL is a pre-signed CDN URL — do not log it.
func (c *Client) ResolveFreeURL(upload Upload) (string, error) {
	parsed, err := url.Parse(upload.URL)
	if err != nil {
		return "", fmt.Errorf("parse resolver URL: %w", err)
	}
	key := parsed.Query().Get("key")
	csrf := parsed.Query().Get("csrf")

	keyID := extractKeyID(key)
	baseURL := parsed.Scheme + "://" + parsed.Host + parsed.Path
	logger.Debug("uploads: POST resolver csrf=%s key=%s", presentAbsent(csrf), presentAbsent(key))

	form := url.Values{"csrf_token": {csrf}, "download_key_id": {keyID}}
	resp, err := c.http.Post(baseURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("resolve CDN URL: %w", err)
	}
	defer resp.Body.Close()

	rawBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", fmt.Errorf("read resolver response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error("uploads: resolver HTTP %d: %.200s", resp.StatusCode, rawBody)
		return "", fmt.Errorf("resolve CDN URL: HTTP %d: %.200s", resp.StatusCode, rawBody)
	}

	var result struct {
		URL    string   `json:"url"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		logger.Error("uploads: parse resolver response: %v (body: %.200s)", err, rawBody)
		return "", fmt.Errorf("parse CDN URL response: %w (body: %.200s)", err, rawBody)
	}
	if len(result.Errors) > 0 {
		logger.Error("uploads: resolver error: %s", strings.Join(result.Errors, "; "))
		return "", fmt.Errorf("resolver error: %s", strings.Join(result.Errors, "; "))
	}
	if result.URL == "" {
		logger.Error("uploads: empty CDN URL from resolver (file may require purchase)")
		return "", fmt.Errorf("empty CDN URL from resolver (file may require purchase)")
	}
	return result.URL, nil
}

// DownloadFree resolves the CDN URL for a free game upload and streams it.
func (c *Client) DownloadFree(upload Upload, dest string, progress func(int64, int64)) error {
	cdnURL, err := c.ResolveFreeURL(upload)
	if err != nil {
		return err
	}
	return c.streamToFile(cdnURL, dest, progress)
}
```

Replace `DownloadAuthUpload`:

```go
// ResolveAuthURL resolves the CDN download URL for an owned upload.
// It calls the itch.io API but does NOT stream the file.
// The returned URL is a pre-signed CDN URL — do not log it.
func (c *Client) ResolveAuthURL(apiKey, uploadID, downloadKeyID string) (string, error) {
	dlURL := fmt.Sprintf("%s/api/1/%s/upload/%s/download?download_key_id=%s",
		c.base, apiKey, uploadID, downloadKeyID)
	logger.Debug("auth: resolving CDN for upload id=%s", uploadID)

	resp, err := c.http.Get(dlURL)
	if err != nil {
		return "", fmt.Errorf("resolve auth CDN URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("auth: CDN resolve HTTP %d", resp.StatusCode)
		return "", fmt.Errorf("auth CDN resolve status %d", resp.StatusCode)
	}

	var result struct {
		URL    string   `json:"url"`
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode auth CDN response: %w", err)
	}
	if len(result.Errors) > 0 {
		logger.Error("auth: CDN error: %s", strings.Join(result.Errors, "; "))
		return "", fmt.Errorf("auth CDN error: %s", strings.Join(result.Errors, "; "))
	}
	if result.URL == "" {
		logger.Error("auth: empty CDN URL from resolver")
		return "", fmt.Errorf("empty CDN URL from auth resolver")
	}
	return result.URL, nil
}

// DownloadAuthUpload resolves the CDN URL for an owned upload and streams it to dest.
func (c *Client) DownloadAuthUpload(apiKey, uploadID, downloadKeyID, dest string, progress func(int64, int64)) error {
	cdnURL, err := c.ResolveAuthURL(apiKey, uploadID, downloadKeyID)
	if err != nil {
		return err
	}
	logger.Info("auth: streaming to %s", dest)
	return c.streamToFile(cdnURL, dest, progress)
}
```

Remove the old implementations of `DownloadFree` and `DownloadAuthUpload` (they are fully replaced above).

- [ ] **Step 6.5: Run the full test suite — all existing tests must still pass**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|ok"
```
Expected: all packages pass.

- [ ] **Step 6.6: Commit**

```bash
git add internal/itchio/download.go internal/itchio/download_test.go
git commit -m "refactor(itchio): split ResolveFreeURL/ResolveAuthURL from download functions"
```

---

## Task 7: `ZIPPlan` type and `ZIPInspectScreen`

**Files:**
- Create: `internal/ui/screen_zip_inspect.go`

- [ ] **Step 7.1: Create `screen_zip_inspect.go`**

```go
//go:build !headless

package ui

import (
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// ZIPPlan carries the result of ZIP inspection and user choices through the screen flow.
type ZIPPlan struct {
	Upload   roms.Upload
	CDNURL   string
	Manifest roms.ZIPManifest

	// Set by ZIPContentsScreen or route() for the auto path.
	DownloadROMs  bool
	DownloadMusic bool
	// SelectedROMs maps lowercase extension → chosen entry Name.
	// Empty map means all ROMs in the manifest are selected.
	SelectedROMs map[string]string
	MusicDir     string // resolved music destination directory (empty = not downloading music)
}

type zipInspectState int32

const (
	zipInspectLoading zipInspectState = iota
	zipInspectDone
	zipInspectError
)

// ZIPInspectScreen is a transitional screen that resolves the CDN URL and
// reads the ZIP central directory before routing to the appropriate next screen.
type ZIPInspectScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	upload  roms.Upload
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	state zipInspectState
	plan  ZIPPlan
	err   error
}

func (s *ZIPInspectScreen) loadState() zipInspectState {
	return zipInspectState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *ZIPInspectScreen) storeState(st zipInspectState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}

func NewZIPInspectScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache,
	game itchio.Game, detail *itchio.GameDetail, upload roms.Upload,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *ZIPInspectScreen {
	s := &ZIPInspectScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, upload: upload,
		inv: inv, invPath: invPath, prev: prev,
	}
	go s.runInspect()
	return s
}

func (s *ZIPInspectScreen) runInspect() {
	defer func() { sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT}) }()

	var cdnURL string
	var err error
	if s.upload.DownloadKeyID != "" {
		cdnURL, err = s.client.ResolveAuthURL(s.cfg.APIKey, s.upload.UploadID, s.upload.DownloadKeyID)
	} else {
		itchUpload := itchio.Upload{Filename: s.upload.Filename, URL: s.upload.URL}
		cdnURL, err = s.client.ResolveFreeURL(itchUpload)
	}
	if err != nil {
		logger.Error("zip-inspect: resolve CDN URL: %v", err)
		s.err = err
		s.storeState(zipInspectError)
		return
	}

	manifest, err := roms.InspectRemoteZIP(s.client.HTTPClient(), cdnURL)
	if err != nil {
		logger.Error("zip-inspect: inspect ZIP: %v", err)
		s.err = err
		s.storeState(zipInspectError)
		return
	}
	logger.Info("zip-inspect: manifest ROMs=%d music=%d", manifest.ROMCount(), manifest.MusicCount())

	s.plan = ZIPPlan{Upload: s.upload, CDNURL: cdnURL, Manifest: manifest}
	s.storeState(zipInspectDone)
}

func (s *ZIPInspectScreen) NeedsRedraw() bool      { return true }
func (s *ZIPInspectScreen) HasPendingAnimation() bool { return false }

func (s *ZIPInspectScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(40)
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	mt := r.Theme.MainText
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	ht := r.Theme.HintText
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 6
	contentH := r.H - headerH - footerH
	mid := contentTop + contentH/2

	switch s.loadState() {
	case zipInspectLoading:
		r.DrawTextCentered("Inspecting ZIP…", 0, mid-mainFH/2, r.W, mt[0], mt[1], mt[2])
	case zipInspectError:
		r.DrawText("Inspection failed:", 20, mid-mainFH-smallFH-8, 200, 60, 60)
		r.DrawWrappedText(s.err.Error(), 20, mid-smallFH, r.W-40, smallFH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.loadState() {
	case zipInspectLoading:
		r.DrawSmallText("Please wait…", 10, ftrY, ht[0], ht[1], ht[2])
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *ZIPInspectScreen) HandleEvent(e sdl.Event) Screen {
	if s.loadState() == zipInspectDone {
		return s.route()
	}
	switch ev := e.(type) {
	case *sdl.UserEvent:
		_ = ev
		if s.loadState() == zipInspectDone {
			return s.route()
		}
	case *sdl.KeyboardEvent:
		if ev.Type == sdl.KEYDOWN && s.loadState() == zipInspectError {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE, sdl.K_RETURN:
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type == sdl.CONTROLLERBUTTONDOWN && s.loadState() == zipInspectError {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
				return s.prev
			}
		}
	}
	return s
}

func (s *ZIPInspectScreen) route() Screen {
	m := s.plan.Manifest

	if !m.HasROMs() && !m.HasMusic() {
		logger.Warn("zip-inspect: manifest empty, returning to prev")
		return s.prev
	}

	// Single ROM, no music → keep ZIP, use DownloadScreen unchanged.
	if m.IsSingleROMOnly() {
		ext := strings.ToLower(filepath.Ext(s.upload.Filename))
		dest := roms.DestinationDir(ext) + s.upload.Filename
		if existing := s.inv.ExistingDestPath(s.game.URL, s.upload.Filename); existing != "" {
			dest = existing
		}
		// Patch with already-resolved CDN URL so DownloadScreen doesn't re-resolve.
		patched := s.upload
		patched.URL = s.plan.CDNURL
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, patched, dest, s.inv, s.invPath, s.prev)
	}

	// Multiple ROMs of same extension always require a version picker.
	if m.HasDuplicateROMExt() || s.cfg.MusicDownload == "ask" {
		return NewZIPContentsScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, s.plan, s.inv, s.invPath, s.prev)
	}

	// Auto path: apply settings defaults.
	plan := s.plan
	plan.DownloadROMs = m.HasROMs()
	plan.DownloadMusic = m.HasMusic() && s.cfg.MusicDownload == "auto"
	if plan.DownloadMusic {
		if s.cfg.MusicLocation == "ask" {
			return NewMusicLocationPickerScreen(s.client, s.cfg, s.cfgPath,
				s.game, s.detail, plan, s.inv, s.invPath, s.prev)
		}
		plan.MusicDir = roms.MusicDestinationDir(s.game.Title)
	}
	return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
}
```

- [ ] **Step 7.2: Build to check for compile errors (headless)**

```bash
./scripts/build.sh native 2>&1 | tail -20
```
Expected: success (or only unresolved references to `NewZIPContentsScreen`, `NewMusicLocationPickerScreen`, `NewZIPDownloadScreen` which will be added in later tasks).

- [ ] **Step 7.3: Commit**

```bash
git add internal/ui/screen_zip_inspect.go
git commit -m "feat(ui): add ZIPInspectScreen and ZIPPlan type"
```

---

## Task 8: `ZIPContentsScreen`

**Files:**
- Create: `internal/ui/screen_zip_contents.go`

- [ ] **Step 8.1: Create `screen_zip_contents.go`**

```go
//go:build !headless

package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type zipContentRowKind int

const (
	zipRowROM        zipContentRowKind = iota // selectable ROM entry (for version picker)
	zipRowMusicToggle                         // music download yes/no
)

type zipContentRow struct {
	kind    zipContentRowKind
	entry   roms.ZIPEntry // for zipRowROM
	ext     string        // lowercase extension group, for zipRowROM
	toggled bool          // for zipRowMusicToggle: current value
}

// ZIPContentsScreen shows the ZIP manifest to the user.
// It is used for two cases:
//  1. Multiple ROMs of same extension → version picker (user selects one per ext).
//  2. MusicDownload="ask" → music toggle (user chooses whether to download music).
type ZIPContentsScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	plan    ZIPPlan
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	rows         []zipContentRow
	cursor       int
	selectedROMs map[string]string // ext → selected Name
}

func NewZIPContentsScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	cache *renderer.ImageCache,
	game itchio.Game, detail *itchio.GameDetail, plan ZIPPlan,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *ZIPContentsScreen {
	s := &ZIPContentsScreen{
		client: client, cfg: cfg, cfgPath: cfgPath, cache: cache,
		game: game, detail: detail, plan: plan,
		inv: inv, invPath: invPath, prev: prev,
		selectedROMs: make(map[string]string),
	}
	s.buildRows()
	return s
}

func (s *ZIPContentsScreen) buildRows() {
	s.rows = nil
	byExt := s.plan.Manifest.ROMsByExt()

	// Add ROM rows only for extensions that have duplicates (need user choice).
	for ext, entries := range byExt {
		if len(entries) < 2 {
			// Auto-select the only entry for this extension.
			s.selectedROMs[ext] = entries[0].Name
			continue
		}
		// Default: select first entry.
		if _, ok := s.selectedROMs[ext]; !ok {
			s.selectedROMs[ext] = entries[0].Name
		}
		for _, e := range entries {
			s.rows = append(s.rows, zipContentRow{kind: zipRowROM, entry: e, ext: ext})
		}
	}

	// Add music toggle when MusicDownload="ask" and manifest has music.
	if s.cfg.MusicDownload == "ask" && s.plan.Manifest.HasMusic() {
		s.rows = append(s.rows, zipContentRow{kind: zipRowMusicToggle, toggled: false})
	}
}

func (s *ZIPContentsScreen) NeedsRedraw() bool      { return false }
func (s *ZIPContentsScreen) HasPendingAnimation() bool { return false }

func (s *ZIPContentsScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := mainFH + smallFH + 16

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	at := r.Theme.AccentText
	lt := r.Theme.ListText
	mt := r.Theme.MainText
	ht := r.Theme.HintText

	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	y := headerH + 10
	// Summary line
	m := s.plan.Manifest
	summary := fmt.Sprintf("ZIP contains: %d ROM(s)", m.ROMCount())
	if m.HasMusic() {
		summary += fmt.Sprintf("  ·  %d music track(s)", m.MusicCount())
	}
	r.DrawSmallText(summary, 20, y, 140, 140, 140)
	y += smallFH + 12

	rowH := mainFH + 14
	for i, row := range s.rows {
		selected := i == s.cursor
		if selected {
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		var tr, tg, tb uint8
		if selected {
			tr, tg, tb = at[0], at[1], at[2]
		} else {
			tr, tg, tb = lt[0], lt[1], lt[2]
		}

		switch row.kind {
		case zipRowROM:
			isChosen := s.selectedROMs[row.ext] == row.entry.Name
			marker := "  "
			if isChosen {
				marker = "● " // filled circle
			}
			label := marker + row.entry.Name
			r.DrawText(label, 20, y, tr, tg, tb)
		case zipRowMusicToggle:
			val := "NO"
			if row.toggled {
				val = "YES"
			}
			label := "Download soundtrack:"
			r.DrawText(label, 20, y, tr, tg, tb)
			lw, _ := r.TextSize(label)
			r.DrawText(" "+val, 20+lw, y, tr, tg, tb)
		}
		y += rowH
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Select/Toggle"},
		{Kind: renderer.BadgePill, Label: "START", Text: "Confirm"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Back"},
	}, ftrY)
	r.Present()
}

func (s *ZIPContentsScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN: // B = select/toggle
			s.activate()
		case sdl.K_s: // START = confirm
			return s.confirm()
		case sdl.K_ESCAPE: // A = back
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B: // physical A = select/toggle
			s.activate()
		case sdl.CONTROLLER_BUTTON_START:
			return s.confirm()
		case sdl.CONTROLLER_BUTTON_A: // physical B = back
			return s.prev
		}
	}
	return s
}

func (s *ZIPContentsScreen) activate() {
	if s.cursor >= len(s.rows) {
		return
	}
	row := s.rows[s.cursor]
	switch row.kind {
	case zipRowROM:
		s.selectedROMs[row.ext] = row.entry.Name
	case zipRowMusicToggle:
		s.rows[s.cursor].toggled = !s.rows[s.cursor].toggled
	}
}

func (s *ZIPContentsScreen) confirm() Screen {
	plan := s.plan
	plan.SelectedROMs = s.selectedROMs
	plan.DownloadROMs = true

	// Find music toggle state (if present)
	plan.DownloadMusic = false
	for _, row := range s.rows {
		if row.kind == zipRowMusicToggle {
			plan.DownloadMusic = row.toggled
			break
		}
	}
	// When MusicDownload="auto" (screen shown only for duplicate ROMs), include music.
	if s.cfg.MusicDownload == "auto" && s.plan.Manifest.HasMusic() {
		plan.DownloadMusic = true
	}

	if plan.DownloadMusic {
		if s.cfg.MusicLocation == "ask" {
			return NewMusicLocationPickerScreen(s.client, s.cfg, s.cfgPath,
				s.game, s.detail, plan, s.inv, s.invPath, s.prev)
		}
		plan.MusicDir = roms.MusicDestinationDir(s.game.Title)
	}

	_ = strings.TrimSpace("") // keep import
	return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
}
```

- [ ] **Step 8.2: Remove unused import**

The `strings` import in `confirm()` is a placeholder; remove it if unused after the full implementation. Review and clean up imports.

- [ ] **Step 8.3: Build to check for errors**

```bash
./scripts/build.sh native 2>&1 | tail -20
```

- [ ] **Step 8.4: Commit**

```bash
git add internal/ui/screen_zip_contents.go
git commit -m "feat(ui): add ZIPContentsScreen for version picker and music toggle"
```

---

## Task 9: `MusicLocationPickerScreen`

**Files:**
- Create: `internal/ui/screen_music_location_picker.go`

- [ ] **Step 9.1: Create `screen_music_location_picker.go`**

This screen mirrors `LocationPickerScreen` but starts at `/mnt/SDCARD/Music/` and
routes to `ZIPDownloadScreen` on confirm.

```go
//go:build !headless

package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

const musicLocationRoot = "/mnt/SDCARD/Music"

// MusicLocationPickerScreen lets the user choose where to save a game soundtrack.
// It works like LocationPickerScreen but is rooted at /mnt/SDCARD/Music and
// routes to ZIPDownloadScreen on confirmation.
type MusicLocationPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	game    itchio.Game
	detail  *itchio.GameDetail
	plan    ZIPPlan
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	currentDir   string
	rows         []pickerRow // reuses the pickerRow type from screen_location_picker.go
	cursor       int
	scrollOffset int
}

func NewMusicLocationPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail, plan ZIPPlan,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *MusicLocationPickerScreen {
	startDir := roms.MusicDestinationDir(game.Title)
	// If the default dir doesn't exist yet, start at the root music dir.
	if _, err := os.Stat(startDir); err != nil {
		startDir = musicLocationRoot + "/"
	}
	s := &MusicLocationPickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath,
		game: game, detail: detail, plan: plan,
		inv: inv, invPath: invPath, prev: prev,
	}
	s.loadDir(startDir)
	return s
}

func (s *MusicLocationPickerScreen) loadDir(dir string) {
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	s.currentDir = dir
	s.rows = buildRows(dir) // reuses buildRows from screen_location_picker.go
	s.cursor = 0
	s.scrollOffset = 0
}

func (s *MusicLocationPickerScreen) atRoot() bool {
	return strings.TrimRight(s.currentDir, "/") == musicLocationRoot
}

func (s *MusicLocationPickerScreen) NeedsRedraw() bool      { return false }
func (s *MusicLocationPickerScreen) HasPendingAnimation() bool { return false }

func (s *MusicLocationPickerScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	_, mainFH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	footerH := int32(52)
	ac := r.Theme.Accent
	hBG := r.Theme.HeaderBG
	mt := r.Theme.MainText
	ht := r.Theme.HintText
	at := r.Theme.AccentText
	lt := r.Theme.ListText

	headerH := mainFH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, hBG[0], hBG[1], hBG[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, 8+mainFH+4, ht[0], ht[1], ht[2])

	pathBarY := headerH + 2
	pathBarH := smallFH + 10
	r.DrawRect(0, pathBarY, r.W, pathBarH, hBG[0], hBG[1], hBG[2])
	r.DrawSmallText(leftTruncatePath(r, s.currentDir, r.W-24), 12, pathBarY+5, 120, 160, 200)

	confirmY := pathBarY + pathBarH
	confirmH := mainFH + 10
	if s.cursor == 0 {
		r.DrawRect(0, confirmY, r.W, confirmH, 26, 58, 34)
	} else {
		r.DrawRect(0, confirmY, r.W, confirmH, 15, 32, 22)
	}
	r.DrawText("[ ✓  Save here ]", 12, confirmY+5, 80, 200, 120)
	r.DrawRect(0, confirmY+confirmH, r.W, 1, 28, 58, 28)

	listTop := confirmY + confirmH + 2
	rowH := mainFH + 14
	visibleCount := int((r.H - footerH - listTop) / rowH)
	if visibleCount < 1 {
		visibleCount = 1
	}
	s.clampScroll(visibleCount)

	listRowsDrawn := int32(0)
	for i := 1 + s.scrollOffset; i < len(s.rows); i++ {
		row := s.rows[i]
		y := listTop + listRowsDrawn*rowH
		if y+rowH > r.H-footerH {
			break
		}
		selected := s.cursor == i
		switch row.kind {
		case rowUp:
			if selected {
				r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			var tr, tg, tb uint8
			if selected {
				tr, tg, tb = at[0], at[1], at[2]
			} else {
				tr, tg, tb = 100, 140, 180
			}
			r.DrawSmallText("↑  .. (go up)", 20, y+(rowH-smallFH)/2, tr, tg, tb)
		case rowEntry:
			if selected {
				r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			}
			var tr, tg, tb uint8
			if selected {
				tr, tg, tb = at[0], at[1], at[2]
			} else {
				tr, tg, tb = lt[0], lt[1], lt[2]
			}
			r.DrawText("▸ "+row.name, 20, y, tr, tg, tb)
		}
		listRowsDrawn++
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Confirm/Enter"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Go up"},
		{Kind: renderer.BadgePill, Label: "START", Text: "Cancel"},
	}, ftrY)
	r.Present()
}

func (s *MusicLocationPickerScreen) clampScroll(visibleCount int) {
	if s.cursor == 0 {
		return
	}
	idx := s.cursor - 1
	if idx < s.scrollOffset {
		s.scrollOffset = idx
	}
	if idx >= s.scrollOffset+visibleCount {
		s.scrollOffset = idx - visibleCount + 1
	}
}

func (s *MusicLocationPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			return s.activate()
		case sdl.K_ESCAPE:
			return s.goUp()
		case sdl.K_s:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.rows)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.activate()
		case sdl.CONTROLLER_BUTTON_A:
			return s.goUp()
		case sdl.CONTROLLER_BUTTON_START:
			return s.prev
		}
	}
	return s
}

func (s *MusicLocationPickerScreen) activate() Screen {
	if s.cursor >= len(s.rows) {
		return s
	}
	row := s.rows[s.cursor]
	switch row.kind {
	case rowSaveHere:
		return s.confirm()
	case rowUp:
		return s.goUp()
	case rowEntry:
		s.loadDir(s.currentDir + row.name + "/")
	}
	return s
}

func (s *MusicLocationPickerScreen) goUp() Screen {
	if s.atRoot() {
		return s
	}
	parent := filepath.Dir(strings.TrimRight(s.currentDir, "/"))
	s.loadDir(parent)
	return s
}

func (s *MusicLocationPickerScreen) confirm() Screen {
	plan := s.plan
	plan.MusicDir = s.currentDir
	return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
}
```

- [ ] **Step 9.2: Build check**

```bash
./scripts/build.sh native 2>&1 | tail -20
```

- [ ] **Step 9.3: Commit**

```bash
git add internal/ui/screen_music_location_picker.go
git commit -m "feat(ui): add MusicLocationPickerScreen for soundtrack save location"
```

---

## Task 10: `ZIPDownloadScreen`

**Files:**
- Create: `internal/ui/screen_zip_download.go`

- [ ] **Step 10.1: Create `screen_zip_download.go`**

```go
//go:build !headless

package ui

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type zipDLState int32

const (
	zipDLDownloading zipDLState = iota
	zipDLExtracting
	zipDLDone
	zipDLError
)

// ZIPDownloadScreen downloads a ZIP to a temp path, extracts ROM and music files
// to their respective destinations, and records all extracted files in inventory.
type ZIPDownloadScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	game    itchio.Game
	detail  *itchio.GameDetail
	plan    ZIPPlan
	prev    Screen
	inv     *inventory.Inventory
	invPath string

	state       zipDLState
	downloaded  int64
	total       int64
	extracted   []string // paths of successfully extracted files (for summary)
	skipped     []string // filenames skipped due to extraction errors
	musicFailed bool     // true if music dir creation failed
	err         error
}

func (s *ZIPDownloadScreen) loadState() zipDLState {
	return zipDLState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *ZIPDownloadScreen) storeState(st zipDLState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}

func NewZIPDownloadScreen(
	client *itchio.Client, cfg *settings.Config,
	game itchio.Game, detail *itchio.GameDetail, plan ZIPPlan,
	inv *inventory.Inventory, invPath string,
	prev Screen,
) *ZIPDownloadScreen {
	s := &ZIPDownloadScreen{
		client: client, cfg: cfg,
		game: game, detail: detail, plan: plan,
		inv: inv, invPath: invPath, prev: prev,
	}
	go s.run()
	return s
}

func (s *ZIPDownloadScreen) run() {
	defer func() { sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT}) }()

	// Phase 1: Download to temp file.
	tmp, err := os.CreateTemp("", "itchio-zip-*.zip")
	if err != nil {
		s.err = fmt.Errorf("create temp file: %w", err)
		s.storeState(zipDLError)
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	progress := func(dl, total int64) {
		atomic.StoreInt64(&s.downloaded, dl)
		atomic.StoreInt64(&s.total, total)
	}
	logger.Info("zip-download: streaming %s → %s", s.plan.Upload.Filename, tmpPath)
	if err := s.client.DownloadURL(s.plan.CDNURL, tmpPath, progress); err != nil {
		s.err = fmt.Errorf("download ZIP: %w", err)
		s.storeState(zipDLError)
		return
	}

	// Phase 2: Extract.
	s.storeState(zipDLExtracting)
	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})

	r, err := zip.OpenReader(tmpPath)
	if err != nil {
		s.err = fmt.Errorf("open ZIP: %w", err)
		s.storeState(zipDLError)
		return
	}
	defer r.Close()

	now := time.Now()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		baseName := filepath.Base(f.Name)
		kind := roms.ClassifyEntry(baseName)

		switch kind {
		case roms.KindROM:
			if !s.shouldExtractROM(baseName) {
				continue
			}
			dest, err := s.extractROM(f, baseName, now)
			if err != nil {
				logger.Warn("zip-download: ROM %s: %v", baseName, err)
				s.skipped = append(s.skipped, baseName)
				continue
			}
			s.extracted = append(s.extracted, dest)

		case roms.KindMusic:
			if !s.plan.DownloadMusic || s.plan.MusicDir == "" {
				continue
			}
			dest, err := s.extractMusic(f, baseName, now)
			if err != nil {
				logger.Warn("zip-download: music %s: %v", baseName, err)
				s.skipped = append(s.skipped, baseName)
				continue
			}
			s.extracted = append(s.extracted, dest)
		}
	}

	if err := s.inv.Save(s.invPath); err != nil {
		logger.Warn("zip-download: save inventory: %v", err)
	}

	if len(s.extracted) == 0 {
		s.err = fmt.Errorf("no files could be extracted from ZIP")
		s.storeState(zipDLError)
		return
	}
	logger.Info("zip-download: done, extracted %d file(s)", len(s.extracted))
	s.storeState(zipDLDone)
}

func (s *ZIPDownloadScreen) shouldExtractROM(name string) bool {
	if !s.plan.DownloadROMs {
		return false
	}
	if len(s.plan.SelectedROMs) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	chosen, ok := s.plan.SelectedROMs[ext]
	if !ok {
		return true
	}
	return chosen == name
}

func (s *ZIPDownloadScreen) extractROM(f *zip.File, baseName string, now time.Time) (string, error) {
	ext := strings.ToLower(filepath.Ext(baseName))
	destDir := roms.DestinationDir(ext)
	stem := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	safeName := roms.SanitiseFilename(stem, ext)
	if safeName == "" {
		safeName = baseName
	}
	dest := destDir + safeName

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("mkdirall %s: %w", destDir, err)
	}
	if err := extractZIPEntry(f, dest); err != nil {
		return "", err
	}

	// Apply unified naming.
	finalDest := dest
	unifiedName := false
	if s.cfg.UnifiedNaming {
		entry, entryExists := s.inv.Lookup(s.game.URL)
		disabled := entryExists && entry.UnifiedNamingDisabled
		if !disabled {
			newDest, didRename := roms.ResolveUnifiedDest(dest, s.game.Title)
			if didRename {
				if err := os.Rename(dest, newDest); err != nil {
					logger.Warn("zip-download: unified rename: %v", err)
				} else {
					finalDest = newDest
					unifiedName = true
				}
			} else {
				unifiedName = true
			}
		}
	}

	if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
		logger.Warn("zip-download: cover art: %v", artErr)
	}
	s.inv.Add(s.game.URL, inventory.Entry{
		GameURL: s.game.URL, Title: s.game.Title,
		Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
	}, inventory.DownloadedFile{
		Filename:     s.plan.Upload.Filename,
		DestPath:     finalDest,
		DownloadedAt: now,
		UnifiedName:  unifiedName,
		FileType:     inventory.FileTypeROM,
	})
	return finalDest, nil
}

func (s *ZIPDownloadScreen) extractMusic(f *zip.File, baseName string, now time.Time) (string, error) {
	if err := os.MkdirAll(s.plan.MusicDir, 0755); err != nil {
		s.musicFailed = true
		return "", fmt.Errorf("mkdirall music dir %s: %w", s.plan.MusicDir, err)
	}
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	safeName := roms.SanitiseFilename(stem, ext)
	if safeName == "" {
		safeName = baseName
	}
	dest := s.plan.MusicDir + safeName

	if err := extractZIPEntry(f, dest); err != nil {
		return "", err
	}
	s.inv.Add(s.game.URL, inventory.Entry{
		GameURL: s.game.URL, Title: s.game.Title,
		Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
	}, inventory.DownloadedFile{
		Filename:     s.plan.Upload.Filename,
		DestPath:     dest,
		DownloadedAt: now,
		FileType:     inventory.FileTypeMusic,
	})
	return dest, nil
}

// extractZIPEntry copies a ZIP file entry to dest on disk.
func extractZIPEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func (s *ZIPDownloadScreen) NeedsRedraw() bool      { return true }
func (s *ZIPDownloadScreen) HasPendingAnimation() bool { return false }

func (s *ZIPDownloadScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16

	hdr := r.Theme.HeaderBG
	ac := r.Theme.Accent
	mt := r.Theme.MainText
	ht := r.Theme.HintText
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawText(truncateToWidth(r, s.game.Title, r.W-24), 12, 8, mt[0], mt[1], mt[2])
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, ht[0], ht[1], ht[2])

	contentTop := headerH + 10
	contentH := r.H - headerH - footerH
	mid := headerH + contentH/2

	switch s.loadState() {
	case zipDLDownloading:
		dl := atomic.LoadInt64(&s.downloaded)
		tot := atomic.LoadInt64(&s.total)
		r.DrawSmallText(s.plan.Upload.Filename, 20, contentTop+4, ht[0], ht[1], ht[2])
		barW := r.W - 80
		r.DrawRect(40, mid-10, barW, 20, 60, 60, 60)
		if tot > 0 {
			filled := int32(float64(barW) * float64(dl) / float64(tot))
			r.DrawRect(40, mid-10, filled, 20, 80, 200, 80)
			r.DrawText(fmt.Sprintf("%d%%  (%s / %s)", dl*100/tot, humanBytes(dl), humanBytes(tot)),
				40, mid+18, mt[0], mt[1], mt[2])
		} else {
			if dl > 0 {
				r.DrawRect(40, mid-10, barW/3, 20, 80, 200, 80)
			}
			r.DrawText(humanBytes(dl)+" downloaded", 40, mid+18, mt[0], mt[1], mt[2])
		}

	case zipDLExtracting:
		r.DrawTextCentered("Extracting…", 0, mid-fontH/2, r.W, mt[0], mt[1], mt[2])

	case zipDLDone:
		r.DrawTextCentered("Extraction complete!", 0, mid-fontH-8, r.W, 80, 200, 80)
		y := mid + 4
		for _, p := range s.extracted {
			dir := truncateSmallToWidth(r, filepath.Dir(p)+"/", r.W-40)
			r.DrawSmallTextCentered(filepath.Base(p), 0, y, r.W, 120, 120, 120)
			y += smallFH + 2
			r.DrawSmallTextCentered(dir, 0, y, r.W, 80, 80, 80)
			y += smallFH + 6
		}
		if s.musicFailed {
			r.DrawSmallTextCentered("Note: music folder could not be created", 0, y, r.W, 200, 160, 60)
		}

	case zipDLError:
		y := contentTop + 8
		r.DrawText("Extraction failed:", 20, y, 200, 60, 60)
		y += fontH + 6
		r.DrawWrappedText(s.err.Error(), 20, y, r.W-40, fontH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.loadState() {
	case zipDLDownloading, zipDLExtracting:
		r.DrawSmallText("Please wait…", 10, ftrY, ht[0], ht[1], ht[2])
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *ZIPDownloadScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		if s.loadState() != zipDLDownloading && s.loadState() != zipDLExtracting {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE, sdl.K_RETURN:
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		if s.loadState() != zipDLDownloading && s.loadState() != zipDLExtracting {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
				return s.prev
			}
		}
	}
	return s
}

// IsBusy implements BusyChecker. Returns true while download or extraction is in flight.
func (s *ZIPDownloadScreen) IsBusy() bool {
	st := s.loadState()
	return st == zipDLDownloading || st == zipDLExtracting
}
```

- [ ] **Step 10.2: Build check**

```bash
./scripts/build.sh native 2>&1 | tail -20
```

- [ ] **Step 10.3: Commit**

```bash
git add internal/ui/screen_zip_download.go
git commit -m "feat(ui): add ZIPDownloadScreen with two-phase download and extraction"
```

---

## Task 11: Wire existing screens to route ZIP uploads through `ZIPInspectScreen`

**Files:**
- Modify: `internal/ui/screen_fetch_uploads.go`
- Modify: `internal/ui/screen_rom_picker.go`

- [ ] **Step 11.1: Update `screen_fetch_uploads.go` — `nextScreen()` method**

In `nextScreen()` (around line 265), in the `len(known) == 1` branch, add a ZIP check before creating `DownloadScreen`:

```go
func (s *FetchUploadsScreen) nextScreen() Screen {
	var known, unknown []roms.Upload
	for _, u := range s.uploads {
		if u.NeedsFormat {
			unknown = append(unknown, u)
		} else {
			known = append(known, u)
		}
	}

	if len(known) > 0 {
		if len(known) == 1 {
			upload := known[0]
			// Route ZIP uploads through ZIPInspectScreen for smart handling.
			if strings.ToLower(filepath.Ext(upload.Filename)) == ".zip" {
				return NewZIPInspectScreen(s.client, s.cfg, s.cfgPath, s.cache,
					s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
			}
			if s.cfg.ROMLocation == "ask" {
				return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
			}
			ext := strings.ToLower(filepath.Ext(upload.Filename))
			dest := roms.DestinationDir(ext) + upload.Filename
			if existing := s.inv.ExistingDestPath(s.game.URL, upload.Filename); existing != "" {
				dest = existing
			}
			return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
		}
		return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, known, s.inv, s.inventoryPath, s.prev)
	}
	return NewFormatPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, unknown, s.inv, s.inventoryPath, s.prev)
}
```

Ensure `"path/filepath"` and `"strings"` are in the imports (they already are).

- [ ] **Step 11.2: Update `screen_rom_picker.go` — `chooseUpload()` method**

Replace the body of `chooseUpload`:

```go
func (s *ROMPickerScreen) chooseUpload(upload roms.Upload) Screen {
	// ZIP uploads go through ZIPInspectScreen for smart content handling.
	if strings.ToLower(filepath.Ext(upload.Filename)) == ".zip" {
		return NewZIPInspectScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
	}
	if s.cfg.ROMLocation == "ask" {
		return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.inv, s.inventoryPath, s.prev)
	}
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename
	if existing := s.inv.ExistingDestPath(s.game.URL, upload.Filename); existing != "" {
		dest = existing
	}
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.inv, s.inventoryPath, s.prev)
}
```

Ensure `"path/filepath"` is in imports (it already is).

- [ ] **Step 11.3: Build — must compile cleanly**

```bash
./scripts/build.sh native 2>&1 | tail -20
```

- [ ] **Step 11.4: Commit**

```bash
git add internal/ui/screen_fetch_uploads.go internal/ui/screen_rom_picker.go
git commit -m "feat(ui): route ZIP uploads through ZIPInspectScreen"
```

---

## Task 12: Settings UI — Music Download and Music Location

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 12.1: Add two new `settingsItem` constants**

In `screen_settings.go`, add after `sItemROMLocation`:

```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemROMLocation
	sItemMusicDownload // new
	sItemMusicLocation // new
	sItemUnifiedNaming
	sItemNextUITheme
	sItemLogLevel
	sItemClearCache
	sItemRefreshCache
	sItemUpdateInventory
	sItemContentModeration
	sItemAbout
	sItemCount
)
```

- [ ] **Step 12.2: Add items to the menu in `Draw()`**

In `Draw()`, after the `sItemROMLocation` append, add:

```go
items = append(items, menuItem{sItemMusicDownload, "Music Download: " + cfg.musicDownloadLabel(s.cfg.MusicDownload)})
if s.cfg.MusicDownload != "off" {
	items = append(items, menuItem{sItemMusicLocation, "Music Location: " + s.cfg.MusicLocation})
}
```

Add a helper method (can be a package-level function in settings or a local function in the file):

```go
// musicDownloadLabel returns a human-readable label for a MusicDownload value.
func musicDownloadLabel(v string) string {
	switch v {
	case "auto":
		return "auto"
	case "ask":
		return "ask"
	default:
		return "off"
	}
}
```

Add the call inline in Draw():

```go
items = append(items, menuItem{sItemMusicDownload, "Music Download: " + musicDownloadLabel(s.cfg.MusicDownload)})
if s.cfg.MusicDownload != "off" {
	items = append(items, menuItem{sItemMusicLocation, "Music Location: " + s.cfg.MusicLocation})
}
```

- [ ] **Step 12.3: Add activation logic in `activate()`**

In `activate()`, add cases after `sItemROMLocation`:

```go
case sItemMusicDownload:
	switch s.cfg.MusicDownload {
	case "off":
		s.cfg.MusicDownload = "auto"
	case "auto":
		s.cfg.MusicDownload = "ask"
	default:
		s.cfg.MusicDownload = "off"
	}
	s.cfg.Save(s.cfgPath)
	logger.Info("settings: music download changed to %s", s.cfg.MusicDownload)
case sItemMusicLocation:
	if s.cfg.MusicLocation == "auto" {
		s.cfg.MusicLocation = "ask"
	} else {
		s.cfg.MusicLocation = "auto"
	}
	s.cfg.Save(s.cfgPath)
	logger.Info("settings: music location changed to %s", s.cfg.MusicLocation)
```

- [ ] **Step 12.4: Update `moveCursor` to skip `sItemMusicLocation` when MusicDownload is "off"**

In `moveCursor`, after the NextUI Theme skip logic, add:

```go
if s.cursor == sItemMusicLocation && s.cfg.MusicDownload == "off" {
	if dir >= 0 {
		if int(s.cursor) < int(sItemCount)-1 {
			s.cursor++
		} else {
			s.cursor--
		}
	} else {
		if s.cursor > 0 {
			s.cursor--
		} else {
			s.cursor++
		}
	}
}
```

- [ ] **Step 12.5: Build check**

```bash
./scripts/build.sh native 2>&1 | tail -20
```

- [ ] **Step 12.6: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat(ui): add Music Download and Music Location settings"
```

---

## Task 13: FileType-aware deletion in `ManageDownloadsScreen`

**Files:**
- Modify: `internal/ui/screen_manage_downloads.go`

- [ ] **Step 13.1: Add delete-mode constants**

At the top of `screen_manage_downloads.go`, add:

```go
const (
	deleteIdxAll        = -1 // existing
	deleteIdxROMs       = -2 // new: delete only ROM-typed files
	deleteIdxSoundtrack = -3 // new: delete only music-typed files
)
```

- [ ] **Step 13.2: Add helper to categorise files**

Add after the constants:

```go
func hasFileType(files []inventory.DownloadedFile, ft string) bool {
	for _, f := range files {
		if f.FileType == ft || (ft == inventory.FileTypeROM && f.FileType == "") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 13.3: Update `Draw()` — action rows**

Replace the "Delete all" rendering block in `Draw()` with:

```go
sepY := contentTop + int32(len(entry.Files))*rowH
r.DrawRect(margin, sepY, r.W-margin*2, 1, 50, 50, 50)
actionY := sepY + 8

hasROM := hasFileType(entry.Files, inventory.FileTypeROM)
hasMusic := hasFileType(entry.Files, inventory.FileTypeMusic)
deleteAllIdx := len(entry.Files)

if hasROM && hasMusic {
	// Delete ROM row
	if s.cursor == deleteAllIdx && !s.confirmActive {
		r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
	}
	r.DrawText("Delete ROM", margin, actionY, 200, 120, 80)
	actionY += rowH

	// Delete Soundtrack row
	if s.cursor == deleteAllIdx+1 && !s.confirmActive {
		r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
	}
	r.DrawText("Delete Soundtrack", margin, actionY, 200, 120, 80)
	actionY += rowH

	// Delete All row
	if s.cursor == deleteAllIdx+2 && !s.confirmActive {
		r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
	}
	r.DrawText("Delete all", margin, actionY, 200, 80, 80)
	actionY += rowH
} else {
	if s.cursor == deleteAllIdx && !s.confirmActive {
		r.DrawPill(4, actionY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
	}
	r.DrawText("Delete all", margin, actionY, 200, 80, 80)
	actionY += rowH
}

sep2Y := actionY + 4
// (continue with existing unified naming toggle using sep2Y instead of sep2Y)
```

Update the unified naming toggle `sep2Y` and `toggleY` references to use the new `actionY` variable.

- [ ] **Step 13.4: Update `HandleEvent()` — row count and action dispatch**

Update `rowCount` calculation:

```go
entry, ok := s.inv.Lookup(s.gameURL)
if !ok {
	return s.prev
}
hasROM := hasFileType(entry.Files, inventory.FileTypeROM)
hasMusic := hasFileType(entry.Files, inventory.FileTypeMusic)
extraActions := 1 // just "Delete all"
if hasROM && hasMusic {
	extraActions = 3 // Delete ROM + Delete Soundtrack + Delete All
}
rowCount := len(entry.Files) + extraActions + 1 // +1 for unified naming
deleteAllIdx := len(entry.Files)
```

In the B-button / K_RETURN handler, replace:

```go
if s.cursor == len(entry.Files) {
    s.confirmActive = true
    s.confirmFileIdx = -1
```

with:

```go
switch {
case s.cursor < len(entry.Files):
    s.confirmActive = true
    s.confirmFileIdx = s.cursor
case hasROM && hasMusic && s.cursor == deleteAllIdx:
    s.confirmActive = true
    s.confirmFileIdx = deleteIdxROMs
case hasROM && hasMusic && s.cursor == deleteAllIdx+1:
    s.confirmActive = true
    s.confirmFileIdx = deleteIdxSoundtrack
case s.cursor == deleteAllIdx+(func() int { if hasROM && hasMusic { return 2 }; return 0 }()):
    s.confirmActive = true
    s.confirmFileIdx = deleteIdxAll
case s.cursor == len(entry.Files)+extraActions:
    // unified naming toggle
    if s.cfg.UnifiedNaming {
        return s.startUnifiedNamingMigration(entry)
    }
}
```

- [ ] **Step 13.5: Update `performDelete()` to handle the new delete modes**

Add cases for `deleteIdxROMs` and `deleteIdxSoundtrack` before the existing `fileIdx == -1` branch:

```go
func (s *ManageDownloadsScreen) performDelete(gameURL string, fileIdx int) (bool, int) {
	entry, ok := s.inv.Lookup(gameURL)
	if !ok {
		return true, 0
	}

	var toDelete []inventory.DownloadedFile
	switch fileIdx {
	case deleteIdxAll:
		toDelete = make([]inventory.DownloadedFile, len(entry.Files))
		copy(toDelete, entry.Files)
	case deleteIdxROMs:
		for _, f := range entry.Files {
			if f.FileType == inventory.FileTypeROM || f.FileType == "" {
				toDelete = append(toDelete, f)
			}
		}
	case deleteIdxSoundtrack:
		for _, f := range entry.Files {
			if f.FileType == inventory.FileTypeMusic {
				toDelete = append(toDelete, f)
			}
		}
	default:
		if fileIdx >= 0 && fileIdx < len(entry.Files) {
			toDelete = []inventory.DownloadedFile{entry.Files[fileIdx]}
		}
	}

	for _, f := range toDelete {
		if err := os.Remove(f.DestPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("inventory: delete file=%s: %v", f.DestPath, err)
		} else {
			logger.Debug("inventory: deleted file=%s", f.DestPath)
		}
		if f.FileType != inventory.FileTypeMusic {
			if artPath := inventory.CoverArtPath(entry.CoverURL, f.DestPath); artPath != "" {
				if err := os.Remove(artPath); err != nil && !os.IsNotExist(err) {
					logger.Warn("inventory: delete cover-art=%s: %v", artPath, err)
				}
			}
		}
	}
	logger.Info("inventory: deleted game=%q files=%d", entry.Title, len(toDelete))

	if fileIdx == deleteIdxAll {
		s.inv.Remove(gameURL)
		if err := s.inv.Save(s.inventoryPath); err != nil {
			logger.Warn("inventory: save after delete failed: %v", err)
		}
		return true, 0
	}

	for _, f := range toDelete {
		s.inv.RemoveFile(gameURL, f.DestPath)
	}
	remaining := 0
	if e, ok := s.inv.Lookup(gameURL); ok {
		remaining = len(e.Files)
	}
	allGone := remaining == 0
	if allGone {
		s.inv.Remove(gameURL)
	}
	if err := s.inv.Save(s.inventoryPath); err != nil {
		logger.Warn("inventory: save after delete failed: %v", err)
	}
	return allGone, remaining
}
```

- [ ] **Step 13.6: Build check**

```bash
./scripts/build.sh native 2>&1 | tail -20
```

- [ ] **Step 13.7: Commit**

```bash
git add internal/ui/screen_manage_downloads.go
git commit -m "feat(ui): FileType-aware deletion in ManageDownloadsScreen"
```

---

## Task 14: Final build and test pass

- [ ] **Step 14.1: Run the full test suite**

```bash
./scripts/test.sh 2>&1
```
Expected: all packages pass, no `FAIL` lines.

- [ ] **Step 14.2: Build all platforms**

```bash
./scripts/build.sh all 2>&1 | tail -30
```
Expected: `tg5040`, `tg5050`, `my355` all succeed.

- [ ] **Step 14.3: Commit any last fixes, then tag**

```bash
git add -p  # stage any remaining changes
git commit -m "chore: final build fixes for smart ZIP support"
```
