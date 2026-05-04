package renderer

import (
	"image"
	"image/color"
	stdraw "image/draw"
	"image/gif"
	"time"

	"golang.org/x/image/draw"
)

const (
	maxGIFFrames        = 64
	gifZeroDelayDefault = 100 * time.Millisecond
)

// gifAnim holds the pre-rendered RGBA frames for one animated GIF.
type gifAnim struct {
	frames     [][]uint8       // RGBA pixel data per frame; freed after GPU upload
	delays     []time.Duration // per-frame display duration
	w, h       int32
	pitch      int        // w * 4
	cur        int        // current frame index
	nextAt     time.Time  // wall-clock time to advance to cur+1
	frameCount int        // total stored frames; stays valid after frames is freed
}

// advance moves to the next frame if now is strictly after nextAt.
// Returns the current (possibly updated) frame index and whether the frame changed.
func (a *gifAnim) advance(now time.Time) (int, bool) {
	if !now.After(a.nextAt) {
		return a.cur, false
	}
	a.cur = (a.cur + 1) % a.frameCount
	a.nextAt = now.Add(a.delays[a.cur])
	return a.cur, true
}

// renderGIFFrames renders every frame of g using the standard GIF compositing
// algorithm (disposal methods, frame offsets, background colour) and returns a
// gifAnim with up to maxGIFFrames RGBA pixel slices. When the GIF has more
// frames than the cap, frames are sampled evenly across the full animation so
// the complete duration is preserved rather than truncating at frame N. The
// delay of each stored frame is the sum of the delays of all source frames it
// covers. All frames are scaled to at most 640 px wide.
const maxGIFSourcePixels = 1280 * 1280 // ~6 MB canvas; skip larger GIFs to avoid OOM

func renderGIFFrames(g *gif.GIF) *gifAnim {
	if len(g.Image) == 0 {
		return &gifAnim{}
	}
	total := len(g.Image)
	srcW, srcH := g.Config.Width, g.Config.Height
	if srcW == 0 || srcH == 0 {
		b := g.Image[0].Bounds()
		srcW, srcH = b.Dx(), b.Dy()
	}
	if srcW*srcH > maxGIFSourcePixels {
		return &gifAnim{} // too large to render safely on constrained hardware
	}
	srcBounds := image.Rect(0, 0, srcW, srcH)

	dstW, dstH := srcW, srcH
	if srcW > 640 {
		dstH = srcH * 640 / srcW
		dstW = 640
	}
	dstBounds := image.Rect(0, 0, dstW, dstH)

	bgColor := color.Color(color.RGBA{A: 255}) // opaque black default
	if pal, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(pal) {
		bgColor = pal[g.BackgroundIndex]
	}
	bgFill := image.NewUniform(bgColor)

	canvas := image.NewRGBA(srcBounds)
	stdraw.Draw(canvas, srcBounds, bgFill, image.Point{}, stdraw.Src)

	stored := total
	if stored > maxGIFFrames {
		stored = maxGIFFrames
	}

	// Pre-compute which source frame index maps to each stored slot, and the
	// accumulated delay for that slot (sum of all source-frame delays it covers).
	sampleSrc := make([]int, stored)
	delays := make([]time.Duration, stored)
	for k := 0; k < stored; k++ {
		sampleSrc[k] = k * total / stored
	}
	sampleSet := make(map[int]int, stored) // source idx → stored slot
	for k, idx := range sampleSrc {
		sampleSet[idx] = k
	}
	for k := 0; k < stored; k++ {
		start := sampleSrc[k]
		end := total
		if k+1 < stored {
			end = sampleSrc[k+1]
		}
		acc := 0
		for j := start; j < end && j < len(g.Delay); j++ {
			acc += g.Delay[j]
		}
		if acc == 0 {
			delays[k] = gifZeroDelayDefault
		} else {
			delays[k] = time.Duration(acc) * 10 * time.Millisecond
		}
	}

	frames := make([][]uint8, stored)

	// Allocate the scaled-output image once and reuse across all stored frames.
	// draw.Src overwrites every pixel so no zeroing is needed between uses.
	dst := image.NewRGBA(dstBounds)

	// preCanvas is used only for DisposalPrevious frames; allocate lazily.
	var preCanvas *image.RGBA

	for i := 0; i < total; i++ {
		frame := g.Image[i]
		disposal := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}

		if disposal == gif.DisposalPrevious {
			if preCanvas == nil {
				preCanvas = image.NewRGBA(srcBounds)
			}
			stdraw.Draw(preCanvas, srcBounds, canvas, image.Point{}, stdraw.Src)
		}

		// Composite the paletted frame onto the RGBA canvas.
		// Using a direct palette-expanded path avoids the slow RGBA64At generic
		// fallback that image/draw uses for Paletted sources with the Over operator.
		compositePalettedOver(canvas, frame)

		if slot, ok := sampleSet[i]; ok {
			if srcW == dstW {
				stdraw.Draw(dst, dstBounds, canvas, image.Point{}, stdraw.Src)
			} else {
				draw.NearestNeighbor.Scale(dst, dstBounds, canvas, srcBounds, draw.Src, nil)
			}
			pix := make([]uint8, len(dst.Pix))
			copy(pix, dst.Pix)
			frames[slot] = pix
		}

		switch disposal {
		case gif.DisposalBackground:
			stdraw.Draw(canvas, frame.Bounds(), bgFill, image.Point{}, stdraw.Src)
		case gif.DisposalPrevious:
			if preCanvas != nil {
				stdraw.Draw(canvas, frame.Bounds(), preCanvas, frame.Bounds().Min, stdraw.Src)
			}
		}
	}

	return &gifAnim{
		frames:     frames,
		delays:     delays,
		w:          int32(dstW),
		h:          int32(dstH),
		pitch:      dstW * 4,
		frameCount: stored,
	}
}

