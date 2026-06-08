# Browser-Only Pico-8 Game Detection — Design Spec

**Date:** 2026-06-08
**Status:** Approved

## Overview

Pico-8 games tagged `pico-8` on itch.io include a subset that are published as HTML5-only — no `.p8` or `.p8.png` download is provided. These games currently appear in the Pak's game list but result in an error only after a wasted network request. This spec adds early detection so the user sees a clear "browser-only" indicator on the detail screen and never waits on a doomed download attempt.

---

## Section 1 — Detection in `FetchGameDetail` (`internal/itchio/game.go`)

### New field

```go
type GameDetail struct {
    // ... existing fields ...
    BrowserOnly bool // true when page has HTML5 embed but no downloadable files
}
```

### Detection logic

After reading the full page body into `s string`, add three substring checks and derive `BrowserOnly`:

```go
hasHTML5Embed  := strings.Contains(s, "html.itch.zone")
hasDownloadBtn := strings.Contains(s, "download_btn")
hasBuySection  := strings.Contains(s, "buy_row")
detail.BrowserOnly = hasHTML5Embed && !hasDownloadBtn && !hasBuySection
logger.Debug("game: browserOnly=%v (embed=%v downloadBtn=%v buySection=%v)",
    detail.BrowserOnly, hasHTML5Embed, hasDownloadBtn, hasBuySection)
```

**Signal rationale:**

| Signal | Source | Meaning |
|--------|--------|---------|
| `html.itch.zone` | `data-src` of the HTML5 iframe embed | Game has a browser player |
| `download_btn` | CSS class on the free-download button | Game has at least one downloadable file |
| `buy_row` | CSS class on the paid-purchase section | Game requires purchase; not browser-only |

**Edge-case coverage:**

| Game type | `hasHTML5Embed` | `hasDownloadBtn` | `hasBuySection` | `BrowserOnly` |
|-----------|:-:|:-:|:-:|:-:|
| Free HTML5-only | ✓ | ✗ | ✗ | **true** |
| Free HTML5 + `.p8` download | ✓ | ✓ | ✗ | false |
| Free `.p8` download only | ✗ | ✓ | ✗ | false |
| Paid game with HTML5 demo | ✓ | ✗ | ✓ | false |
| Paid download only | ✗ | ✗ | ✓ | false |

Detection is conservative: a false negative (missed browser-only game) is harmless — the user hits the existing no-downloads message. A false positive would block a valid download, so we require all three conditions.

---

## Section 2 — `DetailScreen` (`internal/ui/screen_detail.go`)

### 2a — Action button label

In `Draw()`, where the action row renders "Download", add a branch for `BrowserOnly`:

```go
if s.detail != nil && s.detail.BrowserOnly {
    drawActionRow("A", "Browser-only", 140, 140, 140, ...)  // muted grey, no price
} else if s.game.IsFree {
    drawActionRow("A", "Download", 80, 200, 80, ...)
} else {
    drawActionRow("A", "Download", 80, 200, 80, ..., s.game.Price)
}
```

The same muted-grey colour is applied wherever "Download" or "Download again" are rendered for a browser-only game (the "already downloaded" branch is unreachable if `BrowserOnly` is set, so only the primary label needs changing).

### 2b — Modal in `startDownload()`

At the top of `startDownload()`, before any auth or routing logic:

```go
if s.detail != nil && s.detail.BrowserOnly {
    s.ShowModal("Browser-only game",
        "This game has no downloadable files and can only be played "+
        "in a web browser. Press B to dismiss, then scan the QR code "+
        "to open the game page.")
    return s
}
```

Uses the existing `ShowModal` system — no new screen, no network call. The QR code for the game URL is already present on the detail screen.

---

## Section 3 — `FetchUploadsScreen` safety net (`internal/ui/screen_fetch_uploads.go`)

`FetchUploadsScreen` may still be reached if `detail` is nil or from a future code path. Two changes:

### 3a — Early exit when `BrowserOnly` is known

At the top of the goroutine in `NewFetchUploadsScreen`, before the free/auth branch:

```go
if detail != nil && detail.BrowserOnly {
    s.err = fmt.Errorf("no downloadable files found for this game")
    s.storeState(fetchError)
    sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
    return
}
```

This skips both `FetchUploads` (free path) and `FetchOwnedKeys` (auth path) — zero network requests.

### 3b — Message wording

In `Draw()`, the existing `fetchError` branch already shows a "no downloadable files" message (lines ~198–206). Change "it **may be** browser-only" to "it **is** browser-only" when `detail != nil && detail.BrowserOnly`, since the detection is now definitive:

```go
if s.err != nil && s.err.Error() == "no downloadable files found for this game" {
    var msg string
    if s.detail != nil && s.detail.BrowserOnly {
        msg = "This game is browser-only and has no downloadable files. " +
              "Press B to return and scan the QR code to open the game page."
    } else {
        msg = "This game does not have any downloadable files — it may be " +
              "browser-only. Press B to return and scan the QR code to open the game page."
    }
    // ... render msg ...
}
```

---

## Section 4 — Tests (`internal/itchio/game_test.go`)

Three new test cases covering the detection logic. Fixtures go in `testdata/`.

| Test | Fixture | Expected `BrowserOnly` |
|------|---------|:---:|
| `TestBrowserOnlyDetection_HTML5Only` | Page with `html.itch.zone` in body, no `download_btn`, no `buy_row` | `true` |
| `TestBrowserOnlyDetection_FreeDownloadable` | Page with `html.itch.zone` + `download_btn` | `false` |
| `TestBrowserOnlyDetection_PaidGame` | Page with `html.itch.zone` + `buy_row`, no `download_btn` | `false` |

Fixtures are minimal HTML stubs — only the elements needed to trigger each code path, not full itch.io pages. Named `testdata/game_browser_only_html5.html`, `testdata/game_browser_only_free_dl.html`, `testdata/game_browser_only_paid.html`.

---

## Files Changed Summary

| File | Change |
|------|--------|
| `internal/itchio/game.go` | `BrowserOnly bool` field on `GameDetail`; detection logic in `FetchGameDetail` |
| `internal/ui/screen_detail.go` | Muted-grey label in `Draw()`; early modal in `startDownload()` |
| `internal/ui/screen_fetch_uploads.go` | Early-exit goroutine; definitive wording in error message |
| `internal/itchio/game_test.go` | Three new `TestBrowserOnlyDetection_*` cases |
| `testdata/game_browser_only_*.html` | Three new minimal HTML fixtures |
