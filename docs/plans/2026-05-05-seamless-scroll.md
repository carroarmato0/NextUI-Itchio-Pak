# Seamless Scroll Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the page-based game list with a single continuous scrollable list, add L1/R1 press-and-hold acceleration, and progressively populate the list as the background cache builds.

**Architecture:** `cursor` becomes an absolute index into `viewGames`; `games` and `page` fields are removed. The renderer iterates `viewGames` directly. `FetchAllGames` progress callback changes to pass the accumulated `[]Game` slice so `buildCache` can call `rebuildView` progressively.

**Tech Stack:** Go, SDL2 (`//go:build !headless`), `internal/itchio`, `internal/ui`

---

## File Map

| File | Change |
|---|---|
| `internal/itchio/feed.go` | Callback signature: `func(fetched int)` → `func(partial []Game)` |
| `internal/itchio/feed_test.go` | Update one test callback to match new signature |
| `internal/ui/screen_cache_refresh.go` | Update callback: `fetched` → `len(partial)` |
| `internal/ui/screen_list.go` | Core refactor (struct, navigation, render, L1/R1 hold, progressive loading) |

---

## Task 1: Change FetchAllGames callback to pass partial []Game

**Files:**
- Modify: `internal/itchio/feed.go`
- Modify: `internal/itchio/feed_test.go`

- [ ] **Step 1: Update the FetchAllGames signature in feed.go**

  In `internal/itchio/feed.go`, change line 172 and both `progress(...)` call sites:

  ```go
  // signature (line 172)
  func (c *Client) FetchAllGames(ctx context.Context, progress func(partial []Game)) ([]Game, error) {

  // after page 1 (line ~188)
  if progress != nil {
      progress(all)
  }

  // after each subsequent page (line ~210)
  if progress != nil {
      progress(all)
  }
  ```

- [ ] **Step 2: Update TestFetchAllGames in feed_test.go**

  Lines 122-127 currently read:
  ```go
  var progressCalls int
  var lastFetched int
  games, err := c.FetchAllGames(context.Background(), func(fetched int) {
      progressCalls++
      lastFetched = fetched
  })
  ```

  Change to:
  ```go
  var progressCalls int
  var lastFetched int
  games, err := c.FetchAllGames(context.Background(), func(partial []Game) {
      progressCalls++
      lastFetched = len(partial)
  })
  ```

  The two `nil`-callback tests (`TestFetchAllGames_ContextCancellation` and `TestFetchAllGames_MidFetchCancellation`) require no changes.

- [ ] **Step 3: Run tests — expect PASS**

  ```bash
  ./scripts/test.sh
  ```

  Expected: all tests pass. Look for `ok github.com/carroarmato0/nextui-itchio-pak/internal/itchio`.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/itchio/feed.go internal/itchio/feed_test.go
  git commit -m "refactor(feed): change FetchAllGames progress callback to pass partial []Game"
  ```

---

## Task 2: Update FetchAllGames call sites

**Files:**
- Modify: `internal/ui/screen_cache_refresh.go`
- Modify: `internal/ui/screen_list.go` (buildCache only)

- [ ] **Step 1: Update screen_cache_refresh.go**

  Lines 59-60 currently read:
  ```go
  games, err := client.FetchAllGames(context.Background(), func(fetched int) {
      atomic.StoreInt64(&s.fetched, int64(fetched))
  })
  ```

  Change to:
  ```go
  games, err := client.FetchAllGames(context.Background(), func(partial []itchio.Game) {
      atomic.StoreInt64(&s.fetched, int64(len(partial)))
  })
  ```

- [ ] **Step 2: Update buildCache in screen_list.go**

  The `buildCache` method (~line 1133) currently reads:
  ```go
  games, err := s.client.FetchAllGames(context.Background(), func(fetched int) {
      logger.Debug("cache: fetched %d games so far", fetched)
  })
  ```

  Change just the callback for now (progressive logic comes in Task 5):
  ```go
  games, err := s.client.FetchAllGames(context.Background(), func(partial []itchio.Game) {
      logger.Debug("cache: fetched %d games so far", len(partial))
  })
  ```

- [ ] **Step 3: Build check — expect success**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: binary produced with no errors.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/ui/screen_cache_refresh.go internal/ui/screen_list.go
  git commit -m "refactor(ui): update FetchAllGames call sites for new callback signature"
  ```

