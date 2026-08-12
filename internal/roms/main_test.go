package roms

import (
	"os"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
)

// Destination paths now come from internal/firmware rather than from constants
// in this package, so the tests need a firmware pinned. NextUI is the right
// choice: these tests encode NextUI's folder names ("Game Boy (GB)") and its
// save/state layout, which is exactly the behaviour they exist to protect.
func TestMain(m *testing.M) {
	firmware.SetActive(firmware.ForTest(firmware.KindNextUI, ""))
	os.Exit(m.Run())
}
