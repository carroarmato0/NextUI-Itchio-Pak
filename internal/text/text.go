package text

import "unicode/utf8"

// IsEmoji reports whether r is an emoji or symbol codepoint.
func IsEmoji(r rune) bool {
	return (r >= 0x2600 && r <= 0x26FF) || // Miscellaneous Symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0x2B00 && r <= 0x2BFF) || // Miscellaneous Symbols and Arrows
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F650 && r <= 0x1F67F) || // Ornamental Dingbats
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1FFFF) || // Various supplementary emoji blocks
		(r >= 0xFE00 && r <= 0xFE0F) // Variation Selectors (e.g. U+FE0F emoji presentation)
}

// StripEmoji returns s with all emoji codepoints removed.
// Fast path: if no emoji are present, s is returned unchanged (zero allocations).
func StripEmoji(s string) string {
	if s == "" {
		return s
	}
	hasEmoji := false
	for _, r := range s {
		if IsEmoji(r) {
			hasEmoji = true
			break
		}
	}
	if !hasEmoji {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !IsEmoji(r) {
			out = append(out, s[i:i+size]...)
		}
		i += size
	}
	return string(out)
}
