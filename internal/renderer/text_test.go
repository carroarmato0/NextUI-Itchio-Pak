package renderer

import (
	"testing"
)

func TestSplitTextRuns(t *testing.T) {
	// Mock: ASCII (≤127) → font 0 (primary), Arabic block → font 1, CJK → font 2.
	mockIndex := func(r rune) int {
		switch {
		case r <= 127:
			return 0
		case r >= 0x0600 && r <= 0x06FF:
			return 1
		default:
			return 2
		}
	}

	cases := []struct {
		name  string
		input string
		want  []textRun
	}{
		{
			"empty returns nil",
			"",
			nil,
		},
		{
			"all primary — single run",
			"Hello",
			[]textRun{{"Hello", 0}},
		},
		{
			"all arabic — single run",
			"خالي",
			[]textRun{{"خالي", 1}},
		},
		{
			"primary then arabic",
			"Hi خالي",
			[]textRun{{"Hi ", 0}, {"خالي", 1}},
		},
		{
			"arabic then primary",
			"خالي Hi",
			[]textRun{{"خالي", 1}, {" Hi", 0}},
		},
		{
			"single-char alternating runs",
			"Aخ",
			[]textRun{{"A", 0}, {"خ", 1}},
		},
		{
			"latin with embedded arabic",
			"gameخali",
			[]textRun{{"game", 0}, {"خ", 1}, {"ali", 0}},
		},
		{
			"multi-byte CJK rune treated as one unit",
			"日",
			[]textRun{{"日", 2}},
		},
		{
			"three distinct font indices",
			"Aخ日",
			[]textRun{{"A", 0}, {"خ", 1}, {"日", 2}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitTextRuns(tc.input, mockIndex)
			if len(got) != len(tc.want) {
				t.Fatalf("splitTextRuns(%q) = %v (len %d), want %v (len %d)",
					tc.input, got, len(got), tc.want, len(tc.want))
			}
			for i, run := range got {
				if run != tc.want[i] {
					t.Errorf("run[%d]: got %+v, want %+v", i, run, tc.want[i])
				}
			}
		})
	}
}

func TestBuildGlyphRanges(t *testing.T) {
	// Verify the primary font covers expected scripts and excludes Arabic.
	ranges := buildGlyphRanges("../../assets/font.ttf")
	if ranges == nil {
		t.Fatal("buildGlyphRanges returned nil — font parse failed")
	}
	cases := []struct {
		name      string
		ch        rune
		wantIn    bool
	}{
		{"ASCII letter", 'A', true},
		{"accented Latin", 'é', true},
		{"Japanese hiragana", 'あ', true},
		{"CJK ideograph", '中', true},
		{"Arabic alef U+0627", 'ا', false},
		{"Arabic U+0600", '؀', false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inRanges(ranges, tc.ch)
			if got != tc.wantIn {
				t.Errorf("inRanges(%U) = %v, want %v", tc.ch, got, tc.wantIn)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain latin unchanged", "Hello World", "Hello World"},
		{"accented latin unchanged", "Feliz polaridad Maré", "Feliz polaridad Maré"},
		{"cyrillic unchanged", "Привет", "Привет"},
		{"japanese unchanged", "彼は私の中の少女", "彼は私の中の少女"},
		{"traditional chinese unchanged", "桑之巫韻", "桑之巫韻"},
		{"single emoji stripped", "🎳", ""},
		{"emoji in title stripped", "B🎳wling", "Bwling"},
		{"emoji in description stripped", "Great game 🦾", "Great game "},
		{"misc symbol stripped", "★ cool", " cool"},
		{"dingbat stripped", "✂ cut", " cut"},
		{"misc symbols arrows stripped", "⭐ star", " star"},
		{"transport emoji stripped", "🚀 launch", " launch"},
		{"emoticon stripped", "😀 fun", " fun"},
		{"mixed cjk and emoji", "かぞくロボット 🎮", "かぞくロボット "},
		{"invalid utf-8 preserved", "\xff\xfe", "\xff\xfe"},
		{"empty string", "", ""},
		{"floppy disk U+1F4BE passes through", "\U0001F4BE", "\U0001F4BE"},
		{"codepoint before floppy disk still stripped", "\U0001F4BD", ""},
		{"codepoint after floppy disk still stripped", "\U0001F4BF", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeText(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
