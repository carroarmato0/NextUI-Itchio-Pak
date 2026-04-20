# Content Filter Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand the two-category parental advisory system into a general-purpose content filter with five categories useful to all users, reframing the feature from child-protection to content-awareness.

**Architecture:** The `itchio` package gains a `FilterConfig` struct (with `CategoryFilter` sub-struct) that `IsAdvisoryTriggered` consumes — keeping it free of settings imports. The `settings` package gains a `ContentFilter` struct with a `CategoryFilter` type that mirrors the itchio one; screen_detail.go converts between them. The existing `SensitiveTagsScreen` is replaced by a generic `TagFilterScreen` parameterised via function fields, reused for both per-tag categories (LGBTQ+, Heavy Themes).

**Tech Stack:** Go 1.22, SDL2 (go-sdl2), existing renderer/settings/itchio packages. Tests via `./scripts/test.sh`. Build via `./scripts/build.sh`.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/itchio/advisory.go` | Modify | New tag lists, `FilterConfig`/`CategoryFilter` structs, updated `IsAdvisoryTriggered` |
| `internal/itchio/advisory_test.go` | Modify | Updated + new tests for all five categories |
| `internal/settings/settings.go` | Modify | `ContentFilter` + `CategoryFilter` structs, new defaults, `HasActiveTag` method |
| `internal/settings/settings_test.go` | Modify | Updated tests for new field names and defaults |
| `internal/ui/screen_sensitive_tags.go` | Delete | Replaced by generic screen |
| `internal/ui/screen_tag_filter.go` | Create | Generic `TagFilterScreen` + constructors for LGBTQ+ and Heavy Themes |
| `internal/ui/screen_settings.go` | Modify | Five filter items, updated labels, updated `activate()` |
| `internal/ui/screen_detail.go` | Modify | Updated `IsAdvisoryTriggered` call, overlay text changed to "Content Warning" |
| `README.md` | Modify | Reframe as content filter for all users, document new categories |

---

## Task 1: advisory.go — new tag lists, FilterConfig, updated IsAdvisoryTriggered

**Files:**
- Modify: `internal/itchio/advisory.go`
- Modify: `internal/itchio/advisory_test.go`

- [ ] **Step 1.1: Write the failing tests**

Replace the entire `internal/itchio/advisory_test.go` with:

```go
package itchio_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func cfg(mature bool, lgbtq, heavy, substance, sexual itchio.CategoryFilter) itchio.FilterConfig {
	return itchio.FilterConfig{
		Mature:        mature,
		LGBTQ:         lgbtq,
		HeavyThemes:   heavy,
		SubstanceUse:  substance,
		SexualContent: sexual,
	}
}

var offAll = itchio.CategoryFilter{}

// ── Mature ────────────────────────────────────────────────────────────────────

func TestMatureMatch(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"nsfw"}, cfg(true, offAll, offAll, offAll, offAll)) {
		t.Error("expected trigger for mature tag 'nsfw'")
	}
}

func TestMatureCaseInsensitive(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"NSFW"}, cfg(true, offAll, offAll, offAll, offAll)) {
		t.Error("expected trigger for uppercase 'NSFW'")
	}
}

func TestMatureDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when mature filter disabled")
	}
}

func TestMatureWhitespaceTrimmed(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{" nsfw "}, cfg(true, offAll, offAll, offAll, offAll)) {
		t.Error("expected trigger for tag with surrounding whitespace")
	}
}

// ── LGBTQ ─────────────────────────────────────────────────────────────────────

func TestLGBTQMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"lgbtq"}, cfg(false, on, offAll, offAll, offAll)) {
		t.Error("expected trigger for lgbtq tag")
	}
}

func TestLGBTQMasterDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when lgbtq filter disabled")
	}
}

func TestLGBTQTagIndividuallyDisabled(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq"}}
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, cfg(false, on, offAll, offAll, offAll)) {
		t.Error("expected no trigger when lgbtq tag individually disabled")
	}
}

func TestLGBTQOtherTagStillActive(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq"}}
	if !itchio.IsAdvisoryTriggered([]string{"gay"}, cfg(false, on, offAll, offAll, offAll)) {
		t.Error("expected trigger for 'gay' even when 'lgbtq' individually disabled")
	}
}

func TestLGBTQExpandedTags(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	for _, tag := range []string{"queer", "bisexual", "trans", "non-binary", "pansexual"} {
		if !itchio.IsAdvisoryTriggered([]string{tag}, cfg(false, on, offAll, offAll, offAll)) {
			t.Errorf("expected trigger for expanded lgbtq tag %q", tag)
		}
	}
}

// ── Heavy Themes ──────────────────────────────────────────────────────────────

func TestHeavyThemesMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"suicide"}, cfg(false, offAll, on, offAll, offAll)) {
		t.Error("expected trigger for heavy theme tag 'suicide'")
	}
}

func TestHeavyThemesDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"suicide"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when heavy themes filter disabled")
	}
}

func TestHeavyThemesTagIndividuallyDisabled(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"grief"}}
	if itchio.IsAdvisoryTriggered([]string{"grief"}, cfg(false, offAll, on, offAll, offAll)) {
		t.Error("expected no trigger when grief individually disabled")
	}
}

// ── Substance Use ─────────────────────────────────────────────────────────────

func TestSubstanceUseMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"drugs"}, cfg(false, offAll, offAll, on, offAll)) {
		t.Error("expected trigger for substance use tag 'drugs'")
	}
}

func TestSubstanceUseDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"drugs"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when substance use filter disabled")
	}
}

// ── Sexual Content ────────────────────────────────────────────────────────────

func TestSexualContentMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"suggestive"}, cfg(false, offAll, offAll, offAll, on)) {
		t.Error("expected trigger for sexual content tag 'suggestive'")
	}
}

func TestSexualContentDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"suggestive"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when sexual content filter disabled")
	}
}

// ── Cross-category ────────────────────────────────────────────────────────────

func TestAllFiltersOff(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw", "lgbtq", "suicide", "drugs", "suggestive"},
		cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when all filters disabled")
	}
}

func TestNonFlaggedTag(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if itchio.IsAdvisoryTriggered([]string{"platformer", "adventure"},
		cfg(true, on, on, on, on)) {
		t.Error("expected no trigger for non-flagged tags")
	}
}

func TestEmptyTags(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if itchio.IsAdvisoryTriggered(nil, cfg(true, on, on, on, on)) {
		t.Error("expected no trigger for nil tags")
	}
	if itchio.IsAdvisoryTriggered([]string{}, cfg(true, on, on, on, on)) {
		t.Error("expected no trigger for empty tags")
	}
}
```

- [ ] **Step 1.2: Run tests to confirm they fail**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|undefined|FilterConfig"
```

Expected: compile errors — `FilterConfig`, `CategoryFilter` undefined.

- [ ] **Step 1.3: Replace `internal/itchio/advisory.go`**

