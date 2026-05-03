package roms

import (
	"archive/zip"
	"path/filepath"
	"strings"
)

// ZipInnerFilename returns the filename of the first recognized ROM file inside
// a zip archive. Returns "" if the zip cannot be opened or contains no ROM.
func ZipInnerFilename(zipPath string) string {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return ""
	}
	defer r.Close()
	for _, f := range r.File {
		switch strings.ToLower(filepath.Ext(f.Name)) {
		case ".gb", ".gbc", ".gba":
			return f.Name
		}
	}
	return ""
}
