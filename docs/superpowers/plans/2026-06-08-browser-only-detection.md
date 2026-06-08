# Browser-Only Pico-8 Game Detection — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect Pico-8 games that are HTML5-only (no downloadable cart) from their public page and surface a clear "Browser-only" label on the detail screen before the user attempts a doomed download.

**Architecture:** Add `BrowserOnly bool` to `GameDetail` populated during `FetchGameDetail` using three substring checks on the already-fetched HTML body. The detail screen uses this flag to change the action button label and short-circuit `startDownload()` with a modal. `FetchUploadsScreen` also checks the flag to skip network calls and update its message wording.

**Tech Stack:** Go, `golang.org/x/net/html`, SDL2 (UI screens exclude from headless CI via `//go:build !headless`)

---

## File Map

| File | Change |
|------|--------|
| `internal/itchio/game.go` | Add `BrowserOnly bool` to `GameDetail`; add detection in `FetchGameDetail` |
| `internal/itchio/game_test.go` | Add `TestBrowserOnlyDetection_*` — three cases |
| `internal/ui/screen_detail.go` | Browser-only label in `Draw()`; early modal in `startDownload()` |
| `internal/ui/screen_fetch_uploads.go` | Early-exit goroutine; updated wording in `Draw()` |

---

### Task 1: Add `BrowserOnly` field and detection (TDD)

**Files:**
- Modify: `internal/itchio/game.go`
- Modify: `internal/itchio/game_test.go`

- [ ] **Step 1.1: Write three failing tests in `game_test.go`**

Add after the existing `TestFetchGameDetailNoBundleNames` test (around line 329):

```go
func TestBrowserOnlyDetection_HTML5Only(t *testing.T) {
	const pageHTML = `<html><body>
<div class="html_embed_widget">
  <div class="iframe_placeholder" data-src="//html.itch.zone/html/12345/"></div>
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
	if !detail.BrowserOnly {
		t.Error("BrowserOnly should be true: html.itch.zone embed, no download_btn, no buy_row")
	}
}

func TestBrowserOnlyDetection_FreeDownloadable(t *testing.T) {
	const pageHTML = `<html><body>
<div class="html_embed_widget">
  <div class="iframe_placeholder" data-src="//html.itch.zone/html/12345/"></div>
</div>
<a class="button download_btn" href="/game/dl">Download Now</a>
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
	if detail.BrowserOnly {
		t.Error("BrowserOnly should be false when download_btn is present")
	}
}

func TestBrowserOnlyDetection_PaidGame(t *testing.T) {
	const pageHTML = `<html><body>
<div class="html_embed_widget">
  <div class="iframe_placeholder" data-src="//html.itch.zone/html/12345/"></div>
</div>
<div class="buy_row">
  <a class="button buy_now_button" href="/game/purchase">Buy Now</a>
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
	if detail.BrowserOnly {
		t.Error("BrowserOnly should be false when buy_row is present (paid game with HTML5 demo)")
	}
}
```

- [ ] **Step 1.2: Run tests — expect compile failure**

```bash
./scripts/test.sh
```

Expected: build error — `detail.BrowserOnly undefined (type *itchio.GameDetail has no field or method BrowserOnly)`

- [ ] **Step 1.3: Add `BrowserOnly` field to `GameDetail` in `game.go`**

The `GameDetail` struct starts at line 18. Add `BrowserOnly bool` as the last field:

```go
type GameDetail struct {
	Game
	Description    string
	ScreenshotURLs []string
	Uploads        []Upload
	GameID         string
	CSRFToken      string
	PageTags       []string
	BundleNames    []string
	BrowserOnly    bool // true when page has HTML5 embed but no downloadable or paid files
}
```

- [ ] **Step 1.4: Add detection logic in `FetchGameDetail`**

In `FetchGameDetail`, find the line `return detail, nil` near the bottom of the function (after the bundle names loop, around line 119). Insert the detection block immediately before it:

```go
	// Detect browser-only: page has an HTML5 game embed but no free-download or
	// purchase button visible to anonymous users.
	hasHTML5Embed  := strings.Contains(s, "html.itch.zone")
	hasDownloadBtn := strings.Contains(s, "download_btn")
	hasBuySection  := strings.Contains(s, "buy_row")
	detail.BrowserOnly = hasHTML5Embed && !hasDownloadBtn && !hasBuySection
	logger.Debug("game: browserOnly=%v (embed=%v downloadBtn=%v buySection=%v)",
		detail.BrowserOnly, hasHTML5Embed, hasDownloadBtn, hasBuySection)

	return detail, nil
