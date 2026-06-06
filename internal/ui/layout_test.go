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
	if l.CoverMaxW != 0.75 {
		t.Errorf("CoverMaxW = %v, want 0.75", l.CoverMaxW)
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

func TestLayoutForBoundary(t *testing.T) {
	// exactly at narrowScreenW should use small layout
	l := LayoutFor(narrowScreenW, 480)
	if l.RowPad != 2 {
		t.Errorf("at boundary RowPad = %d, want 2 (small)", l.RowPad)
	}
	// one pixel wider → wide layout
	l = LayoutFor(narrowScreenW+1, 720)
	if l.RowPad != 4 {
		t.Errorf("above boundary RowPad = %d, want 4 (wide)", l.RowPad)
	}
}
