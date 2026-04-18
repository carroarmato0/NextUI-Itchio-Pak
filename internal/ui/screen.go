//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

// Screen is implemented by every UI screen.
// Draw renders the current frame.
// HandleEvent processes one SDL event and returns the next screen.
// Returning nil exits the application. Returning self means no transition.
type Screen interface {
	Draw(r *renderer.Renderer)
	HandleEvent(e sdl.Event) Screen
}
