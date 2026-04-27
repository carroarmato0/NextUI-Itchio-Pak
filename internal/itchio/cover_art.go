package itchio

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// coverArtBasename returns the exact ROM filename stem (no extension).
// NextUI's cover art lookup matches on the full stem including tags like [v1.2],
// even though it strips those tags for the display name in the ROM browser.
func coverArtBasename(romDestPath string) string {
	return strings.TrimSuffix(filepath.Base(romDestPath), filepath.Ext(romDestPath))
}

// DownloadCoverArt fetches the cover image at coverURL and saves it as a JPEG
// into the .media/ subdirectory of the ROM's directory. The filename is the
// exact ROM stem (matching NextUI's art lookup convention) with a .jpg extension.
// GIF, PNG, and other formats are all re-encoded as JPEG. Any stale art files
// with the same stem but a different extension are removed. Returns nil for an
// empty coverURL.
func (c *Client) DownloadCoverArt(coverURL, romDestPath string) error {
	if coverURL == "" {
		logger.Debug("cover-art: no cover URL, skipping")
		return nil
	}

	dir := filepath.Dir(romDestPath)
	mediaDir := filepath.Join(dir, ".media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		return fmt.Errorf("cover-art: mkdir: %w", err)
	}

	base := coverArtBasename(romDestPath)
	artPath := filepath.Join(mediaDir, base+".jpg")

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

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return fmt.Errorf("cover-art: read body: %w", err)
	}

	img, format, err := image.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("cover-art: decode image (%s): %w", coverURL, err)
	}
	logger.Debug("cover-art: decoded %s as %s", filepath.Base(artPath), format)

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

	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: 90}); err != nil {
		logger.Error("cover-art: encode jpeg: %v", err)
		return fmt.Errorf("cover-art: encode jpeg: %w", err)
	}
	if err := tmp.Close(); err != nil {
		logger.Error("cover-art: close temp %s: %v", tmpPath, err)
		return fmt.Errorf("cover-art: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, artPath); err != nil {
		logger.Error("cover-art: rename to %s: %v", artPath, err)
		return fmt.Errorf("cover-art: rename: %w", err)
	}
	logger.Info("cover-art: saved → %s", artPath)

	// Remove stale art files with the same stem but a different extension
	// (e.g. an old .gif or .png left over from a previous download).
	artBase := filepath.Base(artPath)
	if entries, err := os.ReadDir(mediaDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.TrimSuffix(name, filepath.Ext(name)) == base && name != artBase {
				stale := filepath.Join(mediaDir, name)
				if removeErr := os.Remove(stale); removeErr == nil {
					logger.Debug("cover-art: removed stale %s", name)
				}
			}
		}
	}
	return nil
}
