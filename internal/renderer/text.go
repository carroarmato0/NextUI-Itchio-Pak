package renderer

import (
	"encoding/binary"
	"os"
	"unicode/utf8"
)

// cmapRange is a contiguous inclusive range of supported Unicode codepoints.
type cmapRange struct{ lo, hi rune }

// buildGlyphRanges reads a TTF file and returns the codepoint ranges covered by
// its best cmap subtable (Format 12 preferred over Format 4). Returns nil on
// any parse error; callers treat nil as "assume all characters supported."
func buildGlyphRanges(path string) []cmapRange {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 12 {
		return nil
	}

	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	cmapOff := -1
	for i := 0; i < numTables; i++ {
		base := 12 + i*16
		if base+16 > len(data) {
			break
		}
		if string(data[base:base+4]) == "cmap" {
			cmapOff = int(binary.BigEndian.Uint32(data[base+8 : base+12]))
			break
		}
	}
	if cmapOff < 0 || cmapOff+4 > len(data) {
		return nil
	}

	numSubs := int(binary.BigEndian.Uint16(data[cmapOff+2 : cmapOff+4]))
	bestOff, bestPri := -1, 0
	for i := 0; i < numSubs; i++ {
		base := cmapOff + 4 + i*8
		if base+8 > len(data) {
			break
		}
		pid := int(binary.BigEndian.Uint16(data[base : base+2]))
		eid := int(binary.BigEndian.Uint16(data[base+2 : base+4]))
		subOff := cmapOff + int(binary.BigEndian.Uint32(data[base+4:base+8]))
		if subOff+2 > len(data) {
			continue
		}
		format := int(binary.BigEndian.Uint16(data[subOff : subOff+2]))
		var pri int
		switch {
		case pid == 3 && eid == 10 && format == 12:
			pri = 4
		case pid == 0 && format == 12:
			pri = 3
		case pid == 3 && eid == 1 && format == 4:
			pri = 2
		case pid == 0 && format == 4:
			pri = 1
		}
		if pri > bestPri {
			bestPri, bestOff = pri, subOff
		}
	}
	if bestOff < 0 {
		return nil
	}

	format := int(binary.BigEndian.Uint16(data[bestOff : bestOff+2]))
	switch format {
	case 4:
		return parseCmap4(data, bestOff)
	case 12:
		return parseCmap12(data, bestOff)
	}
	return nil
}

func parseCmap4(data []byte, off int) []cmapRange {
	if off+14 > len(data) {
		return nil
	}
	segCountX2 := int(binary.BigEndian.Uint16(data[off+6 : off+8]))
	segCount := segCountX2 / 2
	endOff := off + 14
	startOff := endOff + segCountX2 + 2 // +2 for reservedPad
	deltaOff := startOff + segCountX2
	rangeOff := deltaOff + segCountX2
	if rangeOff+segCountX2 > len(data) {
		return nil
	}
	var ranges []cmapRange
	for i := 0; i < segCount; i++ {
		end := rune(binary.BigEndian.Uint16(data[endOff+i*2 : endOff+i*2+2]))
		start := rune(binary.BigEndian.Uint16(data[startOff+i*2 : startOff+i*2+2]))
		if start == 0xFFFF {
			break
		}
		if start > end {
			continue
		}
		delta := int16(binary.BigEndian.Uint16(data[deltaOff+i*2 : deltaOff+i*2+2]))
		rngOff := int(binary.BigEndian.Uint16(data[rangeOff+i*2 : rangeOff+i*2+2]))
		if rngOff == 0 {
			// glyphID = (codepoint + delta) & 0xFFFF; skip if maps to .notdef
			if (int(start)+int(delta))&0xFFFF != 0 {
				ranges = append(ranges, cmapRange{start, end})
			}
		} else {
			// Offset-based lookup — assume the segment contains real glyphs.
			ranges = append(ranges, cmapRange{start, end})
		}
	}
	return ranges
}

func parseCmap12(data []byte, off int) []cmapRange {
	if off+16 > len(data) {
		return nil
	}
	nGroups := int(binary.BigEndian.Uint32(data[off+12 : off+16]))
	var ranges []cmapRange
	for i := 0; i < nGroups; i++ {
		base := off + 16 + i*12
		if base+12 > len(data) {
			break
		}
		startCode := rune(binary.BigEndian.Uint32(data[base : base+4]))
		endCode := rune(binary.BigEndian.Uint32(data[base+4 : base+8]))
		startGlyph := int(binary.BigEndian.Uint32(data[base+8 : base+12]))
		if startGlyph != 0 {
			ranges = append(ranges, cmapRange{startCode, endCode})
		}
	}
	return ranges
}

// inRanges reports whether ch falls within any of the sorted cmap ranges.
// Binary search: O(log n).
func inRanges(ranges []cmapRange, ch rune) bool {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := (lo + hi) / 2
		r := ranges[mid]
		switch {
		case ch < r.lo:
			hi = mid
		case ch > r.hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// textRun is a contiguous sequence of characters rendered with the same font.
// fontIdx 0 = primary font; 1..N = fallback fonts in load order.
type textRun struct {
	text    string
	fontIdx int
}

// splitTextRuns segments s into runs where consecutive runes that resolve to
// the same font index are merged. fontIndex(r) returns 0 for the primary font
// or a positive index for a fallback font. An empty input returns nil.
func splitTextRuns(s string, fontIndex func(rune) int) []textRun {
	if s == "" {
		return nil
	}
	var runs []textRun
	runStart := 0
	runIdx := -1
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		idx := fontIndex(r)
		if runIdx < 0 {
			runIdx = idx
		} else if idx != runIdx {
			runs = append(runs, textRun{text: s[runStart:i], fontIdx: runIdx})
			runStart = i
			runIdx = idx
		}
		i += size
	}
	runs = append(runs, textRun{text: s[runStart:], fontIdx: runIdx})
	return runs
}

// sanitizeText strips emoji and symbol characters that the bundled font cannot
// render. This prevents SDL2_ttf from emitting tofu boxes for missing glyphs.
// CJK, Cyrillic, and accented Latin are preserved — the font covers them.
func sanitizeText(s string) string {
	if s == "" {
		return s
	}
	// Fast path: scan once; if no emoji found, return s unchanged (zero allocs).
	hasEmoji := false
	for _, r := range s {
		if isEmoji(r) {
			hasEmoji = true
			break
		}
	}
	if !hasEmoji {
		return s
	}
	// Slow path: at least one emoji present — rebuild without emoji runes.
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
