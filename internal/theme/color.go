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

// TitlePillText returns the text colour for a pill filled with TitlePill.
//
// NextUI pairs its color2 title pill with color6, not color5 (nextui.c:2849).
// The distinction is easy to get wrong and invisible in a dark-on-dark theme:
// color5 is "list text selected", which belongs on the color1 pill. On
// Catppuccin Macchiato, color5 is #24273A and color2 is #1E2030 — dark text on
// a darker fill, which renders the pill unreadable.
func (t Theme) TitlePillText() [3]uint8 { return t.HintText }

// Contrast is the absolute difference in perceived brightness between two
// colours. Around 60 is the point at which text stops being a strain to read.
func Contrast(a, b [3]uint8) int {
	d := Luma(a) - Luma(b)
	if d < 0 {
		return -d
	}
	return d
}

// Lighten raises every channel by amount, saturating at 255.
func Lighten(c [3]uint8, amount int) [3]uint8 {
	return [3]uint8{clamp8(int(c[0]) + amount), clamp8(int(c[1]) + amount), clamp8(int(c[2]) + amount)}
}

// Darken lowers every channel by amount, saturating at 0.
func Darken(c [3]uint8, amount int) [3]uint8 {
	return [3]uint8{clamp8(int(c[0]) - amount), clamp8(int(c[1]) - amount), clamp8(int(c[2]) - amount)}
}

// Mix blends pct percent of b into a. pct is clamped to 0..100. Being a true
// interpolation between two in-range colours, the result cannot overflow.
func Mix(a, b [3]uint8, pct int) [3]uint8 {
	switch {
	case pct < 0:
		pct = 0
	case pct > 100:
		pct = 100
	}
	var out [3]uint8
	for i := range a {
		out[i] = clamp8(int(a[i]) + (int(b[i])-int(a[i]))*pct/100)
	}
	return out
}

// minContrast is the luma gap below which text is a strain to read.
const minContrast = 60

// ContrastText returns a text colour legible on the given fill.
//
// It prefers the palette's own foregrounds so text keeps the theme's character,
// and only falls back to black or white when neither clears minContrast — which
// happens on mid-greys, where a palette simply has no suitable colour.
func (t Theme) ContrastText(fill [3]uint8) [3]uint8 {
	best, bestGap := t.MainText, Contrast(fill, t.MainText)
	if gap := Contrast(fill, t.AccentText); gap > bestGap {
		best, bestGap = t.AccentText, gap
	}
	if gap := Contrast(fill, t.HintText); gap > bestGap {
		best, bestGap = t.HintText, gap
	}
	if bestGap >= minContrast {
		return best
	}
	if IsLight(fill) {
		return [3]uint8{0x00, 0x00, 0x00}
	}
	return [3]uint8{0xFF, 0xFF, 0xFF}
}

// ToneOn adapts a hue to be readable on a specific surface, rather than on the
// theme background.
//
// Tone only knows about Background, which is wrong the moment something is drawn
// on a raised surface. A modal panel is shaded off the background by design, so
// a colour toned against the background can still land too close to the panel:
// on Catppuccin Macchiato the confirm hint drew Error #C83C3C on panel #383B4E,
// a contrast of 41, and that string is the destructive-action prompt.
func (t Theme) ToneOn(c, surface [3]uint8) [3]uint8 {
	if Contrast(c, surface) >= minContrast {
		return c
	}
	target := [3]uint8{0xFF, 0xFF, 0xFF}
	if IsLight(surface) {
		target = [3]uint8{0x00, 0x00, 0x00}
	}
	for pct := toneStep; pct <= toneMaxPct; pct += toneStep {
		if out := Mix(c, target, pct); Contrast(out, surface) >= minContrast {
			return out
		}
	}
	return Mix(c, target, toneMaxPct)
}

// MutedOn is de-emphasised text on a specific surface, for the same reason
// ToneOn exists: Muted is mixed from the background and drifts on a panel.
//
// It mixes halfway toward whatever reads on that surface rather than toward
// ListText. Mixing toward ListText assumes ListText is readable there, and on
// the MinUI palette — white text, mid-grey chip — that produced #D6D6D6 on
// #ADADAD, a contrast of 41. On Background the two definitions agree, so
// MutedOn(Background) is still Muted().
func (t Theme) MutedOn(surface [3]uint8) [3]uint8 {
	return Mix(surface, t.ContrastText(surface), 50)
}

// ModalPanel returns the fill for a modal or raised panel.
//
// This replaces a literal `bg[0]+20` on a uint8, which wrapped: on Catppuccin
// Latte (background #EFF1F5) it produced #030509, a near-black panel carrying
// near-black text. On the app's own dark default, ShadeBG(2) is #282828 —
// exactly what the old arithmetic produced, so nothing moves there.
func (t Theme) ModalPanel() [3]uint8 { return t.ShadeBG(2) }

// ModalBorder outlines a modal panel: one more step away from the background,
// so it stays visible whichever direction the theme shades in.
func (t Theme) ModalBorder() [3]uint8 { return t.Shade(t.ModalPanel(), 3) }

// ModalScrim dims the screen behind a modal.
func (t Theme) ModalScrim() [3]uint8 {
	if t.IsLightTheme() {
		return Lighten(t.Background, 30)
	}
	return Darken(t.Background, 10)
}

// Chip returns the fill for a small inline tag pill.
//
// Derived from Surface rather than Accent. Blending toward Accent seems natural
// but is wrong: Accent is a full-strength selection colour, and on the MinUI
// palette — where Accent is white and color3 a mid grey — it produced a #D1D1D1
// chip carrying white text. A chip is a raised surface, not a selection.
//
// Pair with ContrastText(Chip()) for the label.
func (t Theme) Chip() [3]uint8 { return t.Shade(t.Surface(), 2) }

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
