# Donation-Aware Pricing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect name-your-own-price games (currently indistinguishable from free games in the feed) from the game page the app already fetches, and surface the developer's suggested donation amount as a full-width band on the detail screen — without touching the QR code, the download path, or the list screen.

**Architecture:** A `PricingModel` enum (`PricingFree` / `PricingNameYourOwnPrice` / `PricingPaid`) is derived structurally from the buy-row markup already read for `BrowserOnly` detection, inside `FetchGameDetail`. A second, optional request (`FetchSuggestedPrice`) reads the pre-filled amount on `<gameURL>/purchase` and is fired progressively — after the detail screen's first redraw — so it never delays initial paint. `DetailScreen.Draw()` renders a full-width `drawDonationBand()` between the screenshot/QR row and the action row whenever `Pricing == PricingNameYourOwnPrice`. Nothing about the download path, the QR target, or the list screen changes.

**Tech Stack:** Go, `net/http`, `regexp` (structural HTML matching, consistent with the rest of `internal/itchio`), SDL2 rendering via `internal/renderer`/`internal/theme` (excluded from headless CI by `//go:build !headless`).

**Spec:** `docs/superpowers/specs/2026-09-22-donation-aware-pricing-design.md`

## Global Constraints

- No payment happens on device; every call to action ends off-device, at the existing QR code's target.
- The QR code must never be retargeted at `/purchase` — it always encodes the game page. (Considered and explicitly rejected during design.)
- No gating, modal, or confirmation before download. `btnA` continues to call `startDownload()` unconditionally.
- The list screen and its "Free" badge are unchanged — deferred by design, not an oversight.
- No `firmware.Env` involvement — this is scraper logic plus one drawing block, identical on NextUI and muOS.
- Pricing classification is **structural** (presence/absence of a `dollars` span inside the buy row) — never match the English phrase "Name your own price"; itch.io localises that string.
- `SuggestedPrice` is stored and displayed exactly as itch.io renders it (e.g. `"$2.00"`) — never parsed to a float or reformatted.
- Donation band text colour is `ToneOn(Price(), Background)` — never raw `Price()`, and never the tinted `PriceBG()`/`SuccessBG()` badge idiom (both measured contrast failures on light palettes during design).
- Wording is `"Consider donating — $2.00 suggested"` / `"Consider donating to the developer"` — never `"$2.00 or more"`, which is itch.io's phrasing for a paid minimum and would misrepresent the terms.
- Every return path of `FetchSuggestedPrice` must be logged, per the project's logging standard (goroutines, HTTP calls).
- No colour literals in drawing code — every colour comes from `internal/theme` accessors (`scripts/no-color-literals.sh` enforces this).
- Screenshots for visual verification go to `/tmp/itchio-screenshots/`, never `docs/screenshots/`.

---

## File Map

| File | Change |
|------|--------|
| `testdata/game_page_free.html` | Renamed to `game_page_nyop.html` (it was mislabeled); a new, genuinely-free fixture is added under this name |
| `testdata/game_page_nyop.html` | New name for the renamed fixture (Opossum Country — name-your-own-price) |
| `testdata/purchase_page.html` | New fixture — a purchase form with a pre-filled suggested amount |
| `internal/itchio/game.go` | Add `PricingModel` type, `GameDetail.Pricing`/`.SuggestedPrice` fields, `extractBuyRow`/`dollarsRegex`, classification logic in `FetchGameDetail` |
| `internal/itchio/game_test.go` | Fix two fixture references; add four `TestPricingClassification_*` tests |
| `internal/itchio/download.go` | Add `FetchSuggestedPrice` |
| `internal/itchio/download_test.go` | Add four `TestFetchSuggestedPrice_*` tests |
| `internal/ui/screen_detail.go` | Fetch the suggested price progressively in the existing detail goroutine; add `drawDonationBand` and its call site |
| `internal/ui/dev_scenes.go` | Add a `detail-donation` scene |

---

### Task 1: Fix the mislabeled "free" fixture

