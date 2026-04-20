# Parental Advisory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-out parental advisory system that shows a full-screen "Grown-Ups Only" cover on game detail pages whose scraped tags match configurable Mature or Sensitive filter lists, with per-tag control for the Sensitive category.

**Architecture:** A new `advisory.go` file in `internal/itchio` holds the tag lists and a pure trigger-check function (no settings import). `settings.Config` gains a `Parental` sub-struct with defaults ON. The detail screen checks the trigger after loading and renders an overlay instead of normal content. Two new settings items wire up a direct Mature toggle and a new `SensitiveTagsScreen`.

**Tech Stack:** Go 1.22, SDL2 (via go-sdl2), existing renderer/settings/itchio packages.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/itchio/advisory.go` | Create | Tag lists + `IsAdvisoryTriggered` helper |
| `internal/itchio/advisory_test.go` | Create | Unit tests for trigger logic |
| `internal/settings/settings.go` | Modify | Add `ParentalAdvisory` struct + defaults |
| `internal/settings/settings_test.go` | Modify | Add round-trip + defaults tests |
| `internal/ui/screen_detail.go` | Modify | Overlay render path + `advisoryTriggered` field |
| `internal/ui/screen_settings.go` | Modify | Two new items + `SensitiveTagsScreen` navigation |
| `internal/ui/screen_sensitive_tags.go` | Create | Per-tag toggle list screen |
| `README.md` | Already done | — |

---

## Task 1: advisory.go — tag lists and trigger function

**Files:**
- Create: `internal/itchio/advisory.go`
- Create: `internal/itchio/advisory_test.go`

- [ ] **Step 1.1: Write the failing tests**

Create `internal/itchio/advisory_test.go`:

```go
package itchio_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestIsAdvisoryTriggered_EmptyTags(t *testing.T) {
	if itchio.IsAdvisoryTriggered(nil, true, true, nil) {
		t.Error("expected no trigger for nil tags")
	}
	if itchio.IsAdvisoryTriggered([]string{}, true, true, nil) {
		t.Error("expected no trigger for empty tags")
	}
}

func TestIsAdvisoryTriggered_MatureMatch(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"nsfw"}, true, false, nil) {
		t.Error("expected trigger for mature tag 'nsfw'")
	}
}

func TestIsAdvisoryTriggered_MatureCaseInsensitive(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"NSFW"}, true, false, nil) {
		t.Error("expected trigger for uppercase 'NSFW'")
	}
}

func TestIsAdvisoryTriggered_MatureDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw"}, false, false, nil) {
		t.Error("expected no trigger when mature filter disabled")
	}
}

func TestIsAdvisoryTriggered_SensitiveMatch(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"lgbtq"}, false, true, nil) {
		t.Error("expected trigger for sensitive tag 'lgbtq'")
	}
}

func TestIsAdvisoryTriggered_SensitiveDisabledMaster(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, false, false, nil) {
		t.Error("expected no trigger when sensitive filter disabled")
	}
}

func TestIsAdvisoryTriggered_SensitiveTagIndividuallyDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, false, true, []string{"lgbtq"}) {
		t.Error("expected no trigger when tag is in SensitiveDisabled")
	}
}

func TestIsAdvisoryTriggered_SensitiveOtherTagsStillActive(t *testing.T) {
	// "gay" is not in SensitiveDisabled, so it should still trigger
	if !itchio.IsAdvisoryTriggered([]string{"gay"}, false, true, []string{"lgbtq"}) {
		t.Error("expected trigger for 'gay' even when 'lgbtq' is individually disabled")
	}
}

func TestIsAdvisoryTriggered_BothFiltersOff(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw", "lgbtq"}, false, false, nil) {
		t.Error("expected no trigger when both filters disabled")
	}
}

func TestIsAdvisoryTriggered_NonFlaggedTag(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"platformer", "adventure"}, true, true, nil) {
		t.Error("expected no trigger for non-flagged tags")
	}
}
```

- [ ] **Step 1.2: Run tests to confirm they fail**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|advisory"
```

Expected: compile errors — `itchio.IsAdvisoryTriggered` undefined.

- [ ] **Step 1.3: Create `internal/itchio/advisory.go`**