---

## Task 3: Core ListScreen refactor

This is one large atomic commit. Work through the steps, then commit everything together at Step 17. The code will not compile between individual steps — that is expected.

**Files:**
- Modify: `internal/ui/screen_list.go`

### 3a — Struct and constructor

- [ ] **Step 1: Remove fields `games`, `page`, `jumpToEnd`; add new fields**

  In the `ListScreen` struct, remove:
  ```go
  games      []itchio.Game
  page       int
  jumpToEnd  bool
  ```

  Add after `warmedGameURL string`:
  ```go
  // Shoulder button (L1/R1) auto-repeat state — mirrors heldDir/heldSince/lastRepeat
  heldShoulderDir    int
  heldShoulderSince  time.Time
  lastShoulderRepeat time.Time

  // lastVisibleRows is set each Draw so HandleEvent can jump by a screen's worth.
  lastVisibleRows int
  ```

- [ ] **Step 2: Update NewListScreen — remove `page: 1`**

  In `NewListScreen`, the struct literal currently has `page: 1`. Remove that line. No other changes to `NewListScreen` are needed.

### 3b — Navigation functions

- [ ] **Step 3: Rewrite moveCursor**

  Replace the entire `moveCursor` function body:

  ```go
  func (s *ListScreen) moveCursor(dir int) {
      if dir > 0 {
          if s.cursor < len(s.viewGames)-1 {
              s.cursor++
              s.titleScrollX = 0
              s.titleScrollAt = time.Now()
              s.tagScrollY = 0
              s.tagScrollAt = time.Now()
              s.lastCursorMove = time.Now()
              s.warmedGameURL = ""
          }
      } else if dir < 0 {
          if s.cursor > 0 {
              s.cursor--
              s.titleScrollX = 0
              s.titleScrollAt = time.Now()
              s.tagScrollY = 0
              s.tagScrollAt = time.Now()
              s.lastCursorMove = time.Now()
              s.warmedGameURL = ""
          }
      }
  }
  ```

- [ ] **Step 4: Add jumpCursor helper (after moveCursor)**

  ```go
  // jumpCursor moves the cursor by n items, clamping to the list bounds.
  // Used by L1/R1 shoulder buttons to jump by one visible screen at a time.
  func (s *ListScreen) jumpCursor(n int) {
      if n == 0 {
          n = 1
      }
      newPos := s.cursor + n
      if newPos < 0 {
          newPos = 0
      }
      if newPos >= len(s.viewGames) {
          newPos = len(s.viewGames) - 1
      }
      if newPos < 0 {
          newPos = 0 // viewGames may be empty
      }
      s.cursor = newPos
      s.titleScrollX = 0
      s.titleScrollAt = time.Now()
      s.tagScrollY = 0
      s.tagScrollAt = time.Now()
      s.lastCursorMove = time.Now()
      s.warmedGameURL = ""
  }
  ```

- [ ] **Step 5: Simplify loadPage — remove cache-ready branch, set viewGames**

  Replace the entire `loadPage` function:

  ```go
  func (s *ListScreen) loadPage(page int, query string) {
      s.loading = true
      s.err = nil
      logger.Debug("feed: loading page %d query=%q", page, query)
      games, err := s.client.FetchGames(page, query)
      if err != nil {
          logger.Error("feed: page %d error: %v", page, err)
      } else {
          logger.Info("feed: page %d returned %d games", page, len(games))
      }
      s.viewGames = games
      s.err = err
      s.cursor = 0
      s.titleScrollX = 0
      s.titleScrollAt = time.Now()
      s.tagScrollY = 0
      s.tagScrollAt = time.Now()
      s.lastCursorMove = time.Now()
      s.warmedGameURL = ""
      s.loading = false
  }
  ```

- [ ] **Step 6: Delete placeCursor**

  Remove the entire `placeCursor` function (it was only called from `loadPage`).

