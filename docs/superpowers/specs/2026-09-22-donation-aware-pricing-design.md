# Donation-Aware Pricing — Design Spec

**Date:** 2026-09-22
**Status:** Proposed

## Overview

The Pak sorts every game into Free or Paid on the strength of one number: the
`<price>` element in itch.io's RSS feed. Developers who publish under *name your
own price* — free to download, but with a suggested amount and an explicit ask
for support — land in the Free bucket and are presented to the user as if the
developer had asked for nothing. The Pak downloads their work and never repeats
their request.

This spec adds a third pricing class, detects it from markup the app already
downloads, fetches the developer's suggested amount, and surfaces the ask on the
detail screen as a full-width line between the screenshot row and the action
row. Nothing is blocked, gated, or nagged: the download stays one press of A.

---

## Evidence

Measured against live itch.io pages on 2026-09-22, sampling the
`made-with-gb-studio` feed.

### The feed cannot distinguish the two

29 of 36 items in page 1 of the feed report `<price>$0.00</price>`. Name-your-own-price
games and genuinely free games are identical at this level. `parsePrice` maps
both to `0`, and `Game.IsFree` to `true`. **Classification is only possible after
the game page is fetched**, which happens when the user opens the detail screen.

### The game page distinguishes them cleanly

| Class | Markup in the game page | Sample |
|---|---|---|
| Genuinely free | `download_btn`, **no** `buy_row` | `leafthief.itch.io/crisscrosscove` |
| Name your own price | `buy_row` → `buy_btn` → `/purchase`, `<span class="sub">Name your own price</span>`, **no** `dollars` span | `benjelter.itch.io/opossum-country` |
| Paid | `buy_row` with `<span class="dollars" itemprop="price">$3.00 USD</span>` | `testdata/game_page_paid.html` |

Four of five `$0.00` games sampled were name-your-own-price, not free — so this
affects the majority of the catalogue, not an edge case.

`FetchGameDetail` already reads `buy_row` for `BrowserOnly` detection
(`internal/itchio/game.go:125`). The new classification costs two more checks on
a string already in memory.

### The suggested amount lives on the purchase page

`<gameURL>/purchase` renders the developer's suggested amount as the pre-filled
value of the money input:

```html
<input value="$3.00" name="price" class="money_input" placeholder="$0.00" type="text" data-currency="USD"/>
```

Every name-your-own-price game sampled had one:

| Game | Suggested |
|---|---|
| `joelj.itch.io/fall-from-space` | $1.00 |
| `playinstinct.itch.io/capybara-village` | $1.00 |
| `benjelter.itch.io/opossum-country` | $2.00 |
| `fnife-games.itch.io/emo` | $2.69 |
| `kaimatten.itch.io/delia-the-traveling-witch` | $3.00 |
| `objetdiscret.itch.io/tai-fab-museum` | $8.00 |
| `benjelter.itch.io/the-machine` | `/purchase` → 404 (genuinely free) |

The page is ~11 KB. The `404` on a genuinely free game is a useful corroborating
signal but is **not** relied upon — the game page has already answered the
question by then.

---

## Non-goals

- **No payment on device.** There is no browser and no payment sheet. Every call
  to action necessarily ends off-device, at the URL behind the existing QR code.
- **No change to the QR code.** It continues to encode the game page. It was
  considered and rejected: retargeting it at `/purchase` changes what a scan
  means for every game.
- **No gating, modal, or interruption.** No confirmation before download, no
  "are you sure you don't want to pay", no per-game suppression state.
- **No change to the list screen.** See Deferred Decisions.
- **No firmware-specific behaviour.** This is scraper plus one drawing block;
  `firmware.Env` is not consulted and muOS and NextUI behave identically.

---

## Section 1 — Pricing classification (`internal/itchio/game.go`)

### New type

```go
// PricingModel is how a developer has chosen to charge for a game. It is
// derived from the game page, not the feed: the feed reports $0.00 for both
// free and name-your-own-price games.
type PricingModel int

const (
	PricingFree            PricingModel = iota // downloadable, no purchase section
	PricingNameYourOwnPrice                    // purchase section with no minimum price
	PricingPaid                                // purchase section with a minimum price
)
```

