# API Key Help Overlay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show an opaque centered overlay panel with explanation text and a QR code when the user presses A on "API Key: (not set)" in Settings.

**Architecture:** All changes live in `SettingsScreen`. A `showAPIKeyHelp bool` flag gates an overlay drawing path inside `Draw()` and swallows all input in `HandleEvent()` while visible. The QR texture is generated lazily on first show and destroyed on dismiss. No new screen types, no renderer changes.

**Tech Stack:** Go, SDL2 via go-sdl2, internal/renderer (existing `DrawRect`, `DrawText`, `DrawWrappedText`, `DrawTextureAt`, `QRTexture`, `WrapText`), internal/logger.

---

## Note on tests

The entire `internal/ui` package is excluded from headless CI (`//go:build !headless`). There are no unit tests for any screen, and this feature — pure SDL2 drawing — cannot be tested headlessly. Verification is compilation + visual inspection via the native build.

---

## File map

- **Modify only:** `internal/ui/screen_settings.go`

---

### Task 1: Add package-level constants and new struct fields

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 1: Add two package-level constants below the existing `const` block (after line ~40)**

  Add below the `sItemCount` iota block:

  ```go
  const (
  	apiKeySetupURL  = "https://github.com/carroarmato0/NextUI-Itchio-Pak#adding-the-key-to-the-pak"
  	apiKeySetupBody = "Paid games require an Itch.io API key. Scan the QR code below for instructions on how to add it."
  )
  ```

- [ ] **Step 2: Add two fields to `SettingsScreen` struct**

  In the `SettingsScreen` struct (currently ending with `lastRepeat time.Time`), add:

  ```go
  	showAPIKeyHelp bool
  	apiKeyHelpQR   *sdl.Texture
  ```

- [ ] **Step 3: Build to verify it compiles**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: build succeeds, binary present in `bin/`.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/ui/screen_settings.go
  git commit -m "feat(settings): add API key help overlay fields and constants"
  ```

---

### Task 2: Open the overlay from `activate()`

**Files:**
- Modify: `internal/ui/screen_settings.go` — `activate()` method (around line 400)

- [ ] **Step 1: Add a case for `sItemAPIKey` in the `activate()` switch**

  The existing `switch s.cursor` in `activate()` has no case for `sItemAPIKey`. Add one at the top of the switch body (before `sItemROMMode`):

  ```go
  case sItemAPIKey:
  	if s.cfg.APIKey == "" {
  		s.showAPIKeyHelp = true
  		logger.Info("settings: API key help overlay shown")
  	}
  ```

- [ ] **Step 2: Build to verify it compiles**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: build succeeds.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/ui/screen_settings.go
  git commit -m "feat(settings): open API key help overlay when key is not configured"
  ```

---

### Task 3: Dismiss the overlay from `HandleEvent()`

**Files:**
- Modify: `internal/ui/screen_settings.go` — `HandleEvent()` method (around line 309)

- [ ] **Step 1: Add an early-return guard at the top of `HandleEvent()`**

  Insert this block as the first thing inside `func (s *SettingsScreen) HandleEvent(e sdl.Event) Screen {`, before the outer `switch ev := e.(type)`:

  ```go
  if s.showAPIKeyHelp {
  	dismiss := false
  	switch ev := e.(type) {
  	case *sdl.KeyboardEvent:
  		dismiss = ev.Type == sdl.KEYDOWN
  	case *sdl.ControllerButtonEvent:
  		dismiss = ev.Type == sdl.CONTROLLERBUTTONDOWN
  	}
  	if dismiss {
  		s.showAPIKeyHelp = false
  		if s.apiKeyHelpQR != nil {
  			s.apiKeyHelpQR.Destroy()
  			s.apiKeyHelpQR = nil
  		}
  		logger.Debug("settings: API key help overlay dismissed")
  	}
  	return s
  }
  ```

- [ ] **Step 2: Build to verify it compiles**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: build succeeds.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/ui/screen_settings.go
  git commit -m "feat(settings): dismiss API key help overlay on any button press"
  ```

---

### Task 4: Update footer hint for empty API key row

**Files:**
- Modify: `internal/ui/screen_settings.go` — `Draw()` method, footer hints block (around line 281)

- [ ] **Step 1: Expand the existing footer hint condition**

  The current code reads:

  ```go
  if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
  	hints[1].Text = "Test API key"
  }
  ```

  Replace it with:

  ```go
  if s.cursor == sItemAPIKey {
  	if s.cfg.APIKey != "" {
  		hints[1].Text = "Test API key"
  	} else {
  		hints[1].Text = "Setup guide"
  	}
  }
  ```

- [ ] **Step 2: Build to verify it compiles**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: build succeeds.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/ui/screen_settings.go
  git commit -m "feat(settings): show 'Setup guide' hint when API key is not configured"
  ```

---

