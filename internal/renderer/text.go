package renderer

import "unicode/utf8"

// sanitizeText strips emoji and symbol characters that the bundled font cannot
// render. This prevents SDL2_ttf from emitting tofu boxes for missing glyphs.
// CJK, Cyrillic, and accented Latin are preserved — the font covers them.
func sanitizeText(s string) string {
	if s == "" {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !isEmoji(r) {
			out = append(out, s[i:i+size]...)
		}
		i += size
	}
	return string(out)
}

// isEmoji reports whether r falls within a Unicode block reserved for emoji or
// symbol characters that the bundled font is known not to support.
func isEmoji(r rune) bool {
	if r == 0x1F4BE { // floppy disk — used as download indicator
		return false
	}
	return (r >= 0x2600 && r <= 0x26FF) || // Miscellaneous Symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0x2B00 && r <= 0x2BFF) || // Miscellaneous Symbols and Arrows
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F650 && r <= 0x1F67F) || // Ornamental Dingbats
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1FFFF) // Various supplementary emoji blocks
}
