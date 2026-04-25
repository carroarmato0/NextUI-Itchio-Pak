# Cover Art Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After a ROM download completes, automatically fetch the game's cover art from Itch.io and save it into the `.media/` directory alongside the ROM so NextUI can display box art.

**Architecture:** A new `DownloadCoverArt` method on `*Client` handles path derivation, directory creation, and streaming. It is called sequentially in the existing download goroutine in `screen_download.go` after the ROM file is fully written. Any error is logged at Warn level and the download is considered complete regardless.

**Tech Stack:** Go standard library (`net/http`, `net/url`, `os`, `path/filepath`, `io`), `httptest` for tests, existing `logger` package.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/itchio/cover_art.go` | Create | `DownloadCoverArt` method — path derivation, mkdir, HTTP fetch, file write |
| `internal/itchio/cover_art_test.go` | Create | Unit tests for all `DownloadCoverArt` branches |
| `internal/ui/screen_download.go` | Modify | Call `DownloadCoverArt` after successful ROM write, log any error |

---

## Task 1: Implement `DownloadCoverArt`

**Files:**
- Create: `internal/itchio/cover_art.go`

- [ ] **Step 1: Create the file with the method stub**

```go
package itchio

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// DownloadCoverArt fetches the cover image at coverURL and saves it into the
// .media/ subdirectory of the ROM's containing directory. The art filename
// matches the ROM basename with the extension taken from the cover URL path
// (falling back to .png if none is present). Returns nil for an empty coverURL.
func (c *Client) DownloadCoverArt(coverURL, romDestPath string) error {
	if coverURL == "" {
		logger.Debug("cover-art: no cover URL, skipping")
		return nil
	}

	parsed, err := url.Parse(coverURL)
	if err != nil {
		return fmt.Errorf("cover-art: parse URL: %w", err)
	}

	ext := filepath.Ext(parsed.Path)
	if ext == "" {
		ext = ".png"
	}

	dir := filepath.Dir(romDestPath)
	mediaDir := filepath.Join(dir, ".media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		return fmt.Errorf("cover-art: mkdir: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(romDestPath), filepath.Ext(romDestPath))
	artPath := filepath.Join(mediaDir, base+ext)

	logger.Debug("cover-art: fetching → %s", artPath)

	resp, err := c.http.Get(coverURL)
	if err != nil {
		return fmt.Errorf("cover-art: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover-art: HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(artPath)
	if err != nil {
		return fmt.Errorf("cover-art: create file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("cover-art: write: %w", err)
	}
	logger.Info("cover-art: saved %d bytes → %s", n, artPath)
	return nil
}
```

---

## Task 2: Write and run tests for `DownloadCoverArt`

**Files:**
- Create: `internal/itchio/cover_art_test.go`
- Test command: `./scripts/test.sh` (runs `go test -race -tags headless ./...` inside the build container)

- [ ] **Step 1: Create the test file**

```go
package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// TestDownloadCoverArtEmptyURL verifies that an empty coverURL is a no-op
// and does not create the .media directory.
func TestDownloadCoverArtEmptyURL(t *testing.T) {
	c := itchio.NewClientWithBase("http://localhost")
	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gbc")

	if err := c.DownloadCoverArt("", romPath); err != nil {
		t.Fatalf("expected nil error for empty URL, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".media")); !os.IsNotExist(statErr) {
		t.Error(".media dir must not be created when coverURL is empty")
	}
}

// TestDownloadCoverArtHTTP404 verifies that a non-200 response returns an error.
func TestDownloadCoverArtHTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gbc")

	if err := c.DownloadCoverArt(srv.URL+"/cover.jpg", romPath); err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
}

// TestDownloadCoverArtSuccess verifies the happy path: .media dir is created,
// the file is written with the correct name (ROM basename + URL extension),
// and the bytes match the server response.
func TestDownloadCoverArtSuccess(t *testing.T) {
	imgBytes := []byte("\xff\xd8\xff\xe0FAKEJPEG")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(imgBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "Wario Land II.gbc")

	if err := c.DownloadCoverArt(srv.URL+"/cover.jpg", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	artPath := filepath.Join(dir, ".media", "Wario Land II.jpg")
	got, err := os.ReadFile(artPath)
	if err != nil {
		t.Fatalf("read art file: %v", err)
	}
	if string(got) != string(imgBytes) {
		t.Errorf("art content mismatch: got %q, want %q", got, imgBytes)
	}
}

// TestDownloadCoverArtNoExtFallback verifies that a URL with no file extension
// falls back to .png for the saved filename.
func TestDownloadCoverArtNoExtFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("IMGDATA"))
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gbc")

	if err := c.DownloadCoverArt(srv.URL+"/cover", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	artPath := filepath.Join(dir, ".media", "game.png")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Errorf("expected art file at %s (png fallback), not found", artPath)
	}
}

// TestDownloadCoverArtROMWithNoExt verifies that a ROM path with no extension
// uses the full filename as the art base name.
func TestDownloadCoverArtROMWithNoExt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("IMGDATA"))
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "game") // no extension

	if err := c.DownloadCoverArt(srv.URL+"/cover.png", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	artPath := filepath.Join(dir, ".media", "game.png")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Errorf("expected art file at %s, not found", artPath)
	}
}
```

- [ ] **Step 2: Run tests — expect them to pass**

```bash
./scripts/test.sh
```

Expected: all tests pass with no race conditions.

- [ ] **Step 3: Commit**

```bash
git add internal/itchio/cover_art.go internal/itchio/cover_art_test.go
git commit -m "feat(itchio): add DownloadCoverArt method"
```

---

## Task 3: Wire `DownloadCoverArt` into the download screen

**Files:**
- Modify: `internal/ui/screen_download.go`

The download goroutine currently ends with this block (lines 62–69):

```go
if err != nil {
    logger.Error("download: failed file=%s: %v", upload.Filename, err)
    s.err = err
    s.state = dlError
} else {
    logger.Info("download: complete file=%s", upload.Filename)
    s.state = dlDone
}
```

- [ ] **Step 1: Add the `DownloadCoverArt` call in the success branch**

Replace the `else` block above with:

```go
if err != nil {
    logger.Error("download: failed file=%s: %v", upload.Filename, err)
    s.err = err
    s.state = dlError
} else {
    logger.Info("download: complete file=%s", upload.Filename)
    if artErr := client.DownloadCoverArt(game.CoverURL, dest); artErr != nil {
        logger.Warn("cover-art: %v", artErr)
    }
    s.state = dlDone
}
```

- [ ] **Step 2: Run tests to confirm nothing is broken**

```bash
./scripts/test.sh
```

Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_download.go
git commit -m "feat(ui): download cover art after ROM write"
```

---

## Self-Review

**Spec coverage:**
- ✅ New `DownloadCoverArt` method on `Client` (Task 1)
- ✅ Extension derived from cover URL path, falls back to `.png` (Task 1, Task 2)
- ✅ `.media/` created with `os.MkdirAll` if absent (Task 1)
- ✅ Art filename = ROM basename + URL ext (Task 1, verified in TestDownloadCoverArtSuccess)
- ✅ Empty `coverURL` → `nil` return, no dir created (Task 2 TestDownloadCoverArtEmptyURL)
- ✅ HTTP non-200 → error returned (Task 2 TestDownloadCoverArtHTTP404)
- ✅ Called after ROM write; error logged Warn, `dlDone` always set (Task 3)
- ✅ Uses `Client.http` — Chrome TLS transport already configured (method is on `*Client`)

**Placeholder scan:** No TBDs, all code blocks are complete.

**Type consistency:** `DownloadCoverArt(coverURL, romDestPath string) error` used consistently across all tasks.