```go
package itchio

import (
	"slices"
	"strings"
)

// CategoryFilter holds the enabled state and individually-disabled tags for
// one content filter category. Disabled is an opt-out list: tags in this list
// are excluded from filtering even when Enabled is true.
type CategoryFilter struct {
	Enabled  bool
	Disabled []string
}

// FilterConfig is the complete content filter configuration passed to
// IsAdvisoryTriggered. It lives in the itchio package (not settings) to avoid
// import cycles — callers in the ui package convert from settings.ContentFilter.
type FilterConfig struct {
	Mature        bool           // single on/off, no per-tag opt-out
	LGBTQ         CategoryFilter
	HeavyThemes   CategoryFilter
	SubstanceUse  CategoryFilter
	SexualContent CategoryFilter
}

// MatureTags is the list of tag slugs considered explicit adult content.
// Users can toggle the whole category but cannot edit individual tags.
var MatureTags = []string{
	"adult", "boobs", "eroge", "erotic", "femdom", "gore",
	"hentai", "lewd", "nsfw", "nudity", "porn", "softcore",
	"tits", "titties", "xxx", "yaoi", "yuri",
}

// LGBTQTags is the list of tag slugs covering LGBTQ+ content and themes.
// Users can toggle the category and opt individual tags in or out.
// Sorted alphabetically.
var LGBTQTags = []string{
	"achillean", "aromantic", "asexual", "bisexual", "enby",
	"gay", "gender", "intersex", "lesbian", "lgbt", "lgbtq",
	"lgbtqia", "mlm", "non-binary", "nonbinary", "pansexual",
	"pride", "queer", "sapphic", "trans", "transgender", "wlw",
}

// HeavyThemesTags is the list of tag slugs covering potentially distressing
// narrative themes. Users can toggle the category and opt individual tags in
// or out. Sorted alphabetically.
var HeavyThemesTags = []string{
	"abuse", "anxiety", "bereavement", "child-loss", "death",
	"depression", "domestic-abuse", "eating-disorder", "grief",
	"loss", "mental-health", "mental-illness", "miscarriage",
	"self-harm", "sexual-assault", "suicide", "trauma", "war",
}

// SubstanceUseTags is the list of tag slugs covering drug and alcohol themes.
var SubstanceUseTags = []string{
	"addiction", "alcohol", "drug-use", "drugs", "substance-abuse",
}

// SexualContentTags is the list of tag slugs covering suggestive or
// non-explicit sexual content (distinct from the explicit MatureTags list).
var SexualContentTags = []string{
	"ecchi", "innuendo", "sexual-content", "sexy", "suggestive",
}

// IsAdvisoryTriggered returns true if any tag in pageTags matches an active
// filter in cfg. Tag matching is case-insensitive and whitespace-trimmed.
func IsAdvisoryTriggered(pageTags []string, cfg FilterConfig) bool {
	// Normalise opt-out lists once, outside the per-tag loop.
	norm := func(list []string) []string {
		out := make([]string, len(list))
		for i, d := range list {
			out[i] = strings.ToLower(strings.TrimSpace(d))
		}
		return out
	}
	lgbtqDis := norm(cfg.LGBTQ.Disabled)
	heavyDis := norm(cfg.HeavyThemes.Disabled)
	substanceDis := norm(cfg.SubstanceUse.Disabled)
	sexualDis := norm(cfg.SexualContent.Disabled)

	for _, tag := range pageTags {
		slug := strings.ToLower(strings.TrimSpace(tag))
		if cfg.Mature && slices.Contains(MatureTags, slug) {
			return true
		}
		if cfg.LGBTQ.Enabled && slices.Contains(LGBTQTags, slug) && !slices.Contains(lgbtqDis, slug) {
			return true
		}
		if cfg.HeavyThemes.Enabled && slices.Contains(HeavyThemesTags, slug) && !slices.Contains(heavyDis, slug) {
			return true
		}
		if cfg.SubstanceUse.Enabled && slices.Contains(SubstanceUseTags, slug) && !slices.Contains(substanceDis, slug) {
			return true
		}
		if cfg.SexualContent.Enabled && slices.Contains(SexualContentTags, slug) && !slices.Contains(sexualDis, slug) {
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

Expected: all packages pass.

- [ ] **Step 1.5: Commit**

```bash
git add internal/itchio/advisory.go internal/itchio/advisory_test.go
git commit -m "feat: expand content filter — five categories, FilterConfig struct, broader tag lists"
```

---

## Task 2: settings.go — ContentFilter struct, new defaults

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 2.1: Write the failing tests**

Replace the entire `internal/settings/settings_test.go` with:

```go
package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "" {
		t.Errorf("default APIKey = %q, want %q", cfg.APIKey, "")
	}
	if cfg.ROMSelection != "auto" {
		t.Errorf("default ROMSelection = %q, want %q", cfg.ROMSelection, "auto")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{APIKey: "abc123", ROMSelection: "ask"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.APIKey != "abc123" {
		t.Errorf("APIKey = %q, want %q", loaded.APIKey, "abc123")
	}
	if loaded.ROMSelection != "ask" {
		t.Errorf("ROMSelection = %q, want %q", loaded.ROMSelection, "ask")
	}
}

func TestLoadCorruptedFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ROMSelection != "auto" {
		t.Errorf("corrupted load should return defaults, got ROMSelection = %q", cfg.ROMSelection)
	}
}

// Mature content is the only filter that defaults to ON.
func TestDefaultsMatureEnabled(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Filter.MatureEnabled {
		t.Error("expected MatureEnabled=true by default")
	}
	if cfg.Filter.LGBTQ.Enabled {
		t.Error("expected LGBTQ.Enabled=false by default")
	}
	if cfg.Filter.HeavyThemes.Enabled {
		t.Error("expected HeavyThemes.Enabled=false by default")
	}
	if cfg.Filter.SubstanceUse.Enabled {
		t.Error("expected SubstanceUse.Enabled=false by default")
	}
	if cfg.Filter.SexualContent.Enabled {
		t.Error("expected SexualContent.Enabled=false by default")
	}
}

func TestContentFilterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{
		APIKey:       "",
		ROMSelection: "auto",
		Filter: settings.ContentFilter{
			MatureEnabled: false,
			LGBTQ:         settings.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq", "gay"}},
			HeavyThemes:   settings.CategoryFilter{Enabled: true, Disabled: []string{"grief"}},
			SubstanceUse:  settings.CategoryFilter{Enabled: true},
			SexualContent: settings.CategoryFilter{},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Filter.MatureEnabled {
		t.Error("MatureEnabled not preserved")
	}
	if !loaded.Filter.LGBTQ.Enabled {
		t.Error("LGBTQ.Enabled not preserved")
	}
	if len(loaded.Filter.LGBTQ.Disabled) != 2 {
		t.Errorf("LGBTQ.Disabled: expected 2 entries, got %v", loaded.Filter.LGBTQ.Disabled)
	}
	if len(loaded.Filter.HeavyThemes.Disabled) != 1 {
		t.Errorf("HeavyThemes.Disabled: expected 1 entry, got %v", loaded.Filter.HeavyThemes.Disabled)
	}
	if !loaded.Filter.SubstanceUse.Enabled {
		t.Error("SubstanceUse.Enabled not preserved")
	}
}

func TestHasActiveTag(t *testing.T) {
	tags := []string{"grief", "suicide", "war"}

	// All enabled, none disabled → true
	cf := settings.CategoryFilter{Enabled: true}
	if !cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=true when all tags enabled")
	}

	// Master off → false
	cf = settings.CategoryFilter{Enabled: false}
	if cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=false when master disabled")
	}

	// All individually disabled → false
	cf = settings.CategoryFilter{Enabled: true, Disabled: []string{"grief", "suicide", "war"}}
	if cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=false when all tags individually disabled")
	}

	// One still active → true
	cf = settings.CategoryFilter{Enabled: true, Disabled: []string{"grief", "suicide"}}
	if !cf.HasActiveTag(tags) {
		t.Error("expected HasActiveTag=true when one tag still active")
	}
}
```

- [ ] **Step 2.2: Run tests to confirm they fail**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|ContentFilter|undefined"
```

Expected: compile errors — `ContentFilter`, `CategoryFilter`, `cfg.Filter` undefined.

- [ ] **Step 2.3: Replace `internal/settings/settings.go`**

```go
package settings

import (
	"encoding/json"
	"os"
)

// CategoryFilter holds the enabled state and individually-disabled tags for
// one content filter category.
type CategoryFilter struct {
	Enabled  bool     `json:"enabled"`
	Disabled []string `json:"disabled,omitempty"`
}

// HasActiveTag reports whether at least one tag from tagList would be filtered
// (Enabled is true and the tag is not in Disabled).
func (cf CategoryFilter) HasActiveTag(tagList []string) bool {
	if !cf.Enabled {
		return false
	}
	for _, tag := range tagList {
		inDisabled := false
		for _, d := range cf.Disabled {
			if d == tag {
				inDisabled = true
				break
			}
		}
		if !inDisabled {
			return true
		}
	}
	return false
}

// ContentFilter holds the complete content filter configuration.
// MatureEnabled defaults to true; all other categories default to false.
type ContentFilter struct {
	MatureEnabled bool           `json:"mature_enabled"`
	LGBTQ         CategoryFilter `json:"lgbtq"`
	HeavyThemes   CategoryFilter `json:"heavy_themes"`
	SubstanceUse  CategoryFilter `json:"substance_use"`
	SexualContent CategoryFilter `json:"sexual_content"`
}

// Config is the top-level application configuration.
type Config struct {
	APIKey       string        `json:"api_key"`
	ROMSelection string        `json:"rom_selection"`
	Filter       ContentFilter `json:"content_filter"`
}

func defaults() *Config {
	return &Config{
		APIKey:       "",
		ROMSelection: "auto",
		Filter: ContentFilter{
			MatureEnabled: true,
			// All other categories default to disabled (zero value).
		},
	}
}

// Load reads the config from path. If the file is missing or corrupted,
// defaults are returned without an error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults(), nil
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
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
git commit -m "feat: replace ParentalAdvisory with ContentFilter — five categories, mature-only default ON"
```

---

## Task 3: screen_tag_filter.go — generic per-tag sub-screen

**Files:**
- Delete: `internal/ui/screen_sensitive_tags.go`
- Create: `internal/ui/screen_tag_filter.go`

This screen is used for the two categories that support per-tag control: LGBTQ+ and Heavy Themes.
The caller passes function closures that read/write the relevant fields in `*settings.Config`.

- [ ] **Step 3.1: Delete the old screen**

```bash
rm internal/ui/screen_sensitive_tags.go
```

- [ ] **Step 3.2: Create `internal/ui/screen_tag_filter.go`**

