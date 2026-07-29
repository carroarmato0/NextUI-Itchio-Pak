package theme

// Semantic colours: meanings rather than palette slots.
//
// A NextUI palette carries seven colours and none of them means "this download
// failed". Deriving status colours from the palette would collapse them — on
// MinUI, which is white, grey and black, success and error would become the
// same grey and the app would lose the distinction it most needs to make.
//
// So the hues are fixed and only their treatment varies. Green always means
// success, which keeps the app legible to anyone who has used it before.
//
// The treatment splits by how much the colour needs to be noticed:
//
//   - Routine badges (Success, Info, Owned, Price) are blended toward the
//     palette background so they belong to the theme. Before this they drew the
//     same #50C850 on every one of the eighteen shipped palettes, as loud on
//     Catppuccin Latte's near-white as on Forest Lime's dark green.
//   - Alerting colours (Warning, Error) are not blended at all. Rendering the
//     full matrix showed why: blended 50% toward a dark background, Error
//     becomes #763939, which reads as disabled rather than broken.
//
// Muting the routine badges also sharpens the alerts, since amber and red end up
// the only saturated things on screen.
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
	// themedBlend is how far a routine status hue is pulled toward the palette
	// background so it reads as part of the theme. Measured against all eighteen
	// shipped palettes: below this the badge still looks pasted on, and by 50%
	// the hue has lost its identity.
	themedBlend = 30
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

// themed pulls a fixed hue toward the palette background so it belongs to the
// theme, then lets Tone restore contrast if the blend went too far. Because Tone
// is the corrective step, no palette can blend a status colour into illegibility.
//
// Applied to the routine badges only. Without it every palette drew the same
// #50C850 — as loud on Catppuccin Latte's near-white as on Forest Lime's dark
// green, belonging to neither.
func (t Theme) themed(c [3]uint8) [3]uint8 {
	return t.Tone(Mix(c, t.Background, themedBlend))
}

// Success is the colour for a completed download, an enabled setting, a healthy
// state. Routine, so it adapts to the palette.
func (t Theme) Success() [3]uint8 { return t.themed(baseSuccess) }

// Info is the colour for an in-progress download or a neutral notice. Routine.
func (t Theme) Info() [3]uint8 { return t.themed(baseInfo) }

// Price marks a paid item. Descriptive rather than actionable — "costs $4.99" is
// not a call to action — so it adapts rather than competing with Warning.
func (t Theme) Price() [3]uint8 { return t.themed(baseWarning) }

// Error is the colour for a failure, a removed game, a rejected request.
//
// Deliberately NOT blended. Blending measurably destroys it: at 50% toward a
// dark background this becomes #763939, which reads as disabled rather than
// broken. An alert has to compete with the theme, not join it.
func (t Theme) Error() [3]uint8 { return t.Tone(baseError) }

// Warning is the colour for an available update or a caution the user should
// act on. Full strength for the same reason as Error.
func (t Theme) Warning() [3]uint8 { return t.Tone(baseWarning) }

// Muted is de-emphasised text: secondary labels, "(not set)", disabled rows.
// On the app's default theme this is #787878, the grey those sites hardcoded.
func (t Theme) Muted() [3]uint8 { return Mix(t.Background, t.ListText, 50) }

// SuccessBG, ErrorBG and WarningBG tint the background for a whole row or card
// without shouting. Kept subtle deliberately: the status colour itself carries
// the meaning, and these only need to group it.
func (t Theme) SuccessBG() [3]uint8 { return Mix(t.Background, t.Success(), statusBlend) }
func (t Theme) ErrorBG() [3]uint8   { return Mix(t.Background, t.Error(), statusBlend) }
func (t Theme) WarningBG() [3]uint8 { return Mix(t.Background, t.Warning(), statusBlend) }

// PriceBG backs a price pill.
func (t Theme) PriceBG() [3]uint8 { return Mix(t.Background, t.Price(), statusBlend) }

// ErrorText is Error softened toward the body-text colour, for the multi-line
// message under an error heading. Full-strength red over several wrapped lines
// is tiring to read, but dropping the red entirely loses the signal.
func (t Theme) ErrorText() [3]uint8 { return Mix(t.Error(), t.MainText, 45) }

// Owned marks a game the user owns but has not downloaded. Deliberately a
// different green from Success so the two badges stay tellable apart. Routine.
func (t Theme) Owned() [3]uint8 { return t.themed(baseOwned) }

// Shadow returns a drop-shadow tone for a coloured pill. Shadows go toward black
// regardless of theme — a shadow that lightens is not a shadow.
func Shadow(c [3]uint8) [3]uint8 { return Mix(c, [3]uint8{0, 0, 0}, 35) }

// ProgressTrack is the unfilled part of a progress bar.
func (t Theme) ProgressTrack() [3]uint8 { return t.Shade(t.Surface(), 2) }

// ProgressFill is the filled part.
func (t Theme) ProgressFill() [3]uint8 { return t.Success() }
