package theme

import (
	"fmt"
	"strconv"
	"strings"
)

// RGBA is a packed 0xRRGGBBAA colour, the form NextUI stores in
// minuisettings.txt and in palette files.
type RGBA uint32

// RGB returns the red, green and blue channels, dropping alpha.
func (c RGBA) RGB() [3]uint8 {
	return [3]uint8{uint8(c >> 24), uint8(c >> 16), uint8(c >> 8)}
}

// A returns the alpha channel.
func (c RGBA) A() uint8 { return uint8(c) }

// Opaque reports whether the colour is fully opaque.
func (c RGBA) Opaque() bool { return c.A() == 0xFF }

// maxHexDigits is the width of a packed RGBA literal.
const maxHexDigits = 8

// ParseColor parses a NextUI colour literal into a packed 0xRRGGBBAA value.
//
// It mirrors NextUI's CFG_parseHexColor (workspace/all/common/config.c:111),
// which branches on the *number of hex digits* rather than the numeric value:
// six or fewer digits are a legacy RGB triple promoted to opaque RGBA, seven or
// more are already packed RGBA. An optional "0x"/"0X" prefix and surrounding
// whitespace are accepted, and scanning stops at the first non-hex byte, just as
// strtoul does.
//
// Note this means "0xfff" is 0x000FFFFF, not CSS shorthand for white — matching
// the device is what matters here.
//
// Where NextUI would silently yield black (no hex digits at all, or more than
// eight) this returns an error instead, so the caller can keep a visible default
// and log the mismatch rather than painting a black screen.
func ParseColor(s string) (RGBA, error) {
	body := strings.TrimSpace(s)
	if len(body) >= 2 && body[0] == '0' && (body[1] == 'x' || body[1] == 'X') {
		body = body[2:]
	}

	digits := 0
	for digits < len(body) && isHexDigit(body[digits]) {
		digits++
	}
	switch {
	case digits == 0:
		return 0, fmt.Errorf("no hex digits in %q", s)
	case digits > maxHexDigits:
		return 0, fmt.Errorf("too many hex digits (%d, max %d) in %q", digits, maxHexDigits, s)
	}

	v, err := strconv.ParseUint(body[:digits], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}

	if digits <= 6 {
		return RGBA(v<<8 | 0xFF), nil
	}
	return RGBA(v), nil
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

const (
	// shadeStep is one step of separation between stacked surfaces.
	shadeStep = 10
	// surfaceMinDelta is the smallest per-channel difference at which two
	// colours read as distinct surfaces rather than one flat area.
	surfaceMinDelta = 8
)

// clamp8 saturates v into a uint8 instead of wrapping. Every derived colour goes
// through here: plain uint8 arithmetic silently wrapped a light background to
// near-black, which is what made light palettes unusable.
func clamp8(v int) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v)
}

// Luma returns the BT.601 perceived brightness of c, 0-255.
func Luma(c [3]uint8) int {
	return (299*int(c[0]) + 587*int(c[1]) + 114*int(c[2])) / 1000
}

// IsLight reports whether c reads as a light colour.
func IsLight(c [3]uint8) bool { return Luma(c) >= 128 }

// IsLightTheme reports whether this theme sits on a light background. NextUI
// ships Catppuccin Latte, so this is a real case and not a hypothetical.
func (t Theme) IsLightTheme() bool { return IsLight(t.Background) }

// Shade moves c away from the theme background by the given number of steps:
// lighter on a dark theme, darker on a light one. Saturating.
func (t Theme) Shade(c [3]uint8, steps int) [3]uint8 {
	delta := steps * shadeStep
	if t.IsLightTheme() {
		delta = -delta
	}
	return [3]uint8{
		clamp8(int(c[0]) + delta),
		clamp8(int(c[1]) + delta),
		clamp8(int(c[2]) + delta),
	}
}

// ShadeBG is Shade applied to the background — the usual way to raise a panel
// off the backdrop.
func (t Theme) ShadeBG(steps int) [3]uint8 { return t.Shade(t.Background, steps) }

// Surface returns the fill for a bar or panel sitting on the background.
//
// It prefers color3, which is what a palette author would use for exactly this,
// but only when that colour is actually distinguishable from the background.
// In every palette NextUI ships, color3 is equal or near-equal to color7
// (byte-identical in Catppuccin Mocha and Latte), so drawing it raw would make
// the header and footer bars disappear.
func (t Theme) Surface() [3]uint8 {
	if chebyshev(t.HeaderBG, t.Background) >= surfaceMinDelta {
		return t.HeaderBG
	}
	return t.ShadeBG(1)
}

// Separator returns the colour for hairlines and dividers.
func (t Theme) Separator() [3]uint8 { return t.ShadeBG(3) }

// chebyshev is the largest per-channel difference between two colours.
func chebyshev(a, b [3]uint8) int {
	worst := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
	}
	return worst
}
