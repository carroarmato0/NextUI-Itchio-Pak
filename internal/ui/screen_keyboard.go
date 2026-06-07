//go:build !headless

package ui

import (
	"time"
	"unicode/utf8"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

// kbGrid defines the 4×8 character grid for each keyboard page.
// 0=lowercase, 1=uppercase, 2=digits+symbols.
// No empty spacer cells — every position is interactive.
var kbGrid = [3][4][8]string{
	{ // page 0: lowercase a–z + common chars
		{"a", "b", "c", "d", "e", "f", "g", "h"},
		{"i", "j", "k", "l", "m", "n", "o", "p"},
		{"q", "r", "s", "t", "u", "v", "w", "x"},
		{"y", "z", ".", "@", ",", "SPC", "DEL", "OK"},
	},
	{ // page 1: uppercase A–Z + common chars
		{"A", "B", "C", "D", "E", "F", "G", "H"},
		{"I", "J", "K", "L", "M", "N", "O", "P"},
		{"Q", "R", "S", "T", "U", "V", "W", "X"},
		{"Y", "Z", ".", "@", ",", "SPC", "DEL", "OK"},
	},
	{ // page 2: digits + symbols
		{"0", "1", "2", "3", "4", "5", "6", "7"},
		{"8", "9", ".", "-", "_", "'", "!", "?"},
		{"@", "#", ":", ";", "(", ")", "+", "="},
		{"*", "/", "&", "%", "$", "SPC", "DEL", "OK"},
	},
}

var kbPageLabels = [3]string{"abc", "ABC", "0-9"}

// KeyboardScreen is a full-screen virtual keyboard.
// Pressing "OK" fires onConfirm(typed value) and returns prev.
// Pressing B fires onConfirm(seed) (cancel, unchanged value) and returns prev.
type KeyboardScreen struct {
	prev      Screen
	value     []rune
	seed      string
	onConfirm func(string)

	page int // 0=lower, 1=upper, 2=digits
	row  int // 0–3; -1 = text field focused
	col  int // 0–7

	blinkOn   bool
	lastBlink time.Time
}

// NewKeyboardScreen returns a KeyboardScreen pre-filled with seed.
// onConfirm is called with the result when the user confirms or cancels.
func NewKeyboardScreen(prev Screen, seed string, onConfirm func(string)) *KeyboardScreen {
	page := 0
	if len(seed) == 0 {
		page = 1 // start on uppercase when typing from scratch
	}
	return &KeyboardScreen{
		prev:      prev,
		value:     []rune(seed),
		seed:      seed,
		onConfirm: onConfirm,
		page:      page,
		row:       0,
		col:       0,
		blinkOn:   true,
		lastBlink: time.Now(),
	}
}

func (s *KeyboardScreen) NeedsRedraw() bool        { return true }
func (s *KeyboardScreen) HasPendingAnimation() bool { return false }

func (s *KeyboardScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	headerH := int32(48)
	footerH := int32(44)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Enter text", 12, textY, mt[0], mt[1], mt[2])

	// Page indicator right-aligned in header
	pageLabel := kbPageLabels[s.page]
	ht := r.Theme.HintText
	pw, _ := r.SmallTextSize(pageLabel)
	r.DrawSmallText(pageLabel, r.W-pw-12, textY, ht[0], ht[1], ht[2])

	// Blink update
	if time.Since(s.lastBlink) > 500*time.Millisecond {
		s.blinkOn = !s.blinkOn
		s.lastBlink = time.Now()
	}

	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	contentY := headerH + 6

	// Text input field
	fieldH := fontH + 16
	fieldX := int32(8)
	fieldW := r.W - 16

	r.DrawRect(fieldX, contentY, fieldW, fieldH, 25, 25, 38)
	var bR, bG, bB uint8
	if s.row == -1 {
		ac := r.Theme.Accent
		bR, bG, bB = ac[0], ac[1], ac[2]
	} else {
		bR, bG, bB = 60, 60, 80
	}
	r.DrawRect(fieldX, contentY, fieldW, 1, bR, bG, bB)
	r.DrawRect(fieldX, contentY+fieldH-1, fieldW, 1, bR, bG, bB)
	r.DrawRect(fieldX, contentY, 1, fieldH, bR, bG, bB)
	r.DrawRect(fieldX+fieldW-1, contentY, 1, fieldH, bR, bG, bB)

	displayText := string(s.value)
	if s.blinkOn {
		displayText += "▌"
	}
	r.DrawText(displayText, fieldX+8, contentY+(fieldH-fontH)/2, mt[0], mt[1], mt[2])

	contentY += fieldH + 4

	// Page tabs
	tabH := smallFH + 4
	tabW := r.W / 3
	for i, label := range kbPageLabels {
		tx := int32(i) * tabW
		if i == s.page {
			ac := r.Theme.Accent
			aT := r.Theme.AccentText
			r.DrawRect(tx, contentY, tabW, tabH, ac[0], ac[1], ac[2])
			r.DrawSmallTextCentered(label, tx, contentY+(tabH-smallFH)/2, tabW, aT[0], aT[1], aT[2])
		} else {
			r.DrawSmallTextCentered(label, tx, contentY+(tabH-smallFH)/2, tabW, 80, 80, 100)
		}
	}
	contentY += tabH + 4

	// Character grid
	cols := int32(8)
	rows := int32(4)
	margin := int32(4)
	availW := r.W - margin*2
	cellW := availW / cols
	availH := r.H - footerH - contentY - 4
	cellH := availH / rows

	for row := 0; row < 4; row++ {
		for col := 0; col < 8; col++ {
			ch := kbGrid[s.page][row][col]
			if ch == "" {
				continue
			}
			cx := margin + int32(col)*cellW
			cy := contentY + int32(row)*cellH
			isSelected := s.row == row && s.col == col

			var cellR, cellG, cellB uint8
			if isSelected {
				ac := r.Theme.Accent
				cellR, cellG, cellB = ac[0], ac[1], ac[2]
			} else {
				cellR, cellG, cellB = 28, 28, 42
			}
			r.DrawRect(cx+1, cy+1, cellW-2, cellH-2, cellR, cellG, cellB)

			var fR, fG, fB uint8
			if isSelected {
				aT := r.Theme.AccentText
				fR, fG, fB = aT[0], aT[1], aT[2]
			} else {
				fR, fG, fB = 190, 190, 210
			}
			r.DrawSmallTextCenteredInRect(ch, cx+1, cy+1, cellW-2, cellH-2, fR, fG, fB)
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Type/Confirm"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"},
		{Kind: renderer.BadgeCircle, Label: "X", Text: "Delete"},
		{Kind: renderer.BadgePill, Label: "L1R1", Text: "Shift"},
	}, ftrY)
	r.Present()
}