### New fields on `GameDetail`

```go
type GameDetail struct {
	// ... existing fields ...
	Pricing        PricingModel // how the developer charges; see PricingModel
	SuggestedPrice string       // developer's suggested amount as itch.io displays it,
	                            // e.g. "$2.00"; empty when unknown or not applicable
}
```

`SuggestedPrice` is stored as the **display string itch.io produced**, not a
parsed float. itch.io has already applied the developer's currency and
formatting; re-deriving them from a float would mean owning a currency table to
turn `2` back into `$2.00` versus `€2,00`. The value is only ever displayed,
never compared or arithmetic'd, so the string is the honest representation.

### Detection

Inside `FetchGameDetail`, after the existing `BrowserOnly` block (which already
computes `hasBuySection`):

```go
// Classify how the developer charges. The check is structural rather than
// textual: itch.io renders a `dollars` span only when a minimum price exists,
// so its absence inside the buy row means name-your-own-price. Matching the
// English string "Name your own price" would break on a localised page.
buyRow := extractBuyRow(s)
hasMinPrice := buyRow != "" && dollarsRegex.MatchString(buyRow)
switch {
case hasBuySection && !hasMinPrice:
	detail.Pricing = PricingNameYourOwnPrice
case hasBuySection:
	detail.Pricing = PricingPaid
default:
	detail.Pricing = PricingFree
}
logger.Info("game: pricing=%v (buyRow=%v minPrice=%v) for %s",
	detail.Pricing, hasBuySection, hasMinPrice, gameURL)
```

`extractBuyRow` returns the `buy_row` div's markup so the `dollars` lookup is
**scoped to the buy row**. A page-wide search would misfire: bundle banners and
related-game tiles carry their own prices, and a name-your-own-price game inside
a bundle would be misread as paid — the worst failure available, since it would
tell the user to purchase a game they can download.

`dollarsRegex` matches `<span[^>]*class="[^"]*\bdollars\b[^"]*"`.

**Failure direction.** If itch.io changes the markup and `buy_row` stops
matching, every game classifies as `PricingFree`: no band, current behaviour,
nothing blocked. If the `dollars` span moves, a paid game could classify as
name-your-own-price and show a donation line — cosmetically wrong, but the
existing `IsFree`/API-key logic still governs whether a download is attempted,
so no download path changes.

---

## Section 2 — Suggested amount (`internal/itchio/download.go`)

### New method

```go
// FetchSuggestedPrice returns the developer's suggested amount for a
// name-your-own-price game, as itch.io displays it (e.g. "$2.00"). It returns
// an empty string, and no error, when the game has no purchase page or the
// developer suggested nothing: both are ordinary states, not failures.
func (c *Client) FetchSuggestedPrice(gameURL string) (string, error)
```

Behaviour:

1. `GET <gameURL>/purchase`.
2. `404` → `("", nil)`. The game has no purchase flow.
3. Parse the value attribute of `<input name="price">`.
4. A value of `$0.00`, or one that is absent, empty, or fails a
   `^\p{Sc}?[\d.,]+$`-shaped sanity check → `("", nil)`. The developer enabled
   donations without suggesting an amount.
5. Otherwise return the trimmed value.

Log at `Info` on success, at `Warn` on a parse miss, and at `Error` only for
transport failures, using the `uploads:` prefix that the rest of
`download.go` uses. Every return path is logged, per the project's logging
standard.

### Call site (`internal/ui/screen_detail.go`)

The existing goroutine in `NewDetailScreen` fetches the detail, assigns
`s.detail`, and pushes an `sdl.UserEvent` to wake the render loop. The suggested
price is fetched **in that same goroutine, after** `s.detail` is assigned, and
only when `d.Pricing == itchio.PricingNameYourOwnPrice`:

```go
if d != nil && err == nil && d.Pricing == itchio.PricingNameYourOwnPrice {
	if price, perr := client.FetchSuggestedPrice(game.URL); perr != nil {
		logger.Warn("detail: FetchSuggestedPrice: %v", perr)
	} else {
		d.SuggestedPrice = price
	}
	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
}
```

