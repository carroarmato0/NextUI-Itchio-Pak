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
