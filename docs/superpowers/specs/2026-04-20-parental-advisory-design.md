# Parental Advisory Feature — Design Spec

**Date:** 2026-04-20
**Status:** Approved

---

## Background

itch.io's RSS feed contains no content ratings. Tags (e.g. `nsfw`, `adult`, `lgbtq`) are only
discoverable by scraping individual game detail pages — which the pak already does when a user
opens a game. Loading all pages ahead of time is not feasible given hardware constraints.

The parental advisory feature provides a child-friendly full-screen cover overlay that blocks the
detail view when a game's scraped tags match a configured filter. A parent can fine-tune the
filter in Settings. The feature is enabled by default.

---

## Tag Categories

Tags are hardcoded in the app. Parents can toggle them but cannot add new ones.

### Mature (single ON/OFF — no per-tag control)

> Explicit adult content. Either you filter it or you don't.

`adult`, `boobs`, `eroge`, `erotic`, `femdom`, `gore`, `hentai`, `lewd`, `nsfw`, `nudity`,
`porn`, `softcore`, `tits`, `titties`, `xxx`, `yaoi`, `yuri`

### Sensitive (master ON/OFF + individual tag toggles)

> Topics that may be sensitive depending on family values, but can otherwise appear in
> wholesome games (e.g. LGBTQ representation).

`gay`, `gender`, `lesbian`, `lgbtq`, `sexy`, `transgender`

---

## Data Model

New nested struct added to `settings.Config`:

```go
type ParentalAdvisory struct {
    MatureEnabled     bool     `json:"mature_enabled"`
    SensitiveEnabled  bool     `json:"sensitive_enabled"`
    SensitiveDisabled []string `json:"sensitive_disabled"` // tags individually turned off
}
```

Added to `Config` as `Parental ParentalAdvisory`. Defaults:

```go
ParentalAdvisory{
    MatureEnabled:     true,
    SensitiveEnabled:  true,
    SensitiveDisabled: nil,
}
```

**Trigger logic:** the overlay fires if `PageTags` contains any tag that:
- belongs to the Mature list AND `MatureEnabled == true`, OR
- belongs to the Sensitive list AND `SensitiveEnabled == true` AND the tag is NOT in `SensitiveDisabled`

Tag matching is case-insensitive, compared against the lowercase slug (e.g. `"NSFW"` → `"nsfw"`).

New tags added in future app versions are automatically active without requiring config migration
(opt-out model via `SensitiveDisabled`).

---

## Settings Screen Changes

Two new items added after "Clear Image Cache":

| Item | Behaviour |
|------|-----------|
| `Mature Content: ON/OFF` | A toggles `MatureEnabled` directly, saves config, no sub-screen |
| `Sensitive Topics: ON ▶` / `OFF ▶` | ▶ always shown (indicates sub-screen); A opens `SensitiveTagsScreen` |

`sItemMature` and `sItemSensitive` are added to the `settingsItem` iota before `sItemAbout`.

### SensitiveTagsScreen (new screen)

- Header: "Sensitive Topics"
- First row: `All: ON/OFF` — master toggle; mirrors `SensitiveEnabled`; toggling it saves config
  and visually updates all child rows
- Remaining rows: one per tag alphabetically (`gay`, `gender`, `lesbian`, `lgbtq`, `sexy`,
  `transgender`) showing `ON` or `OFF` based on whether the tag is in `SensitiveDisabled`
- A on a tag row toggles that tag in/out of `SensitiveDisabled`, saves config immediately
- B returns to Settings
- Cursor clamps at top and bottom (consistent with existing Settings screen behaviour)

---

## Detail Screen Changes

After `FetchGameDetail` completes, `DetailScreen` checks whether any tag in
`detail.PageTags` triggers the active filter (using a helper `isAdvisoryTriggered(tags []string, cfg *settings.Config) bool`).

If triggered:
- Render a full-screen cover overlay *instead of* the normal content
- Overlay layout:
  - Warning symbol drawn as the text string `"[!]"` (emoji not used — TTF font may not support it)
  - "Grown-Ups Only" in amber
  - "This game may have content that is not suitable for all ages. Please ask a parent or guardian before continuing."
  - Separator line
  - `B  Go back` in dim red — the only available action
- No mention of Settings
- No "continue anyway" path
- Start button is suppressed on this overlay (does not open Settings) — prevents a child from
  navigating directly to Settings from the blocked screen to disable the filter themselves

If NOT triggered (or both filters disabled): normal detail view renders as today.

The check runs once after loading completes. If the user changes filter settings and
re-enters the detail, the check re-runs.

---

## README Update

Add a section under "Known Limitations" (or a dedicated "Parental Advisory" section) covering:

- itch.io provides no machine-readable content ratings in its RSS feed
- Tags are scraped from individual game pages on first view — there is no pre-filtering
- The tag list is curated but not exhaustive; creators may use unlisted tags
- The feature is not a substitute for parental supervision
- Default state: enabled for both Mature and Sensitive categories

---

## Files Changed

| File | Change |
|------|--------|
| `internal/settings/settings.go` | Add `ParentalAdvisory` struct and field to `Config`; update `defaults()` |
| `internal/settings/settings_test.go` | Tests for default values and save/load round-trip |
| `internal/itchio/advisory.go` | New file: tag lists + `IsAdvisoryTriggered()` helper |
| `internal/itchio/advisory_test.go` | New file: unit tests for trigger logic |
| `internal/ui/screen_detail.go` | Add overlay render path after detail loads |
| `internal/ui/screen_settings.go` | Add two new setting items; wire `SensitiveTagsScreen` |
| `internal/ui/screen_sensitive_tags.go` | New screen: per-tag toggle list |
| `README.md` | Parental advisory section under Known Limitations |
</content>
