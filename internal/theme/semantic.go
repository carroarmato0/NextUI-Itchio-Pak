package theme

// Semantic colours: meanings rather than palette slots.
//
// A NextUI palette carries seven colours and none of them means "this download
// failed". Deriving status colours from the palette would collapse them — on
// MinUI, which is white, grey and black, success and error would become the
// same grey and the app would lose the distinction it most needs to make.
//
// So the hues are fixed and only their tone adapts. Green always means success,
// which keeps the app legible to anyone who has used it before, while Tone
// guarantees the colour stays readable against whatever background the palette
// sets. On a dark theme Tone is the identity function, so the app's existing
// look is preserved exactly.
var (
	baseSuccess = [3]uint8{0x50, 0xC8, 0x50} // 80,200,80
	baseError   = [3]uint8{0xC8, 0x3C, 0x3C} // 200,60,60
	baseWarning = [3]uint8{0xF0, 0xB4, 0x3C} // 240,180,60
	baseInfo    = [3]uint8{0x50, 0xC8, 0xDC} // 80,200,220 — the "DL" cyan
	baseOwned   = [3]uint8{0x3C, 0xC8, 0x78} // 60,200,120 — owned, distinct from free
)

const (
	// toneStep is how far each attempt pushes a hue toward the extreme.
	toneStep = 12
	// toneMaxPct bounds the loop so a hue can never be washed out completely.
	toneMaxPct = 96
	// statusBlend is how much of a status hue is mixed into the background to
	// make a tinted row or badge backing.
	statusBlend = 18
)

// Tone adapts a fixed semantic hue so it stays readable on this theme's
// background: pushed toward white on a dark theme, toward black on a light one,
// stopping as soon as it clears minContrast.
//
// Returns c unchanged when it already reads well, which is the case for every
// hue on a dark background — so this is the identity function on the app's own
// default theme and on most NextUI palettes.
func (t Theme) Tone(c [3]uint8) [3]uint8 {
	if Contrast(c, t.Background) >= minContrast {
		return c
	}
	target := [3]uint8{0xFF, 0xFF, 0xFF}
	if t.IsLightTheme() {
		target = [3]uint8{0x00, 0x00, 0x00}
	}
	for pct := toneStep; pct <= toneMaxPct; pct += toneStep {
		if out := Mix(c, target, pct); Contrast(out, t.Background) >= minContrast {
			return out
		}
	}
	return Mix(c, target, toneMaxPct)
}

// Success is the colour for a completed download, an enabled setting, a healthy
// state.
func (t Theme) Success() [3]uint8 { return t.Tone(baseSuccess) }

// Error is the colour for a failure, a removed game, a rejected request.
func (t Theme) Error() [3]uint8 { return t.Tone(baseError) }

// Warning is the colour for an available update, a paid item, a caution.
func (t Theme) Warning() [3]uint8 { return t.Tone(baseWarning) }

// Info is the colour for an in-progress download or a neutral notice.
func (t Theme) Info() [3]uint8 { return t.Tone(baseInfo) }

// Muted is de-emphasised text: secondary labels, "(not set)", disabled rows.
// On the app's default theme this is #787878, the grey those sites hardcoded.
func (t Theme) Muted() [3]uint8 { return Mix(t.Background, t.ListText, 50) }

// SuccessBG, ErrorBG and WarningBG tint the background for a whole row or card
// without shouting. Kept subtle deliberately: the status colour itself carries
// the meaning, and these only need to group it.
func (t Theme) SuccessBG() [3]uint8 { return Mix(t.Background, t.Success(), statusBlend) }
func (t Theme) ErrorBG() [3]uint8   { return Mix(t.Background, t.Error(), statusBlend) }
func (t Theme) WarningBG() [3]uint8 { return Mix(t.Background, t.Warning(), statusBlend) }

// ErrorText is Error softened toward the body-text colour, for the multi-line
// message under an error heading. Full-strength red over several wrapped lines
// is tiring to read, but dropping the red entirely loses the signal.
func (t Theme) ErrorText() [3]uint8 { return Mix(t.Error(), t.MainText, 45) }

// Owned marks a game the user owns but has not downloaded. Deliberately a
// different green from Success so the two badges stay tellable apart.
func (t Theme) Owned() [3]uint8 { return t.Tone(baseOwned) }

// Shadow returns a drop-shadow tone for a coloured pill. Shadows go toward black
// regardless of theme — a shadow that lightens is not a shadow.
func Shadow(c [3]uint8) [3]uint8 { return Mix(c, [3]uint8{0, 0, 0}, 35) }

// ProgressTrack is the unfilled part of a progress bar.
func (t Theme) ProgressTrack() [3]uint8 { return t.Shade(t.Surface(), 2) }

// ProgressFill is the filled part.
func (t Theme) ProgressFill() [3]uint8 { return t.Success() }