```go
//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// TagFilterScreen is a generic per-tag toggle screen used for content filter
// categories that support individual tag opt-out (LGBTQ+, Heavy Themes).
//
// Row 0: master "All: Filtered / Allowed" toggle.
// Rows 1..N: one row per tag, showing Blocked or Allowed.
//
// Controller mapping (TrimUI convention): B = toggle, A = back.
type TagFilterScreen struct {
	title       string
	tags        []string
	cfg         *settings.Config
	cfgPath     string
	getEnabled  func() bool
	setEnabled  func(bool)
	getDisabled func() []string
	setDisabled func([]string)
	cursor      int
	prev        Screen
}

// NewLGBTQFilterScreen returns a TagFilterScreen configured for the LGBTQ+
// content category.
func NewLGBTQFilterScreen(cfg *settings.Config, cfgPath string, prev Screen) *TagFilterScreen {
	return &TagFilterScreen{
		title:       "LGBTQ+ Content",
		tags:        itchio.LGBTQTags,
		cfg:         cfg,
		cfgPath:     cfgPath,
		getEnabled:  func() bool { return cfg.Filter.LGBTQ.Enabled },
		setEnabled:  func(v bool) { cfg.Filter.LGBTQ.Enabled = v },
		getDisabled: func() []string { return cfg.Filter.LGBTQ.Disabled },
		setDisabled: func(v []string) { cfg.Filter.LGBTQ.Disabled = v },
		prev:        prev,
	}
}

// NewHeavyThemesFilterScreen returns a TagFilterScreen configured for the
// Heavy Themes content category.
func NewHeavyThemesFilterScreen(cfg *settings.Config, cfgPath string, prev Screen) *TagFilterScreen {
	return &TagFilterScreen{
		title:       "Heavy Themes",
		tags:        itchio.HeavyThemesTags,
		cfg:         cfg,
		cfgPath:     cfgPath,
		getEnabled:  func() bool { return cfg.Filter.HeavyThemes.Enabled },
		setEnabled:  func(v bool) { cfg.Filter.HeavyThemes.Enabled = v },
		getDisabled: func() []string { return cfg.Filter.HeavyThemes.Disabled },
		setDisabled: func(v []string) { cfg.Filter.HeavyThemes.Disabled = v },
		prev:        prev,
	}
}

func (s *TagFilterScreen) rowCount() int {
	return 1 + len(s.tags)
}

func (s *TagFilterScreen) isTagEnabled(tag string) bool {
	for _, d := range s.getDisabled() {
		if d == tag {
			return false
		}
	}
	return true
}

func (s *TagFilterScreen) anyTagEnabled() bool {
	for _, tag := range s.tags {
		if s.isTagEnabled(tag) {
			return true
		}
	}
	return false
}

func (s *TagFilterScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText(s.title, 20, 20, colorText, colorText, colorText)

	// Row 0 — master toggle
	y := int32(80)
	if s.cursor == 0 {
		r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
	}
	allLabel := "All: Allowed"
	if s.getEnabled() && s.anyTagEnabled() {
		allLabel = "All: Filtered"
	}
	r.DrawText(allLabel, 20, y, colorText, colorText, colorText)

	// Individual tag rows
	for i, tag := range s.tags {
		y = int32(120 + i*40)
		if s.cursor == i+1 {
			r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		state := "Allowed"
		if s.getEnabled() && s.isTagEnabled(tag) {
			state = "Blocked"
		}
		r.DrawText("  "+tag+": "+state, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("B toggle · A back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *TagFilterScreen) HandleEvent(e sdl.Event) Screen {
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

func (s *TagFilterScreen) toggle() {
	if s.cursor == 0 {
		s.setEnabled(!s.getEnabled())
		s.cfg.Save(s.cfgPath)
		return
	}
	tag := s.tags[s.cursor-1]
	if s.isTagEnabled(tag) {
		s.setDisabled(append(s.getDisabled(), tag))
	} else {
		var updated []string
		for _, d := range s.getDisabled() {
			if d != tag {
				updated = append(updated, d)
			}
		}
		s.setDisabled(updated)
	}
	s.cfg.Save(s.cfgPath)
}
```

- [ ] **Step 3.3: Build to verify no compile errors**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: `Built: bin/native/itchio-pak`

- [ ] **Step 3.4: Run full test suite**

```bash
./scripts/test.sh 2>&1
```

Expected: all packages pass.

- [ ] **Step 3.5: Commit**

```bash
git add internal/ui/screen_tag_filter.go
git rm internal/ui/screen_sensitive_tags.go
git commit -m "feat: replace SensitiveTagsScreen with generic TagFilterScreen"
```

---

## Task 4: screen_settings.go — five filter items

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 4.1: Read the current file**

Read `internal/ui/screen_settings.go` in full before editing.

- [ ] **Step 4.2: Replace the entire file**

