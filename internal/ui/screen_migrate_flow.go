//go:build !headless

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/veandco/go-sdl2/sdl"
)

type migrateFlowState int

const (
	mfsCheckSave   migrateFlowState = iota // checking / prompting save
	mfsCheckStates                         // prompting state files
	mfsRunning                             // executing migration
	mfsDone                                // finished — show result
	mfsError                               // error
)

// MigrateFlowScreen guides the user through the unified-naming migration for
// a single DownloadedFile. It implements SaveDataCallback synchronously by
// collecting answers during the prompt states before calling MigrateFile.
type MigrateFlowScreen struct {
	inv           *inventory.Inventory
	inventoryPath string
	gameURL       string
	gameTitle     string
	file          inventory.DownloadedFile
	enable        bool
	formats       inventory.MigrateFormats
	prev          Screen

	state migrateFlowState

	// Prompt state — save
	saveExists   bool
	savePath     string
	newSavePath  string
	saveConflict bool // save exists at new path too
	saveAnswer   bool // true = rename, false = skip
	saveAsked    bool

	// Prompt state — overwrite guard
	overwriteAsked  bool
	overwriteAnswer bool

	// Prompt state — states
	existingStates []string
	statesAnswer   bool
	statesAsked    bool

	// Result
	result inventory.MigrateResult
	err    error
}

// NewMigrateFlowScreen creates a migration flow screen and immediately runs
// detection. Caller must push the returned screen onto the screen stack.
func NewMigrateFlowScreen(
	inv *inventory.Inventory,
	invPath string,
	gameURL, gameTitle string,
	file inventory.DownloadedFile,
	enable bool,
	formats inventory.MigrateFormats,
	prev Screen,
) *MigrateFlowScreen {
	s := &MigrateFlowScreen{
		inv: inv, inventoryPath: invPath,
		gameURL: gameURL, gameTitle: gameTitle,
		file: file, enable: enable, formats: formats, prev: prev,
	}
	s.detect()
	return s
}

// detect runs pre-flight checks to decide whether prompts are needed.
func (s *MigrateFlowScreen) detect() {
	currentPath := s.file.DestPath

	var targetPath string
	if s.enable {
		targetPath, _ = roms.ResolveUnifiedDest(currentPath, s.gameTitle, false)
	} else {
		targetPath = filepath.Join(filepath.Dir(currentPath), s.file.Filename)
	}

	var innerFilename string
	if s.formats.UseExtractedFileName && filepath.Ext(currentPath) == ".zip" {
		innerFilename = roms.ZipInnerFilename(currentPath)
	}

	oldSave := roms.SaveGamePath(currentPath, s.formats.SaveFormat, innerFilename)
	newSave := roms.SaveGamePath(targetPath, s.formats.SaveFormat, innerFilename)

	if oldSave != "" && oldSave != newSave {
		if _, err := os.Stat(oldSave); err == nil {
			s.saveExists = true
			s.savePath = oldSave
			s.newSavePath = newSave
			if _, err2 := os.Stat(newSave); err2 == nil {
				s.saveConflict = true
			}
		}
	}

	coreTag, coreName := roms.RomCoreInfo(currentPath)
	if coreTag != "" {
		allStates := roms.SaveStatePaths(currentPath, s.formats.StateFormat, innerFilename, coreTag, coreName)
		for _, sp := range allStates {
			if _, err := os.Stat(sp); err == nil {
				s.existingStates = append(s.existingStates, sp)
			}
		}
	}

	if s.saveExists {
		s.state = mfsCheckSave
	} else if len(s.existingStates) > 0 {
		s.state = mfsCheckStates
	} else {
		s.runMigration()
	}
}

// AskRenameExistingSave implements SaveDataCallback.
func (s *MigrateFlowScreen) AskRenameExistingSave(_ string) bool { return s.saveAnswer }

// AskOverwriteExistingSave implements SaveDataCallback.
func (s *MigrateFlowScreen) AskOverwriteExistingSave(_ string) bool { return s.overwriteAnswer }

// AskRenameExistingStates implements SaveDataCallback.
func (s *MigrateFlowScreen) AskRenameExistingStates(_ []string) bool { return s.statesAnswer }

