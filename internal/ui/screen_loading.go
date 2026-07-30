//go:build !headless

package ui

import (
	"fmt"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
)

// formatKB formats a byte count as a human-readable KB or MB string.
func formatKB(n int64) string {
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.2f MB", float64(n)/1024/1024)
}

// drawLoadingDots renders a 4-dot chase animation centred horizontally at y.
// One dot cycles through the accent colour every 250 ms; inactive dots use a
// dimmed version of the accent blended toward the background so they remain
// visible without competing with the active dot.
func drawLoadingDots(r *renderer.Renderer, y int32) {
	const (
		numDots = 4
		dotDiam = int32(10)
		dotGap  = int32(16)
	)
	totalW := int32(numDots)*dotDiam + int32(numDots-1)*dotGap
	startX := (r.W - totalW) / 2
	active := int(time.Now().UnixMilli()/250) % numDots

	ac := r.Theme.Accent
	bg := r.Theme.Background

	for i := 0; i < numDots; i++ {
		var dr, dg, db uint8
		if i == active {
			dr, dg, db = ac[0], ac[1], ac[2]
		} else {
			// A third of the way from the background toward the accent, for a
			// subtle unlit dot. Same result as the arithmetic it replaces, but
			// via a helper that cannot overflow.
			unlit := theme.Mix(bg, ac, 33)
			dr, dg, db = unlit[0], unlit[1], unlit[2]
		}
		dotX := startX + int32(i)*(dotDiam+dotGap)
		r.DrawPill(dotX, y, dotDiam, dotDiam, dr, dg, db)
	}
}