// compositePalettedOver composites a paletted GIF frame onto an RGBA canvas
// using the Porter-Duff Over operator. It avoids the generic image/draw fallback
// (which calls RGBA64At per pixel) by expanding the palette once and writing
// directly into the canvas pixel buffer.
func compositePalettedOver(canvas *image.RGBA, src *image.Paletted) {
	srcB := src.Bounds()

	// Expand palette to RGBA once.
	pal := make([]color.RGBA, len(src.Palette))
	for i, c := range src.Palette {
		r16, g16, b16, a16 := c.RGBA()
		pal[i] = color.RGBA{uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8), uint8(a16 >> 8)}
	}

	canvasB := canvas.Bounds()
	for y := srcB.Min.Y; y < srcB.Max.Y; y++ {
		srcRow := src.Pix[(y-srcB.Min.Y)*src.Stride:]
		dstBase := (y-canvasB.Min.Y)*canvas.Stride + (srcB.Min.X-canvasB.Min.X)*4
		for x := 0; x < srcB.Dx(); x++ {
			c := pal[srcRow[x]]
			dp := dstBase + x*4
			switch c.A {
			case 255:
				// Fully opaque: direct write, no blend.
				canvas.Pix[dp] = c.R
				canvas.Pix[dp+1] = c.G
				canvas.Pix[dp+2] = c.B
				canvas.Pix[dp+3] = 255
			case 0:
				// Fully transparent: canvas unchanged.
			default:
				// Partial alpha: Porter-Duff Over.
				srcA := uint32(c.A)
				invA := 255 - srcA
				canvas.Pix[dp] = uint8((uint32(c.R)*srcA + uint32(canvas.Pix[dp])*invA) / 255)
				canvas.Pix[dp+1] = uint8((uint32(c.G)*srcA + uint32(canvas.Pix[dp+1])*invA) / 255)
				canvas.Pix[dp+2] = uint8((uint32(c.B)*srcA + uint32(canvas.Pix[dp+2])*invA) / 255)
				canvas.Pix[dp+3] = uint8((srcA*255 + uint32(canvas.Pix[dp+3])*invA) / 255)
			}
		}
	}
}