**Files:**
- Modify: `testdata/game_page_free.html` → renamed to `testdata/game_page_nyop.html`
- Modify: `internal/itchio/game_test.go:27,41,52`

**Interfaces:**
- Produces: `testdata/game_page_nyop.html` (a name-your-own-price capture: `buy_row`, `buy_btn`, `<span class="sub">Name your own price</span>`, no `download_btn`, no `dollars` span), used by Task 2's classification tests.

`testdata/game_page_free.html` is currently a capture of `benjelter.itch.io/opossum-country`, which is name-your-own-price, not free — it has a `buy_row` and no `download_btn`. Two existing tests treat it as the free case. This must be corrected before any new pricing code is added, so later tests are asserting against fixtures that are actually what their names say.

- [ ] **Step 1.1: Rename the fixture**

```bash
git mv testdata/game_page_free.html testdata/game_page_nyop.html
```

- [ ] **Step 1.2: Update the two call sites and the comment that names the fixture**

In `internal/itchio/game_test.go`:

Line 27, inside `TestFetchGameDetailExtractsGameID`:

```go
	srv := serveFile(t, "../../testdata/game_page_free.html")
```

becomes:

```go
	srv := serveFile(t, "../../testdata/game_page_nyop.html")
```

Line 41, inside `TestFetchGameDetailExtractsPageTags`:

```go
	srv := serveFile(t, "../../testdata/game_page_free.html")
```

becomes:

```go
	srv := serveFile(t, "../../testdata/game_page_nyop.html")
```

Line 52, the comment above the tag assertion:

```go
	// The free fixture (Opossum Country) has "horror" slug among its tags
```

becomes:

```go
	// The name-your-own-price fixture (Opossum Country) has "horror" slug among its tags
```

- [ ] **Step 1.3: Run the two affected tests to confirm they still pass**

Run: `go test ./internal/itchio/ -run 'TestFetchGameDetailExtractsGameID|TestFetchGameDetailExtractsPageTags' -v`
Expected: both `PASS` — neither test asserts anything about pricing, so the rename changes nothing about their behaviour, only its accuracy.

- [ ] **Step 1.4: Commit**

```bash
git add testdata/game_page_nyop.html internal/itchio/game_test.go
git status --short   # confirm the old testdata/game_page_free.html is gone (renamed, not duplicated)
git commit -m "$(cat <<'EOF'
test(itchio): rename mislabeled free fixture to game_page_nyop.html

testdata/game_page_free.html was a capture of a name-your-own-price
game (buy_row, no download_btn), not a free one. Two tests asserted
free-game behaviour against it; neither actually tests pricing, so
the rename is a no-op for their behaviour and a prerequisite for the
pricing classification tests that follow.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Classify pricing in `FetchGameDetail` (TDD)

**Files:**
- Modify: `internal/itchio/game.go`
- Modify: `internal/itchio/game_test.go`
- Create: `testdata/game_page_free.html` (genuine free capture — download_btn, no buy_row)

**Interfaces:**
- Consumes: nothing new — reads the same `s string` (page body) already computed inside `FetchGameDetail`, and the existing `hasBuySection` local.
- Produces: `itchio.PricingModel` (`PricingFree`, `PricingNameYourOwnPrice`, `PricingPaid`), `GameDetail.Pricing PricingModel`, `GameDetail.SuggestedPrice string` (populated by Task 3, zero-valued here). Task 4 and 5 read `detail.Pricing` and `detail.SuggestedPrice`.

- [ ] **Step 2.1: Add the new genuine-free fixture**

Create `testdata/game_page_free.html`:

```html
<!DOCTYPE HTML><html lang="en"><head><meta charset="UTF-8"/><meta name="itch:path" content="games/999001"/><meta name="csrf_token" value="ZmFrZS1jc3JmLXRva2Vu"/><title>Crisscross Cove by LeafThief</title></head><body>
<a href="https://itch.io/games/tag-adventure">Adventure</a>
<a href="https://itch.io/games/tag-pixel-art">Pixel Art</a>
<div class="formatted_description user_formatted"><p>A cozy exploration game for Game Boy Color.</p></div>
<div class="game_download_page"><a class="button download_btn" href="https://leafthief.itch.io/crisscrosscove/download_url">Download Now</a></div>
<div class="uploads"><p>Click download now to get access to the following files:</p><div id="upload_list_9999999" class="upload_list_widget base_widget"><div class="upload"><div class="info_column"><div class="upload_name"><strong class="name" title="crisscrosscove.gbc">crisscrosscove.gbc</strong></div></div></div></div></div>
</body></html>
```

This mirrors the evidence table's "genuinely free" row: `download_btn` present, no `buy_row` anywhere on the page.

- [ ] **Step 2.2: Write four failing tests**

Add to `internal/itchio/game_test.go`, after the last `TestBrowserOnlyDetection_*` test (end of that group, before `TestAnnotateBundleNames`):

```go
func TestPricingClassification_Free(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_free.html")
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.Pricing != itchio.PricingFree {
		t.Errorf("Pricing = %v, want PricingFree", detail.Pricing)
	}
}

