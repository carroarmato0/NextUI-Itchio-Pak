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
// SDL names face buttons by position, using the Xbox arrangement, while these
// handhelds print Nintendo labels on the shell — so on a retro layout the
// A-labelled button arrives as CONTROLLER_BUTTON_B. muOS lets the user switch
// to a modern layout, where the two line up instead.
//
// Screens should compare against these rather than the SDL constants, so a
// layout change moves every binding at once instead of inverting confirm and
// cancel throughout the app.
//
// Defaults are the retro layout, which is what NextUI presents and what muOS
// falls back to. Shoulder buttons, START, SELECT and the d-pad are unaffected.
var (
	btnA uint8 = sdl.CONTROLLER_BUTTON_B
	btnB uint8 = sdl.CONTROLLER_BUTTON_A
	btnX uint8 = sdl.CONTROLLER_BUTTON_Y
	btnY uint8 = sdl.CONTROLLER_BUTTON_X
)

// SetButtonLayout points the face-button bindings at the given layout. Called
// once at startup, before any screen handles an event.
func SetButtonLayout(layout firmware.ButtonLayout) {
	if layout == firmware.LayoutModern {
		btnA, btnB = sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B
		btnX, btnY = sdl.CONTROLLER_BUTTON_X, sdl.CONTROLLER_BUTTON_Y
	} else {
		btnA, btnB = sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A
		btnX, btnY = sdl.CONTROLLER_BUTTON_Y, sdl.CONTROLLER_BUTTON_X
	}
	logger.Info("input: %s layout — A=SDL_%s B=SDL_%s", layout,
		sdlFaceButtonName(btnA), sdlFaceButtonName(btnB))
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
