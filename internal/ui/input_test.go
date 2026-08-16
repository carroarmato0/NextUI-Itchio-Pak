//go:build !headless

package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/veandco/go-sdl2/sdl"
)

// Screens compare against btnA/btnB/btnX/btnY rather than SDL constants, so
// getting this table wrong inverts confirm and cancel on a whole platform.
func TestSetFaceMappingBindsEachArrangement(t *testing.T) {
	t.Cleanup(func() { SetFaceMapping(firmware.FaceSwapped) })

	for _, tc := range []struct {
		mapping      firmware.FaceMapping
		wantA, wantB uint8
		wantX, wantY uint8
	}{
		{firmware.FaceSwapped, sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_Y, sdl.CONTROLLER_BUTTON_X},
		{firmware.FaceDirect, sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_X, sdl.CONTROLLER_BUTTON_Y},
		{firmware.FaceABDirect, sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_Y, sdl.CONTROLLER_BUTTON_X},
	} {
		SetFaceMapping(tc.mapping)
		if btnA != tc.wantA || btnB != tc.wantB || btnX != tc.wantX || btnY != tc.wantY {
			t.Errorf("%s: got A=%d B=%d X=%d Y=%d, want A=%d B=%d X=%d Y=%d",
				tc.mapping, btnA, btnB, btnX, btnY, tc.wantA, tc.wantB, tc.wantX, tc.wantY)
		}
	}
}

// An unrecognised value must not leave the bindings half-set: every other
// NextUI device is swapped, so that is the safe answer.
func TestSetFaceMappingFallsBackToSwapped(t *testing.T) {
	t.Cleanup(func() { SetFaceMapping(firmware.FaceSwapped) })

	SetFaceMapping(firmware.FaceMapping("nonsuch"))
	if btnA != sdl.CONTROLLER_BUTTON_B || btnX != sdl.CONTROLLER_BUTTON_Y {
		t.Errorf("unknown mapping gave A=%d X=%d, want swapped (%d, %d)",
			btnA, btnX, sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_Y)
	}
}