```go
//go:build !headless

package ui

import (
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type settingsItem int

const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemClearCache
	sItemMature
	sItemLGBTQ
	sItemHeavyThemes
	sItemSubstanceUse
	sItemSexualContent
	sItemAbout
	sItemCount
)

type SettingsScreen struct {
	cfg     *settings.Config
	cfgPath string
	cursor  settingsItem
	prev    Screen
}

func NewSettingsScreen(cfg *settings.Config, cfgPath string, prev Screen) *SettingsScreen {
	return &SettingsScreen{cfg: cfg, cfgPath: cfgPath, prev: prev}
}

func (s *SettingsScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Settings", 20, 20, colorText, colorText, colorText)

	f := s.cfg.Filter

	matureLabel := "Mature Content: Allowed"
	if f.MatureEnabled {
		matureLabel = "Mature Content: Blocked"
	}

	lgbtqLabel := "LGBTQ+ Content: Allowed >"
	if f.LGBTQ.HasActiveTag(itchio.LGBTQTags) {
		lgbtqLabel = "LGBTQ+ Content: Filtered >"
	}

	heavyLabel := "Heavy Themes: Allowed >"
	if f.HeavyThemes.HasActiveTag(itchio.HeavyThemesTags) {
		heavyLabel = "Heavy Themes: Filtered >"
	}

	substanceLabel := "Substance Use: Allowed"
	if f.SubstanceUse.Enabled {
		substanceLabel = "Substance Use: Blocked"
	}

	sexualLabel := "Sexual Content: Allowed"
	if f.SexualContent.Enabled {
		sexualLabel = "Sexual Content: Blocked"
	}

	items := []string{
		"API Key: " + maskKey(s.cfg.APIKey),
		"ROM Selection: " + s.cfg.ROMSelection,
		"Clear Image Cache",
		matureLabel,
		lgbtqLabel,
		heavyLabel,
		substanceLabel,
		sexualLabel,
		"About",
	}

	for i, label := range items {
		y := int32(60 + i*36)
		if settingsItem(i) == s.cursor {
			r.DrawRect(0, y-4, r.W, 32, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(label, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("D-pad navigate · A select · B back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *SettingsScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			return s.activate()
		case sdl.K_ESCAPE:
			return s.prev
		case sdl.K_s:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.activate()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		case sdl.CONTROLLER_BUTTON_START:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

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
		s.cfg.Filter.MatureEnabled = !s.cfg.Filter.MatureEnabled
		s.cfg.Save(s.cfgPath)
	case sItemLGBTQ:
		return NewLGBTQFilterScreen(s.cfg, s.cfgPath, s)
	case sItemHeavyThemes:
		return NewHeavyThemesFilterScreen(s.cfg, s.cfgPath, s)
	case sItemSubstanceUse:
		s.cfg.Filter.SubstanceUse.Enabled = !s.cfg.Filter.SubstanceUse.Enabled
		s.cfg.Save(s.cfgPath)
	case sItemSexualContent:
		s.cfg.Filter.SexualContent.Enabled = !s.cfg.Filter.SexualContent.Enabled
		s.cfg.Save(s.cfgPath)
	}
	return s
}

func maskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}
```

**Note on layout:** The settings screen now has 9 items. Row spacing is reduced from 40px to 36px, and the top offset from 80px to 60px, so the bottom item lands at 60 + 8×36 = 348px — comfortably within the 768px display. Verify this looks right on device in Task 6.

- [ ] **Step 4.3: Build to verify no compile errors**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: `Built: bin/native/itchio-pak`

- [ ] **Step 4.4: Run full test suite**

```bash
./scripts/test.sh 2>&1
```

Expected: all packages pass.

- [ ] **Step 4.5: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat: add five content filter items to Settings screen"
```

---

## Task 5: screen_detail.go — update FilterConfig call and overlay text

**Files:**
- Modify: `internal/ui/screen_detail.go`

Two changes:
1. The `IsAdvisoryTriggered` call now takes a `FilterConfig` struct instead of individual bool/slice params.
2. The overlay is renamed "Content Warning" with neutral body text (no longer child-specific).

- [ ] **Step 5.1: Update the `IsAdvisoryTriggered` call in `NewDetailScreen`**

Find these lines (around line 63–70):

```go
		s.advisoryTriggered = itchio.IsAdvisoryTriggered(
			d.PageTags,
			cfg.Parental.MatureEnabled,
			cfg.Parental.SensitiveEnabled,
			cfg.Parental.SensitiveDisabled,
		)
```

Replace with:

```go
		s.advisoryTriggered = itchio.IsAdvisoryTriggered(
			d.PageTags,
			itchio.FilterConfig{
				Mature: cfg.Filter.MatureEnabled,
				LGBTQ: itchio.CategoryFilter{
					Enabled:  cfg.Filter.LGBTQ.Enabled,
					Disabled: cfg.Filter.LGBTQ.Disabled,
				},
				HeavyThemes: itchio.CategoryFilter{
					Enabled:  cfg.Filter.HeavyThemes.Enabled,
					Disabled: cfg.Filter.HeavyThemes.Disabled,
				},
				SubstanceUse: itchio.CategoryFilter{
					Enabled:  cfg.Filter.SubstanceUse.Enabled,
					Disabled: cfg.Filter.SubstanceUse.Disabled,
				},
				SexualContent: itchio.CategoryFilter{
					Enabled:  cfg.Filter.SexualContent.Enabled,
					Disabled: cfg.Filter.SexualContent.Disabled,
				},
			},
		)
