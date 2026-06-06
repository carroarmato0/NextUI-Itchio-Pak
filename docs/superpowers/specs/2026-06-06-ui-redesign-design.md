# UI Redesign — Design Spec
**Date:** 2026-06-06
**Status:** Approved

## Problem

The game catalogue has grown from ~1 300 to 10 000+ titles across 6 platforms (GB, GBC, GBA, NES, MD, Pico-8). The current UI was designed for a single-platform, single-feed world and does not scale:

- No way to filter by platform
- No search
- L1/R1 cycles sort modes — no room for alpha-jump navigation
- Sort badge in top-right corner is redundant once filter pills exist
- Spacing constants are hardcoded; no systematic small-screen adaptation
- Confirmation modals are drawn inconsistently across screens

## Goals

1. Platform filtering + search without sacrificing the discovery-first browsing experience
2. Alpha-jump navigation in A-Z sorted lists
3. A shared virtual keyboard usable for search and API key entry
4. Dynamic layout that tightens automatically on the Miyoo Flip (640×480)
5. Consistent footer hint ordering and modal style across all screens

## Non-Goals

- Detail, download, manage-downloads, migrate, zip-inspect screens — unchanged
- ROM placement, inventory, power management, settings persistence — unchanged
- Cache/fetch logic — unchanged
- Tag filter screen — unchanged
- New itch.io platforms beyond the existing 6 (GB, GBC, GBA, NES, MD, P8)

---

## Architecture

### Implementation approach

**Hybrid screen-stack + inline modals:**

- Large overlays (`FilterScreen`, `KeyboardScreen`) are full `Screen` implementations that wrap the screen beneath them via `prev Screen`. They draw a solid dark background (no live redraw of the list, no SDL snapshot) — clean and cheap on embedded hardware.
- Small one-shot confirmations (delete-all, clear cache, dismiss update) stay inline in their respective screens, drawn with the new `renderer.DrawModal` helper for visual consistency.

No changes to the `Screen` interface, `BusyChecker`, event routing, power management, inventory, or any existing download/detail/settings screens.

---

## Layout System

**File:** `internal/ui/layout.go`

All spacing values are derived from screen dimensions at draw time. Two size classes, selected by the existing `narrowScreenW = 640` threshold on `r.W`:

| Constant | Wide (r.W > 640) | Small (r.W ≤ 640) |
|---|---|---|
| `headerPad` | 6 px | 3 px |
| `rowPad` | 4 px | 2 px |
| `footerPad` | 5 px | 2 px |
| `contentGap` | 6 px | 3 px |
| `coverMaxW` | 75% of right panel width | 100% of right panel width |
| `overlayMargin` | 14% of screen width | 3% of screen width |

New screens use these constants from the start. Existing screens adopt them incrementally; `screen_list.go` is updated as part of this redesign.

---

## List Screen (`screen_list.go`)

### Header

- Left: "Itch.io" title (unchanged)
- Right: two active-filter pills — `● <platform>` and `● <sort>` — replace the existing single sort badge pill
- Pills use accent colour with border when active; dimmed when default (All / RSS)

### Right panel

- Split ratio: 55% list / 45% right panel (unchanged)
- Cover art: rendered at `coverMaxW` of the right panel width, centred horizontally
- Below cover art (static, no scrolling needed): title, author, tag pills, price/status badge
- Tags shown as accent-coloured pills wrapping to multiple lines if needed; clipped to available height

### L1/R1 behaviour

| Sort mode | L1/R1 action | Footer hint |
|---|---|---|
| A-Z or Z-A | Alpha-jump (next/prev letter boundary) | `L1R1: A→Z` |
| All other modes | Page-scroll (existing behaviour) | `L1R1: Page` |

### Alpha-jump logic

On L1/R1 press, scan `viewGames` forward (R1) or backward (L1) from `cursor` until `SortKey(title)[0]` differs from the current game's first character. Jump cursor to that index. Clamps at list boundaries — no wrap. Standard shoulder auto-repeat applies on hold.

### SELECT button

Opens `FilterScreen` on top of `ListScreen`.

### New state fields

```go
platformFilter string  // "" = All; persisted to config.json
searchQuery    string  // "" = no filter; not persisted (cleared on exit)
```

`rebuildView()` applies `platformFilter` and `searchQuery` before calling `ApplySort`.

### Persistence

`platformFilter` is saved to `config.json` alongside `SortMode`. `searchQuery` is session-only — cleared when the app exits.

---

## Filter Overlay (`screen_filter.go`)