```

- [ ] **Step 1.5: Run tests — expect all three to pass**

```bash
./scripts/test.sh
```

Expected: all `TestBrowserOnlyDetection_*` tests PASS. The full test suite should still pass with no regressions.

- [ ] **Step 1.6: Commit**

```bash
git add internal/itchio/game.go internal/itchio/game_test.go
git commit -m "feat: detect browser-only Pico-8 games in FetchGameDetail"
```

---

### Task 2: Browser-only label in `screen_detail.go` Draw()

**Files:**
- Modify: `internal/ui/screen_detail.go`

SDL2 screens have `//go:build !headless` and cannot be unit tested in CI. Verify visually with a native build.

- [ ] **Step 2.1: Update the "not downloaded yet" action branch in `Draw()`**

Find the `} else {` block around line 561 that renders the download button for games not yet in the inventory. It currently reads:

```go
	} else {
		if s.game.IsFree {
			drawActionRow("A", "Download", 80, 200, 80, ac[0], ac[1], ac[2], 0)
		} else if s.cfg.APIKey == "" {
			drawActionRow("A", "Purchase required", 220, 180, 60, 100, 80, 20, s.game.Price)
		} else {
			drawActionRow("A", "Download", 80, 200, 80, ac[0], ac[1], ac[2], s.game.Price)
		}
		y += 4
	}
```

Replace it with:

```go
	} else {
		if s.detail != nil && s.detail.BrowserOnly {
			drawActionRow("A", "Browser-only", 140, 140, 140, ac[0], ac[1], ac[2], 0)
		} else if s.game.IsFree {
			drawActionRow("A", "Download", 80, 200, 80, ac[0], ac[1], ac[2], 0)
		} else if s.cfg.APIKey == "" {
			drawActionRow("A", "Purchase required", 220, 180, 60, 100, 80, 20, s.game.Price)
		} else {
			drawActionRow("A", "Download", 80, 200, 80, ac[0], ac[1], ac[2], s.game.Price)
		}
		y += 4
	}
```

The `140, 140, 140` RGB gives a muted grey so the label visually signals non-downloadable status.

- [ ] **Step 2.2: Build the native binary to confirm it compiles**

```bash
./scripts/build.sh native
```

Expected: build succeeds with no errors.

- [ ] **Step 2.3: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat: show 'Browser-only' label for non-downloadable Pico-8 games"
```

---

### Task 3: Early modal in `screen_detail.go` startDownload()

**Files:**
- Modify: `internal/ui/screen_detail.go`

- [ ] **Step 3.1: Add the browser-only guard at the top of `startDownload()`**

Find `startDownload()` around line 958. It currently reads:

```go
func (s *DetailScreen) startDownload() Screen {
	if s.loading {
		return s
	}
	if !s.game.IsFree && s.cfg.APIKey == "" {
		return s
	}
	return NewFetchUploadsScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s.inv, s.inventoryPath, s)
}
```

Replace it with:

```go
func (s *DetailScreen) startDownload() Screen {
	if s.loading {
		return s
	}
	if s.detail != nil && s.detail.BrowserOnly {
		s.ShowModal("Browser-only game",
			"This game has no downloadable files and can only be played in a web browser. "+
				"Press B to dismiss, then scan the QR code to open the game page.")
		return s
	}
	if !s.game.IsFree && s.cfg.APIKey == "" {
		return s
	}
	return NewFetchUploadsScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, s.inv, s.inventoryPath, s)
}
```

- [ ] **Step 3.2: Build to confirm it compiles**

```bash
./scripts/build.sh native
```

Expected: build succeeds.

- [ ] **Step 3.3: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat: show modal when attempting to download a browser-only game"
```

