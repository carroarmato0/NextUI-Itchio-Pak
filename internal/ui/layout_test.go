package ui

import "testing"

func TestLayoutForWideScreen(t *testing.T) {
	l := LayoutFor(1280, 720)
	if l.HeaderPad != 6 {
		t.Errorf("HeaderPad = %d, want 6", l.HeaderPad)
	}
	if l.RowPad != 4 {
		t.Errorf("RowPad = %d, want 4", l.RowPad)
	}
	if l.FooterPad != 5 {
		t.Errorf("FooterPad = %d, want 5", l.FooterPad)
	}
	if l.ContentGap != 6 {
		t.Errorf("ContentGap = %d, want 6", l.ContentGap)
	}
	if l.CoverMaxW != 0.90 {
		t.Errorf("CoverMaxW = %v, want 0.90", l.CoverMaxW)
	}
	if l.OverlayMarginX != 1280*14/100 {
		t.Errorf("OverlayMarginX = %d, want %d", l.OverlayMarginX, 1280*14/100)
	}
}

func TestLayoutForSmallScreen(t *testing.T) {
	l := LayoutFor(640, 480)
	if l.HeaderPad != 3 {
		t.Errorf("HeaderPad = %d, want 3", l.HeaderPad)
	}
	if l.RowPad != 2 {
		t.Errorf("RowPad = %d, want 2", l.RowPad)
	}
	if l.FooterPad != 2 {
		t.Errorf("FooterPad = %d, want 2", l.FooterPad)
	}
	if l.ContentGap != 3 {
		t.Errorf("ContentGap = %d, want 3", l.ContentGap)
	}
	if l.CoverMaxW != 1.0 {
		t.Errorf("CoverMaxW = %v, want 1.0", l.CoverMaxW)
	}
	if l.OverlayMarginX != 640*3/100 {
		t.Errorf("OverlayMarginX = %d, want %d", l.OverlayMarginX, 640*3/100)
	}
}

func TestLayoutForOverlayMargin(t *testing.T) {
	wide := LayoutFor(1280, 720)
	if wide.OverlayMarginX != 1280*14/100 {
		t.Errorf("wide OverlayMarginX = %d, want %d", wide.OverlayMarginX, 1280*14/100)
	}
	small := LayoutFor(640, 480)
	if small.OverlayMarginX != 640*3/100 {
		t.Errorf("small OverlayMarginX = %d, want %d", small.OverlayMarginX, 640*3/100)
	}
}

// The size class is what decides whether a panel gets abbreviated footer hints,
// a narrower QR column, and 3% instead of 14% overlay margins. H700 introduced
// two geometries the width test got wrong, so the rule is pinned per device
// here: a panel is compact unless it is roomy in *both* directions.
func TestCompactSizeClass(t *testing.T) {
	for _, tc := range []struct {
		w, h int32
		want bool
		why  string
	}{
		{640, 480, true, "Miyoo Flip and most RG XX"},
		{720, 480, true, "RG34XX / RG34XX SP / RG SP"},
		{720, 720, false, "RG Cube XX"},
		{480, 640, true, "RG28XX if SDL_ROTATION misses our window"},
		{1024, 768, false, "TrimUI Brick"},
		{1280, 720, false, "TrimUI Smart Pro"},
		{641, 481, false, "one pixel past both limits"},
		{641, 480, true, "wide enough, too short"},
		{640, 481, true, "tall enough, too narrow"},
	} {
		if got := compact(tc.w, tc.h); got != tc.want {
			t.Errorf("compact(%d, %d) = %v, want %v (%s)", tc.w, tc.h, got, tc.want, tc.why)
		}
	}
}

// LayoutFor must follow the same rule, not a second copy of it.
func TestLayoutForFollowsSizeClass(t *testing.T) {
	if l := LayoutFor(720, 480); l.RowPad != 2 {
		t.Errorf("720x480 RowPad = %d, want 2 (compact)", l.RowPad)
	}
	if l := LayoutFor(720, 720); l.RowPad != 4 {
		t.Errorf("720x720 RowPad = %d, want 4 (roomy)", l.RowPad)
	}
	if l := LayoutFor(640, 480); l.OverlayMarginX != 640*3/100 {
		t.Errorf("640x480 OverlayMarginX = %d, want %d", l.OverlayMarginX, 640*3/100)
	}
	if l := LayoutFor(1280, 720); l.OverlayMarginX != 1280*14/100 {
		t.Errorf("1280x720 OverlayMarginX = %d, want %d", l.OverlayMarginX, 1280*14/100)
	}
}