```go
package itchio

import "strings"

// MatureTags is the hardcoded list of tag slugs considered mature content.
// Parents can enable/disable the whole category but cannot edit this list.
var MatureTags = []string{
	"adult", "boobs", "eroge", "erotic", "femdom", "gore",
	"hentai", "lewd", "nsfw", "nudity", "porn", "softcore",
	"tits", "titties", "xxx", "yaoi", "yuri",
}

// SensitiveTags is the hardcoded list of tag slugs considered sensitive topics.
// Parents can enable/disable the whole category and toggle individual tags.
// Sorted alphabetically.
var SensitiveTags = []string{
	"gay", "gender", "lesbian", "lgbtq", "sexy", "transgender",
}

// IsAdvisoryTriggered returns true if any tag in pageTags should trigger the
// parental advisory overlay. It takes the filter configuration as plain values
// so it does not depend on the settings package.
//
//   - matureEnabled:    whether the Mature Content filter is active
//   - sensitiveEnabled: whether the Sensitive Topics filter is active
//   - sensitiveDisabled: individual sensitive tags that are turned off
func IsAdvisoryTriggered(pageTags []string, matureEnabled, sensitiveEnabled bool, sensitiveDisabled []string) bool {
	for _, tag := range pageTags {
		slug := strings.ToLower(strings.TrimSpace(tag))
		if matureEnabled && containsStr(MatureTags, slug) {
			return true
		}
		if sensitiveEnabled && containsStr(SensitiveTags, slug) && !containsStr(sensitiveDisabled, slug) {
			return true
		}
	}
	return false
}

// containsStr reports whether list contains s.
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 1.4: Run tests and confirm they pass**

```bash
./scripts/test.sh 2>&1
```

Expected:
```
ok  	github.com/carroarmato0/nextui-itchio-pak/internal/itchio	...
ok  	github.com/carroarmato0/nextui-itchio-pak/internal/roms	...
ok  	github.com/carroarmato0/nextui-itchio-pak/internal/settings	...
```

- [ ] **Step 1.5: Commit**

```bash
git add internal/itchio/advisory.go internal/itchio/advisory_test.go
git commit -m "feat: add parental advisory tag lists and trigger function"
```

---

## Task 2: settings.go — ParentalAdvisory struct

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 2.1: Write the failing tests**

Add to `internal/settings/settings_test.go` (append after existing tests):

```go
func TestDefaultsHaveParentalAdvisoryEnabled(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Parental.MatureEnabled {
		t.Error("expected MatureEnabled=true by default")
	}
	if !cfg.Parental.SensitiveEnabled {
		t.Error("expected SensitiveEnabled=true by default")
	}
	if cfg.Parental.SensitiveDisabled != nil {
		t.Errorf("expected SensitiveDisabled=nil by default, got %v", cfg.Parental.SensitiveDisabled)
	}
}

func TestParentalAdvisoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	cfg := &Config{
		APIKey:       "",
		ROMSelection: "auto",
		Parental: ParentalAdvisory{
			MatureEnabled:     false,
			SensitiveEnabled:  true,
			SensitiveDisabled: []string{"lgbtq", "sexy"},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Parental.MatureEnabled != false {
		t.Error("MatureEnabled not preserved")
	}
	if loaded.Parental.SensitiveEnabled != true {
		t.Error("SensitiveEnabled not preserved")
	}
	if len(loaded.Parental.SensitiveDisabled) != 2 {
		t.Errorf("SensitiveDisabled: expected 2 entries, got %v", loaded.Parental.SensitiveDisabled)
	}
}
```

- [ ] **Step 2.2: Run tests to confirm they fail**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|Parental"
```

Expected: compile errors — `ParentalAdvisory` undefined.

- [ ] **Step 2.3: Update `internal/settings/settings.go`**

Replace the entire file:

```go
package settings

import (
	"encoding/json"
	"os"
)

// ParentalAdvisory holds the parental content filter configuration.
type ParentalAdvisory struct {
	MatureEnabled     bool     `json:"mature_enabled"`
	SensitiveEnabled  bool     `json:"sensitive_enabled"`
	SensitiveDisabled []string `json:"sensitive_disabled"` // tags individually turned off
}

type Config struct {
	APIKey       string           `json:"api_key"`
	ROMSelection string           `json:"rom_selection"`
	Parental     ParentalAdvisory `json:"parental"`
}

func defaults() *Config {
	return &Config{
		APIKey:       "",
		ROMSelection: "auto",
		Parental: ParentalAdvisory{
			MatureEnabled:    true,
			SensitiveEnabled: true,
		},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// File missing → return defaults (not an error)
		return defaults(), nil
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		// Corrupted file → return defaults (not an error)
		return defaults(), nil
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
```

- [ ] **Step 2.4: Run tests and confirm they pass**

```bash
./scripts/test.sh 2>&1
```

Expected: all packages pass.

- [ ] **Step 2.5: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat: add ParentalAdvisory config struct with ON defaults"
```

---

## Task 3: screen_detail.go — advisory overlay

**Files:**
- Modify: `internal/ui/screen_detail.go`

The overlay is rendered instead of normal content when `advisoryTriggered` is true.
`advisoryTriggered` is set once in the goroutine after `FetchGameDetail` returns.
The Start button is suppressed on the overlay — only B (go back) is available.

- [ ] **Step 3.1: Add `advisoryTriggered` field to `DetailScreen`**

In `screen_detail.go`, find the struct definition and add the field:

```go
type DetailScreen struct {
	client        *itchio.Client
	cfg           *settings.Config
	cfgPath       string
	cache         *renderer.ImageCache
	game          itchio.Game
	detail        *itchio.GameDetail
	loading       bool
	err           error
	screenshotIdx int
	scrollY       int32
	contentHeight int32
	viewportH     int32

	advisoryTriggered bool // true when a filter match is found after loading

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time

	prev Screen
}
```

- [ ] **Step 3.2: Set `advisoryTriggered` in the loading goroutine**

Find the goroutine in `NewDetailScreen` that calls `FetchGameDetail`. Replace the block that sets `s.detail` and `s.err` with:

```go
		s.detail = d
		s.err = err
		s.loading = false
		if d != nil && err == nil {
			s.advisoryTriggered = itchio.IsAdvisoryTriggered(
				d.PageTags,
				cfg.Parental.MatureEnabled,
				cfg.Parental.SensitiveEnabled,
				cfg.Parental.SensitiveDisabled,
			)
		}
```

Note: `cfg` is already captured in the goroutine closure via the `NewDetailScreen` parameter — verify it is passed as `cfg *settings.Config` and referenced correctly. Check the function signature:

```go
func NewDetailScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, game itchio.Game, prev Screen) *DetailScreen {
	s := &DetailScreen{client: client, cfg: cfg, cfgPath: cfgPath, cache: cache, game: game, prev: prev, loading: true}
	go func() {
		// ...
		d, err := client.FetchGameDetail(game.URL)
		// ...
		s.detail = d
		s.err = err
		s.loading = false
		if d != nil && err == nil {
			s.advisoryTriggered = itchio.IsAdvisoryTriggered(
				d.PageTags,
				cfg.Parental.MatureEnabled,
				cfg.Parental.SensitiveEnabled,
				cfg.Parental.SensitiveDisabled,
			)
		}
	}()
	return s
}
```

- [ ] **Step 3.3: Add `drawAdvisoryOverlay` method**

Add this method to `screen_detail.go` before the closing of the file:

```go
// drawAdvisoryOverlay renders the full-screen parental advisory cover.
// Only B (go back) is available — Start is suppressed.
func (s *DetailScreen) drawAdvisoryOverlay(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	cy := r.H / 2

	r.DrawTextCentered("[!]", 0, cy-90, r.W, 240, 180, 60)
	r.DrawTextCentered("Grown-Ups Only", 0, cy-54, r.W, 240, 180, 60)

	r.DrawRect(r.W/4, cy-28, r.W/2, 1, 60, 60, 60)

	_, lh := r.TextSize("Ag")
	if lh < 20 {
		lh = 20
	}
	r.DrawWrappedText(
		"This game may have content that is not suitable for all ages.",
		r.W/8, cy-16, r.W*3/4, lh+2, 180, 180, 180,
	)
	r.DrawWrappedText(
		"Please ask a parent or guardian before continuing.",
		r.W/8, cy-16+lh+6, r.W*3/4, lh+2, 180, 180, 180,
	)

	r.DrawRect(r.W/4, cy+60, r.W/2, 1, 60, 60, 60)
	r.DrawTextCentered("B  Go back", 0, cy+72, r.W, 180, 80, 80)
}
```

- [ ] **Step 3.4: Insert overlay check at the top of `Draw`**

Place this block as the very first thing in `Draw`, right after `s.processAutoScroll()` and
before `r.Clear(...)`. This skips all normal rendering when the overlay is active:

```go
func (s *DetailScreen) Draw(r *renderer.Renderer) {
	s.processAutoScroll()

	// ── Parental advisory overlay ────────────────────────────
	if !s.loading && s.err == nil && s.advisoryTriggered {
		s.drawAdvisoryOverlay(r)
		r.Present()
		return
	}

	r.Clear(colorBG, colorBG, colorBG)
	// ... rest of Draw unchanged
```

- [ ] **Step 3.5: Suppress Start button in `HandleEvent` when overlay is active**

In `HandleEvent`, find the keyboard case for `sdl.K_s` and the controller case for `sdl.CONTROLLER_BUTTON_START`. Wrap each in a guard:

For keyboard:
```go
			case sdl.K_s:
				if !s.advisoryTriggered {
					return NewSettingsScreen(s.cfg, s.cfgPath, s)
				}
```

For controller:
```go
			case sdl.CONTROLLER_BUTTON_START:
				if !s.advisoryTriggered {
					return NewSettingsScreen(s.cfg, s.cfgPath, s)
				}
```

- [ ] **Step 3.6: Build to verify no compile errors**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: `Built: bin/native/itchio-pak` (or similar success line).

- [ ] **Step 3.7: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat: show parental advisory overlay on flagged game detail pages"
```

---

## Task 4: screen_sensitive_tags.go — per-tag toggle screen

**Files:**
- Create: `internal/ui/screen_sensitive_tags.go`

The screen has `len(SensitiveTags) + 1` rows:
- Row 0: "All: ON/OFF" — master toggle for `SensitiveEnabled`
- Rows 1‥N: one per tag in `itchio.SensitiveTags` (alphabetical), showing ON/OFF based on `SensitiveDisabled`

Controller mapping follows the rest of the UI: **B = activate/toggle**, **A = back**.

- [ ] **Step 4.1: Create `internal/ui/screen_sensitive_tags.go`**

```go
//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// SensitiveTagsScreen lets a parent toggle individual Sensitive Topic tags.
// Row 0 is the master "All" toggle; rows 1..N are individual tags alphabetically.
type SensitiveTagsScreen struct {
	cfg     *settings.Config
	cfgPath string
	cursor  int
	prev    Screen
}

func NewSensitiveTagsScreen(cfg *settings.Config, cfgPath string, prev Screen) *SensitiveTagsScreen {
	return &SensitiveTagsScreen{cfg: cfg, cfgPath: cfgPath, prev: prev}
}

func (s *SensitiveTagsScreen) rowCount() int {
	return 1 + len(itchio.SensitiveTags) // "All" row + one per tag
}

// isTagEnabled reports whether an individual sensitive tag is enabled
// (i.e. not in SensitiveDisabled).
func (s *SensitiveTagsScreen) isTagEnabled(tag string) bool {
	for _, d := range s.cfg.Parental.SensitiveDisabled {
		if d == tag {
			return false
		}
	}
	return true
}

func (s *SensitiveTagsScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Sensitive Topics", 20, 20, colorText, colorText, colorText)

	// Row 0 — master toggle
	y := int32(80)
	if s.cursor == 0 {
		r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
	}
	allLabel := "All: OFF"
	if s.cfg.Parental.SensitiveEnabled {
		allLabel = "All: ON"
	}
	r.DrawText(allLabel, 20, y, colorText, colorText, colorText)

	// Individual tag rows
	for i, tag := range itchio.SensitiveTags {
		y = int32(120 + i*40)
		if s.cursor == i+1 {
			r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		state := "OFF"
		if s.cfg.Parental.SensitiveEnabled && s.isTagEnabled(tag) {
			state = "ON"
		}
		r.DrawText("  "+tag+": "+state, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("B toggle · A back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *SensitiveTagsScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < s.rowCount()-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			s.toggle()
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < s.rowCount()-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			s.toggle()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

// toggle activates the currently selected row.
func (s *SensitiveTagsScreen) toggle() {
	if s.cursor == 0 {
		// Master toggle
		s.cfg.Parental.SensitiveEnabled = !s.cfg.Parental.SensitiveEnabled
		s.cfg.Save(s.cfgPath)
		return
	}
	tag := itchio.SensitiveTags[s.cursor-1]
	if s.isTagEnabled(tag) {
		// Disable: add to SensitiveDisabled
		s.cfg.Parental.SensitiveDisabled = append(s.cfg.Parental.SensitiveDisabled, tag)
	} else {
		// Enable: remove from SensitiveDisabled
		updated := s.cfg.Parental.SensitiveDisabled[:0]
		for _, d := range s.cfg.Parental.SensitiveDisabled {
			if d != tag {
				updated = append(updated, d)
			}
		}
		s.cfg.Parental.SensitiveDisabled = updated
	}
	s.cfg.Save(s.cfgPath)
}
```

- [ ] **Step 4.2: Build to verify no compile errors**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: `Built: bin/native/itchio-pak`

- [ ] **Step 4.3: Run full test suite**

```bash
./scripts/test.sh 2>&1
```

Expected: all packages pass.

- [ ] **Step 4.4: Commit**

```bash
git add internal/ui/screen_sensitive_tags.go
git commit -m "feat: add SensitiveTagsScreen with per-tag toggle controls"
```

---

## Task 5: screen_settings.go — Mature toggle + Sensitive Topics entry

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 5.1: Add new iota values**

Find the `settingsItem` iota block and add two items before `sItemAbout`:

```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemClearCache
	sItemMature    // new
	sItemSensitive // new
	sItemAbout
	sItemCount
)
```

- [ ] **Step 5.2: Update the `items` slice in `Draw`**

Replace the existing `items` slice:

```go
	matureLabel := "Mature Content: OFF"
	if s.cfg.Parental.MatureEnabled {
		matureLabel = "Mature Content: ON"
	}
	sensitiveLabel := "Sensitive Topics: OFF >"
	if s.cfg.Parental.SensitiveEnabled {
		sensitiveLabel = "Sensitive Topics: ON  >"
	}

	items := []string{
		"API Key: " + maskKey(s.cfg.APIKey),
		"ROM Selection: " + s.cfg.ROMSelection,
		"Clear Image Cache",
		matureLabel,
		sensitiveLabel,
		"About",
	}
```

- [ ] **Step 5.3: Update `activate()` to handle the new items**

Replace the `activate` method:

```go
func (s *SettingsScreen) activate() Screen {
	switch s.cursor {
	case sItemROMMode:
		if s.cfg.ROMSelection == "auto" {
			s.cfg.ROMSelection = "ask"
		} else {
			s.cfg.ROMSelection = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case sItemClearCache:
		os.RemoveAll("/tmp/itchio-pak/cache/")
	case sItemMature:
		s.cfg.Parental.MatureEnabled = !s.cfg.Parental.MatureEnabled
		s.cfg.Save(s.cfgPath)
	case sItemSensitive:
		return NewSensitiveTagsScreen(s.cfg, s.cfgPath, s)
	}
	return s
}
```

- [ ] **Step 5.4: Build to verify no compile errors**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: `Built: bin/native/itchio-pak`

- [ ] **Step 5.5: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat: add Mature Content toggle and Sensitive Topics entry to Settings"
```

---

## Task 6: Build, deploy and smoke-test on device

**Files:** none changed — verification only.

- [ ] **Step 6.1: Build for tg5040**

```bash
./scripts/build.sh tg5040 2>&1
```

Expected: `Built: bin/tg5040/itchio-pak`

- [ ] **Step 6.2: Push binary to connected device**

```bash
adb push bin/tg5040/itchio-pak /mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio-pak
```

Expected: `1 file pushed`.

- [ ] **Step 6.3: Smoke-test on device — advisory overlay**

On the device:
1. Launch Itch.io pak
2. Open a game whose scraped tags include a mature or sensitive tag (check log: `FetchGameDetail: N page tags: [...]`)
3. Verify the "Grown-Ups Only" cover appears instead of the normal detail view
4. Verify only B (go back) works — Start does nothing
5. Press B and verify return to the game list

- [ ] **Step 6.4: Smoke-test — Settings flow**

1. From the game list, press Start → Settings
2. Verify "Mature Content: ON" appears
3. Press A (down) to highlight it, press B to toggle → verifies it shows "OFF"
4. Highlight "Sensitive Topics: ON >" and press B → verifies it opens the SensitiveTagsScreen
5. In SensitiveTagsScreen: toggle "lgbtq" off → verify it shows "OFF"
6. Press A to go back → verify back in Settings
7. Re-enter a previously-blocked game — if Mature was turned off and no sensitive tags remain, verify the detail loads normally

- [ ] **Step 6.5: Verify config persists across restart**

```bash
adb shell "cat /mnt/SDCARD/.userdata/shared/Itch-io/config.json"
```

Expected: JSON with `"parental": { "mature_enabled": false, "sensitive_enabled": true, "sensitive_disabled": ["lgbtq"] }` (or whatever was set in 6.4).

- [ ] **Step 6.6: Commit smoke-test sign-off**

```bash
git commit --allow-empty -m "chore: parental advisory smoke-tested on tg5040"
```
</content>
