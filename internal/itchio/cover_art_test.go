package itchio_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// minimalJPEG encodes a 1×1 white pixel as a JPEG.
func minimalJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

// minimalPNG encodes a 1×1 white pixel as a PNG.
func minimalPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// minimalGIF encodes a 1×1 white pixel as a GIF.
func minimalGIF() []byte {
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), []color.Color{color.White})
	var buf bytes.Buffer
	_ = gif.Encode(&buf, img, nil)
	return buf.Bytes()
}

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
	} else if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to mention 404, got: %v", err)
	}
}

// TestDownloadCoverArtSuccess verifies a JPEG is saved as .jpg with correct name.
func TestDownloadCoverArtSuccess(t *testing.T) {
	imgBytes := minimalJPEG()
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
	fi, err := os.Stat(artPath)
	if os.IsNotExist(err) {
		t.Fatalf("expected art file at %s, not found", artPath)
	}
	if fi.Size() == 0 {
		t.Errorf("art file at %s is empty", artPath)
	}
}

// TestDownloadCoverArtGIFConvertedToJPEG verifies that a GIF source is decoded
// and re-encoded as JPEG (so NextUI can display it).
func TestDownloadCoverArtGIFConvertedToJPEG(t *testing.T) {
	gifBytes := minimalGIF()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		w.Write(gifBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "Opossum Country.gbc")

	if err := c.DownloadCoverArt(srv.URL+"/cover.gif", romPath); err != nil {
		t.Fatalf("DownloadCoverArt (gif): %v", err)
	}

	// Must be saved as .jpg, not .gif
	artPath := filepath.Join(dir, ".media", "Opossum Country.jpg")
	fi, err := os.Stat(artPath)
	if os.IsNotExist(err) {
		t.Fatalf("expected .jpg at %s (gif should be converted), not found", artPath)
	}
	if fi.Size() == 0 {
		t.Errorf("art file at %s is empty", artPath)
	}
	if _, gifErr := os.Stat(filepath.Join(dir, ".media", "Opossum Country.gif")); !os.IsNotExist(gifErr) {
		t.Errorf(".gif file must not be saved; only .jpg should exist")
	}
}

// TestDownloadCoverArtPNGConvertedToJPEG verifies PNG is also re-encoded as JPEG.
func TestDownloadCoverArtPNGConvertedToJPEG(t *testing.T) {
	pngBytes := minimalPNG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gbc")

	if err := c.DownloadCoverArt(srv.URL+"/cover.png", romPath); err != nil {
		t.Fatalf("DownloadCoverArt (png): %v", err)
	}

	artPath := filepath.Join(dir, ".media", "game.jpg")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Fatalf("expected .jpg at %s (png should be converted), not found", artPath)
	}
}

// TestDownloadCoverArtBracketTagsStripped verifies that [...] and (...) tags
// are stripped from the ROM filename when deriving the cover art name, so it
// matches NextUI's display-name convention.
func TestDownloadCoverArtBracketTagsStripped(t *testing.T) {
	imgBytes := minimalJPEG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(imgBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "Kero Kero Cowboy [v1.2].gbc")

	if err := c.DownloadCoverArt(srv.URL+"/cover.jpg", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	// Must strip "[v1.2]" → "Kero Kero Cowboy.jpg"
	artPath := filepath.Join(dir, ".media", "Kero Kero Cowboy.jpg")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Fatalf("expected art at %q (brackets stripped), not found", artPath)
	}
	// The bracketed name must NOT exist
	if _, err := os.Stat(filepath.Join(dir, ".media", "Kero Kero Cowboy [v1.2].jpg")); !os.IsNotExist(err) {
		t.Errorf("bracketed filename should not exist; brackets must be stripped")
	}
}

// TestDownloadCoverArtParenTagsStripped verifies (Region) paren tags are stripped.
func TestDownloadCoverArtParenTagsStripped(t *testing.T) {
	imgBytes := minimalJPEG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(imgBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "Some Game (USA).gb")

	if err := c.DownloadCoverArt(srv.URL+"/cover.jpg", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	artPath := filepath.Join(dir, ".media", "Some Game.jpg")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Fatalf("expected art at %q (parens stripped), not found", artPath)
	}
}