- [ ] **Step 7: Simplify rebuildView — replace page/games assignments with direct cursor**

  In `rebuildView`, change the two lines that compute `selectedViewIdx`:
  ```go
  // before
  selectedViewIdx := (s.page-1)*itchio.PerPage + s.cursor
  if s.cursor < len(s.games) {
      selectedURL = s.games[s.cursor].URL
  }
  ```
  ```go
  // after
  selectedViewIdx := s.cursor
  if s.cursor < len(s.viewGames) {
      selectedURL = s.viewGames[s.cursor].URL
  }
  ```

  In the "found selected URL" branch, replace:
  ```go
  page := i/itchio.PerPage + 1
  cursor := i % itchio.PerPage
  s.page = page
  s.games = pageSlice(s.viewGames, page)
  s.cursor = cursor
  s.titleScrollX = 0
  s.titleScrollAt = time.Now()
  s.tagScrollY = 0
  s.tagScrollAt = time.Now()
  s.lastCursorMove = time.Now()
  s.warmedGameURL = ""
  logger.Debug("sort: view rebuilt — %d games visible (mode=%s), restored selection to %q (page=%d, cursor=%d)",
      len(s.viewGames), itchio.SortModeBadge(s.sortMode), selectedURL, page, cursor)
  ```
  with:
  ```go
  s.cursor = i
  s.titleScrollX = 0
  s.titleScrollAt = time.Now()
  s.tagScrollY = 0
  s.tagScrollAt = time.Now()
  s.lastCursorMove = time.Now()
  s.warmedGameURL = ""
  logger.Debug("sort: view rebuilt — %d games visible (mode=%s), restored selection to %q (cursor=%d)",
      len(s.viewGames), itchio.SortModeBadge(s.sortMode), selectedURL, i)
  ```

  In the "nearest position" branch, replace:
  ```go
  page := selectedViewIdx/itchio.PerPage + 1
  cursor := selectedViewIdx % itchio.PerPage
  s.page = page
  s.games = pageSlice(s.viewGames, page)
  s.cursor = cursor
  s.titleScrollX = 0
  s.titleScrollAt = time.Now()
  s.tagScrollY = 0
  s.tagScrollAt = time.Now()
  s.lastCursorMove = time.Now()
  s.warmedGameURL = ""
  logger.Debug("sort: view rebuilt — %d games visible (mode=%s), selection gone; landing at nearest position (page=%d, cursor=%d)",
      len(s.viewGames), itchio.SortModeBadge(s.sortMode), page, cursor)
  ```
  with:
  ```go
  s.cursor = selectedViewIdx
  s.titleScrollX = 0
  s.titleScrollAt = time.Now()
  s.tagScrollY = 0
  s.tagScrollAt = time.Now()
  s.lastCursorMove = time.Now()
  s.warmedGameURL = ""
  logger.Debug("sort: view rebuilt — %d games visible (mode=%s), selection gone; landing at nearest position (cursor=%d)",
      len(s.viewGames), itchio.SortModeBadge(s.sortMode), selectedViewIdx)
  ```

  In the empty-view fallback at the end, replace `s.page = 1` with `s.cursor = 0`.

- [ ] **Step 8: Delete pageSlice**

  Remove the entire `pageSlice` function at the bottom of the file.

### 3c — warmPreloadWindow

- [ ] **Step 9: Simplify warmPreloadWindow**

  Replace:
  ```go
  func (s *ListScreen) warmPreloadWindow() {
      if s.cursor >= len(s.games) {
          return
      }
      absIdx := (s.page-1)*itchio.PerPage + s.cursor
      for i := absIdx - preloadRadius; i <= absIdx+preloadRadius; i++ {
          if i < 0 || i >= len(s.viewGames) {
              continue
          }
          if url := s.viewGames[i].CoverURL; url != "" {
              s.cache.Warm(url)
          }
      }
      s.warmedGameURL = s.games[s.cursor].CoverURL
      logger.Debug("cover: warmed window abs=%d ±%d (%d games in view)", absIdx, preloadRadius, len(s.viewGames))
  }
  ```
  with:
  ```go
  func (s *ListScreen) warmPreloadWindow() {
      if s.cursor >= len(s.viewGames) {
          return
      }
      absIdx := s.cursor
      for i := absIdx - preloadRadius; i <= absIdx+preloadRadius; i++ {
          if i < 0 || i >= len(s.viewGames) {
              continue
          }
          if url := s.viewGames[i].CoverURL; url != "" {
              s.cache.Warm(url)
          }
      }
      s.warmedGameURL = s.viewGames[s.cursor].CoverURL
      logger.Debug("cover: warmed window abs=%d ±%d (%d games in view)", absIdx, preloadRadius, len(s.viewGames))
  }
  ```