func (s *MigrateFlowScreen) runMigration() {
	s.state = mfsRunning
	res, err := inventory.MigrateFile(
		s.inv, s.inventoryPath, s.gameURL, s.file,
		s.gameTitle, s.enable, s.formats, s,
	)
	if err != nil {
		logger.Error("migrate-flow: %v", err)
		s.err = err
		s.state = mfsError
	} else {
		s.result = res
		s.state = mfsDone
	}
}

func (s *MigrateFlowScreen) NeedsRedraw() bool         { return s.state == mfsRunning }
func (s *MigrateFlowScreen) HasPendingAnimation() bool { return false }

func (s *MigrateFlowScreen) Draw(r *renderer.Renderer) {
	bad := r.Theme.Error()
	badTx := r.Theme.ErrorText()
	mu := r.Theme.Muted()
	ok := r.Theme.Success()
	warn := r.Theme.Warning()
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	hdr := r.Theme.Surface()
	ac := r.Theme.Accent
	mt := r.Theme.MainText
	ht := r.Theme.HintText

	headerH := fontH + smallFH + 16
	r.DrawRect(0, 0, r.W, headerH, hdr[0], hdr[1], hdr[2])
	r.DrawRect(0, headerH, r.W, 2, ac[0], ac[1], ac[2])

	action := "Enabling"
	if !s.enable {
		action = "Disabling"
	}
	r.DrawText(truncateToWidth(r, action+" title filename — "+s.gameTitle, r.W-24), 12, (headerH-fontH)/2, mt[0], mt[1], mt[2])

	mid := headerH + (r.H-headerH-footerH)/2

	switch s.state {
	case mfsCheckSave:
		saveBase := s.savePath
		if len(saveBase) > 40 {
			saveBase = "…" + saveBase[len(saveBase)-40:]
		}
		r.DrawTextCentered("Save file detected", 0, mid-fontH*2, r.W, warn[0], warn[1], warn[2])
		r.DrawSmallTextCentered(saveBase, 0, mid-fontH, r.W, ht[0], ht[1], ht[2])
		if s.saveConflict && s.saveAsked {
			r.DrawSmallTextCentered("A save already exists at the new path.", 0, mid, r.W, badTx[0], badTx[1], badTx[2])
			r.DrawSmallTextCentered("Overwrite it?", 0, mid+smallFH+4, r.W, ht[0], ht[1], ht[2])
			ftrY := r.DrawFooterBar(footerH)
			r.DrawFooterHints([]renderer.FooterHint{
				{Kind: renderer.BadgeCircle, Label: "A", Text: "Overwrite"},
				{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
			}, ftrY)
		} else {
			r.DrawSmallTextCentered("Rename it to match the new ROM name?", 0, mid, r.W, ht[0], ht[1], ht[2])
			r.DrawSmallTextCentered("If you skip, your save will not load until renamed manually.", 0, mid+smallFH+4, r.W, mu[0], mu[1], mu[2])
			ftrY := r.DrawFooterBar(footerH)
			r.DrawFooterHints([]renderer.FooterHint{
				{Kind: renderer.BadgeCircle, Label: "A", Text: "Rename save"},
				{Kind: renderer.BadgeCircle, Label: "B", Text: "Skip"},
			}, ftrY)
		}

	case mfsCheckStates:
		r.DrawTextCentered("Save states detected", 0, mid-fontH*2, r.W, warn[0], warn[1], warn[2])
		r.DrawSmallTextCentered(fmt.Sprintf("%d save state(s) found.", len(s.existingStates)), 0, mid-fontH, r.W, ht[0], ht[1], ht[2])
		r.DrawSmallTextCentered("Rename them to match the new ROM name?", 0, mid, r.W, ht[0], ht[1], ht[2])
		r.DrawSmallTextCentered("If you skip, they will not load until renamed manually.", 0, mid+smallFH+4, r.W, mu[0], mu[1], mu[2])
		ftrY := r.DrawFooterBar(footerH)
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "A", Text: "Rename states"},
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Skip"},
		}, ftrY)

	case mfsRunning:
		r.DrawTextCentered("Renaming", 0, mid-fontH-10, r.W, mt[0], mt[1], mt[2])
		drawLoadingDots(r, mid+8)

	case mfsDone:
		r.DrawTextCentered("Done!", 0, mid-fontH, r.W, ok[0], ok[1], ok[2])
		summary := s.buildSummary()
		r.DrawSmallTextCentered(summary, 0, mid+4, r.W, ht[0], ht[1], ht[2])
		ftrY := r.DrawFooterBar(footerH)
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)

	case mfsError:
		msg := "Error"
		if s.err != nil {
			msg = s.err.Error()
		}
		r.DrawTextCentered("Migration failed", 0, mid-fontH, r.W, bad[0], bad[1], bad[2])
		r.DrawSmallTextCentered(msg, 0, mid+4, r.W, badTx[0], badTx[1], badTx[2])
		ftrY := r.DrawFooterBar(footerH)
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}

	r.Present()
}

