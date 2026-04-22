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
		{"transport emoji stripped", "🚀 launch", " launch"},
		{"emoticon stripped", "😀 fun", " fun"},
		{"mixed cjk and emoji", "かぞくロボット 🎮", "かぞくロボット "},
		{"empty string", "", ""},
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