### 3d — Draw

- [ ] **Step 10: Cache lastVisibleRows in Draw**

  Find the existing line (it appears after `rowH`, `footerH`, and `contentTop` are defined):
  ```go
  visibleRows := (r.H - contentTop - footerH) / rowH
  ```
  Add immediately after it:
  ```go
  s.lastVisibleRows = int(visibleRows)
  ```

- [ ] **Step 11: Update DL-mode separator scan in Draw**

  Find:
  ```go
  if s.sortMode == itchio.SortModeDL && len(s.games) > 0 {
      lastUpdateIdx := -1
      for i, g := range s.games {
          if s.inv.HasPendingUpdates(g.URL) || s.inv.IsRemoved(g.URL) {
              lastUpdateIdx = i
          }
      }
      if lastUpdateIdx >= 0 && lastUpdateIdx < len(s.games)-1 {
          dlSepAfterUpdates = lastUpdateIdx
      }
  }
  ```
  Replace `s.games` with `s.viewGames` in all three places.

- [ ] **Step 12: Update main render loop in Draw**

  Find:
  ```go
  for i, g := range s.games {
  ```
  Change to:
  ```go
  for i, g := range s.viewGames {
  ```

- [ ] **Step 13: Update right-panel guard in Draw**

  Find:
  ```go
  if s.cursor < len(s.games) {
      g := s.games[s.cursor]
  ```
  Change to:
  ```go
  if s.cursor < len(s.viewGames) {
      g := s.viewGames[s.cursor]
  ```

- [ ] **Step 14: Update footer page indicator and hint label in Draw**

  Find:
  ```go
  pageInfo := fmt.Sprintf("Page %d", s.page)
  if s.totalPages > 0 {
      pageInfo = fmt.Sprintf("Page %d/%d", s.page, s.totalPages)
  }
  ```
  Replace with:
  ```go
  currentPage := s.cursor/itchio.PerPage + 1
  pageInfo := fmt.Sprintf("Page %d", currentPage)
  if s.totalPages > 0 {
      pageInfo = fmt.Sprintf("Page %d/%d", currentPage, s.totalPages)
  }
  ```

  Find the footer hint:
  ```go
  {Kind: renderer.BadgePill, Label: "L1/R1", Text: "Page"},
  ```
  Change to:
  ```go
  {Kind: renderer.BadgePill, Label: "L1/R1", Text: "Jump"},
  ```

### 3e — HandleEvent

- [ ] **Step 15: Update s.games references in HandleEvent**

  There are four `s.cursor < len(s.games)` guards and corresponding `s.games[s.cursor]` accesses. Replace all:
  ```go
  // keyboard: K_RETURN
  if s.cursor < len(s.games) {
      return NewDetailScreen(..., s.games[s.cursor], ...)
  }
  // keyboard: K_x
  if s.cursor < len(s.games) {
      g := s.games[s.cursor]
  // controller: CONTROLLER_BUTTON_B
  if s.cursor < len(s.games) {
      return NewDetailScreen(..., s.games[s.cursor], ...)
  // controller: CONTROLLER_BUTTON_X
  if s.cursor < len(s.games) {
      g := s.games[s.cursor]
  ```
  In every case, change `len(s.games)` → `len(s.viewGames)` and `s.games[s.cursor]` → `s.viewGames[s.cursor]`.

