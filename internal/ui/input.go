//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/veandco/go-sdl2/sdl"
)

// The SDL button each physically labelled face button produces.
//
// These are variables, not constants, because the relationship is not fixed.
// SDL names face buttons by position using the Xbox arrangement, while these
// handhelds print Nintendo labels — so the A-labelled button often arrives as
// CONTROLLER_BUTTON_B. Often, but not always: the same TrimUI pad reports
// opposite face buttons under NextUI and under muOS. See firmware.FaceMapping.
//
// Screens compare against these rather than the SDL constants, so the whole
// app follows the device instead of inverting confirm and cancel on one of them.
//
// Defaults are the swapped arrangement, which is what NextUI presents.
// Shoulder buttons, START, SELECT and the d-pad are unaffected.
var (
	btnA uint8 = sdl.CONTROLLER_BUTTON_B
	btnB uint8 = sdl.CONTROLLER_BUTTON_A
	btnX uint8 = sdl.CONTROLLER_BUTTON_Y
	btnY uint8 = sdl.CONTROLLER_BUTTON_X
)

// SetFaceMapping points the face-button bindings at the given arrangement.
// Called once at startup, before any screen handles an event.
func SetFaceMapping(m firmware.FaceMapping) {
	if m == firmware.FaceDirect {
		btnA, btnB = sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B
		btnX, btnY = sdl.CONTROLLER_BUTTON_X, sdl.CONTROLLER_BUTTON_Y
	} else {
		btnA, btnB = sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A
		btnX, btnY = sdl.CONTROLLER_BUTTON_Y, sdl.CONTROLLER_BUTTON_X
	}
	logger.Info("input: face buttons %s — A=SDL_%s B=SDL_%s X=SDL_%s Y=SDL_%s", m,
		sdlFaceButtonName(btnA), sdlFaceButtonName(btnB),
		sdlFaceButtonName(btnX), sdlFaceButtonName(btnY))
}

func sdlFaceButtonName(b uint8) string {
	switch b {
	case sdl.CONTROLLER_BUTTON_A:
		return "A"
	case sdl.CONTROLLER_BUTTON_B:
		return "B"
	case sdl.CONTROLLER_BUTTON_X:
		return "X"
	case sdl.CONTROLLER_BUTTON_Y:
		return "Y"
	default:
		return "?"
	}
}
