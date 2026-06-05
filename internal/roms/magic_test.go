package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestDetectROMExt(t *testing.T) {
	// nesHeader builds a minimal iNES header.
	nesHeader := func() []byte {
		b := make([]byte, 16)
		b[0], b[1], b[2], b[3] = 'N', 'E', 'S', 0x1A
		return b
	}

	// gbaHeader builds the GBA boot ROM entry point + start of Nintendo logo.
	gbaHeader := func() []byte {
		b := make([]byte, 16)
		b[0], b[1], b[2], b[3] = 0x2E, 0x00, 0x00, 0xEA // ARM branch
		b[4], b[5], b[6], b[7] = 0x24, 0xFF, 0xAE, 0x51 // Nintendo logo start
		return b
	}

	// gbHeader builds a minimal GB ROM header (Nintendo logo at 0x104).
	gbHeader := func(cgbFlag byte) []byte {
		b := make([]byte, 0x150)
		logo := []byte{0xCE, 0xED, 0x66, 0x66, 0xCC, 0x0D, 0x00, 0x0B}
		copy(b[0x104:], logo)
		b[0x143] = cgbFlag
		return b
	}

	// mdHeader builds a minimal Sega Genesis ROM header.
	mdHeader := func() []byte {
		b := make([]byte, 0x120)
		copy(b[0x100:], "SEGA GENESIS    ")
		return b
	}

	// pngHeader builds a minimal PNG IHDR with a given width.
	pngHeader := func(width uint32) []byte {
		b := make([]byte, 32)
		// PNG signature
		copy(b[0:], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
		// IHDR chunk length (13 bytes)
		b[8], b[9], b[10], b[11] = 0x00, 0x00, 0x00, 0x0D
		// "IHDR"
		copy(b[12:], "IHDR")
		// Width (big-endian)
		b[16] = byte(width >> 24)
		b[17] = byte(width >> 16)
		b[18] = byte(width >> 8)
		b[19] = byte(width)
		return b
	}

	tests := []struct {
		name  string
		data  []byte
		want  string
	}{
		{"NES magic", nesHeader(), ".nes"},
		{"GBA magic", gbaHeader(), ".gba"},
		{"GB (no CGB flag)", gbHeader(0x00), ".gb"},
		{"GBC compatible (flag 0x80)", gbHeader(0x80), ".gbc"},
		{"GBC only (flag 0xC0)", gbHeader(0xC0), ".gbc"},
		{"Sega Genesis", mdHeader(), ".md"},
		{"PNG width=128 → p8.png", pngHeader(128), ".p8.png"},
		{"PNG width=256 → not p8.png", pngHeader(256), ""},
		{"Pico-8 .p8 text cart", []byte("pico-8 cartridge // http://www.pico-8.com\n"), ".p8"},
		{"empty data", []byte{}, ""},
		{"too short", []byte{0x2E, 0x00, 0x00}, ""},
		{"random bytes", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, ""},
	}

	for _, tt := range tests {
		got := roms.DetectROMExt(tt.data)
		if got != tt.want {
			t.Errorf("%s: DetectROMExt() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
