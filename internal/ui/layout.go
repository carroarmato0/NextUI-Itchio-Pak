package ui

// A panel is compact unless it is roomy in both directions.
//
// Width alone used to be enough: every device was either 640 wide or ≥1024.
// The H700 family broke that — it ships 720×480 (RG34XX, RG34XX SP, RG SP),
// which is as cramped vertically as a Miyoo Flip, and 720×720 (RG Cube XX),
// which is not. The conjunction also keeps RG28XX safe: it normally presents
// 640×480 through SDL_ROTATION, but if rotation does not reach our window we
// see 480×640, and a height-only test would call that roomy.
const (
	compactMaxW = int32(640)
	compactMaxH = int32(480)
)

// compact reports whether a w×h panel should use the tight layout: abbreviated
// footer hints, a narrower QR column, and small overlay margins.
func compact(w, h int32) bool {
	return w <= compactMaxW || h <= compactMaxH
}

// fullTextMinW is the narrowest panel that fits full-length labels and footer
// hints. Below it, labels abbreviate ("● DL", "LR Sort", "Set").
//
// Every shipping panel is 480, 640, 720, 1024 or 1280 wide, so the threshold
// only has to fall somewhere in (720, 1024]; 1024 is the narrowest panel
// observed to fit the full set.
const fullTextMinW = int32(1024)

// abbreviate reports whether a panel of width w is too narrow for full-length
// labels and footer hints.
//
// Deliberately width-only, unlike compact(). Horizontal budget is a property of
// width alone: the RG Cube XX's 720×720 panel has ample vertical room, so it
// takes roomy spacing, but full hints plus a right-aligned page indicator do not
// fit across 720 pixels — they overlapped, drawn in one colour on top of itself.
func abbreviate(w int32) bool {
	return w < fullTextMinW
}

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
// Two size classes, decided by compact().
func LayoutFor(w, h int32) Layout {
	if compact(w, h) {
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
		CoverMaxW:      0.90,
		OverlayMarginX: w * 14 / 100,
	}
}