### Layout

Three sections, top-to-bottom:

1. **Search** — text field showing current query; press A to open `KeyboardScreen`
2. **Platform** — pill row: All · GB · GBC · GBA · NES · MD · P8 (single-select)
3. **Sort** — pill row: RSS · A-Z · Z-A · New · Free · Paid · DL · Owned (single-select)

Overlay panel is centred horizontally, scaled with `overlayMargin`. On small screens it goes nearly full-width.

### Navigation

- D-pad ↑↓: move focus between sections
- D-pad ←→: move cursor within the active section's pill row
- **A**: toggle focused platform/sort pill active; on search field — push `KeyboardScreen`
- **SELECT**: apply selections and pop back to list
- **B**: discard changes and pop back (revert to previous platform/sort/search)
- **Y**: reset all three fields to defaults (All / RSS / empty search), apply, pop

### State

`FilterScreen` is constructed with an `onApply func(platform, sort, query string)` callback — no direct dependency on `ListScreen`. On discard (B), no callback is fired. On apply (SELECT), `onApply` is called with the selected values; the caller (`ListScreen`) updates its state and triggers `rebuildView()`.

---

## Virtual Keyboard (`screen_keyboard.go`)

### Constructor

```go
NewKeyboardScreen(prev Screen, seed string, onConfirm func(string)) *KeyboardScreen
```

`seed` pre-fills the typed field. `onConfirm` is called with the final string on confirm or with `seed` unchanged on cancel (B).

### Layout

- Typed string field at the top with blinking cursor
- Page indicator below field: `abc · ABC · 0-9`
- 4×8 character grid

### Pages (L1/R1 to cycle)

| Page | Characters |
|---|---|
| 0 — lowercase | a–z · SPC · ⌫ |
| 1 — uppercase | A–Z · SPC · ⌫ |
| 2 — digits/symbols | 0–9 · `.` `-` `_` `'` `!` `?` `@` · ⌫ |

### Navigation

- D-pad ←→: move within row; wraps horizontally
- D-pad ↑↓: move between rows; from top row ↑ focuses the typed-string field (A backspaces one character; ↓ returns to grid)
- **A**: type the focused character (or confirm if on `✓ Done`)
- **B**: cancel — fire `onConfirm(seed)` and pop
- **L1/R1**: cycle page

### Reuse

Used for: search field in `FilterScreen`, API key field in `SettingsScreen` (replaces the current `showAPIKeyHelp` inline keyboard flow).

---

## Renderer Helper — `DrawModal`

**Signature:**
```go
func (r *Renderer) DrawModal(title, body string, hints []FooterHint)
```

Draws a centred modal box scaled to screen size. Replaces the ad-hoc inline modal drawing currently duplicated across screens. Used by: delete-all confirmation, clear-cache confirmation, Cloudflare error block, and any future confirmations.

---

## Consistency Rules

### Footer hint order

Fixed left-to-right across all screens:

> A (confirm) → B (back/cancel) → X (secondary) → SELECT (overlay) → L1R1 (nav) → START (settings)

Unused slots are omitted; order is never changed. Small screens abbreviate labels (`Set` not `Settings`, `LR` not `L1R1`) but keep the same order.

### Modal style

All yes/no and informational overlays use `DrawModal`. Dark background rect, white title, dimmed body text, footer hints at bottom. No screen draws its own modal box from scratch.

---

## Button Map Summary (List Screen)

| Button | Action |
|---|---|
| D-pad ↑↓ | Navigate list |
| D-pad ←→ | — (reserved; no action on list screen) |
| A | Open game detail |
| B | Exit app |
| X | Dismiss update / removal notification |
| SELECT | Open filter overlay |
| L1 / R1 | Alpha-jump (A-Z mode) or page-scroll (other modes) |
| START | Open settings |

---

## Files Added / Modified

| File | Change |
|---|---|
| `internal/ui/layout.go` | **New** — layout constants derived from screen size |
| `internal/ui/screen_filter.go` | **New** — FilterScreen |
| `internal/ui/screen_keyboard.go` | **New** — KeyboardScreen |
| `internal/ui/screen_list.go` | **Modified** — header pills, right panel, L1R1 alpha-jump, SELECT handler, platformFilter/searchQuery fields |
| `internal/renderer/renderer.go` | **Modified** — add `DrawModal` helper |
| `internal/ui/screen_settings.go` | **Modified** — use KeyboardScreen for API key entry |
| `internal/settings/config.go` | **Modified** — add `PlatformFilter string` field |
