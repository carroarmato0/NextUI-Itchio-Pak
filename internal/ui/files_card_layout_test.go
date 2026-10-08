package ui

import "testing"

func TestFilesCardInline(t *testing.T) {
	cases := []struct {
		name                        string
		labelEnd, rowStart, usableW int32
		want                        bool
	}{
		{"short label (Download again) at 1024", 20 + 260, 20, 984, true},
		{"long buy label at 1024", 20 + 570, 20, 984, false},
		{"long buy label at 640", 20 + 360, 20, 600, false},
		{"exactly half left", 20 + 480, 20, 1004, true},
	}
	for _, c := range cases {
		if got := filesCardInline(c.labelEnd, c.rowStart, c.usableW); got != c.want {
			t.Errorf("%s: filesCardInline = %v, want %v", c.name, got, c.want)
		}
	}
}
