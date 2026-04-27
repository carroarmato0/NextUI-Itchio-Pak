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

	if err := c.DownloadCoverArt(srv.URL+"/cover.png", romPath); err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	} else if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to mention 404, got: %v", err)
	}
}

// TestDownloadCoverArtSuccess verifies a JPEG source is saved as .png with correct name.
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

	if err := c.DownloadCoverArt(srv.URL+"/cover.png", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	artPath := filepath.Join(dir, ".media", "Wario Land II.png")
	fi, err := os.Stat(artPath)
	if os.IsNotExist(err) {
		t.Fatalf("expected art file at %s, not found", artPath)
	}
	if fi.Size() == 0 {
		t.Errorf("art file at %s is empty", artPath)
	}
}

// TestDownloadCoverArtGIFConvertedToPNG verifies that a GIF source is decoded
// and re-encoded as PNG (the only format NextUI displays for cover art).
func TestDownloadCoverArtGIFConvertedToPNG(t *testing.T) {
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

	// Must be saved as .png, not .gif
	artPath := filepath.Join(dir, ".media", "Opossum Country.png")
	fi, err := os.Stat(artPath)
	if os.IsNotExist(err) {
		t.Fatalf("expected .png at %s (gif should be converted), not found", artPath)
	}
	if fi.Size() == 0 {
		t.Errorf("art file at %s is empty", artPath)
	}
	if _, gifErr := os.Stat(filepath.Join(dir, ".media", "Opossum Country.gif")); !os.IsNotExist(gifErr) {
		t.Errorf(".gif file must not be saved; only .png should exist")
	}
}

// TestDownloadCoverArtPNGRoundTrip verifies PNG source is saved as PNG.
func TestDownloadCoverArtPNGRoundTrip(t *testing.T) {
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

	artPath := filepath.Join(dir, ".media", "game.png")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Fatalf("expected .png at %s (png should be converted), not found", artPath)
	}
}

// TestDownloadCoverArtFullStemPreserved verifies that bracket/paren tags in the
// ROM filename are kept in the cover art name. NextUI looks up art by the full
// ROM stem (including [v1.2] etc.), even though it strips them for display.
func TestDownloadCoverArtFullStemPreserved(t *testing.T) {
	imgBytes := minimalJPEG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(imgBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "Kero Kero Cowboy [v1.2].gbc")

	if err := c.DownloadCoverArt(srv.URL+"/cover.png", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	// Full stem must be preserved: "Kero Kero Cowboy [v1.2].png"
	artPath := filepath.Join(dir, ".media", "Kero Kero Cowboy [v1.2].png")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Fatalf("expected art at %q (full stem), not found", artPath)
	}
	// Stripped name must NOT exist
	if _, err := os.Stat(filepath.Join(dir, ".media", "Kero Kero Cowboy.png")); !os.IsNotExist(err) {
		t.Errorf("stripped filename should not exist; full stem must be preserved")
	}
}

// animatedGIFFirstFrameBlack returns a 2-frame animated GIF where frame 1 is
// entirely black and frame 2 composites a red pixel at (0,0). Calling
// image.Decode on this GIF yields only the first (black) frame; correct
// handling must composite all frames so pixel (0,0) is red in the output PNG.
func animatedGIFFirstFrameBlack() []byte {
	palette := color.Palette{color.Black, color.RGBA{R: 255, A: 255}}

	frame1 := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	frame1.SetColorIndex(0, 0, 0) // black
	frame1.SetColorIndex(1, 0, 0) // black

	frame2 := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	frame2.SetColorIndex(0, 0, 1) // red
	frame2.SetColorIndex(1, 0, 0) // black

	g := &gif.GIF{
		Image:    []*image.Paletted{frame1, frame2},
		Delay:    []int{0, 0},
		Disposal: []byte{gif.DisposalNone, gif.DisposalNone},
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, g)
	return buf.Bytes()
}

// TestDownloadCoverArtAnimatedGIFComposited verifies that when a cover art GIF
// has multiple frames the saved PNG reflects the composited image and is not
// just the (often black/blank) first frame.
func TestDownloadCoverArtAnimatedGIFComposited(t *testing.T) {
	gifBytes := animatedGIFFirstFrameBlack()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		w.Write(gifBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	romPath := filepath.Join(dir, "Moon Escape.gb")

	if err := c.DownloadCoverArt(srv.URL+"/cover.gif", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	artPath := filepath.Join(dir, ".media", "Moon Escape.png")
	f, err := os.Open(artPath)
	if err != nil {
		t.Fatalf("open saved PNG: %v", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode saved PNG: %v", err)
	}

	// Frame 2 places a red pixel at (0,0). If only the first (all-black) frame
	// was encoded, this pixel would be black.
	red, _, _, _ := img.At(0, 0).RGBA()
	if red < 0x8000 {
		t.Errorf("pixel (0,0) red channel = 0x%04x; expected >= 0x8000 — animated GIF compositing produced a black image (first-frame-only bug)", red)
	}
}

// TestDownloadCoverArtStaleFilesCleaned verifies that an old art file with the
// same stem but a different extension (e.g. a stale .gif) is removed when the
// new .png is saved.
func TestDownloadCoverArtStaleFilesCleaned(t *testing.T) {
	imgBytes := minimalJPEG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(imgBytes)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	dir := t.TempDir()
	mediaDir := filepath.Join(dir, ".media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Simulate a stale .gif left over from a previous download.
	staleGIF := filepath.Join(mediaDir, "Opossum Country.gif")
	if err := os.WriteFile(staleGIF, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	romPath := filepath.Join(dir, "Opossum Country.gbc")
	if err := c.DownloadCoverArt(srv.URL+"/cover.png", romPath); err != nil {
		t.Fatalf("DownloadCoverArt: %v", err)
	}

	// New .png must exist.
	if _, err := os.Stat(filepath.Join(mediaDir, "Opossum Country.png")); os.IsNotExist(err) {
		t.Fatalf("expected Opossum Country.png to exist after download")
	}
	// Stale .gif must be gone.
	if _, err := os.Stat(staleGIF); !os.IsNotExist(err) {
		t.Errorf("stale Opossum Country.gif should have been removed")
	}
}
