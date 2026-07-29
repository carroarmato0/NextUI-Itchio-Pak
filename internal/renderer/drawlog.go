//go:build !headless

package renderer

// Draw logging: record what text was drawn, in which colour, over which
// background — so a contrast audit can judge the frame that was actually
// produced rather than the theme accessors in isolation.
//
// Auditing the palette alone is not enough. A colour pair can satisfy every
// theme-level check and still be unreadable on screen because something was
// drawn behind it: a pill, a cover image, a status tint. The only reliable
// background is the one on the target immediately before the glyphs land, which
// is what this samples.
//
// Off unless BeginDrawLog is called, and only reachable from the offscreen
// harness. The sampling costs a 1×1 ReadPixels per string, which is far too slow
// for the device render loop and never runs there.

// DrawLogEntry is one recorded text draw.
type DrawLogEntry struct {
	Text string
	X, Y int32
	// FG is the colour the text was drawn in.
	FG [3]uint8
	// BG is the target pixel under the text's bounding box, sampled before the
	// glyphs were drawn.
	BG [3]uint8
	// Small is true for hint/footer text.
	Small bool
}

// BeginDrawLog starts recording text draws, discarding anything recorded before.
func (r *Renderer) BeginDrawLog() {
	r.drawLog = []DrawLogEntry{}
	r.drawLogOn = true
}

// EndDrawLog stops recording.
func (r *Renderer) EndDrawLog() { r.drawLogOn = false }

// DrawLog returns the entries recorded so far.
func (r *Renderer) DrawLog() []DrawLogEntry { return r.drawLog }

// logTextDraw records one string and the background behind it. Called from
// drawRuns before any glyph is copied to the target, which is what makes the
// sampled pixel the background rather than the text itself.
func (r *Renderer) logTextDraw(text string, x, y int32, fg [3]uint8, small bool) {
	if !r.drawLogOn || text == "" {
		return
	}
	w, h := r.textSizeImpl(text, small, false)
	if w <= 0 || h <= 0 {
		return
	}
	// Sample the middle of the run: the left edge often sits on a border or the
	// rounded end of a pill, which is not the colour the text has to compete with.
	sx, sy := x+w/2, y+h/2
	if sx < 0 || sy < 0 || sx >= r.W || sy >= r.H {
		return
	}
	// Clipped-away text never reaches the screen. Auditing it reports colours
	// nobody can see — the tag rows that overflow a detail page, for one.
	if clip := r.Renderer.GetClipRect(); clip.W > 0 && clip.H > 0 {
		if sx < clip.X || sy < clip.Y || sx >= clip.X+clip.W || sy >= clip.Y+clip.H {
			return
		}
	}
	img, err := r.pixelsRect(sx, sy, 1, 1)
	if err != nil {
		return
	}
	r.drawLog = append(r.drawLog, DrawLogEntry{
		Text: text, X: x, Y: y, FG: fg, Small: small,
		BG: [3]uint8{img.Pix[0], img.Pix[1], img.Pix[2]},
	})
}
