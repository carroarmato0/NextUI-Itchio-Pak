package roms

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// InspectRemote7z downloads the 7z archive at cdnURL to a temporary file,
// reads its directory, and returns a classified ZIPManifest.
// Unlike ZIP, 7z cannot be inspected via HTTP Range requests, so a full
// download is always required.
func InspectRemote7z(client *http.Client, cdnURL string) (ZIPManifest, error) {
	tmp, err := os.CreateTemp("", "itchio-inspect-*.7z")
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	logger.Debug("7z-inspect: downloading to temp=%s", tmpPath)

	resp, err := client.Get(cdnURL)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("download 7z: %w", err)
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

	r, err := sevenzip.OpenReader(tmpPath)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("sevenzip.OpenReader: %w", err)
	}
	defer r.Close()

	logger.Debug("7z-inspect: read %d entries", len(r.File))
	return manifestFrom7zReader(r), nil
}

func manifestFrom7zReader(r *sevenzip.ReadCloser) ZIPManifest {
	var m ZIPManifest
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Normalise path separators (Windows 7z archives may use backslashes).
		fullPath := strings.ReplaceAll(f.Name, "\\", "/")
		if IsInMacOSMetaDir(fullPath) {
			continue
		}
		name := filepath.Base(fullPath)
		if strings.HasPrefix(name, "._") {
			continue // macOS resource-fork stub outside __MACOSX/
		}
		kind := ClassifyEntry(name)

		// Magic-byte detection for entries without a recognised extension.
		// Skip for known image extensions — a .png is always artwork.
		if kind == KindOther && !IsImageExt(strings.ToLower(filepath.Ext(name))) {
			if detected := classify7zByMagic(f); detected != "" {
				stem := strings.TrimSuffix(name, filepath.Ext(name))
				name = stem + detected
				kind = KindROM
			}
		}

		m.Entries = append(m.Entries, ZIPEntry{
			Name: name,
			Kind: kind,
			Size: f.FileHeader.UncompressedSize,
		})
	}
	return m
}

// classify7zByMagic reads the first DetectBufSize bytes of a 7z entry and
// returns the detected playable ROM extension, or "" if unrecognised.
func classify7zByMagic(f *sevenzip.File) string {
	rc, err := f.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()
	buf := make([]byte, DetectBufSize)
	n, _ := io.ReadFull(rc, buf)
	return DetectPlayableROMExt(buf[:n])
}