func (s *MigrateFlowScreen) buildSummary() string {
	var parts []string
	if s.result.ROMRenamed {
		parts = append(parts, "ROM renamed")
	}
	if s.result.SaveRenamed {
		parts = append(parts, "save renamed")
	}
	if s.result.SaveSkipped {
		parts = append(parts, "save skipped")
	}
	if len(s.result.StatesRenamed) > 0 {
		parts = append(parts, fmt.Sprintf("%d state(s) renamed", len(s.result.StatesRenamed)))
	}
	if len(parts) == 0 {
		return "No changes needed."
	}
	return strings.Join(parts, ", ") + "."
}

func (s *MigrateFlowScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		return s.handleKey(ev.Keysym.Sym)
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		return s.handleButton(ev.Button)
	}
	return s
}

func (s *MigrateFlowScreen) handleKey(sym sdl.Keycode) Screen {
	switch s.state {
	case mfsCheckSave:
		switch sym {
		case sdl.K_RETURN: // B = Rename / Overwrite
			return s.handleSaveConfirm()
		case sdl.K_ESCAPE: // A = Skip / Cancel
			return s.handleSaveSkip()
		}
	case mfsCheckStates:
		switch sym {
		case sdl.K_RETURN: // B = Rename
			s.statesAnswer = true
		case sdl.K_ESCAPE: // A = Skip
			s.statesAnswer = false
		}
		s.runMigration()
	case mfsDone, mfsError:
		switch sym {
		case sdl.K_RETURN, sdl.K_ESCAPE:
			return s.prev
		}
	}
	return s
}

func (s *MigrateFlowScreen) handleButton(btn uint8) Screen {
	switch s.state {
	case mfsCheckSave:
		switch btn {
		case btnA: // physical A = Rename / Overwrite
			return s.handleSaveConfirm()
		case btnB: // physical B = Skip / Cancel
			return s.handleSaveSkip()
		}
	case mfsCheckStates:
		switch btn {
		case btnA: // Rename
			s.statesAnswer = true
		case btnB: // Skip
			s.statesAnswer = false
		}
		s.runMigration()
	case mfsDone, mfsError:
		switch btn {
		case btnA, btnB:
			return s.prev
		}
	}
	return s
}

func (s *MigrateFlowScreen) handleSaveConfirm() Screen {
	if s.saveConflict && !s.saveAsked {
		// First confirm press = user wants to rename; now ask about overwrite.
		s.saveAnswer = true
		s.saveAsked = true
		return s // stay on screen to show overwrite prompt
	}
	if s.saveConflict && s.saveAsked {
		// Second press = user confirmed overwrite.
		s.overwriteAnswer = true
	} else {
		s.saveAnswer = true
	}
	s.advanceFromSave()
	return s
}

func (s *MigrateFlowScreen) handleSaveSkip() Screen {
	if s.saveConflict && s.saveAsked {
		// Cancel overwrite → abort.
		s.overwriteAnswer = false
		s.advanceFromSave()
		return s
	}
	s.saveAnswer = false
	s.advanceFromSave()
	return s
}

func (s *MigrateFlowScreen) advanceFromSave() {
	if len(s.existingStates) > 0 {
		s.state = mfsCheckStates
	} else {
		s.runMigration()
	}
}