func TestPricingClassification_NameYourOwnPrice(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_nyop.html")
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.Pricing != itchio.PricingNameYourOwnPrice {
		t.Errorf("Pricing = %v, want PricingNameYourOwnPrice", detail.Pricing)
	}
}

func TestPricingClassification_Paid(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_paid.html")
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.Pricing != itchio.PricingPaid {
		t.Errorf("Pricing = %v, want PricingPaid", detail.Pricing)
	}
}

func TestPricingClassification_BundlePriceDoesNotLeak(t *testing.T) {
	// A name-your-own-price buy row, followed by a bundle promo further down
	// the page that carries its own "dollars" span. A page-wide search for
	// "dollars" would misclassify this as paid; the bundle's price must not
	// leak into this game's classification.
	const pageHTML = `<html><body>
<div class="buy_row">
  <a class="button buy_btn" href="/game/purchase">Download Now</a>
  <span class="buy_message"><span class="sub">Name your own price</span></span>
</div>
<div class="uploads"><p>Click download now to get access to the following files:</p></div>
<div class="related_games">
  <div class="bundle_title"><a href="/b/999">Support Ukraine Bundle</a></div>
  <span class="dollars" itemprop="price">$5.00 USD</span>
</div>
</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.Pricing != itchio.PricingNameYourOwnPrice {
		t.Errorf("Pricing = %v, want PricingNameYourOwnPrice (bundle price leaked into classification)", detail.Pricing)
	}
}
```

- [ ] **Step 2.3: Run the new tests to verify they fail**

Run: `go test ./internal/itchio/ -run TestPricingClassification -v`
Expected: all four `FAIL` with `detail.Pricing undefined` / compile error (the field and type don't exist yet).

- [ ] **Step 2.4: Add `PricingModel` and the new `GameDetail` fields**

In `internal/itchio/game.go`, replace the `GameDetail` struct (lines 18–28):

```go
type GameDetail struct {
	Game
	Description    string
	ScreenshotURLs []string
	Uploads        []Upload
	GameID         string
	CSRFToken      string
	PageTags       []string // itch.io tag labels scraped from the game page
	BundleNames    []string // names of bundles that include this game (from public page)
	BrowserOnly    bool     // true when page has HTML5 embed but no downloadable or paid files
	Pricing        PricingModel // how the developer charges; see PricingModel
	SuggestedPrice string       // developer's suggested amount as itch.io displays it (e.g. "$2.00"); empty when unknown or not applicable
}
```

Immediately above it, add:

```go
// PricingModel is how a developer has chosen to charge for a game. It is
// derived from the game page, not the feed: the feed reports $0.00 for both
// free and name-your-own-price games.
type PricingModel int

const (
	PricingFree             PricingModel = iota // downloadable, no purchase section
	PricingNameYourOwnPrice                     // purchase section with no minimum price
	PricingPaid                                 // purchase section with a minimum price
)
```

- [ ] **Step 2.5: Add `dollarsRegex` and `extractBuyRow`**

In the `var (...)` block (lines 37–49), add a new entry after `bundleNameRegex`:

```go
	// dollars span appears only when a buy row has a minimum price:
	// <span class="dollars" itemprop="price">$3.00 USD</span>
	dollarsRegex = regexp.MustCompile(`class="[^"]*\bdollars\b[^"]*"`)
```

After `FetchGameDetail` closes (after line 131's `}`), add:

```go
// extractBuyRow returns the buy_row section of the page — from its "buy_row"
// class marker up to the uploads list that always follows it on itch.io game
// pages — so pricing detection stays scoped to the purchase widget and never
// reads a bundle banner's or related game's price elsewhere on the page.
func extractBuyRow(s string) string {
	start := strings.Index(s, "buy_row")
	if start == -1 {
		return ""
	}
	rest := s[start:]
	if end := strings.Index(rest, `<div class="uploads">`); end != -1 {
		return rest[:end]
	}
	const maxBuyRowScan = 1000
	if len(rest) > maxBuyRowScan {
		return rest[:maxBuyRowScan]
	}
	return rest
}
```

- [ ] **Step 2.6: Classify pricing inside `FetchGameDetail`**

In `internal/itchio/game.go`, after the existing `BrowserOnly` block:

```go
	hasHTML5Embed := strings.Contains(s, "html.itch.zone")
	hasDownloadBtn := strings.Contains(s, "download_btn")
	hasBuySection := strings.Contains(s, "buy_row")
	detail.BrowserOnly = hasHTML5Embed && !hasDownloadBtn && !hasBuySection
	logger.Debug("game: browserOnly=%v (embed=%v downloadBtn=%v buySection=%v)",
		detail.BrowserOnly, hasHTML5Embed, hasDownloadBtn, hasBuySection)
```

add, before `return detail, nil`:

```go

	// Classify how the developer charges. The check is structural rather than
	// textual: itch.io renders a `dollars` span only when a minimum price
	// exists, so its absence inside the buy row means name-your-own-price.
	// Matching the English string "Name your own price" would break on a
	// localised page.
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
	logger.Info("game: pricing=%d (buyRow=%v minPrice=%v) for %s",
		detail.Pricing, hasBuySection, hasMinPrice, gameURL)
```

- [ ] **Step 2.7: Run the new tests to verify they pass**

Run: `go test ./internal/itchio/ -run TestPricingClassification -v`
Expected: all four `PASS`.

- [ ] **Step 2.8: Run the full `itchio` package tests to check for regressions**

Run: `go test ./internal/itchio/... -v 2>&1 | tail -40`
Expected: all `PASS`, including the three `TestBrowserOnlyDetection_*` tests and the two fixed-in-Task-1 tests.

- [ ] **Step 2.9: Commit**

```bash
git add internal/itchio/game.go internal/itchio/game_test.go testdata/game_page_free.html
git commit -m "$(cat <<'EOF'
feat(itchio): classify name-your-own-price games

The RSS feed reports $0.00 for both free and name-your-own-price
games, so the Pak has been presenting every donation-based game as
if the developer asked for nothing. FetchGameDetail already reads
the buy_row markup for BrowserOnly detection; this adds a third
classification (PricingModel) from the same string, scoped to the
buy row so a bundle promo's price can't leak into it.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Fetch the developer's suggested amount (TDD)

**Files:**
- Modify: `internal/itchio/download.go`
- Modify: `internal/itchio/download_test.go`
- Create: `testdata/purchase_page.html`

**Interfaces:**
- Consumes: `itchio.Client` (existing), a game URL string.
- Produces: `func (c *Client) FetchSuggestedPrice(gameURL string) (string, error)`. Task 4 calls this with `game.URL` and assigns the result to `GameDetail.SuggestedPrice`.

- [ ] **Step 3.1: Add the purchase-page fixture**

Create `testdata/purchase_page.html`:

```html
<!DOCTYPE HTML><html lang="en"><head><title>Support the developer</title></head><body>
<form class="purchase_form" action="/purchase" method="post">
<input value="$3.00" name="price" class="money_input" placeholder="$0.00" type="text" data-currency="USD"/>
<button type="submit">Continue</button>
</form>
</body></html>
```

Note the attribute order: `value` before `name`, matching the real markup sampled during design — the extraction regex must not assume `name="price"` comes first.

- [ ] **Step 3.2: Write four failing tests**

Add to `internal/itchio/download_test.go`, at the end of the file:

```go
func TestFetchSuggestedPrice_ReturnsAmount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/game/purchase", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("../../testdata/purchase_page.html")
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		w.Write(data)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClient()
	price, err := c.FetchSuggestedPrice(srv.URL + "/game")
	if err != nil {
		t.Fatalf("FetchSuggestedPrice: %v", err)
	}
	if price != "$3.00" {
		t.Errorf("price = %q, want %q", price, "$3.00")
	}
}

func TestFetchSuggestedPrice_NoPurchasePage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/game/purchase", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClient()
	price, err := c.FetchSuggestedPrice(srv.URL + "/game")
	if err != nil {
		t.Fatalf("FetchSuggestedPrice: %v", err)
	}
	if price != "" {
		t.Errorf("price = %q, want empty string for a 404 purchase page", price)
	}
}

func TestFetchSuggestedPrice_NoSuggestion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/game/purchase", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><input value="$0.00" name="price" class="money_input"/></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClient()
	price, err := c.FetchSuggestedPrice(srv.URL + "/game")
	if err != nil {
		t.Fatalf("FetchSuggestedPrice: %v", err)
	}
	if price != "" {
		t.Errorf("price = %q, want empty string when the developer suggested $0.00", price)
	}
}

