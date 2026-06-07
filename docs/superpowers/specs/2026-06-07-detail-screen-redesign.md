# Detail Screen Redesign — Design Spec
**Date:** 2026-06-07
**Status:** Approved

## Problem

The current detail screen (`screen_detail.go`, 969 lines) has three key issues that hurt readability and usability:

1. **Screenshot too dominant.** At `contentH × 2/3` it consumes most of the viewport, pushing tags, status, and description permanently below the fold.
2. **QR code wastes prime space.** It occupies 25% of the top row even for downloadable games where users rarely need it.
3. **Description is plain text.** `extractDescription()` strips all HTML structure; paragraph breaks, bold text, and bullet lists are lost, making longer descriptions hard to scan.

## Goals

1. Fit all key information (action, status, tags, start of description) above the fold on both 1024×768 and 640×480.
2. Keep the QR code visible and accessible — just smaller.
3. Render basic HTML formatting in the description: paragraphs, bold, bullet lists, headings.

## Non-Goals

- HTML links are not clickable (gamepad has no pointer).
- CSS, tables, iframes, and inline images in descriptions are stripped.
- No layout change to any other screen.

---

## Layout — "Layout B"

### Header bar (unchanged height)

Two changes to existing header:
- **Platform badge**: small pill showing the game's platform code (e.g. `Pico-8`, `GBA`, `GB`) — derived from `game.Platform`.
- **Download status badge**: `● Downloaded` (green) if in inventory, `● Owned` (teal) if in owned-games list but not yet downloaded, nothing if neither — replaces the need to scroll to see the status card for a quick "do I have this?" check.

### Content area (scrollable, same clip rect as today)

#### Row 1 — Screenshot strip + QR

```
┌────────────────────────────────────┐  ┌──────────────┐
│                                    │  │     QR        │
│        Screenshot / Cover Art      │  │   (square)    │
│                                    │  │               │
│  Image 1/3  (←→)      ____________│  │ Scan to open  │
└────────────────────────────────────┘  │  in browser   │
                                        └──────────────┘
```

- Screenshot height: `min(contentH × 40/100, r.W × 3/8)` — about 40% of the content area, capped so wide-screen screenshots don't become too tall.
- QR column width: `r.W / 5` on wide screens, `r.W / 6` on small screens.
- Screenshot counter label below the image (same as today).

#### Row 2 — Action + Status (one line)

```
[▶ A  Download again]  [DL  poom_16.p8 → /Roms/Pico-8/…  ✕ Delete]
```

- Action button (`drawActionRow`) left-aligned.
- Status card takes the remaining width with scrolling path text.
- Both elements have the same height; aligned to the same baseline.
- On narrow screens (≤ 640 px) the status card truncates with `…` rather than scrolling when focused here.
- When the game is **not** downloaded: Row 2 shows only the action button (full width). The status card and delete control are absent.

#### Row 3 — Separator + Tags

Same tag pill rendering as today, unchanged.

#### Row 4 — Separator + Description (new: HTML-formatted)

Full-width rendered description using `DrawFormattedText` (new renderer method, see below). Scrolls with the rest of the content.

---

## HTML Description Rendering

### Data layer: `extractDescription` (game.go)

**Change**: instead of stripping to plain text, preserve a minimal set of HTML tags and return them as a lightweight markup string. Specifically, keep:

- `<p>`, `<br>` — paragraph/line breaks
- `<strong>`, `<b>` — bold
- `<em>`, `<i>` — italic (rendered as bold for simplicity, since SDL2-TTF italic is separate font)
- `<h1>`–`<h3>` — headings (rendered bold at main font size)
- `<ul>`, `<ol>`, `<li>` — lists (bullet or numbered prefix)

All other tags (links, images, iframes, tables, scripts, etc.) are stripped. The result is stored as `GameDetail.Description` (same field, different content).

### Renderer: `DrawFormattedText` (renderer.go)

New method:

```go
func (r *Renderer) DrawFormattedText(markup string, x, y, maxW, lineH int32,
    baseR, baseG, baseB uint8) int32
```

Returns total height consumed. Parses the markup string with a simple state machine:

| Tag | Rendering |
|---|---|
| `<p>` / `</p>` | Extra vertical gap between paragraphs (`lineH / 2`) |
| `<br>` | Single line break |
| `<strong>`, `<b>` | Switch to bold font for enclosed text |
| `<h2>`, `<h3>` | Bold font, slightly brighter colour, extra gap above |
| `<ul>` / `<ol>` | Push list context (unordered / ordered); pop on `</ul>` / `</ol>` |
| `<li>` inside `<ul>` | `•  ` prefix, indented by `lineH / 2` |
| `<li>` inside `<ol>` | `1.  `, `2.  `, … (auto-increment counter per list), same indent |
| Anything else | Stripped |

Text between tags is word-wrapped using the existing `WrapText` logic but with a font-switch mechanism: bold runs call `DrawBoldText`/`BoldTextSize`, normal runs call `DrawText`/`TextSize`.

The parser does NOT need to be spec-compliant HTML — it only needs to handle the subset produced by itch.io's `formatted_description` div.

### Italic

Italic is mapped to bold (same `DrawBoldText`) because SDL2-TTF italic requires a separate font file we don't currently load. This is acceptable for the small amount of italic used in game descriptions.

---

## Files Changed

| File | Change |
|---|---|
| `internal/itchio/game.go` | `extractDescription`: preserve `<p>`, `<br>`, `<b>`, `<strong>`, `<em>`, `<i>`, `<h1>`–`<h3>`, `<ul>`, `<ol>`, `<li>` instead of stripping all tags |
| `internal/renderer/renderer.go` | Add `DrawFormattedText(markup, x, y, maxW, lineH, r, g, b) int32` |
| `internal/ui/screen_detail.go` | Redesign `Draw()`: new row layout, platform/status in header, smaller screenshot, `DrawFormattedText` for description |

---

## Responsive Behaviour

The layout adapts via the existing `narrowScreenW = 640` threshold and `LayoutFor(r.W, r.H)`:

| Element | Wide (> 640 px) | Small (≤ 640 px) |
|---|---|---|
| Screenshot height | `contentH × 40/100` | `contentH × 38/100` |
| QR column width | `r.W / 5` | `r.W / 6` |
| Font sizes | main + small (unchanged) | main + small (unchanged) |
| Platform badge | shown in header | shown in header |
| Status badge label | "● Downloaded" | "● DL" |

---

## Button Map (unchanged)

| Button | Action |
|---|---|
| B | Back to list |
| ← / → | Previous / next screenshot |
| ↑ / ↓ | Scroll content |
| A | Primary action (Download / Download again / Purchase required) |
| X | Delete (on status card) |
| START | Settings |
