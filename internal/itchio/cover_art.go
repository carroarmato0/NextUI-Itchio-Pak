package itchio

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// compositeGIFFrames renders an animated GIF using the standard GIF compositing
// algorithm (disposal methods, frame offsets, background colour) and returns the
// single rendered frame that has the highest total brightness. That frame is
// typically the most visually complete, making it the best static thumbnail.
func compositeGIFFrames(g *gif.GIF) image.Image {
	w, h := g.Config.Width, g.Config.Height
	if w == 0 || h == 0 {
		b := g.Image[0].Bounds()
		w, h = b.Dx(), b.Dy()
	}
	bounds := image.Rect(0, 0, w, h)

	bgColor := color.Color(color.RGBA{A: 255}) // opaque black default
	if pal, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(pal) {
		bgColor = pal[g.BackgroundIndex]
	}
	bgFill := image.NewUniform(bgColor)

	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, bgFill, image.Point{}, draw.Src)

	var (
		bestCanvas *image.RGBA
		bestSum    uint64
	)

	for i, frame := range g.Image {
		disposal := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}

		// Save canvas before drawing this frame so DisposalPrevious can restore it.
		var preCanvas *image.RGBA
		if disposal == gif.DisposalPrevious {
			preCanvas = image.NewRGBA(bounds)
			draw.Draw(preCanvas, bounds, canvas, image.Point{}, draw.Src)
		}

		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		// Track the brightest rendered frame for use as the static thumbnail.
		if sum := gifFrameBrightness(canvas); bestCanvas == nil || sum > bestSum {
			bestSum = sum
			bestCanvas = image.NewRGBA(bounds)
			draw.Draw(bestCanvas, bounds, canvas, image.Point{}, draw.Src)
		}

		// Apply disposal to prepare the canvas for the next frame.
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), bgFill, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			if preCanvas != nil {
				draw.Draw(canvas, frame.Bounds(), preCanvas, frame.Bounds().Min, draw.Src)
			}
		}
	}

	if bestCanvas != nil {
		return bestCanvas
	}
	return canvas
}

// gifFrameBrightness sums all RGB channel values for the canvas, used to pick
// the most visually complete frame from an animated GIF.
func gifFrameBrightness(img *image.RGBA) uint64 {
	var sum uint64
	for i := 0; i < len(img.Pix); i += 4 {
		sum += uint64(img.Pix[i]) + uint64(img.Pix[i+1]) + uint64(img.Pix[i+2])
	}
	return sum
}

// coverArtBasename returns the exact ROM filename stem (no extension).
// NextUI's cover art lookup matches on the full stem including tags like [v1.2],
// even though it strips those tags for the display name in the ROM browser.
func coverArtBasename(romDestPath string) string {
	return strings.TrimSuffix(filepath.Base(romDestPath), filepath.Ext(romDestPath))
}

// DownloadCoverArt fetches the cover image at coverURL and saves it as a PNG
// into the .media/ subdirectory of the ROM's directory. The filename is the
// exact ROM stem (matching NextUI's art lookup convention) with a .png extension.
// GIF, JPEG, and other formats are all re-encoded as PNG. Any stale art files
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
	artPath := filepath.Join(mediaDir, base+".png")

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

	// image.Decode returns only the first frame of an animated GIF, which is
	// often blank/black. Re-decode with gif.DecodeAll and composite all frames
	// so the saved PNG reflects the complete image.
	if format == "gif" {
		if g, err2 := gif.DecodeAll(bytes.NewReader(buf.Bytes())); err2 == nil && len(g.Image) > 1 {
			img = compositeGIFFrames(g)
		}
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

	if err := png.Encode(tmp, img); err != nil {
		logger.Error("cover-art: encode png: %v", err)
		return fmt.Errorf("cover-art: encode png: %w", err)
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