func TestFetchSuggestedPrice_MalformedInput(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/game/purchase", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>No price input on this page.</body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClient()
	price, err := c.FetchSuggestedPrice(srv.URL + "/game")
	if err != nil {
		t.Fatalf("FetchSuggestedPrice: %v", err)
	}
	if price != "" {
		t.Errorf("price = %q, want empty string when no price input is present", price)
	}
}
```

- [ ] **Step 3.3: Run the new tests to verify they fail**

Run: `go test ./internal/itchio/ -run TestFetchSuggestedPrice -v`
Expected: all four `FAIL` to compile — `FetchSuggestedPrice` does not exist yet.

- [ ] **Step 3.4: Add `"regexp"` to `download.go`'s imports**

In `internal/itchio/download.go`, the import block (lines 3–15) becomes:

```go
import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)
```

- [ ] **Step 3.5: Implement `FetchSuggestedPrice`**

Append to the end of `internal/itchio/download.go` (after `FetchFileHeader`, which currently ends the file at line 340):

```go

var (
	priceInputRegex  = regexp.MustCompile(`<input[^>]+name="price"[^>]*>`)
	priceValueRegex  = regexp.MustCompile(`value="([^"]*)"`)
	priceSanityRegex = regexp.MustCompile(`^\p{Sc}?[\d.,]+$`)
)