- [ ] **Step 16: Fix error-retry path and remove obsolete L1/R1 page-jump code**

  Find the error-retry call:
  ```go
  go s.loadPage(s.page, "")
  ```
  Change to:
  ```go
  go s.loadPage(1, "")
  ```

  Remove the four L1/R1 page-jump cases entirely (they will be replaced in Task 4):

  In the keyboard handler, remove:
  ```go
  case sdl.K_PAGEDOWN:
      if len(s.viewGames) == 0 {
          return s
      }
      if s.totalPages == 0 || s.page < s.totalPages {
          s.page++
          go s.loadPage(s.page, "")
      }
  case sdl.K_PAGEUP:
      if len(s.viewGames) == 0 {
          return s
      }
      if s.page > 1 {
          s.page--
          go s.loadPage(s.page, "")
      }
  ```

  In the controller handler, remove:
  ```go
  case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
      if len(s.viewGames) == 0 {
          return s
      }
      if s.totalPages == 0 || s.page < s.totalPages {
          s.page++
          go s.loadPage(s.page, "")
      }
  case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
      if len(s.viewGames) == 0 {
          return s
      }
      if s.page > 1 {
          s.page--
          go s.loadPage(s.page, "")
      }
  ```

- [ ] **Step 17: Build check — expect success**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: binary produced, no compilation errors. If there are errors, they are almost certainly missed `s.games` or `s.page` references — grep and fix:

  ```bash
  grep -n "s\.games\|s\.page\b" internal/ui/screen_list.go
  ```

- [ ] **Step 18: Run tests**

  ```bash
  ./scripts/test.sh
  ```

  Expected: all tests pass.

- [ ] **Step 19: Commit**

  ```bash
  git add internal/ui/screen_list.go
  git commit -m "refactor(ui): replace page+cursor model with absolute cursor into viewGames"
  ```

---

## Task 4: L1/R1 hold mechanics

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Add startShoulderHold and stopShoulderHold (after stopHold)**

  ```go
  func (s *ListScreen) startShoulderHold(dir int) {
      s.jumpCursor(dir * s.lastVisibleRows)
      s.heldShoulderDir = dir
      s.heldShoulderSince = time.Now()
      s.lastShoulderRepeat = s.heldShoulderSince
  }

  func (s *ListScreen) stopShoulderHold(dir int) {
      if s.heldShoulderDir == dir {
          s.heldShoulderDir = 0
      }
  }
  ```

- [ ] **Step 2: Extend processAutoRepeat to handle shoulder hold**

  Replace the entire `processAutoRepeat` function:

  ```go
  func (s *ListScreen) processAutoRepeat() {
      now := time.Now()
      if s.heldDir != 0 {
          elapsed := now.Sub(s.heldSince)
          if elapsed >= repeatDelay && now.Sub(s.lastRepeat) >= currentRepeatInterval(elapsed-repeatDelay) {
              s.moveCursor(s.heldDir)
              s.lastRepeat = now
          }
      }
      if s.heldShoulderDir != 0 {
          elapsed := now.Sub(s.heldShoulderSince)
          if elapsed >= repeatDelay && now.Sub(s.lastShoulderRepeat) >= currentRepeatInterval(elapsed-repeatDelay) {
              s.jumpCursor(s.heldShoulderDir * s.lastVisibleRows)
              s.lastShoulderRepeat = now
          }
      }
  }
  ```

- [ ] **Step 3: Update NeedsRedraw to include shoulder hold**

  Replace:
  ```go
  func (s *ListScreen) NeedsRedraw() bool {
      if s.heldDir != 0 {
          return true
      }
  ```
  with:
  ```go
  func (s *ListScreen) NeedsRedraw() bool {
      if s.heldDir != 0 || s.heldShoulderDir != 0 {
          return true
      }
  ```

- [ ] **Step 4: Wire up keyboard PAGEDOWN/PAGEUP to startShoulderHold**

  In the `KeyboardEvent` handler, the first `switch` block handles both keydown and keyup for D-pad. Add `K_PAGEDOWN`/`K_PAGEUP` to that block (before the `if ev.Type != sdl.KEYDOWN` guard):

  ```go
  case sdl.K_PAGEDOWN:
      if ev.Type == sdl.KEYDOWN {
          s.startShoulderHold(1)
      } else {
          s.stopShoulderHold(1)
      }
      return s
  case sdl.K_PAGEUP:
      if ev.Type == sdl.KEYDOWN {
          s.startShoulderHold(-1)
      } else {
          s.stopShoulderHold(-1)
      }
      return s
  ```

