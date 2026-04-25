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
		logger.Error("cover-art: parse URL: %v", err)
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

	logger.Info("cover-art: downloading for %s", filepath.Base(romDestPath))

	resp, err := c.http.Get(coverURL)
	if err != nil {
		logger.Error("cover-art: fetch: %v", err)
		return fmt.Errorf("cover-art: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("cover-art: HTTP %d", resp.StatusCode)
		return fmt.Errorf("cover-art: HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(mediaDir, ".art-*.tmp")
	if err != nil {
		logger.Error("cover-art: create temp: %v", err)
		return fmt.Errorf("cover-art: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath) // no-op after successful rename
	}()

	n, err := io.Copy(tmp, resp.Body)
	if err != nil {
		logger.Error("cover-art: write %s: %v", artPath, err)
		return fmt.Errorf("cover-art: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		logger.Error("cover-art: close temp %s: %v", tmpPath, err)
		return fmt.Errorf("cover-art: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, artPath); err != nil {
		logger.Error("cover-art: rename to %s: %v", artPath, err)
		return fmt.Errorf("cover-art: rename: %w", err)
	}
	logger.Info("cover-art: saved %d bytes → %s", n, artPath)
	return nil
}
