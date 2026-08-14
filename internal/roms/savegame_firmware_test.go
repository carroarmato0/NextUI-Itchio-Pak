package roms

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
)

// muOS lets the user name a ROM folder anything at all, so a folder called
// "Game Boy (GB)" is perfectly legal there. The save-tag lookup matches on that
// name, which means without a capability check we would happily compute a save
// path under a Saves directory muOS does not have, and write to it during a
// unified-naming migration. Nothing would report an error; the file would just
// end up somewhere the user never finds.
func TestSaveGamePathRefusedOnFirmwareWithoutSaveSync(t *testing.T) {
	const nextUIShapedPath = "/mnt/mmc/ROMS/Game Boy (GB)/Game.gb"

	// Pinned back to NextUI for the rest of the package by TestMain's env.
	t.Cleanup(func() { firmware.SetActive(firmware.ForTest(firmware.KindNextUI, "")) })

	firmware.SetActive(firmware.ForTest(firmware.KindNextUI, ""))
	if got := SaveGamePath(nextUIShapedPath, 0, ""); got == "" {
		t.Fatal("NextUI should still derive a save path for a recognised folder name")
	}

	firmware.SetActive(firmware.ForTest(firmware.KindHost, ""))
	if got := SaveGamePath(nextUIShapedPath, 0, ""); got != "" {
		t.Errorf("SaveGamePath = %q, want empty on firmware that cannot locate saves", got)
	}
	if got := SaveStatePaths(nextUIShapedPath, 0, "", "GB", "gambatte"); got != nil {
		t.Errorf("SaveStatePaths = %v, want nil on firmware that cannot locate states", got)
	}
}