### Task 5: Implement `drawAPIKeyHelpOverlay()` and wire into `Draw()`

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 1: Add the `drawAPIKeyHelpOverlay` method**

  Add this new method at the end of `screen_settings.go`, after the `activate()` method:

  ```go
  func (s *SettingsScreen) drawAPIKeyHelpOverlay(r *renderer.Renderer) {
  	_, fontH := r.TextSize("Ag")
  	_, smallFH := r.SmallTextSize("Ag")
  	lineH := fontH + 4

  	pad := int32(20)
  	panelW := r.W * 6 / 10
  	bodyMaxW := panelW - pad*2

  	// Pre-measure body text to correctly size the panel.
  	bodyLines := r.WrapText(apiKeySetupBody, bodyMaxW)
  	bodyH := int32(len(bodyLines)) * lineH

  	// QR size: 40% of panel width, clamped to [80, 160].
  	qrSize := panelW * 4 / 10
  	if qrSize > 160 {
  		qrSize = 160
  	}
  	if qrSize < 80 {
  		qrSize = 80
  	}

  	panelH := pad + fontH + 8 + bodyH + 12 + qrSize + 6 + smallFH + pad
  	panelX := (r.W - panelW) / 2
  	panelY := (r.H - panelH) / 2

  	// 2px accent-coloured border drawn as a slightly larger rect behind the panel.
  	ac := r.Theme.Accent
  	r.DrawRect(panelX-2, panelY-2, panelW+4, panelH+4, ac[0], ac[1], ac[2])

  	// Solid panel background.
  	bg := r.Theme.Background
  	r.DrawRect(panelX, panelY, panelW, panelH, bg[0], bg[1], bg[2])

  	// Title.
  	mt := r.Theme.MainText
  	y := panelY + pad
  	r.DrawTextCentered("API Key Setup", panelX, y, panelW, mt[0], mt[1], mt[2])
  	y += fontH + 8

  	// Body text.
  	ht := r.Theme.HintText
  	r.DrawWrappedText(apiKeySetupBody, panelX+pad, y, bodyMaxW, lineH, ht[0], ht[1], ht[2])
  	y += bodyH + 12

  	// QR code — lazily generated and cached for the lifetime of the overlay.
  	if s.apiKeyHelpQR == nil {
  		tex, err := r.QRTexture(apiKeySetupURL, int(qrSize))
  		if err != nil {
  			logger.Warn("settings: API key help QR generation failed: %v", err)
  		} else {
  			s.apiKeyHelpQR = tex
  			logger.Debug("settings: API key help QR texture generated")
  		}
  	}
  	if s.apiKeyHelpQR != nil {
  		qrX := panelX + (panelW-qrSize)/2
  		r.DrawTextureAt(s.apiKeyHelpQR, qrX, y, qrSize, qrSize)
  		y += qrSize + 6
  	}

  	// Caption.
  	r.DrawSmallTextCentered("Scan to open setup guide", panelX, y, panelW, 120, 120, 120)
  }
  ```

- [ ] **Step 2: Wire the overlay into `Draw()` — call it before `r.Present()`**

  In `Draw()`, find the existing last two lines:

  ```go
  	r.DrawFooterHints(hints, ftrY)
  	r.Present()
  ```

  Replace them with:

  ```go
  	r.DrawFooterHints(hints, ftrY)
  	if s.showAPIKeyHelp {
  		s.drawAPIKeyHelpOverlay(r)
  	}
  	r.Present()
  ```

- [ ] **Step 3: Build to verify it compiles**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: build succeeds with no errors.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/ui/screen_settings.go
  git commit -m "feat(settings): draw API key help overlay with QR code when key is not configured"
  ```

---

### Task 6: Visual verification

**Files:** none

- [ ] **Step 1: Run the test suite to confirm nothing is broken**

  ```bash
  ./scripts/test.sh
  ```

  Expected: all tests pass (the UI package is headless-excluded; only non-SDL2 packages run).

- [ ] **Step 2: Take a screenshot of the Settings screen with no API key**

  ```bash
  ./scripts/dev-screenshot.sh --screen settings --out-dir /tmp/itchio-screenshots
  ```

  Verify in `/tmp/itchio-screenshots/`:
  - The "API Key: (not set)" row shows "Setup guide" as the A-button footer hint
  - No regression in other settings rows

- [ ] **Step 3: Take a screenshot of the overlay**

  ```bash
  ./scripts/dev-screenshot.sh --screen apikey-help --out-dir /tmp/itchio-screenshots
  ```

  Verify:
  - Centered panel with accent-coloured border
  - "API Key Setup" title
  - Body text wrapped within panel bounds
  - QR code visible and centred
  - "Scan to open setup guide" caption below QR

  If the screenshot script does not support `--screen apikey-help`, navigate to Settings manually on a device or emulator and confirm visually.
