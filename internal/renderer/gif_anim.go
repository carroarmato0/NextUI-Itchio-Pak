package renderer

import (
	"image"
	"image/color"
	stdraw "image/draw"
	"image/gif"
	"time"

	"golang.org/x/image/draw"
)

const maxGIFFrames = 64

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
// gifAnim with one RGBA pixel slice per frame. Frames beyond maxGIFFrames are
// silently dropped. All frames are scaled to at most 640 px wide (same factor
// for every frame so dimensions are consistent).
func renderGIFFrames(g *gif.GIF) *gifAnim {
	srcW, srcH := g.Config.Width, g.Config.Height
	if srcW == 0 || srcH == 0 {
		b := g.Image[0].Bounds()
		srcW, srcH = b.Dx(), b.Dy()
	}
	srcBounds := image.Rect(0, 0, srcW, srcH)

	// Compute destination size (applied identically to every frame).
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

	limit := len(g.Image)
	if limit > maxGIFFrames {
		limit = maxGIFFrames
	}

	frames := make([][]uint8, limit)
	delays := make([]time.Duration, limit)

	for i := 0; i < limit; i++ {
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

		// Scale and capture pixel data for this frame.
		dst := image.NewRGBA(dstBounds)
		if srcW == dstW {
			stdraw.Draw(dst, dstBounds, canvas, image.Point{}, stdraw.Src)
		} else {
			draw.BiLinear.Scale(dst, dstBounds, canvas, srcBounds, draw.Over, nil)
		}
		pix := make([]uint8, len(dst.Pix))
		copy(pix, dst.Pix)
		frames[i] = pix

		d := 0
		if i < len(g.Delay) {
			d = g.Delay[i]
		}
		if d == 0 {
			delays[i] = 100 * time.Millisecond
		} else {
			delays[i] = time.Duration(d) * 10 * time.Millisecond
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
