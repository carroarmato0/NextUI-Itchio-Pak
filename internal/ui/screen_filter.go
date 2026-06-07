//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

type filterSection int

const (
	filterSectionSearch   filterSection = iota
	filterSectionPlatform
	filterSectionSort
)

// filterPlatforms is the ordered list of platform codes shown in the overlay.
// "" is "All platforms".
var filterPlatforms = []string{"", "GB", "GBC", "GBA", "NES", "MD", "P8"}
var filterPlatformLabels = []string{"All", "GB", "GBC", "GBA", "NES", "MD", "P8"}

// filterSortValues and filterSortLabels define the available sort modes.
var filterSortValues = []string{"", "az", "za", "new", "free", "paid", "dl", "owned"}
var filterSortLabels = []string{"RSS", "A-Z", "Z-A", "New", "Free", "Paid", "DL", "Owned"}

// FilterScreen is a SELECT overlay showing Search / Platform / Sort sections.
// It wraps prev and calls onApply(platform, sort, query) on SELECT/apply.
// On B (cancel) it returns prev without calling onApply.
type FilterScreen struct {
	prev    Screen
	onApply func(platform, sort, query string)

	// working copies modified by the user; not committed until onApply fires
	platform string
	sort     string
	query    string

	section filterSection // which section has focus
	platCol int           // cursor within platform pill row
	sortCol int           // cursor within sort pill row
}

// NewFilterScreen constructs the filter overlay with the current active values.
// onApply is called when the user presses SELECT to apply.
func NewFilterScreen(
	prev Screen,
	platform, sort, query string,
	onApply func(platform, sort, query string),
) *FilterScreen {
	platCol := 0
	for i, p := range filterPlatforms {
		if p == platform {
			platCol = i
			break
		}
	}
	sortCol := 0
	for i, v := range filterSortValues {
		if v == sort {
			sortCol = i
			break
		}
	}
	return &FilterScreen{
		prev:     prev,
		onApply:  onApply,
		platform: platform,
		sort:     sort,
		query:    query,
		section:  filterSectionSearch,
		platCol:  platCol,
		sortCol:  sortCol,
	}
}

func (s *FilterScreen) NeedsRedraw() bool        { return false }
func (s *FilterScreen) HasPendingAnimation() bool { return false }

func (s *FilterScreen) Draw(r *renderer.Renderer) {
	lyt := LayoutFor(r.W, r.H)

	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	footerH := int32(44)

	// Panel bounds
	panelX := lyt.OverlayMarginX
	panelW := r.W - lyt.OverlayMarginX*2
	pad := int32(12)

	sectionGap := int32(10)
	labelH := smallFH + 2
	pillRowH := smallFH + 12
	fieldH := fontH + 12
	headerH := fontH + 16

	panelH := headerH + sectionGap +
		labelH + fieldH + sectionGap +
		labelH + pillRowH + sectionGap +
		labelH + pillRowH + sectionGap +
		footerH
	if panelH > r.H-20 {
		panelH = r.H - 20
	}
	panelY := (r.H - panelH) / 2

	// Panel fill + border
	r.DrawRect(panelX, panelY, panelW, panelH, bg[0]+22, bg[1]+22, bg[2]+22)
	r.DrawRect(panelX, panelY, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY+panelH-1, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY, 1, panelH, 70, 70, 100)
	r.DrawRect(panelX+panelW-1, panelY, 1, panelH, 70, 70, 100)

	// Panel title
	mt := r.Theme.MainText
	r.DrawTextCentered("Filter & Search", panelX, panelY+pad, panelW, mt[0], mt[1], mt[2])
	y := panelY + pad + fontH + pad

	// drawLabel renders a section label with accent colour when focused.
	drawLabel := func(label string, focused bool) {
		var lr, lg, lb uint8
		if focused {
			ac := r.Theme.Accent
			lr, lg, lb = ac[0], ac[1], ac[2]
		} else {
			lr, lg, lb = 160, 175, 200
		}
		r.DrawSmallText(label, panelX+pad, y, lr, lg, lb)
		y += labelH
	}

	// drawPillRow renders a row of selectable pills.
	// activeIdx is the currently active (applied) value.
	// cursorIdx is the currently focused pill (may differ from active).
	drawPillRow := func(labels []string, activeIdx, cursorIdx int, focused bool) {
		x := panelX + pad
		for i, label := range labels {
			isActive := i == activeIdx
			isCursor := focused && i == cursorIdx
			w, _ := r.SmallTextSize(label)
			pw := w + 12
			ph := smallFH + 6
			var bgR, bgG, bgB uint8
			var fgR, fgG, fgB uint8
			switch {
			case isActive && isCursor:
				ac := r.Theme.Accent
				bgR, bgG, bgB = ac[0], ac[1], ac[2]
				aT := r.Theme.AccentText
				fgR, fgG, fgB = aT[0], aT[1], aT[2]
			case isActive:
				ac := r.Theme.Accent
				bgR = ac[0]/2 + 20
				bgG = ac[1]/2 + 20
				bgB = ac[2]/2 + 20
				aT := r.Theme.AccentText
				fgR, fgG, fgB = aT[0], aT[1], aT[2]
			case isCursor:
				bgR, bgG, bgB = 50, 50, 70
				fgR, fgG, fgB = 200, 200, 220
			default:
				bgR, bgG, bgB = 30, 30, 45
				fgR, fgG, fgB = 120, 120, 140
			}
			r.DrawPill(x, y, pw, ph, bgR, bgG, bgB)
			r.DrawSmallTextCenteredInRect(label, x, y, pw, ph, fgR, fgG, fgB)
			x += pw + 4
		}
		y += pillRowH
	}

	// ── Search ──
	searchFocused := s.section == filterSectionSearch
	drawLabel("SEARCH", searchFocused)
	var sfR, sfG, sfB uint8
	if searchFocused {
		ac := r.Theme.Accent
		sfR, sfG, sfB = ac[0], ac[1], ac[2]
	} else {
		sfR, sfG, sfB = 60, 60, 80
	}
	r.DrawRect(panelX+pad, y, panelW-pad*2, fieldH, 22, 22, 35)
	r.DrawRect(panelX+pad, y, panelW-pad*2, 1, sfR, sfG, sfB)
	r.DrawRect(panelX+pad, y+fieldH-1, panelW-pad*2, 1, sfR, sfG, sfB)
	if s.query == "" {
		ht := r.Theme.HintText
		r.DrawText("(press A to type)", panelX+pad+6, y+(fieldH-fontH)/2, ht[0], ht[1], ht[2])
	} else {
		r.DrawText(s.query, panelX+pad+6, y+(fieldH-fontH)/2, mt[0], mt[1], mt[2])
	}
	y += fieldH + sectionGap

	// ── Platform ──
	platFocused := s.section == filterSectionPlatform
	drawLabel("PLATFORM", platFocused)
	platActiveIdx := 0
	for i, p := range filterPlatforms {
		if p == s.platform {
			platActiveIdx = i
			break
		}
	}
	drawPillRow(filterPlatformLabels, platActiveIdx, s.platCol, platFocused)
	y += sectionGap

	// ── Sort ──
	sortFocused := s.section == filterSectionSort
	drawLabel("SORT", sortFocused)
	sortActiveIdx := 0
	for i, v := range filterSortValues {
		if v == s.sort {
			sortActiveIdx = i
			break
		}
	}
	drawPillRow(filterSortLabels, sortActiveIdx, s.sortCol, sortFocused)

	// Footer hints
	ftrY := r.DrawFooterBar(footerH)
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select/Keyboard"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		{Kind: renderer.BadgePill, Label: "SELECT", Text: "Apply"},
	}
	if s.query != "" || s.platform != "" || s.sort != "" {
		hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "Y", Text: "Clear"})
	}
	r.DrawFooterHints(hints, ftrY)
	r.Present()
}

