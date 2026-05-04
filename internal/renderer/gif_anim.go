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
	frames [][]uint8       // RGBA pixel data per frame, capped at maxGIFFrames
	delays []time.Duration // per-frame display duration
	w, h   int32
	pitch  int       // w * 4
	cur    int       // current frame index
	nextAt time.Time // wall-clock time to advance to cur+1
}

// advance moves to the next frame if now is strictly after nextAt.
// Returns the current (possibly updated) frame index and whether the frame changed.
func (a *gifAnim) advance(now time.Time) (int, bool) {
	if !now.After(a.nextAt) {
		return a.cur, false
	}
	a.cur = (a.cur + 1) % len(a.frames)
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
	sampleSrc := make([]int, stored)    // source frame index to snapshot
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

	for i := 0; i < total; i++ {
		frame := g.Image[i]
		disposal := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}

		var preCanvas *image.RGBA
		if disposal == gif.DisposalPrevious {
			preCanvas = image.NewRGBA(srcBounds)
			stdraw.Draw(preCanvas, srcBounds, canvas, image.Point{}, stdraw.Src)
		}

		stdraw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, stdraw.Over)

		if slot, ok := sampleSet[i]; ok {
			dst := image.NewRGBA(dstBounds)
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
		frames: frames,
		delays: delays,
		w:      int32(dstW),
		h:      int32(dstH),
		pitch:  dstW * 4,
	}
}
