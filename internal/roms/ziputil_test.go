package roms_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func makeTestZip(t *testing.T, innerName string) string {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "rom.zip")
	w, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(w)
	f, err := zw.Create(innerName)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("rom data"))
	zw.Close()
	w.Close()
	return zipPath
}

func TestZipInnerFilename_GB(t *testing.T) {
	zipPath := makeTestZip(t, "Pokemon - Red Version (USA, Europe).gb")
	got := roms.ZipInnerFilename(zipPath)
	if got != "Pokemon - Red Version (USA, Europe).gb" {
		t.Errorf("got %q", got)
	}
}

func TestZipInnerFilename_GBC(t *testing.T) {
	zipPath := makeTestZip(t, "Solastra.gbc")
	got := roms.ZipInnerFilename(zipPath)
	if got != "Solastra.gbc" {
		t.Errorf("got %q", got)
	}
}

func TestZipInnerFilename_NoROMInside(t *testing.T) {
	zipPath := makeTestZip(t, "readme.txt")
	got := roms.ZipInnerFilename(zipPath)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestZipInnerFilename_MissingFile(t *testing.T) {
	got := roms.ZipInnerFilename("/nonexistent/path.zip")
	if got != "" {
		t.Errorf("expected empty for missing file, got %q", got)
	}
}