- [ ] **Step 5: Wire up controller RIGHTSHOULDER/LEFTSHOULDER to startShoulderHold**

  In the `ControllerButtonEvent` handler, add RIGHTSHOULDER/LEFTSHOULDER to the first `switch` block (the one that handles both button-down and button-up, before the `if ev.Type != sdl.CONTROLLERBUTTONDOWN` guard):

  ```go
  case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
      if ev.Type == sdl.CONTROLLERBUTTONDOWN {
          s.startShoulderHold(1)
      } else {
          s.stopShoulderHold(1)
      }
      return s
  case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
      if ev.Type == sdl.CONTROLLERBUTTONDOWN {
          s.startShoulderHold(-1)
      } else {
          s.stopShoulderHold(-1)
      }
      return s
  ```

- [ ] **Step 6: Build check**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: binary produced, no errors.

- [ ] **Step 7: Commit**

  ```bash
  git add internal/ui/screen_list.go
  git commit -m "feat(ui): add L1/R1 press-and-hold acceleration for seamless jumping"
  ```

---

## Task 5: Progressive loading in buildCache

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Update buildCache callback to progressively rebuildView**

  Find the current `buildCache` function. The callback currently reads:
  ```go
  games, err := s.client.FetchAllGames(context.Background(), func(partial []itchio.Game) {
      logger.Debug("cache: fetched %d games so far", len(partial))
  })
  ```

  Replace the callback body:
  ```go
  games, err := s.client.FetchAllGames(context.Background(), func(partial []itchio.Game) {
      logger.Debug("cache: fetched %d games so far", len(partial))
      s.cachedGames = partial
      s.cacheReady = true
      s.rebuildView()
  })
  ```

  The lines after the `FetchAllGames` call that set `s.cachedGames`, `s.cacheReady`, and call `s.rebuildView()` remain unchanged — they ensure the final complete result is committed even if the last progress callback was for the last page minus one.

- [ ] **Step 2: Build check**

  ```bash
  ./scripts/build.sh native
  ```

  Expected: binary produced, no errors.

- [ ] **Step 3: Run tests**

  ```bash
  ./scripts/test.sh
  ```

  Expected: all tests pass.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/ui/screen_list.go
  git commit -m "feat(ui): progressively populate game list as background cache builds"
  ```

---

## Spec coverage check

| Spec requirement | Task |
|---|---|
| Remove `games`, `page`, `jumpToEnd` fields | Task 3, Step 1 |
| `cursor` becomes absolute index into `viewGames` | Task 3, Steps 3–7 |
| Add shoulder hold fields + `lastVisibleRows` | Task 3, Step 1 |
| `moveCursor` simplified (no page transitions) | Task 3, Step 3 |
| `jumpCursor` helper | Task 3, Step 4 |
| `loadPage` simplified (live-fetch only) | Task 3, Step 5 |
| `placeCursor` deleted | Task 3, Step 6 |
| `rebuildView` simplified | Task 3, Step 7 |
| `pageSlice` deleted | Task 3, Step 8 |
| `warmPreloadWindow` simplified | Task 3, Step 9 |
| `lastVisibleRows` set in Draw | Task 3, Step 10 |
| DL separator uses `viewGames` | Task 3, Step 11 |
| Render loop uses `viewGames` | Task 3, Step 12 |
| Right-panel guard uses `viewGames` | Task 3, Step 13 |
| Footer page computed from cursor | Task 3, Step 14 |
| Footer hint `"Jump"` | Task 3, Step 14 |
| `HandleEvent` `s.games` → `s.viewGames` | Task 3, Step 15 |
| Retry path `loadPage(1, "")` | Task 3, Step 16 |
| Old L1/R1 page-jump code removed | Task 3, Step 16 |
| `startShoulderHold` / `stopShoulderHold` | Task 4, Step 1 |
| `processAutoRepeat` extended for shoulder | Task 4, Step 2 |
| `NeedsRedraw` checks `heldShoulderDir` | Task 4, Step 3 |
| Keyboard PAGEDOWN/PAGEUP wired to shoulder hold | Task 4, Step 4 |
| Controller RIGHTSHOULDER/LEFTSHOULDER wired to shoulder hold | Task 4, Step 5 |
| `FetchAllGames` callback → `func(partial []Game)` | Task 1 |
| `screen_cache_refresh.go` updated | Task 2, Step 1 |
| `buildCache` progressive rebuildView | Task 5, Step 1 |
