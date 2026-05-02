# NextUI Theme Integration Design

**Date:** 2026-05-02
**Status:** Approved

## Summary

Integrate the NextUI system theme into the Itch.io Pak UI so that users who customise their device's color palette in NextUI settings see those colors reflected inside the pak. The implementation reads `/mnt/SDCARD/.userdata/shared/minuisettings.txt` at startup, maps 7 color fields to a `Theme` struct, and applies them across all screens via renderer helpers. Shape improvements (pill selection, circle/pill button badges, tag pills) are delivered as part of the same change.

Falls back silently to the current static grayscale values on non-NextUI devices — zero visible change for those users.

---

## Background

### Settings files on device

`msettings.bin` — binary struct of hardware knobs (brightness, contrast, volume, turbo). **No color fields.** Not relevant to this feature.

`minuisettings.txt` — plain text `key=value` file at `/mnt/SDCARD/.userdata/shared/minuisettings.txt`. Contains 7 color fields stored as `0xRRGGBB` hex integers, plus unrelated fields (font, clock, Wi-Fi, etc.) that are ignored.

| Key | Semantic role | NextUI default |
|-----|--------------|----------------|
| `color1` | Main UI color | `0xFFFFFF` |
| `color2` | Primary accent / selection highlight | `0x9B2257` |
| `color3` | Secondary accent (header/footer bg) | `0x1E2329` |
| `color4` | List text | `0xFFFFFF` |
| `color5` | Selected item text | `0x000000` |
| `color6` | Hint / button label text | `0xFFFFFF` |
| `color7` | Background | `0x000000` |

---

## Architecture

Three areas change; nothing else is touched.

### 1. `internal/theme/` (new package)

```
internal/theme/
  theme.go       — Theme struct + Load(path string) Theme
  theme_test.go  — unit tests (no SDL dependency, no build tag)
```

`Theme` holds 7 RGB color triples. `Load` returns hardcoded grayscale defaults silently when the file is missing or unreadable; logs one `WARN` line per unparseable field when the file exists but contains bad data. Partial themes are valid.

`Load` never returns an error — callers always receive a usable `Theme`.

### 2. `internal/renderer/` (extended)

`renderer.go` gains four new drawing helpers and stores the theme:

| New method | Purpose |
|---|---|
| `DrawPill(x, y, w, h, r, g, b)` | Rounded rect — radius clamped to `h/2` (true capsule) |
| `DrawCircleBadge(cx, cy, d, r, g, b)` | Filled circle for face-button labels (A, B) |
| `DrawTagPills(tags []string, x, y, maxW, lineH, fgR,fgG,fgB, bgR,bgG,bgB) int32` | Wraps and renders tag pill badges; returns total height used |
| `DrawFooterHints(hints []FooterHint, y int32)` | Renders the hint bar from a typed slice using circle/pill badge shapes |

`FooterHint` is a small struct: `{Kind: Circle|Pill, Label: "A", Text: "Open"}`.

`Renderer` gains a `Theme theme.Theme` field set once at construction.

`DrawHeaderBar` and `DrawFooterBar` updated to use `r.Theme.HeaderBG` for the bar background and `r.Theme.Accent` for the 2 px separator line.

### 3. Screen files (updated)

All screen `Draw` methods updated in-place. No new screen files created.

---

## Data Flow

```
main.go
  ├─ theme.Load(SHARED_USERDATA_PATH + "/minuisettings.txt")
  │     → Theme{} (defaults if file missing)
  ├─ renderer.New(title, w, h, theme)      ← theme stored in r.Theme
  └─ ui.NewListScreen(r, ...)
       └─ all screens receive *renderer.Renderer
            └─ read r.Theme at draw time (read-only, no locking needed)
```

- Theme is read once at startup, same lifecycle as `config.json`.
- Screens never import `internal/theme` directly — colors reached via `r.Theme`, keeping the import graph flat.
- `internal/theme` has no build tag — fully testable without SDL.

---

## Color Mapping