```

- [ ] **Step 5.2: Update `drawAdvisoryOverlay`**

Find the `drawAdvisoryOverlay` method and replace its body:

```go
func (s *DetailScreen) drawAdvisoryOverlay(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	cy := r.H / 2

	r.DrawTextCentered("[!]", 0, cy-90, r.W, 240, 180, 60)
	r.DrawTextCentered("Content Warning", 0, cy-54, r.W, 240, 180, 60)

	r.DrawRect(r.W/4, cy-28, r.W/2, 1, 60, 60, 60)

	_, lh := r.TextSize("Ag")
	if lh < 20 {
		lh = 20
	}
	r.DrawWrappedText(
		"This game contains content matched by one of your active filters.",
		r.W/8, cy-16, r.W*3/4, lh+2, 180, 180, 180,
	)
	r.DrawWrappedText(
		"You can adjust your filters in Settings.",
		r.W/8, cy-16+lh+6, r.W*3/4, lh+2, 180, 180, 180,
	)

	r.DrawRect(r.W/4, cy+60, r.W/2, 1, 60, 60, 60)
	r.DrawTextCentered("B  Go back", 0, cy+72, r.W, 180, 80, 80)
}
```

**Note:** The second line now says "You can adjust your filters in Settings." Since this overlay is now aimed at all users (not just children), hinting at Settings is appropriate — adults using content warnings know why the overlay appeared and may want to adjust it. The Start button suppression can be relaxed here too (see Step 5.3).

- [ ] **Step 5.3: Allow Start on the overlay for adults**

Find the two Start button guards in `HandleEvent`:

```go
case sdl.K_s:
    if !s.advisoryTriggered {
        return NewSettingsScreen(s.cfg, s.cfgPath, s)
    }
```

```go
case sdl.CONTROLLER_BUTTON_START:
    if !s.advisoryTriggered {
        return NewSettingsScreen(s.cfg, s.cfgPath, s)
    }
```

Remove the `advisoryTriggered` guard — allow Start to open Settings from the overlay:

```go
case sdl.K_s:
    return NewSettingsScreen(s.cfg, s.cfgPath, s)
```

```go
case sdl.CONTROLLER_BUTTON_START:
    return NewSettingsScreen(s.cfg, s.cfgPath, s)
```

**Rationale:** The overlay now shows a Settings hint. Blocking Start was designed to prevent children from discovering the bypass. With the system reframed as a content filter for all users, adults should be able to navigate directly to Settings from the warning.

- [ ] **Step 5.4: Build to verify no compile errors**

```bash
./scripts/build.sh native 2>&1 | tail -5
```

Expected: `Built: bin/native/itchio-pak`

- [ ] **Step 5.5: Run full test suite**

```bash
./scripts/test.sh 2>&1
```

Expected: all packages pass.

- [ ] **Step 5.6: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat: update advisory call to FilterConfig, rename overlay to Content Warning"
```

---

## Task 6: README.md — reframe as content filter for all users

**Files:**
- Modify: `README.md`

- [ ] **Step 6.1: Read the current README**

Read `README.md` in full before editing.

- [ ] **Step 6.2: Update the Features — Parental advisory subsection**

Find the "### Parental advisory" subsection and replace it with:

```markdown
### Content filters
- All filters are **off by default** except Mature Content, which is on.
- **Mature Content** — blocks explicit adult content. Single on/off toggle.
- **LGBTQ+ Content** — per-tag filter for LGBTQ+ themes and representation.
- **Heavy Themes** — per-tag filter for potentially distressing narrative topics
  (grief, loss, suicide, trauma, abuse, and similar).
- **Substance Use** — single toggle for drug and alcohol themes.
- **Sexual Content** — single toggle for suggestive but non-explicit content.
- When a filter triggers, a full-screen **Content Warning** cover replaces the
  detail view. Press **B** to go back or **Start** to open Settings and adjust
  your filters.
- Filters are configured in **Settings** (press Start from any screen).
```

- [ ] **Step 6.3: Update the Settings subsection**

Find the Settings bullet list and update the filter-related items:

```markdown
- **Mature Content** — block explicit adult content (default: on)
- **LGBTQ+ Content** — filter LGBTQ+ tags with per-tag control (default: off)
- **Heavy Themes** — filter distressing narrative themes with per-tag control (default: off)
- **Substance Use** — filter drug and alcohol themes (default: off)
- **Sexual Content** — filter suggestive non-explicit content (default: off)
```