func (s *FilterScreen) HandleEvent(e sdl.Event) Screen {
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

func (s *FilterScreen) handleKey(sym sdl.Keycode) Screen {
	switch sym {
	case sdl.K_UP:
		s.moveSectionUp()
	case sdl.K_DOWN:
		s.moveSectionDown()
	case sdl.K_LEFT:
		s.movePillLeft()
	case sdl.K_RIGHT:
		s.movePillRight()
	case sdl.K_RETURN: // physical A
		return s.activate()
	case sdl.K_ESCAPE: // physical B — cancel
		logger.Debug("filter: cancelled")
		return s.prev
	case sdl.K_TAB: // SELECT — apply
		return s.applyAndClose()
	case sdl.K_y: // Y — clear all
		s.clearAll()
	}
	return s
}

func (s *FilterScreen) handleButton(btn uint8) Screen {
	switch btn {
	case sdl.CONTROLLER_BUTTON_DPAD_UP:
		s.moveSectionUp()
	case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
		s.moveSectionDown()
	case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
		s.movePillLeft()
	case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
		s.movePillRight()
	case sdl.CONTROLLER_BUTTON_B: // physical A — select/activate
		return s.activate()
	case sdl.CONTROLLER_BUTTON_A: // physical B — cancel
		logger.Debug("filter: cancelled")
		return s.prev
	case sdl.CONTROLLER_BUTTON_BACK: // SELECT — apply
		return s.applyAndClose()
	case sdl.CONTROLLER_BUTTON_Y: // Y — clear all
		s.clearAll()
	}
	return s
}

func (s *FilterScreen) moveSectionUp() {
	if s.section > filterSectionSearch {
		s.section--
	}
}

func (s *FilterScreen) moveSectionDown() {
	if s.section < filterSectionSort {
		s.section++
	}
}

func (s *FilterScreen) movePillLeft() {
	switch s.section {
	case filterSectionPlatform:
		if s.platCol > 0 {
			s.platCol--
		}
	case filterSectionSort:
		if s.sortCol > 0 {
			s.sortCol--
		}
	}
}

func (s *FilterScreen) movePillRight() {
	switch s.section {
	case filterSectionPlatform:
		if s.platCol < len(filterPlatforms)-1 {
			s.platCol++
		}
	case filterSectionSort:
		if s.sortCol < len(filterSortValues)-1 {
			s.sortCol++
		}
	}
}

func (s *FilterScreen) activate() Screen {
	switch s.section {
	case filterSectionSearch:
		return NewKeyboardScreen(s, s.query, func(result string) {
			s.query = result
		})
	case filterSectionPlatform:
		s.platform = filterPlatforms[s.platCol]
	case filterSectionSort:
		s.sort = filterSortValues[s.sortCol]
	}
	return s
}

func (s *FilterScreen) applyAndClose() Screen {
	logger.Info("filter: applying platform=%q sort=%q query=%q", s.platform, s.sort, s.query)
	if s.onApply != nil {
		s.onApply(s.platform, s.sort, s.query)
	}
	return s.prev
}

func (s *FilterScreen) clearAll() {
	s.platform = ""
	s.sort = ""
	s.query = ""
	s.platCol = 0
	s.sortCol = 0
	logger.Debug("filter: cleared all")
}