func (s *KeyboardScreen) HandleEvent(e sdl.Event) Screen {
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

func (s *KeyboardScreen) handleKey(sym sdl.Keycode) Screen {
	switch sym {
	case sdl.K_RETURN: // physical A
		return s.activate()
	case sdl.K_ESCAPE: // physical B — cancel
		if s.onConfirm != nil {
			s.onConfirm(s.seed)
		}
		return s.prev
	case sdl.K_UP:
		s.moveUp()
	case sdl.K_DOWN:
		s.moveDown()
	case sdl.K_LEFT:
		if s.row >= 0 {
			s.col = kbSkipLeft(s.page, s.row, s.col)
		}
	case sdl.K_RIGHT:
		if s.row >= 0 {
			s.col = kbSkipRight(s.page, s.row, s.col)
		}
	case sdl.K_PAGEUP: // L1 — previous page
		s.page = (s.page + 2) % 3
		s.clampCol()
	case sdl.K_PAGEDOWN: // R1 — next page
		s.page = (s.page + 1) % 3
		s.clampCol()
	case sdl.K_y: // physical X — delete last character
		if len(s.value) > 0 {
			s.value = s.value[:len(s.value)-1]
		}
	}
	return s
}

func (s *KeyboardScreen) handleButton(btn uint8) Screen {
	switch btn {
	case sdl.CONTROLLER_BUTTON_B: // physical A — type/confirm
		return s.activate()
	case sdl.CONTROLLER_BUTTON_A: // physical B — cancel
		if s.onConfirm != nil {
			s.onConfirm(s.seed)
		}
		return s.prev
	case sdl.CONTROLLER_BUTTON_DPAD_UP:
		s.moveUp()
	case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
		s.moveDown()
	case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
		if s.row >= 0 {
			s.col = kbSkipLeft(s.page, s.row, s.col)
		}
	case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
		if s.row >= 0 {
			s.col = kbSkipRight(s.page, s.row, s.col)
		}
	case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
		s.page = (s.page + 2) % 3
		s.clampCol()
	case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
		s.page = (s.page + 1) % 3
		s.clampCol()
	case sdl.CONTROLLER_BUTTON_Y: // physical X — delete last character
		if len(s.value) > 0 {
			s.value = s.value[:len(s.value)-1]
		}
	}
	return s
}

func (s *KeyboardScreen) activate() Screen {
	if s.row == -1 {
		// Text field focused — backspace on A
		if len(s.value) > 0 {
			s.value = s.value[:len(s.value)-1]
		}
		return s
	}
	ch := kbGrid[s.page][s.row][s.col]
	switch ch {
	case "OK":
		logger.Debug("keyboard: confirmed value len=%d", len(s.value))
		if s.onConfirm != nil {
			s.onConfirm(string(s.value))
		}
		return s.prev
	case "DEL":
		if len(s.value) > 0 {
			s.value = s.value[:len(s.value)-1]
		}
	case "SPC":
		s.value = append(s.value, ' ')
	default:
		r, size := utf8.DecodeRuneInString(ch)
		if size > 0 && r != utf8.RuneError {
			s.value = append(s.value, r)
			// Auto-switch from uppercase to lowercase after the very first character.
			if s.page == 1 && len(s.value) == 1 {
				s.page = 0
				s.clampCol()
			}
		}
	}
	return s
}

func (s *KeyboardScreen) moveUp() {
	if s.row <= 0 {
		s.row = -1
		return
	}
	s.row--
	s.clampCol()
}

func (s *KeyboardScreen) moveDown() {
	if s.row == -1 {
		s.row = 0
		s.clampCol()
		return
	}
	if s.row < 3 {
		s.row++
		s.clampCol()
	}
}

// clampCol moves col to the nearest non-empty cell on the current row/page.
func (s *KeyboardScreen) clampCol() {
	if s.row < 0 {
		return
	}
	if kbGrid[s.page][s.row][s.col] != "" {
		return
	}
	for d := 1; d < 8; d++ {
		if c := (s.col + d) % 8; kbGrid[s.page][s.row][c] != "" {
			s.col = c
			return
		}
		if c := (s.col - d + 8) % 8; kbGrid[s.page][s.row][c] != "" {
			s.col = c
			return
		}
	}
}

// kbSkipRight returns the column of the next non-empty cell to the right, wrapping.
func kbSkipRight(page, row, col int) int {
	for d := 1; d <= 8; d++ {
		c := (col + d) % 8
		if kbGrid[page][row][c] != "" {
			return c
		}
	}
	return col
}

// kbSkipLeft returns the column of the next non-empty cell to the left, wrapping.
func kbSkipLeft(page, row, col int) int {
	for d := 1; d <= 8; d++ {
		c := (col - d + 8) % 8
		if kbGrid[page][row][c] != "" {
			return c
		}
	}
	return col
}