// FetchSuggestedPrice returns the developer's suggested amount for a
// name-your-own-price game, as itch.io displays it (e.g. "$2.00"). It returns
// an empty string, and no error, when the game has no purchase page or the
// developer suggested nothing: both are ordinary states, not failures.
func (c *Client) FetchSuggestedPrice(gameURL string) (string, error) {
	purchaseURL := gameURL + "/purchase"
	resp, err := c.http.Get(purchaseURL)
	if err != nil {
		logger.Error("uploads: fetch purchase page: %v", err)
		return "", fmt.Errorf("fetch purchase page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		logger.Info("uploads: no purchase page for %s (genuinely free)", gameURL)
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error("uploads: purchase page HTTP %d for %s", resp.StatusCode, gameURL)
		return "", fmt.Errorf("fetch purchase page: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("uploads: read purchase page: %v", err)
		return "", fmt.Errorf("read purchase page: %w", err)
	}

	tag := priceInputRegex.FindString(string(body))
	if tag == "" {
		logger.Warn("uploads: no price input found on purchase page for %s", gameURL)
		return "", nil
	}
	m := priceValueRegex.FindStringSubmatch(tag)
	if len(m) < 2 {
		logger.Warn("uploads: price input has no value attribute for %s", gameURL)
		return "", nil
	}
	value := strings.TrimSpace(m[1])
	if value == "" || !priceSanityRegex.MatchString(value) {
		logger.Warn("uploads: price value %q failed sanity check for %s", value, gameURL)
		return "", nil
	}
	if isZeroAmount(value) {
		logger.Info("uploads: suggested price is zero for %s (donations enabled, no suggestion)", gameURL)
		return "", nil
	}
	logger.Info("uploads: suggested price %s for %s", value, gameURL)
	return value, nil
}

// isZeroAmount reports whether value's digits are all zero (e.g. "$0.00"),
// without parsing it as a number — the value is only ever displayed, never
// computed with.
func isZeroAmount(value string) bool {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	return digits == "" || strings.Trim(digits, "0") == ""
}
```

- [ ] **Step 3.6: Run the new tests to verify they pass**

Run: `go test ./internal/itchio/ -run TestFetchSuggestedPrice -v`
Expected: all four `PASS`.

- [ ] **Step 3.7: Run the full `itchio` package tests to check for regressions**

Run: `go test ./internal/itchio/... -v 2>&1 | tail -60`
Expected: all `PASS`.

- [ ] **Step 3.8: Commit**

```bash
git add internal/itchio/download.go internal/itchio/download_test.go testdata/purchase_page.html
git commit -m "$(cat <<'EOF'
feat(itchio): fetch the developer's suggested donation amount

The suggested amount for a name-your-own-price game only appears on
its /purchase page, as the pre-filled value of the price input.
FetchSuggestedPrice reads it structurally (attribute order varies —
value comes before name in real captures) and treats a missing
purchase page, a $0.00 suggestion, or a malformed input as ordinary
empty results rather than errors, since all three are states a
genuinely free or lightly-configured game can be in.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Fetch the suggested price progressively on the detail screen

**Files:**
- Modify: `internal/ui/screen_detail.go:203-204`

**Interfaces:**
- Consumes: `itchio.PricingNameYourOwnPrice` (Task 2), `client.FetchSuggestedPrice` (Task 3).
- Produces: `d.SuggestedPrice` populated in place after the detail screen's first redraw, for `drawDonationBand` (Task 5) to read.

This is UI code gated behind `//go:build !headless`; there is no existing precedent in this codebase for unit-testing the fetch goroutine itself (the equivalent `BrowserOnly` wiring in this same goroutine has no dedicated test either), so verification here is a successful build plus the visual check in Task 5/6. Do not invent a new test harness for this — it would be testing SDL event plumbing, not application logic.

- [ ] **Step 4.1: Add the progressive fetch**

In `internal/ui/screen_detail.go`, the goroutine in `NewDetailScreen` currently ends:

```go
		s.loading = false // publish last — renderer sees consistent state
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}
```

Insert a new block between the `sdl.PushEvent` call and the closing `}()`:

```go
		s.loading = false // publish last — renderer sees consistent state
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})

		// Progressive enhancement: the classification above is already enough
		// to draw the donation band with generic wording, so this second
		// request runs after that first redraw rather than blocking it. A
		// slow or failed purchase-page fetch costs only the amount, never
		// the ask, and is skipped entirely for free and paid games.
		if d != nil && err == nil && d.Pricing == itchio.PricingNameYourOwnPrice {
			price, perr := client.FetchSuggestedPrice(game.URL)
			if perr != nil {
				logger.Warn("detail: FetchSuggestedPrice: %v", perr)
			} else {
				d.SuggestedPrice = price
			}
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
		}
	}()
	return s
}
```

- [ ] **Step 4.2: Build to verify it compiles**

Run: `go build ./...`
Expected: succeeds (requires SDL2 dev headers on the host — the same requirement `./scripts/test.sh`'s non-headless pass has).

- [ ] **Step 4.3: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "$(cat <<'EOF'
feat(ui): fetch the suggested donation amount progressively

Runs after the detail screen's first redraw, not before it, so a
slow or failed /purchase request never delays the screen the user
is already looking at — the donation band shows generic wording
first and gains the amount when the second request lands.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Draw the donation band

**Files:**
- Modify: `internal/ui/screen_detail.go:440-442` (call site)
- Modify: `internal/ui/screen_detail.go` (new method, placed after `drawQR`, before `goBack`)

**Interfaces:**
- Consumes: `s.detail.Pricing`, `s.detail.SuggestedPrice` (Task 2/4), `r.Theme.Separator()`, `r.Theme.ToneOn()`, `r.Theme.Price()`, `r.Theme.Background`, `r.TextSize()`, `r.DrawRect()`, `r.DrawTextCentered()` (all existing).
- Produces: `func (s *DetailScreen) drawDonationBand(r *renderer.Renderer, x, y, w int32) int32` — returns the new `y` after the band, matching the pattern every other section of `Draw()` already follows for `s.contentHeight` accounting.

- [ ] **Step 5.1: Insert the call site**

In `internal/ui/screen_detail.go`, the top row currently ends:

```go
		s.drawQR(r, margin, y, usableW, qrBoxH)
		y += qrBoxH + 10
	}

	// ── Action area (full width) ────────────────────────────
```

Insert the donation band between the closing `}` and the action-area comment:

```go
		s.drawQR(r, margin, y, usableW, qrBoxH)
		y += qrBoxH + 10
	}

	if s.detail != nil && s.detail.Pricing == itchio.PricingNameYourOwnPrice {
		y = s.drawDonationBand(r, margin, y, usableW)
	}

	// ── Action area (full width) ────────────────────────────
```

No change is needed to the `s.contentHeight = y - (contentTop - s.scrollY)` line further down — it already derives the total from the final `y`, so the new row is included in the scroll extent automatically.

- [ ] **Step 5.2: Add `drawDonationBand`**

In `internal/ui/screen_detail.go`, immediately after `drawQR` closes (the `}` before `// goBack destroys any cached SDL resources...`), add:

```go

// drawDonationBand renders the developer's request for support as a
// full-width section between the screenshot row and the action row. It is
// drawn only for name-your-own-price games and never blocks or alters the
// download path.
func (s *DetailScreen) drawDonationBand(r *renderer.Renderer, x, y, w int32) int32 {
	_, fontH := r.TextSize("Ag")
	text := "Consider donating to the developer"
	if s.detail.SuggestedPrice != "" {
		amountText := "Consider donating — " + s.detail.SuggestedPrice + " suggested"
		tw, _ := r.TextSize(amountText)
		if tw <= w {
			text = amountText
		}
		// Falls back to the no-amount wording above if the amount text would
		// overflow — not observed at any shipping geometry during design, but
		// an unusually wide currency format is not something to overflow on.
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

- [ ] **Step 5.3: Build to verify it compiles**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 5.4: Run the no-color-literals check**

Run: `./scripts/no-color-literals.sh`
Expected: exit 0 — every colour argument above is a theme-derived variable (`sep[...]`, `tone[...]`), never a literal triple.

- [ ] **Step 5.5: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "$(cat <<'EOF'
feat(ui): draw the donation band on the detail screen

Full-width row between the screenshot/QR area and the action row,
shown only for name-your-own-price games. Text tone is
ToneOn(Price(), Background) rather than raw Price() or a tinted
PriceBG()/SuccessBG() badge — both measured contrast failures on
light palettes during design (Catppuccin Latte: 52-53 contrast).
Wording avoids "or more", which is itch.io's phrasing for a paid
minimum and would misrepresent a $0-permitted suggestion as a floor.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Add a `detail-donation` devshot scene and verify contrast

**Files:**
- Modify: `internal/ui/dev_scenes.go:245-246` (new scene entry)

**Interfaces:**
- Consumes: `devDetail` (existing scene helper), `itchio.PricingNameYourOwnPrice` (Task 2).
- Produces: a `detail-donation` entry in the `devScenes` registry, picked up automatically by `ui.Scenes()`/`--all` and therefore by `scripts/palette-audit.sh` on every future test run.

This is the empirical check the drawing code in Task 5 cannot get from a build alone: real text, rendered against all 18 bundled palettes at all 4 shipping geometries, measured for contrast rather than reasoned about.

- [ ] **Step 6.1: Add the scene**

In `internal/ui/dev_scenes.go`, the registry currently has:

```go
	{"detail-modal-confirm", "Detail with the destructive delete confirmation open", func(d SceneDeps) Screen {
		s := devDetail(d)
		s.modal = detailModal{
			active: true, kind: modalKindDeleteConfirm,
			title:     "Delete ROM?",
			body:      "This removes tobu-tobu-girl-deluxe.gbc from your SD card. The save file is kept.",
			onConfirm: func() {},
		}
		return s
	}},
	{"settings", "Settings menu", func(d SceneDeps) Screen {
```

Insert a new entry between them:

```go
	{"detail-modal-confirm", "Detail with the destructive delete confirmation open", func(d SceneDeps) Screen {
		s := devDetail(d)
		s.modal = detailModal{
			active: true, kind: modalKindDeleteConfirm,
			title:     "Delete ROM?",
			body:      "This removes tobu-tobu-girl-deluxe.gbc from your SD card. The save file is kept.",
			onConfirm: func() {},
		}
		return s
	}},
	{"detail-donation", "Detail for a name-your-own-price game, showing the donation band", func(d SceneDeps) Screen {
		s := devDetail(d)
		detailCopy := *d.Detail
		detailCopy.Pricing = itchio.PricingNameYourOwnPrice
		detailCopy.SuggestedPrice = "$2.00"
		s.detail = &detailCopy
		return s
	}},
	{"settings", "Settings menu", func(d SceneDeps) Screen {
```

`detailCopy` is a shallow copy of the shared fixture detail so this scene doesn't mutate the struct other scenes read from the same `SceneDeps.Detail` pointer.

- [ ] **Step 6.2: Render the new scene for a manual look**

Run: `go run ./cmd/devshot --scene detail-donation --palette default --out-dir /tmp/itchio-screenshots/devshot`

Open `/tmp/itchio-screenshots/devshot/detail-donation@default.png` and confirm the band reads "Consider donating — $2.00 suggested" between the screenshot/QR row and the action row.

- [ ] **Step 6.3: Run the full palette audit**

Run: `./scripts/palette-audit.sh`
Expected: `ok` for all four geometries (1024x768, 640x480, 720x480, 720x720), no `FAIL` lines. This exercises the new scene across all 18 bundled palettes automatically, since it renders every registered scene.

If it reports a finding against `detail-donation`, the fix is a theme accessor problem in `drawDonationBand`, not a wording change — re-check Step 5.2 against `internal/theme/color.go`'s `ToneOn` before changing anything else.

- [ ] **Step 6.4: Commit**

```bash
git add internal/ui/dev_scenes.go
git commit -m "$(cat <<'EOF'
test(ui): add a detail-donation devshot scene

Puts the donation band into scripts/palette-audit.sh's coverage, so
it is re-checked against 18 palettes x 4 geometries on every test.sh
run rather than once, by hand, during design.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Full verification

**Files:** none — this task runs the project's standard verification, which no earlier task substitutes for.

- [ ] **Step 7.1: Run the full test suite**

Run: `./scripts/test.sh`
Expected: exit 0. This runs `no-color-literals.sh`, `palette-audit.sh`, the headless `go test -race ./...`, and the non-headless `go test ./internal/ui/` compile-and-run pass.

- [ ] **Step 7.2: Real cross-compile**

Run: `./scripts/build.sh nextui/tg5040`
Expected: succeeds. `-tags headless` (used by CI and part of `test.sh`) does not compile `screen_detail.go` at all, so this is the only step in the whole plan that proves the SDL drawing path — Task 5's actual deliverable — builds for a device.

- [ ] **Step 7.3: Confirm a clean tree**

Run: `git status --short`
Expected: empty — every change from Tasks 1–6 has already been committed; this is a check, not a commit point.
