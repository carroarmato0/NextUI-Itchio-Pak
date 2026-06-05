package roms

// DetectBufSize is the number of bytes read from a file header to identify
// its ROM type. 336 bytes covers the longest signature (GB/GBC Nintendo logo
// at 0x104 + CGB flag at 0x143 = 324 bytes, rounded up).
const DetectBufSize = 336

// DetectROMExt returns the ROM file extension inferred from the leading bytes
// of a file. Returns "" when no known ROM signature matches. Callers should
// pass at least DetectBufSize bytes; shorter slices are handled gracefully
// (signatures requiring more bytes than available are simply skipped).
func DetectROMExt(data []byte) string {
	// NES (iNES): "NES\x1A" at offset 0
	if len(data) >= 4 &&
		data[0] == 'N' && data[1] == 'E' && data[2] == 'S' && data[3] == 0x1A {
		return ".nes"
	}

	// Pico-8 text cartridge: file starts with "pico-8 cartridge"
	if len(data) >= 16 && string(data[:16]) == "pico-8 cartridge" {
		return ".p8"
	}

	// PNG: \x89PNG magic. Pico-8 .p8.png carts are always 128 pixels wide —
	// check the IHDR width field at offset 16 to distinguish them from regular
	// artwork images.
	if len(data) >= 24 &&
		data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		// PNG IHDR: 8 magic + 4 chunk-length + 4 "IHDR" + 4 width (big-endian)
		width := uint32(data[16])<<24 | uint32(data[17])<<16 |
			uint32(data[18])<<8 | uint32(data[19])
		if width == 128 {
			return ".p8.png"
		}
	}

	// GBA: ARM branch instruction at 0 + Nintendo logo at 4
	if len(data) >= 8 &&
		data[0] == 0x2E && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0xEA &&
		data[4] == 0x24 && data[5] == 0xFF && data[6] == 0xAE && data[7] == 0x51 {
		return ".gba"
	}

	// GB / GBC: Nintendo logo at 0x104, CGB flag at 0x143
	if len(data) >= 0x144 {
		logo := [8]byte{0xCE, 0xED, 0x66, 0x66, 0xCC, 0x0D, 0x00, 0x0B}
		if [8]byte(data[0x104:0x10C]) == logo {
			if data[0x143] == 0x80 || data[0x143] == 0xC0 {
				return ".gbc"
			}
			return ".gb"
		}
	}

	// Sega Genesis / Mega Drive: "SEGA " at 0x100
	if len(data) >= 0x106 &&
		data[0x100] == 'S' && data[0x101] == 'E' &&
		data[0x102] == 'G' && data[0x103] == 'A' && data[0x104] == ' ' {
		return ".md"
	}

	return ""
}