This is deliberately **progressive**, not blocking. The band appears as soon as
the classification is known, with the generic wording; when the second request
lands a fraction of a second later, the band redraws with the amount. The user
never waits on the extra request, and a failed or slow purchase page costs only
the amount, never the ask.

The request is skipped entirely for free and paid games, so it adds no traffic
for the games where it would tell us nothing.

---

## Section 3 — The donation band (`internal/ui/screen_detail.go`)

### Placement

A full-width row inserted between the screenshot/QR top row and the action area
— immediately before the `// ── Action area (full width)` comment, at the point
where `y` has finished accounting for the top row in both the with-screenshots
and no-screenshots branches:

```go
if s.detail != nil && s.detail.Pricing == itchio.PricingNameYourOwnPrice {
	y = s.drawDonationBand(r, margin, y, usableW)
}
```

`s.contentHeight` is derived from the final `y`
(`s.contentHeight = y - (contentTop - s.scrollY)`), so the scroll extent absorbs
the new row with no further change.

### Drawing

```go
// drawDonationBand renders the developer's request for support as a full-width
// section between the screenshot row and the action row. It is drawn only for
// name-your-own-price games and never blocks or alters the download path.
func (s *DetailScreen) drawDonationBand(r *renderer.Renderer, x, y, w int32) int32 {
	_, fontH := r.TextSize("Ag")
	text := "Consider donating to the developer"
	if s.detail.SuggestedPrice != "" {
		text = "Consider donating — " + s.detail.SuggestedPrice + " suggested"
	}
	sep := r.Theme.Separator()
	tone := r.Theme.ToneOn(r.Theme.Price(), r.Theme.Background)

	y += 8
	r.DrawRect(x, y, w, 1, sep[0], sep[1], sep[2])
	y += 10
	r.DrawTextCentered(text, x, y, w, tone[0], tone[1], tone[2])
	y += fontH + 10
	r.DrawRect(x, y, w, 1, sep[0], sep[1], sep[2])
	return y + 10
}
```

Total height `fontH + 38`.

### Colour

Two constraints, both learned from failed renders rather than reasoning:

- **The text tone is `ToneOn(Price(), Background)`, not `Price()`.** Raw
  `Price()` on a light palette measured contrast 53 against Catppuccin Latte's
  background — legible in theory, washed out in the frame.
- **Nothing here uses the tinted `PriceBG`/`SuccessBG` badge idiom.** A tinted
  pill with matching text measured contrast 52 on light palettes. That treatment
  is fine for a status badge nobody has to read at a glance and wrong for a call
  to action. Where a filled shape is wanted, the rule is a **solid** fill with
  `ContrastText()` on top.

The band as specified reported no findings from `--audit` across all 18 bundled
palettes at 1024×768, 720×720 and 640×480.

### Geometry

Measured width of the rendered string in the main font versus usable width
(`r.W - 40`):

| Geometry | Usable | `— $2.00 suggested` | `— $10.00 suggested` | No amount |
|---|---|---|---|---|
| 1024×768 | 984 | 593 | 612 | 566 |
| 720×720 | 680 | 557 | 575 | 528 |
| 720×480 | 680 | 377 | 389 | 358 |
| 640×480 | 600 | 377 | 389 | 358 |

Every case fits, with 720×720 the tightest at 85% of usable width. Going
full-width is what makes this work: the same call to action does **not** fit
beside the QR code, whose column is `r.W/6` — 106 px on the Miyoo Flip, 120 px
on the RG XX family — where `Donate $2.00` overflows by ~13 px on each side and
crosses into the screenshot box.

**The implementation must still truncate defensively**: if the measured text
exceeds `w`, fall back to the no-amount wording rather than overflowing. A
three-digit suggestion in a wide currency is not in the sample above.

### Wording

`Consider donating — $2.00 suggested`, and `Consider donating to the developer`
when the amount is unknown.

"$2.00 **or more**" was considered and rejected. That is itch.io's phrasing for a
paid game with a *minimum* price. Under name-your-own-price the amount is a
suggestion and $0 is genuinely permitted, so "or more" asserts a floor that does
not exist — misrepresenting the developer's terms in the opposite direction from
the bug this spec fixes.

