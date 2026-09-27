//go:build !headless

package ui

import "testing"

// The gear glyph "⚙" (U+2699) used to stand in for "Settings" in the
// abbreviated footer, but it isn't covered by font.ttf or any bundled
// fallback font (confirmed against the cmap parser in internal/renderer),
// so it rendered as an empty pill. The abbreviated hint must use a label
// every bundled font actually has a glyph for.
func TestBackHintsAbbreviatedLabelIsRenderable(t *testing.T) {
	hints := backHints(640)
	if len(hints) < 2 {
		t.Fatalf("backHints(640) = %v, want at least 2 hints", hints)
	}
	settings := hints[1]
	if settings.Label == "⚙" {
		t.Errorf("backHints(640) settings hint label = %q, want a renderable label (not the gear glyph)", settings.Label)
	}
	if settings.Label == "" {
		t.Errorf("backHints(640) settings hint has empty label")
	}
}
