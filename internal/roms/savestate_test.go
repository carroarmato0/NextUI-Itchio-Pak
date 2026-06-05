package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestSaveStatePaths_Format0_GB(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Doomslinger Dungeon.gb"
	paths := roms.SaveStatePaths(romPath, 0, "", "GB", "gambatte")
	if len(paths) != 10 {
		t.Fatalf("format 0 should return 10 paths, got %d", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Doomslinger Dungeon.gb"
	if paths[0] != base+".st0" {
		t.Errorf("slot 0: got %q", paths[0])
	}
	if paths[5] != base+".st5" {
		t.Errorf("slot 5: got %q", paths[5])
	}
	if paths[9] != base+".st9" {
		t.Errorf("auto (slot 9): got %q", paths[9])
	}
}

func TestSaveStatePaths_Format1_GB(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Doomslinger Dungeon.gb"
	paths := roms.SaveStatePaths(romPath, 1, "", "GB", "gambatte")
	if len(paths) != 9 {
		t.Fatalf("format 1 should return 9 paths, got %d", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Doomslinger Dungeon"
	if paths[0] != base+".state.1" {
		t.Errorf("slot 1: got %q", paths[0])
	}
	if paths[4] != base+".state.5" {
		t.Errorf("slot 5: got %q", paths[4])
	}
	if paths[8] != base+".state.auto" {
		t.Errorf("auto: got %q", paths[8])
	}
}

func TestSaveStatePaths_Format3_GB(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Doomslinger Dungeon.gb"
	paths := roms.SaveStatePaths(romPath, 3, "", "GB", "gambatte")
	if len(paths) != 10 {
		t.Fatalf("format 3 should return 10 paths, got %d", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Doomslinger Dungeon"
	if paths[0] != base+".state" {
		t.Errorf("slot 0: got %q", paths[0])
	}
	if paths[5] != base+".state5" {
		t.Errorf("slot 5: got %q", paths[5])
	}
	if paths[9] != base+".state.auto" {
		t.Errorf("auto: got %q", paths[9])
	}
}

func TestSaveStatePaths_ZipWithInnerFilename_Format0(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip"
	inner := "Pokemon - Red Version (USA, Europe).gb"
	paths := roms.SaveStatePaths(romPath, 0, inner, "GB", "gambatte")
	if len(paths) != 10 {
		t.Fatalf("got %d paths", len(paths))
	}
	base := "/mnt/SDCARD/.userdata/shared/GB-gambatte/Pokemon - Red Version (USA, Europe).gb"
	if paths[0] != base+".st0" {
		t.Errorf("slot 0 with inner filename: got %q", paths[0])
	}
}

func TestSaveStatePaths_ZipWithInnerFilename_Format1_SameAsStemOnly(t *testing.T) {
	romPath := "/mnt/SDCARD/Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip"
	inner := "Pokemon - Red Version (USA, Europe).gb"
	withInner := roms.SaveStatePaths(romPath, 1, inner, "GB", "gambatte")
	withoutInner := roms.SaveStatePaths(romPath, 1, "", "GB", "gambatte")
	if len(withInner) != len(withoutInner) {
		t.Fatalf("format 1 zip paths should be equal with or without innerFilename: %d vs %d", len(withInner), len(withoutInner))
	}
	for i := range withInner {
		if withInner[i] != withoutInner[i] {
			t.Errorf("path[%d] differs: %q vs %q", i, withInner[i], withoutInner[i])
		}
	}
}

func TestSaveStatePaths_UnrecognisedDir_ReturnsNil(t *testing.T) {
	paths := roms.SaveStatePaths("/mnt/SDCARD/Roms/Unknown/foo.rom", 0, "", "", "")
	if paths != nil {
		t.Errorf("expected nil for unrecognised dir, got %v", paths)
	}
}

func TestRomCoreInfo_NES(t *testing.T) {
	tag, core := roms.RomCoreInfo("/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/game.nes")
	if tag != "FC" || core != "fceumm" {
		t.Errorf("RomCoreInfo NES: got (%q, %q), want (FC, fceumm)", tag, core)
	}
}

func TestRomCoreInfo_Genesis(t *testing.T) {
	tag, core := roms.RomCoreInfo("/mnt/SDCARD/Roms/Sega Genesis (MD)/game.md")
	if tag != "MD" || core != "picodrive" {
		t.Errorf("RomCoreInfo Genesis: got (%q, %q), want (MD, picodrive)", tag, core)
	}
}
