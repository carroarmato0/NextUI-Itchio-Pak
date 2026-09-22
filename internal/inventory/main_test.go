package inventory

import (
	"os"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
)

// Save and save-state migration is gated on a firmware capability, and these
// tests encode NextUI's layout — its folder display names, its Saves directory
// beside Roms, its .media cover art. Pinning NextUI keeps them testing that
// behaviour rather than silently passing because the feature switched itself off.
func TestMain(m *testing.M) {
	firmware.SetActive(firmware.ForTest(firmware.KindNextUI, ""))
	os.Exit(m.Run())
}
