//go:build !headless

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/carroarmato0/nextui-itchio-pak/internal/ui"
)

// result is one rendered scene.
type result struct {
	img      image.Image
	note     string
	findings []finding
}

// render draws one scene under one palette and returns the image.
//
// A fresh Renderer per render is deliberate. SDL caches textures per renderer,
// and the pill cache is keyed on colour, so reusing one across palettes would
// leak the previous palette's pills into the next render.
func render(sc ui.Scene, p palChoice, w, h int, full, audit bool, settle time.Duration) (result, error) {
	dir, err := os.MkdirTemp("", "devshot-*")
	if err != nil {
		return result{}, err
	}
	defer os.RemoveAll(dir)

	deps, err := ui.DevSetup(filepath.Join(dir, "state"), p.th)
	if err != nil {
		return result{}, err
	}

	r, err := renderer.NewOffscreen(w, h, p.th)
	if err != nil {
		return result{}, err
	}
	defer r.Close()

	if audit {
		r.BeginDrawLog()
	}

	screen := sc.Build(deps)

	// Overlay screens (the filter panel, the pickers) draw a panel over whatever
	// was already on screen and never clear the full target themselves. On device
	// that is the previous frame; here it would be uninitialised memory, which
	// shows up in an audit as text over random colours. Start from the theme
	// background so every scene begins from a defined state.
	bg := p.th.Background
	r.Clear(bg[0], bg[1], bg[2])

	// One draw always happens: it produces the image, and for scrollable screens
	// it is also what populates the content/viewport extents.
	screen.Draw(r)

	// Cover art is fetched in the background, so a single draw catches only the
	// "Loading…" placeholder. Pump the image cache and redraw until the textures
	// land. Off by default: the audit does not care about artwork and this would
	// add seconds per render across the matrix.
	if settle > 0 {
		deadline := time.Now().Add(settle)
		for time.Now().Before(deadline) {
			time.Sleep(80 * time.Millisecond)
			deps.Cache.ProcessPending(r)
			screen.Draw(r)
		}
	}

	res := result{note: ""}
	if full {
		img, note, err := captureFull(r, screen, w, h)
		if err != nil {
			return result{}, err
		}
		res.img, res.note = img, note
	} else {
		img, err := r.Pixels()
		if err != nil {
			return result{}, err
		}
		res.img = img
	}

	if audit {
		res.findings = auditDrawLog(sc.Name, p.label, r.DrawLog())
	}
	return res, nil
}

// captureFull renders a scrollable screen at successive offsets and stitches the
// results into one tall image, with a marker at each fold.
//
// The fold lines are the point of it: they show exactly what a user has to
// scroll to reach, which a single 1024×768 frame cannot.
func captureFull(r *renderer.Renderer, screen ui.Screen, w, h int) (image.Image, string, error) {
	sc, ok := screen.(ui.DevScrollable)
	if !ok {
		img, err := r.Pixels()
		return img, "(not scrollable)", err
	}
	content, viewport := sc.DevScrollExtent()
	if content <= viewport || viewport <= 0 {
		img, err := r.Pixels()
		return img, "(fits on one screen)", err
	}

	// Number of screenfuls needed to cover the content.
	pages := int((content + viewport - 1) / viewport)
	out := image.NewRGBA(image.Rect(0, 0, w, pages*h))

	for i := 0; i < pages; i++ {
		sc.DevSetScroll(int32(i) * viewport)
		screen.Draw(r)
		frame, err := r.Pixels()
		if err != nil {
			return nil, "", err
		}
		draw.Draw(out, image.Rect(0, i*h, w, (i+1)*h), frame, image.Point{}, draw.Src)
		if i > 0 {
			drawFold(out, i*h, w)
		}
	}
	return out, fmt.Sprintf("(%d pages, content %dpx)", pages, content), nil
}

// drawFold marks a viewport boundary with a dashed line.
func drawFold(img *image.RGBA, y, w int) {
	c := color.RGBA{R: 255, G: 0, B: 128, A: 255}
	for x := 0; x < w; x++ {
		if (x/8)%2 == 0 {
			img.Set(x, y, c)
			img.Set(x, y+1, c)
		}
	}
}

// finding is one audited text draw that failed the contrast threshold.
type finding struct {
	scene, palette string
	text           string
	fg, bg         [3]uint8
	contrast       int
	fatal          bool
}

// minAuditContrast matches the threshold the theme package uses for its own
// accessors, so the rendered result is held to the same standard as the palette.
const minAuditContrast = 60

// auditDrawLog turns recorded text draws into findings.
//
// This audits what was actually drawn, with the real background sampled from the
// target underneath each string — not the theme accessors in isolation. A colour
// pair can pass in theory and still fail on screen because something else was
// drawn behind it.
func auditDrawLog(scene, palette string, entries []renderer.DrawLogEntry) []finding {
	var out []finding
	seen := map[string]bool{}
	for _, e := range entries {
		c := theme.Contrast(e.FG, e.BG)
		if c >= minAuditContrast {
			continue
		}
		// One finding per distinct colour pair; the same pair repeated across
		// rows is one problem, not twenty.
		key := fmt.Sprintf("%v|%v", e.FG, e.BG)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, finding{
			scene: scene, palette: palette, text: e.Text,
			fg: e.FG, bg: e.BG, contrast: c,
			fatal: c < minAuditContrast/2, // barely-visible vs merely-weak
		})
	}
	return out
}

func countFailures(fs []finding) int {
	n := 0
	for _, f := range fs {
		if f.fatal {
			n++
		}
	}
	return n
}

func reportAudit(fs []finding) {
	fmt.Printf("\n==> contrast audit (threshold %d)\n", minAuditContrast)
	if len(fs) == 0 {
		fmt.Println("    no low-contrast text found")
		return
	}
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].contrast != fs[j].contrast {
			return fs[i].contrast < fs[j].contrast
		}
		return fs[i].scene < fs[j].scene
	})
	for _, f := range fs {
		level := "WARN"
		if f.fatal {
			level = "FAIL"
		}
		fmt.Printf("    %s %-20s %-22s contrast %3d  #%02X%02X%02X on #%02X%02X%02X  %q\n",
			level, f.scene, f.palette, f.contrast,
			f.fg[0], f.fg[1], f.fg[2], f.bg[0], f.bg[1], f.bg[2], trunc(f.text, 28))
	}
	fmt.Printf("    %d finding(s), %d fatal\n", len(fs), countFailures(fs))
}

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n-1]) + "…"
}