---

### Task 4: Safety net in `screen_fetch_uploads.go`

**Files:**
- Modify: `internal/ui/screen_fetch_uploads.go`

This handles the case where `FetchUploadsScreen` is reached despite `BrowserOnly = true` (e.g. `detail` was nil at detail-screen time), and sharpens the wording when detection is definitive.

- [ ] **Step 4.1: Add early-exit at the top of the goroutine**

In `NewFetchUploadsScreen`, find the `go func() {` block (around line 70). The goroutine currently opens with:

```go
	go func() {
		var err error

		useAuthPath := !game.IsFree && cfg.APIKey != "" &&
			detail != nil && detail.GameID != ""
```

Add the early-exit check before the `useAuthPath` assignment:

```go
	go func() {
		if detail != nil && detail.BrowserOnly {
			s.err = fmt.Errorf("no downloadable files found for this game")
			s.storeState(fetchError)
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
			return
		}

		var err error

		useAuthPath := !game.IsFree && cfg.APIKey != "" &&
			detail != nil && detail.GameID != ""
```

- [ ] **Step 4.2: Update the "no downloads" message in `Draw()` to use definitive wording when `BrowserOnly` is known**

In `Draw()`, find the `fetchError` case around line 198. It currently reads:

```go
	case fetchError:
		if s.err != nil && s.err.Error() == "no downloadable files found for this game" {
			const noDownloadMsg = "This game does not have any downloadable files — it may be browser-only. Press B to return and scan the QR code to open the game page."
			ndLines := r.WrapText(noDownloadMsg, r.W-40)
			ndH := int32(len(ndLines)) * (smallFH + 4)
			startY := mid - (mainFH+10+ndH)/2
			r.DrawTextCentered("No downloads available", 0, startY, r.W, 200, 160, 60)
			r.DrawWrappedText(noDownloadMsg, 20, startY+mainFH+10, r.W-40, smallFH+4, ht[0], ht[1], ht[2])
		} else {
```

Replace it with:

```go
	case fetchError:
		if s.err != nil && s.err.Error() == "no downloadable files found for this game" {
			noDownloadMsg := "This game does not have any downloadable files — it may be browser-only. Press B to return and scan the QR code to open the game page."
			if s.detail != nil && s.detail.BrowserOnly {
				noDownloadMsg = "This game is browser-only and has no downloadable files. Press B to return and scan the QR code to open the game page."
			}
			ndLines := r.WrapText(noDownloadMsg, r.W-40)
			ndH := int32(len(ndLines)) * (smallFH + 4)
			startY := mid - (mainFH+10+ndH)/2
			r.DrawTextCentered("No downloads available", 0, startY, r.W, 200, 160, 60)
			r.DrawWrappedText(noDownloadMsg, 20, startY+mainFH+10, r.W-40, smallFH+4, ht[0], ht[1], ht[2])
		} else {
```

Note: `noDownloadMsg` changes from `const` to `var` (a bare `var` declaration is not needed — just remove the `const` keyword and assign a regular string variable).

- [ ] **Step 4.3: Build and run the full test suite**

```bash
./scripts/test.sh
```

Expected: all tests PASS.

- [ ] **Step 4.4: Commit**

```bash
git add internal/ui/screen_fetch_uploads.go
git commit -m "feat: early-exit and definitive wording for browser-only games in FetchUploadsScreen"
```
