package ui

// narrowScreenW is the display width of the Miyoo Flip (my355). Footer hints
// are abbreviated at or below this width to prevent overflow.
const narrowScreenW = int32(640)

// Layout holds screen-size-dependent spacing constants derived at draw time.
// Use LayoutFor(r.W, r.H) to obtain the appropriate layout for the current screen.
type Layout struct {
	HeaderPad      int32   // vertical padding inside the header bar
	RowPad         int32   // vertical padding inside each list row
	FooterPad      int32   // vertical padding inside the footer bar
	ContentGap     int32   // gap between header and content area
	CoverMaxW      float32 // cover art width as fraction of the right panel width (0–1)
	OverlayMarginX int32   // horizontal margin for centered overlay panels (pixels each side)
}

// LayoutFor returns the layout constants appropriate for a screen of size w×h.
// Two size classes: small (w ≤ narrowScreenW) and wide (w > narrowScreenW).
func LayoutFor(w, h int32) Layout {
	if w <= narrowScreenW {
		return Layout{
			HeaderPad:      3,
			RowPad:         2,
			FooterPad:      2,
			ContentGap:     3,
			CoverMaxW:      1.0,
			OverlayMarginX: w * 3 / 100,
		}
	}
	return Layout{
		HeaderPad:      6,
		RowPad:         4,
		FooterPad:      5,
		ContentGap:     6,
		CoverMaxW:      0.75,
		OverlayMarginX: w * 14 / 100,
	}
}
