# my355 Polish — Design Spec

Date: 2026-04-28

## Overview

Three targeted improvements for the Miyoo Flip (my355, 640×480) and one general fix
for the game-list tag display that affects all platforms.

---

## 1. Footer hint abbreviation on narrow displays

### Problem
The list-screen footer combines pagination info and button hints into one line:

```
Page 1/10 · 200/1000 games  |  A:select  L/R:page  SELECT:sort  B:exit  Start:settings
```

On a 640 px wide display with the small font (~22 px tall, ~9 px/glyph) this overflows
and the right-hand hints are clipped.

### Solution
At draw time, branch on `r.W <= 640` and substitute shorter strings. No new struct
fields. Applied in `screen_list.go` and `screen_detail.go`.

**List screen**

| Display | Hints string |
|---------|-------------|
| Wide (> 640) | `A:select  L/R:page  SELECT:sort  B:exit  Start:settings` |
| Narrow (≤ 640) | `A:sel  L/R  SEL:sort  B:exit  ⚙` |

**Detail screen**

| Display | Hints string |
|---------|-------------|
| Wide (> 640) | `B:back  \|  L/R:screenshots  \|  Start:settings` (+ scroll hint) |
| Narrow (≤ 640) | `B:back  L/R  ⚙` (+ scroll hint) |

Loading and error state footers (`B:back  |  Start:settings`) follow the same
narrow variant: `B:back  ⚙`.

### Scope
- `internal/ui/screen_list.go` — `Draw()` footer block
- `internal/ui/screen_detail.go` — all four footer `DrawSmallText` call sites

---

## 2. Double-input deduplication on my355

### Problem
The Miyoo Flip d-pad generates two `KEYDOWN` events per physical press (no
intervening `KEYUP`). This causes every directional input to move the cursor twice.

### Solution
In `main_sdl.go`, read `PLATFORM` once before the event loop. When
`platform == "my355"`, maintain a `pressedScancodes map[sdl.Scancode]bool`:

- **KEYDOWN**: if the scancode is already in the map, drop the event (it is a
  duplicate). Otherwise add it to the map and pass the event through.
- **KEYUP**: remove the scancode from the map and pass the event through.

All other platforms: map is nil, zero overhead, no code path change.

The filter sits before `current.HandleEvent(e)`, so no screen code changes.

### Scope
- `cmd/itchio-pak/main_sdl.go` — event poll loop

---

## 3. Tag display in the list-screen right panel

### Problem
`g.Tags` (bracket tags from the RSS title, e.g. `[Adventure]`, `[Windows]`,
`[macOS]`) are rendered one tag per line in the right panel below the price row.
Games that list many platform targets (e.g. *Hungry Huey*, *Marron's Day*) produce
enough lines to overflow below the footer bar.

### Solution — comma-join → wrap → vertical scroll if overflow

**Step 1 — Filter and join**
Apply the existing price/free-tag filter, then join the remaining tags with `", "`:
```
"Adventure, Windows, macOS, Linux"
```

**Step 2 — Wrap**
Call `r.WrapText(tagLine, rightW)` to get `[]string` lines that fit the right panel
width.

**Step 3 — Measure available space**
```
availH = r.H - footerH - metaY
wrappedH = int32(len(lines)) * lineGap
```

**Step 4 — Static or scrolling render**

- `wrappedH <= availH`: draw wrapped lines statically; no scroll.
- `wrappedH > availH`: apply a clip rect `[metaY, metaY+availH]`, draw lines at
  `metaY - tagScrollY`, then clear clip rect.

**Scroll animation** (mirrors `titleScrollX` pattern):

- `tagScrollY int32` — current vertical pixel offset.
- `tagScrollAt time.Time` — reset to `time.Now()` whenever the cursor moves
  (same locations where `titleScrollX` is reset).
- Speed: 30 px/s, initial delay: 1 s.
- Formula: `tagScrollY = int32((elapsed - scrollDelay).Seconds() * 30)`
- Clamped to `maxTagScroll = wrappedH - availH`.
- When `tagScrollY == maxTagScroll` and the pause period has elapsed, reset both
  fields to zero/now (same reset logic as `titleScrollX`).

### New struct fields on `ListScreen`
```go
tagScrollY   int32
tagScrollAt  time.Time
```

Both fields reset to zero/zero-value at the same call sites as `titleScrollX`.

### Scope
- `internal/ui/screen_list.go` — `ListScreen` struct + `Draw()` right-panel tag block

---

## Test references

- **Footer overflow**: run on my355 (640×480) and confirm all hint text is visible.
- **Double-input**: navigate the game list on my355; each d-pad press should move
  the cursor exactly one row.
- **Tag overflow**: load *Hungry Huey* and *Marron's Day* from the itch.io feed and
  confirm the tag line wraps and scrolls cleanly within the right panel.