| `Theme` field | `minuisettings.txt` | Fallback default | Used for |
|---|---|---|---|
| `Background` | `color7` | `#141414` | `r.Clear`, panel fills, image box backgrounds |
| `HeaderBG` | `color3` | `#1E1E1E` | Header + footer bar background |
| `Accent` | `color2` | `#3C3C5C` | Selection pill, sort badge, button badges, header separator |
| `AccentText` | `color5` | `#DCDCDC` | Text inside selection pill |
| `ListText` | `color4` | `#DCDCDC` | Unselected row text, general body text |
| `HintText` | `color6` | `#8C8C8C` | Footer hint labels |
| `MainText` | `color1` | `#DCDCDC` | Author, metadata, description text |

Fallback defaults match the current hardcoded values exactly — missing `minuisettings.txt` produces no visible change.

---

## UI Changes Per Screen

### Shared (all screens)

- **Selection highlight**: full-width `DrawRect` → `DrawPill` with side margins, filled with `Accent`. Selected row text switches to `AccentText`.
- **Footer hints**: free-form `DrawSmallText` string → `DrawFooterHints` slice rendering circle badges (A, B face buttons) and pill badges (START, L/R, SEL, D-pad).
- **Header bar**: background `HeaderBG`, 2 px separator line in `Accent`.

### `screen_list.go`

- **Sort badge**: `DrawText` with ad-hoc colors → `DrawPill` in `Accent`. Always accent — no semantic colors for sort mode.
- **Right panel — removed**: price text line (already present next to the game title in the list).
- **Right panel — added**: cover overlay corner badges — `DL` (cyan, semantic) and `UPDATE` (amber, semantic) stacked in the top-right corner of the cover art, drawn after the texture.
- **Right panel — replaced**: comma-joined `DrawText` tag line → `DrawTagPills` with accent-tinted background. Respects existing `tagScrollY` auto-scroll logic.

### `screen_settings.go`

- Selection highlight: shared pill change.
- API key status badge (`WORKING` / `REJECTED` / `PRESENT`): small `DrawPill` with semantic colors (green / red / neutral) — intentionally not themed.

### `screen_detail.go`

- Header background `HeaderBG`, separator `Accent`.
- Action row (`[ A: Download ]`): `DrawText` → `DrawPill` in semantic green (free) or amber (paid).
- Tag line: `DrawWrappedText` → `DrawTagPills` with accent-tinted background.
- Modal box border tinted with `Accent`; title color stays semantic (amber = warning, red = destructive).

### `screen_rom_picker.go`, `screen_format_picker.go`, `screen_purchase_picker.go`

- Selection highlight: shared pill change.
- Format picker format badges (`[GB]` / `[GBC]` / `[ZIP]`): `DrawSmallText` → `DrawPill` with existing semantic colors — intentionally not themed.

### `screen_location_picker.go`

- Path bar background: hardcoded `#252525` → `HeaderBG`.
- Directory rows: shared pill selection.
- **"Save here" confirm row stays semantic green** — it is an action affordance, not a theme element.

### `screen_download.go`

- Progress bar fill stays green (`#50C878`) — semantic success color.
- "Download complete!" text stays green; error text stays red — both semantic.

---

## Error Handling & Fallback

| Condition | Behaviour |
|---|---|
| File not found | Return defaults, no log |
| File unreadable (permissions) | Return defaults, no log |
| File exists, line malformed | Skip line, keep default for that field, log one `WARN` |
| File exists but empty | Return defaults, no log |
| Hex value out of 24-bit range | Treated as malformed |

Partial themes are valid — a user who only set `color2` gets accent theming with everything else at default.

---

## Testing

### `internal/theme/theme_test.go`

| Case | Assertion |
|---|---|
| Valid file, all 7 fields | All RGB values parsed correctly |
| Valid file, subset of fields | Parsed fields correct; missing fields at default |
| Missing file | All defaults returned, no panic |
| Malformed hex on one line | That field at default; others parsed correctly |
| Empty file | All defaults returned |

### Renderer helpers

Gated by `!headless` build tag — no unit tests. Verified visually via `make deploy` to device, same as all other rendering code.

---

## Out of Scope

- Hot-reload of theme while the pak is running.
- Reading `msettings.bin` (hardware knobs only, no color fields).
- Exposing per-screen color overrides.
- Changing any screen layout geometry beyond what the mockups show.