- [ ] **Step 6.4: Replace the Parental Controls section**

Find the `## Parental Controls` section and replace it entirely with:

```markdown
## Content Filters

The pak includes a built-in content filter system. Filters are useful for anyone
who wants to be aware of — or avoid — specific themes before opening a game,
whether that is a parent managing what their child encounters, or an adult who
prefers not to encounter certain content unexpectedly.

When a game's tags match an active filter, a **Content Warning** screen replaces
the detail view. Press **B** to go back, or **Start** to open Settings and adjust
your filters.

### Configuring filters

Press **Start** from any screen to open **Settings**, then scroll to the content
filter section. Each category can be toggled independently:

- **Mature Content** — covers explicit adult content. Defaults to **on**.
- **LGBTQ+ Content** — covers LGBTQ+ themes and representation. Supports
  per-tag control so you can allow some topics while filtering others.
  Defaults to **off**.
- **Heavy Themes** — covers potentially distressing narrative content: grief,
  loss, suicide, trauma, abuse, and similar. Supports per-tag control.
  Defaults to **off**.
- **Substance Use** — covers drug and alcohol themes. Defaults to **off**.
- **Sexual Content** — covers suggestive but non-explicit content. Defaults
  to **off**.

The specific tags covered by each category are listed and togglable directly
in the Settings screen on the device.

### Limitations

> **Filtering is best-effort, not comprehensive.** Be aware of the following:

- **Tag-based only** — itch.io has no machine-readable content rating system.
  Filtering relies entirely on tags that game creators choose to apply. A
  creator can omit tags or use non-standard wording, and content will not be
  caught.
- **Scrape-time only** — tags are fetched when a game's detail page is opened.
  The game list is always unfiltered; cover art alone may hint at content.
- **Curated tag list** — the filter covers known tags but the list is not
  exhaustive. New or community-specific tags may not be included until a
  future update.
- **No substitute for awareness** — filters reduce unexpected encounters but
  cannot guarantee coverage. When in doubt, check the game's itch.io page
  directly.
```

- [ ] **Step 6.5: Update the intro parental advisory note**

Find the intro note block:

```markdown
> **Parental advisory:** A built-in content filter blocks game detail pages that
> contain known mature or sensitive tags. It is enabled by default. See
> [Parental Controls](#parental-controls) for details and limitations.
```

Replace with:

```markdown
> **Content filters:** Built-in filters let you block or flag game detail pages
> by theme — mature content, LGBTQ+ content, heavy themes, substance use, and
> suggestive content. Mature Content is on by default; others are opt-in. See
> [Content Filters](#content-filters) for details and limitations.
```

- [ ] **Step 6.6: Commit**

```bash
git add README.md
git commit -m "docs: reframe parental advisory as content filters for all users"
```

---

## Task 7: Build tg5040, deploy, smoke-test

**Files:** none changed — verification only.

- [ ] **Step 7.1: Build for tg5040**

```bash
./scripts/build.sh tg5040 2>&1 | tail -3
```

Expected: `Built: bin/tg5040/itchio-pak`

- [ ] **Step 7.2: Push binary to connected device**

```bash
adb push bin/tg5040/itchio-pak /mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio-pak
```

Expected: `1 file pushed`

- [ ] **Step 7.3: Smoke-test Settings screen**

On the device:
1. Launch Itch.io pak
2. Press Start → Settings
3. Verify all five filter items appear: Mature Content, LGBTQ+ Content, Heavy Themes, Substance Use, Sexual Content
4. Verify spacing — all items fit on screen without overlap
5. Toggle Mature Content — verify label changes Blocked ↔ Allowed
6. Open LGBTQ+ Content → verify sub-screen shows all tags with Allowed/Blocked state
7. Open Heavy Themes → verify sub-screen shows grief, suicide, trauma etc.
8. Toggle Substance Use → verify label changes

- [ ] **Step 7.4: Smoke-test Content Warning overlay**

1. Find a game tagged with a filtered topic (check log for page tags)
2. Enable the relevant filter in Settings
3. Open the game — verify the "Content Warning" overlay appears
4. Press Start — verify Settings opens (Start is no longer suppressed on overlay)
5. Press B — verify return to game list

- [ ] **Step 7.5: Verify config on device**

```bash
adb shell "cat /mnt/SDCARD/.userdata/shared/Itch-io/config.json"
```

Expected: JSON with `"content_filter"` key containing the five categories. The old `"parental"` key is no longer present.

- [ ] **Step 7.6: Commit smoke-test sign-off**

```bash
git commit --allow-empty -m "chore: content filter expansion smoke-tested on tg5040"
```