---

## Section 4 — Fixtures and tests

### The existing "free" fixture is not free

`testdata/game_page_free.html` is a capture of `benjelter.itch.io/opossum-country`,
which is **name-your-own-price** — `buy_row`, `buy_btn`, no `download_btn`. Two
tests in `internal/itchio/game_test.go` (lines 27 and 41) currently treat it as
the free case. This must be corrected as part of the work, not worked around:

- Rename it to `testdata/game_page_nyop.html` and update both call sites.
- Add `testdata/game_page_free.html` as a genuine free capture — a page with
  `download_btn` and no `buy_row` (`leafthief.itch.io/crisscrosscove` is a
  verified example).
- Add `testdata/purchase_page.html`, a capture of a name-your-own-price purchase
  form including the pre-filled `<input name="price" value="$3.00">`.

### Unit tests (`internal/itchio`)

| Test | Fixture | Asserts |
|---|---|---|
| Classifies a free game | `game_page_free.html` | `PricingFree` |
| Classifies name-your-own-price | `game_page_nyop.html` | `PricingNameYourOwnPrice` |
| Classifies a paid game | `game_page_paid.html` | `PricingPaid` |
| Bundle price does not leak | synthetic: NYOP buy row + bundle tile with a `dollars` span | `PricingNameYourOwnPrice` |
| Parses the suggested amount | `purchase_page.html` | `"$3.00"` |
| Absent purchase page | `httptest` 404 | `("", nil)` |
| No suggestion offered | synthetic: `value="$0.00"` | `("", nil)` |
| Malformed input | synthetic: no price input | `("", nil)` |

All served through `httptest.NewServer`, per the project's no-live-network rule.

### Visual coverage (`internal/ui`)

Add a `detail-donation` scene to `internal/ui/dev_scenes.go` — a detail screen
whose fixture detail carries `PricingNameYourOwnPrice` and a suggested price.
This puts the band into `scripts/palette-audit.sh`, so it is re-checked against
18 palettes × 4 geometries on every run of `./scripts/test.sh` rather than once,
now, by hand.

### Verification before claiming completion

`./scripts/test.sh` (which runs the palette audit and the non-headless `ui`
compile) plus a real cross-compile — `./scripts/build.sh nextui/tg5040` — since
`-tags headless` does not compile the SDL drawing path and a green CI check
proves nothing about it.

---

## Deferred decisions

**The list badge keeps saying "Free" for name-your-own-price games.** The feed
gives `$0.00` for both classes, so the list cannot know the difference without
either fetching a page per row — unacceptable for a list of hundreds — or
persisting what past detail visits learned, which means a new cache file with
its own invalidation story alongside `games_cache.json` and `owned_cache.json`.
The badge is also not *wrong*: these games genuinely are free to download. The
developer's request is surfaced at the point where the user decides to take the
game, which is the detail screen. Revisit if the third class proves worth
showing earlier.

**No footer hint, modal, or pressable donate button.** A pressable affordance
needs somewhere to live; the `X` badge plus label overflows the QR column at
every width below 1024 px, which leaves the footer, which means a modal to open.
That is a larger change than the ask, and the band already carries the message.
Left out deliberately, not overlooked.

---

## Risks

| Risk | Mitigation |
|---|---|
| itch.io changes the buy-row markup | Classification degrades to `PricingFree` — no band, current behaviour, no download path affected |
| A paid game misclassifies as name-your-own-price | Shows an unwanted line; download gating still keyed on the existing `IsFree`/API-key logic, so nothing becomes purchasable-by-accident |
| Extra ~11 KB request per NYOP detail view | Skipped for free and paid games; runs after the detail is on screen, so it is never on the critical path |
| A localised page breaks string matching | Detection is structural (`buy_row` + presence of a `dollars` span), never the English phrase |
| Band pushes the action row below the fold at 640×480 | Accepted. `btnA` calls `startDownload()` unconditionally (`screen_detail.go:994`); the row is a label, not a focusable widget, so nothing becomes unreachable |
