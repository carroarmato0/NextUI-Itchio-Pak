package renderer

import "testing"

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
