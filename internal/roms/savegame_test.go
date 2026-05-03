package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestSaveGamePath(t *testing.T) {
	base := "/mnt/SDCARD"
	cases := []struct {
		saveFormat    int
		innerFilename string
		romPath       string
		want          string
	}{
		// format 0 (MinUI) — full filename + .sav
		{0, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.gb.sav"},
		{0, "", base + "/Roms/Game Boy Color (GBC)/Solastra.gbc", base + "/Saves/GBC/Solastra.gbc.sav"},
		{0, "", base + "/Roms/Game Boy (GB)/My Game v1.2.gb", base + "/Saves/GB/My Game v1.2.gb.sav"},
		// format 1 (Retroarch SRM compressed) — strip ext, .srm
		{1, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.srm"},
		{1, "", base + "/Roms/Game Boy Color (GBC)/Solastra.gbc", base + "/Saves/GBC/Solastra.srm"},
		// format 2 (Generic) — strip ext, .sav
		{2, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.sav"},
		{2, "", base + "/Roms/Game Boy (GB)/My Game v1.2.gb", base + "/Saves/GB/My Game v1.2.sav"},
		// format 3 (Retroarch SRM uncompressed) — same as format 1
		{3, "", base + "/Roms/Game Boy (GB)/Doomslinger Dungeon.gb", base + "/Saves/GB/Doomslinger Dungeon.srm"},
		// unrecognised directory
		{0, "", base + "/Roms/Unknown Emulator/foo.rom", ""},
		// zip + format 0 + no innerFilename → zip.sav
		{0, "", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).zip.sav"},
		// zip + format 0 + innerFilename → inner.gb.sav
		{0, "Pokemon - Red Version (USA, Europe).gb", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).gb.sav"},
		// zip + format 1 + innerFilename → same stem as without (both stripped)
		{1, "Pokemon - Red Version (USA, Europe).gb", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).srm"},
		{1, "", base + "/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip", base + "/Saves/GB/Pokemon - Red Version (USA, Europe).srm"},
	}
	for _, c := range cases {
		got := roms.SaveGamePath(c.romPath, c.saveFormat, c.innerFilename)
		if got != c.want {
			t.Errorf("SaveGamePath(%q, %d, %q)\n  got  %q\n  want %q", c.romPath, c.saveFormat, c.innerFilename, got, c.want)
		}
	}
}
