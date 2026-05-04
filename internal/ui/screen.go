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
// NeedsRedraw returns true when the screen has active time-based state
// (auto-repeat, scroll animation, progress indicator) that requires the
// render loop to keep calling Draw() without an incoming SDL event.
// HasPendingAnimation returns true when the screen has a timed animation
// that hasn't started yet (e.g. a scroll delay). The render loop uses a
// medium timeout (instead of blocking indefinitely) so the animation fires
// on schedule even when no other events arrive.
type Screen interface {
	Draw(r *renderer.Renderer)
	HandleEvent(e sdl.Event) Screen
	NeedsRedraw() bool
	HasPendingAnimation() bool
}

// BusyChecker is implemented by screens that must block safe power-off while
// an operation is in progress. The main event loop uses a type assertion —
// this is intentionally not part of the Screen interface.
type BusyChecker interface {
	IsBusy() bool
}
