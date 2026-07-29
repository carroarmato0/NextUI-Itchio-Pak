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
