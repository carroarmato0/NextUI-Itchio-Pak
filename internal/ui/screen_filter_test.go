//go:build !headless

package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
)

// The footer says Y clears; the physical Y button must be the one that does,
// under every face-button arrangement, and X must leave the input alone.
func TestFilterScreen_YClearsAndXDoesNot(t *testing.T) {
	t.Cleanup(func() { SetFaceMapping(firmware.FaceSwapped) })
	for _, m := range []firmware.FaceMapping{firmware.FaceSwapped, firmware.FaceDirect, firmware.FaceABDirect} {
		SetFaceMapping(m)

		s := NewFilterScreen(nil, "gbc", "az", "ninja", func(string, string, string) {})
		s.handleButton(btnX)
		if s.query != "ninja" || s.platform != "gbc" {
			t.Fatalf("%s: X cleared the filters (query=%q platform=%q)", m, s.query, s.platform)
		}
		s.handleButton(btnY)
		if s.query != "" || s.platform != "" || s.sort != "" {
			t.Fatalf("%s: Y did not clear (query=%q platform=%q sort=%q)", m, s.query, s.platform, s.sort)
		}
	}
}
