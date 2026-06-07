//go:build !headless

package ui

import (
	"fmt"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
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
			// Blend accent 1/3 toward background for a subtle unlit dot.
			dr = uint8((int(ac[0]) + int(bg[0])*2) / 3)
			dg = uint8((int(ac[1]) + int(bg[1])*2) / 3)
			db = uint8((int(ac[2]) + int(bg[2])*2) / 3)
		}
		dotX := startX + int32(i)*(dotDiam+dotGap)
		r.DrawPill(dotX, y, dotDiam, dotDiam, dr, dg, db)
	}
}
