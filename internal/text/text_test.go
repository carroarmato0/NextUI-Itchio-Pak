package text_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/text"
)

func TestIsEmoji(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want bool
	}{
		{"ASCII letter A", 'A', false},
		{"CJK ideograph", '中', false},
		{"Arabic letter", 'ا', false},
		{"accented Latin", 'é', false},
		{"Misc Symbols start U+2600", 0x2600, true},
		{"Misc Symbols end U+26FF", 0x26FF, true},
		{"Dingbats start U+2700", 0x2700, true},
		{"Dingbats end U+27BF", 0x27BF, true},
		{"Misc Symbols Arrows U+2B00", 0x2B00, true},
		{"Misc Pictographs U+1F300", 0x1F300, true},
		{"Emoticons U+1F600", 0x1F600, true},
		{"Transport U+1F680", 0x1F680, true},
		{"Floppy disk U+1F4BE", 0x1F4BE, true},
		{"Supplementary U+1F700", 0x1F700, true},
		{"Supplementary end U+1FFFF", 0x1FFFF, true},
		{"just below Misc Symbols U+25FF", 0x25FF, false},
		{"just above Supplementary U+20000", 0x20000, false},
		{"Variation Selector start U+FE00", 0xFE00, true},
		{"Variation Selector end U+FE0F", 0xFE0F, true},
		{"just below Variation Selectors U+EFFF", 0xEFFF, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := text.IsEmoji(tc.r)
			if got != tc.want {
				t.Errorf("IsEmoji(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

func TestStripEmoji(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain ASCII unchanged", "Hello World", "Hello World"},
		{"CJK unchanged", "日本語", "日本語"},
		{"Arabic unchanged", "مرحبا", "مرحبا"},
		{"single emoji stripped", "🎮", ""},
		{"leading emoji stripped", "🎮 Adventure", " Adventure"},
		{"embedded emoji stripped", "Night 🌙 Crawler", "Night  Crawler"},
		{"trailing emoji stripped", "Dungeon ⚔️", "Dungeon "},
		{"emoji-only title becomes empty", "🎮🌙⚔️", ""},
		{"floppy disk U+1F4BE stripped", "\U0001F4BE", ""},
		{"misc symbol stripped", "★ cool", " cool"},
		{"dingbat stripped", "✂ cut", " cut"},
		{"mixed CJK and emoji", "かぞくロボット 🎮", "かぞくロボット "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := text.StripEmoji(tc.input)
			if got != tc.want {
				t.Errorf("StripEmoji(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestStripEmojiNoAllocFastPath(t *testing.T) {
	input := "Hello World"
	allocs := testing.AllocsPerRun(100, func() {
		_ = text.StripEmoji(input)
	})
	if allocs != 0 {
		t.Errorf("StripEmoji(%q): got %.0f allocs, want 0", input, allocs)
	}
}
