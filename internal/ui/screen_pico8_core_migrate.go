//go:build !headless

package ui

import (
	"fmt"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type pico8MigrateState int32

const (
	pico8StateConfirm  pico8MigrateState = iota // waiting for user confirmation
	pico8StateMigrating                          // migration running in background
	pico8StateDone                               // migration complete
	pico8StateError                              // migration failed
)

// Pico8CoreMigrateScreen asks the user to confirm a Pico-8 core switch and
// then migrates all downloaded Pico-8 games to the new directory with a live
// progress indicator.
type Pico8CoreMigrateScreen struct {
	cfg      *settings.Config
	cfgPath  string
	inv      *inventory.Inventory
	invPath  string
	prev     Screen
	oldCore  string
	newCore  string

	state    pico8MigrateState
	migrated int32 // games migrated so far (atomic)
	total    int32 // total games to migrate (atomic)
	err      error
}

func (s *Pico8CoreMigrateScreen) loadState() pico8MigrateState {
	return pico8MigrateState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *Pico8CoreMigrateScreen) storeState(st pico8MigrateState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}

// NewPico8CoreMigrateScreen creates the confirmation + migration screen.
// oldCore and newCore are "fakeo8" or "pico8".
func NewPico8CoreMigrateScreen(
	cfg *settings.Config, cfgPath string,
	inv *inventory.Inventory, invPath string,
	oldCore, newCore string,
	prev Screen,
) *Pico8CoreMigrateScreen {
	return &Pico8CoreMigrateScreen{
		cfg: cfg, cfgPath: cfgPath,
		inv: inv, invPath: invPath,
		prev: prev, oldCore: oldCore, newCore: newCore,
	}
}

func coreLabel(core string) string {
	if core == "pico8" {
		return "Pico-8 (official)"
	}
	return "FakeO8 (default)"
}

func (s *Pico8CoreMigrateScreen) NeedsRedraw() bool {
	return s.loadState() == pico8StateMigrating
}
func (s *Pico8CoreMigrateScreen) HasPendingAnimation() bool { return false }

func (s *Pico8CoreMigrateScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	headerH := int32(72)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Switch Pico-8 Core", 12, textY, mt[0], mt[1], mt[2])

	contentH := r.H - headerH - footerH
	mid := headerH + contentH/2
	ht := r.Theme.HintText

	switch s.loadState() {

	case pico8StateConfirm:
		// Destination core pill centred slightly above mid.
		ac := r.Theme.Accent
		aT := r.Theme.AccentText
		label := coreLabel(s.newCore)
		lw, _ := r.TextSize(label)
		pillW := lw + 24
		pillH := fontH + 6
		pillY := mid - pillH - smallFH*3 - 12
		r.DrawPill((r.W-pillW)/2, pillY, pillW, pillH, ac[0], ac[1], ac[2])
		r.DrawTextCenteredInRect(label, (r.W-pillW)/2, pillY, pillW, pillH, aT[0], aT[1], aT[2])

		// Warning text below the pill with enough room before the footer.
		warning := fmt.Sprintf(
			"Switching from %s to %s will move all your downloaded Pico-8 games to a new folder on the SD card.",
			coreLabel(s.oldCore), coreLabel(s.newCore),
		)
		warnY := pillY + pillH + 12
		r.DrawWrappedText(warning, 24, warnY, r.W-48, smallFH+4, ht[0], ht[1], ht[2])

	case pico8StateMigrating:
		// Simple title + dots — progress counter omitted because the
		// migration count is only known at completion, not per-game.
		r.DrawTextCentered("Migrating Pico-8 games", 0, mid-fontH-10, r.W, mt[0], mt[1], mt[2])
		drawLoadingDots(r, mid+8)

	case pico8StateDone:
		migrated := atomic.LoadInt32(&s.migrated)
		r.DrawTextCentered("Migration complete", 0, mid-fontH-4, r.W, 80, 200, 80)
		r.DrawSmallTextCentered(fmt.Sprintf("%d game(s) moved to %s", migrated, coreLabel(s.newCore)),
			0, mid+smallFH+4, r.W, ht[0], ht[1], ht[2])

	case pico8StateError:
		r.DrawTextCentered("Migration failed", 0, mid-fontH-smallFH-8, r.W, 200, 60, 60)
		if s.err != nil {
			r.DrawWrappedText(s.err.Error(), 20, mid-smallFH, r.W-40, smallFH+4, 200, 100, 100)
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.loadState() {
	case pico8StateConfirm:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "A", Text: "Confirm"},
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		}, ftrY)
	case pico8StateMigrating:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		}, ftrY)
	default:
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *Pico8CoreMigrateScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.UserEvent:
		_ = ev // migration goroutine pushes this on completion
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_RETURN: // physical A
			if s.loadState() == pico8StateConfirm {
				s.startMigration()
				return s
			}
			if s.loadState() == pico8StateDone || s.loadState() == pico8StateError {
				return s.prev
			}
		case sdl.K_ESCAPE: // physical B — cancel / back
			if s.loadState() == pico8StateConfirm || s.loadState() == pico8StateDone || s.loadState() == pico8StateError {
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_B: // physical A
			if s.loadState() == pico8StateConfirm {
				s.startMigration()
				return s
			}
			if s.loadState() == pico8StateDone || s.loadState() == pico8StateError {
				return s.prev
			}
		case sdl.CONTROLLER_BUTTON_A: // physical B — cancel / back
			if s.loadState() == pico8StateConfirm || s.loadState() == pico8StateDone || s.loadState() == pico8StateError {
				return s.prev
			}
		}
	}
	return s
}

func (s *Pico8CoreMigrateScreen) startMigration() {
	s.storeState(pico8StateMigrating)

	// Count how many Pico-8 games will be migrated so the progress shows X/N.
	oldDir := roms.Pico8ROMDir(s.oldCore)
	count := 0
	for _, url := range s.inv.AllURLs() {
		if entry, ok := s.inv.Lookup(url); ok {
			for _, f := range entry.Files {
				if len(f.DestPath) > len(oldDir) && f.DestPath[:len(oldDir)] == oldDir {
					count++
					break // one game, not per-file
				}
			}
		}
	}
	atomic.StoreInt32(&s.total, int32(count))

	go func() {
		defer func() { sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT}) }()

		newDir := roms.Pico8ROMDir(s.newCore)
		if err := inventory.MigratePico8Files(s.inv, s.invPath, oldDir, newDir); err != nil {
			logger.Warn("pico8-migrate: failed: %v", err)
			s.err = err
			s.storeState(pico8StateError)
			return
		}

		s.cfg.Pico8Core = s.newCore
		if err := s.cfg.Save(s.cfgPath); err != nil {
			logger.Warn("pico8-migrate: save config failed: %v", err)
		}
		logger.Info("pico8-migrate: switched to %s (%d game(s))", s.newCore, count)
		atomic.StoreInt32(&s.migrated, int32(count))
		s.storeState(pico8StateDone)
	}()
}
