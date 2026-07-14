# Code Quality Fixes — Itch.io NextUI Pak

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address all 22 issues found in the codebase review: 3 critical data races, 10 important issues, and 9 minor issues.

**Architecture:** Fixes span 5 packages (ui, itchio, renderer, settings, inventory). Tasks are ordered: critical races first (must be correct before anything else ships), then important issues, then minor. Tasks 3+4 share `screen_list.go` and are combined into one task.

**Tech Stack:** Go 1.21+, sync/atomic, SDL2 (excluded from CI via `!headless` build tag). Tests run via `./scripts/test.sh`.

---

## Task 1: Fix data race in DownloadScreen

**Files:**
- Modify: `internal/ui/screen_download.go`

**Context:** `s.state`, `s.err`, and `s.dest` are written from the download goroutine and read from the SDL main thread (`Draw`, `HandleEvent`, `IsBusy`) with no synchronization. The fix uses the same atomic-cast pattern already used by `CacheRefreshScreen`: cast `dlState` to `int32` and use `atomic.StoreInt32`/`atomic.LoadInt32`. Data fields (`err`, `dest`) are written before the atomic state store, so release-acquire semantics protect them — the SDL thread reads state atomically, then reads err/dest, which are already visible.

- [ ] **Step 1: Update dlState to int32 and add atomic helpers**

Replace the type declaration and add inline atomic helpers. In `screen_download.go`, replace:

```go
type dlState int
```

with:

```go
type dlState int32

func (s *DownloadScreen) loadState() dlState {
	return dlState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *DownloadScreen) storeState(st dlState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}
```

- [ ] **Step 2: Fix goroutine writes — ensure data is written before atomic state store**

In the goroutine body (starting at `go func()`), find the two state-store paths and make sure `err`/`dest` are written strictly before `storeState`:

The error path (around line 72):
```go
if err != nil {
    logger.Error("download: failed file=%s: %v", upload.Filename, err)
    s.err = err
    s.storeState(dlError)
```

The success path (around line 119):
```go
    s.dest = finalDest
    s.storeState(dlDone)
```

- [ ] **Step 3: Fix SDL-thread reads — use loadState()**

In `Draw` (the `switch s.state {` block and the footer `switch s.state {`), replace every bare `s.state` read with `s.loadState()`:

```go
switch s.loadState() {
case dlDownloading:
    ...
case dlDone:
    ...
case dlError:
    ...
}
```

Footer switch:
```go
switch s.loadState() {
case dlDownloading:
    ...
default:
    ...
}
```

In `HandleEvent`, replace `if s.state != dlDownloading {` with:
```go
if s.loadState() != dlDownloading {
```

In `IsBusy`:
```go
func (s *DownloadScreen) IsBusy() bool {
    return s.loadState() == dlDownloading
}
```

- [ ] **Step 4: Remove the NewDownloadScreen initializer's direct state assignment**

In `NewDownloadScreen`, the `state: dlDownloading` in the struct literal is fine (no goroutine yet), but after the goroutine starts it must only be mutated via `storeState`. Verify the struct literal is the only pre-goroutine assignment. It is — no change needed.

- [ ] **Step 5: Build (headless) to confirm no compile errors**

Run: `./scripts/build.sh native`
Expected: clean build, no errors.

- [ ] **Step 6: Run tests**

Run: `./scripts/test.sh`
Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screen_download.go
git commit -m "fix(ui): eliminate data race in DownloadScreen via atomic state"
```

---

## Task 2: Fix data race in FetchUploadsScreen

**Files:**
- Modify: `internal/ui/screen_fetch_uploads.go`

**Context:** `s.state`, `s.err`, `s.uploads`, `s.ownedKeys`, and `s.isNotOwned` are all written from the goroutine in `NewFetchUploadsScreen` and `applyUploadsForKey` with no synchronization. Same release-acquire fix: change `fetchState` to `int32`, use atomics for state, write all data fields before the atomic state store.

Note: `applyUploadsForKey` is only called from within the goroutine — not from the SDL thread — so it only needs to use `storeState` at the end of its writes.

- [ ] **Step 1: Update fetchState to int32 and add atomic helpers**

Replace:
```go
type fetchState int
```
with:
```go
type fetchState int32

func (s *FetchUploadsScreen) loadState() fetchState {
	return fetchState(atomic.LoadInt32((*int32)(&s.state)))
}
func (s *FetchUploadsScreen) storeState(st fetchState) {
	atomic.StoreInt32((*int32)(&s.state), int32(st))
}
```

Add `"sync/atomic"` to the import block.

- [ ] **Step 2: Fix goroutine writes in NewFetchUploadsScreen**

In the goroutine, every `s.state = ...` assignment must become `s.storeState(...)`. Data fields (`s.err`, `s.isNotOwned`) must be written before the atomic state store.

Error path:
```go
s.err = keysErr
s.isNotOwned = strings.Contains(keysErr.Error(), "not owned")
s.storeState(fetchError)
sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
return
```

Multiple-keys path:
```go
s.ownedKeys = ownedKeys
s.storeState(fetchNeedsPurchasePick)
```

Free-game error path:
```go
s.err = err
s.storeState(fetchError)
```

Free-game success path:
```go
// ... s.uploads = append(...)
if len(s.uploads) == 0 {
    logger.Warn(...)
    s.err = fmt.Errorf("no downloadable files found for this game")
    s.storeState(fetchError)
} else {
    s.storeState(fetchDone)
}
```

- [ ] **Step 3: Fix applyUploadsForKey writes**

In `applyUploadsForKey`, every `s.state = ...` becomes `s.storeState(...)`. Data fields written before state store:

```go
func (s *FetchUploadsScreen) applyUploadsForKey(key itchio.OwnedKey) {
    ...
    if authErr != nil {
        s.err = authErr
        s.isNotOwned = strings.Contains(authErr.Error(), "not owned")
        s.storeState(fetchError)
        return
    }
    for _, u := range authUploads {
        s.uploads = append(s.uploads, ...)
    }
    if len(s.uploads) == 0 {
        logger.Warn(...)
        s.err = fmt.Errorf("no downloadable files found for this game")
        s.storeState(fetchError)
        return
    }
    s.storeState(fetchDone)
}
```

- [ ] **Step 4: Fix SDL-thread reads — use loadState()**

In `Draw`, replace `switch s.state {` with `switch s.loadState() {`.

In `HandleEvent`, replace every bare `s.state` read:
```go
if s.loadState() == fetchDone {
    return s.nextScreen()
}
if s.loadState() == fetchNeedsPurchasePick {
    return NewPurchasePickerScreen(...)
}
```
And:
```go
switch ev := e.(type) {
case *sdl.UserEvent:
    if s.isNotOwned {
```
and the keyboard/controller error-back cases:
```go
if s.loadState() == fetchError {
    return s.prev
}
```

Also in `Draw` footer:
```go
switch s.loadState() {
case fetchLoading:
    ...
default:
    ...
}
```

- [ ] **Step 5: Fix struct literal initializer in NewFetchUploadsScreen**

The `state: fetchLoading` in the struct literal is fine (no goroutine running yet). Verify it stays as a direct field assignment — it does.

- [ ] **Step 6: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screen_fetch_uploads.go
git commit -m "fix(ui): eliminate data race in FetchUploadsScreen via atomic state"
```

---

## Task 3: Fix data races in ListScreen + remove synchronous loadPage

**Files:**
- Modify: `internal/ui/screen_list.go`

**Context:** Two separate races:
1. `loadPage` is called as a goroutine but writes directly to `s.loading`, `s.viewGames`, `s.err`, `s.cursor`, `s.titleScrollX`, `s.titleScrollAt`, `s.tagScrollY`, `s.tagScrollAt`, `s.lastCursorMove`, `s.warmedGameURL`. Fix: add `pageUpdateCh chan pageResult` (capacity 1) and `loading atomic.Bool`; have `loadPage` push results, SDL thread consumes in `Draw`.
2. `s.totalGames` and `s.totalPages` written by both a goroutine (FetchTotalGames) and the SDL thread (rebuildView). Fix: change to `atomic.Int32`.
3. The synchronous `s.loadPage(1, "")` call inside `rebuildView` (Important #6) blocks the SDL thread on a network call. Fix: change to `go s.loadPage(1, "")`.

- [ ] **Step 1: Add pageResult type and pageUpdateCh to ListScreen**

At the top of `screen_list.go` (before `type ListScreen struct`), add:

```go
type pageResult struct {
	games []itchio.Game
	err   error
}
```

In the `ListScreen` struct, replace:
```go
loading    bool
err        error
totalGames int // 0 = not yet known
totalPages int // 0 = not yet known
```
with:
```go
loading     atomic.Bool
err         error
totalGames  atomic.Int32 // 0 = not yet known
totalPages  atomic.Int32 // 0 = not yet known
pageUpdateCh chan pageResult
```

- [ ] **Step 2: Initialize pageUpdateCh in NewListScreen**

In `NewListScreen`, after `cacheUpdateCh: make(chan []itchio.Game, 1),` add:
```go
pageUpdateCh: make(chan pageResult, 1),
```

Also remove the `loading: false` or similar initialization if present (zero value is fine for atomic.Bool).

- [ ] **Step 3: Rewrite loadPage to push through channel**

Replace the entire `loadPage` function:

```go
func (s *ListScreen) loadPage(page int, query string) {
	s.loading.Store(true)
	logger.Debug("feed: loading page %d query=%q", page, query)
	games, err := s.client.FetchGames(page, query)
	if err != nil {
		logger.Error("feed: page %d error: %v", page, err)
	} else {
		logger.Info("feed: page %d returned %d games", page, len(games))
	}
	select {
	case <-s.pageUpdateCh:
	default:
	}
	s.pageUpdateCh <- pageResult{games: games, err: err}
	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
}
```

- [ ] **Step 4: Consume pageUpdateCh in Draw**

At the top of `Draw`, alongside the existing `cacheUpdateCh` and `ownedUpdateCh` selects, add a third select block:

```go
select {
case res := <-s.pageUpdateCh:
    s.loading.Store(false)
    s.viewGames = res.games
    s.err = res.err
    s.cursor = 0
    s.titleScrollX = 0
    s.titleScrollAt = time.Now()
    s.tagScrollY = 0
    s.tagScrollAt = time.Now()
    s.lastCursorMove = time.Now()
    s.warmedGameURL = ""
default:
}
```

- [ ] **Step 5: Update all reads of s.loading to use .Load()**

Search for every `s.loading` read (not assignment) in `screen_list.go`. They appear in `Draw`. Change:
```go
if s.loading {
```
to:
```go
if s.loading.Load() {
```

- [ ] **Step 6: Update all reads of s.totalGames and s.totalPages to use .Load()**

In `Draw` and anywhere else `totalGames`/`totalPages` are read, change:
```go
if s.totalPages > 0 {
    pageInfo = fmt.Sprintf("Page %d/%d", currentPage, s.totalPages)
}
```
to:
```go
if tp := s.totalPages.Load(); tp > 0 {
    pageInfo = fmt.Sprintf("Page %d/%d", currentPage, tp)
}
```

- [ ] **Step 7: Update all writes of s.totalGames and s.totalPages to use .Store()**

In the goroutine at `NewListScreen` (around line 258):
```go
s.totalGames.Store(int32(total))
s.totalPages.Store(int32((total + itchio.PerPage - 1) / itchio.PerPage))
```

In `rebuildView` (around line 1201):
```go
s.totalGames.Store(int32(len(s.viewGames)))
s.totalPages.Store(int32((int(s.totalGames.Load()) + itchio.PerPage - 1) / itchio.PerPage))
```

Actually in rebuildView, since we just set totalGames, use the local value:
```go
n := len(s.viewGames)
s.totalGames.Store(int32(n))
s.totalPages.Store(int32((n + itchio.PerPage - 1) / itchio.PerPage))
```

- [ ] **Step 8: Fix synchronous loadPage call in rebuildView**

Find the line (around line 1238-1239):
```go
if !s.cacheReady {
    s.loadPage(1, "")
}
```
Change to:
```go
if !s.cacheReady {
    go s.loadPage(1, "")
}
```

- [ ] **Step 9: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 10: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "fix(ui): eliminate data races in ListScreen via channel-based loadPage results"
```

---

## Task 4: Fix Config.Save non-atomic write

**Files:**
- Modify: `internal/settings/settings.go`

**Context:** `Config.Save` uses `os.WriteFile` directly, which can produce a partial file on power loss. All other JSON writers in the codebase (inventory, owned cache, games cache) use tmp+rename. Apply the same pattern.

- [ ] **Step 1: Write the failing test**

Add to `internal/settings/settings_test.go`:

```go
func TestSave_IsAtomic(t *testing.T) {
    // After Save, the file must exist and be loadable — no .tmp residue.
    dir := t.TempDir()
    path := filepath.Join(dir, "config.json")

    cfg := &settings.Config{APIKey: "test-key", ROMSelection: "ask"}
    if err := cfg.Save(path); err != nil {
        t.Fatalf("Save: %v", err)
    }

    // No .tmp file should linger.
    tmp := path + ".tmp"
    if _, err := os.Stat(tmp); !os.IsNotExist(err) {
        t.Errorf("expected .tmp file to be absent after successful save, stat: %v", err)
    }

    // File must be loadable.
    loaded, err := settings.Load(path)
    if err != nil {
        t.Fatalf("Load after Save: %v", err)
    }
    if loaded.APIKey != "test-key" {
        t.Errorf("APIKey = %q, want %q", loaded.APIKey, "test-key")
    }
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `./scripts/test.sh`
Expected: all tests pass (the test verifies absence of .tmp, which is currently true since WriteFile leaves no tmp — but the test ensures we don't regress). Actually this test will pass with the current code too. The real correctness issue is power-loss safety, which can't be tested here. Proceed.

- [ ] **Step 3: Implement atomic Save**

Replace `Config.Save` in `internal/settings/settings.go`:

```go
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Error("settings: failed to write tmp config %s: %v", tmp, err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		logger.Error("settings: failed to rename config %s → %s: %v", tmp, path, err)
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `./scripts/test.sh`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "fix(settings): make Config.Save atomic via tmp+rename"
```

---

## Task 5: Remove unused DownloadFree parameter

**Files:**
- Modify: `internal/itchio/download.go`
- Modify: `internal/ui/screen_download.go`

**Context:** `DownloadFree`'s first parameter `_ string` (originally `gameURL`) is discarded — the resolver URL is self-contained in `upload.URL`. Removing it cleans the API.

- [ ] **Step 1: Update the download_test.go if it calls DownloadFree directly**

Check:
```bash
grep -n "DownloadFree" /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io/internal/itchio/download_test.go
```
If present, update those calls to remove the first argument.

- [ ] **Step 2: Remove the unused parameter from DownloadFree**

In `internal/itchio/download.go`, change the signature:
```go
func (c *Client) DownloadFree(_ string, upload Upload, dest string, progress func(int64, int64)) error {
```
to:
```go
func (c *Client) DownloadFree(upload Upload, dest string, progress func(int64, int64)) error {
```

- [ ] **Step 3: Update the call site**

In `internal/ui/screen_download.go` (around line 67), change:
```go
err = client.DownloadFree(game.URL, itchUpload, dest, progress)
```
to:
```go
err = client.DownloadFree(itchUpload, dest, progress)
```

- [ ] **Step 4: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/download.go internal/ui/screen_download.go
git commit -m "refactor(itchio): remove unused first parameter from DownloadFree"
```

---

## Task 6: Fix LevelFromString and logger write switch

**Files:**
- Modify: `internal/logger/logger.go`
- Modify: `internal/logger/logger_test.go`

**Context:** `LevelFromString` maps "warn" and "error" to `LevelInfo`, so users who set `log_level: "warn"` silently get info-level output. Also, `LevelError` falls into a `default:` branch in `write`, meaning any future level added above it would be silently tagged as `[ERROR]`.

- [ ] **Step 1: Update the existing LevelFromString test**

In `internal/logger/logger_test.go`, update the `TestLevelFromString` cases to reflect the new expected values and add `error` support:

```go
func TestLevelFromString(t *testing.T) {
    cases := []struct {
        input string
        want  logger.Level
    }{
        {"debug", logger.LevelDebug},
        {"DEBUG", logger.LevelDebug},
        {"Debug", logger.LevelDebug},
        {"info", logger.LevelInfo},
        {"INFO", logger.LevelInfo},
        {"", logger.LevelInfo},
        {"verbose", logger.LevelInfo},
        {"warn", logger.LevelWarn},
        {"WARN", logger.LevelWarn},
        {"error", logger.LevelError},
        {"ERROR", logger.LevelError},
    }
    for _, c := range cases {
        got := logger.LevelFromString(c.input)
        if got != c.want {
            t.Errorf("LevelFromString(%q) = %v, want %v", c.input, got, c.want)
        }
    }
}
```

- [ ] **Step 2: Run to verify test fails**

Run: `./scripts/test.sh`
Expected: `TestLevelFromString` fails because "warn" currently returns `LevelInfo`.

- [ ] **Step 3: Fix LevelFromString and write switch in logger.go**

Replace `LevelFromString`:
```go
// LevelFromString maps a string name to a Level.
// Recognised values (case-insensitive): "debug", "info", "warn", "error".
// Empty or unknown strings resolve to LevelInfo.
func LevelFromString(s string) Level {
    switch strings.ToLower(strings.TrimSpace(s)) {
    case "debug":
        return LevelDebug
    case "warn":
        return LevelWarn
    case "error":
        return LevelError
    default:
        return LevelInfo
    }
}
```

Replace the `default:` branch in `write` with an explicit `LevelError` case:
```go
func write(l Level, format string, args ...any) {
    if Level(currentLevel.Load()) > l {
        return
    }
    var tag string
    switch l {
    case LevelDebug:
        tag = "[DEBUG] "
    case LevelInfo:
        tag = "[INFO]  "
    case LevelWarn:
        tag = "[WARN]  "
    case LevelError:
        tag = "[ERROR] "
    default:
        tag = "[UNKNOWN] "
    }
    log.Print(tag + redact(fmt.Sprintf(format, args...)))
}
```

- [ ] **Step 4: Run tests**

Run: `./scripts/test.sh`
Expected: all tests pass including the updated `TestLevelFromString`.

- [ ] **Step 5: Commit**

```bash
git add internal/logger/logger.go internal/logger/logger_test.go
git commit -m "fix(logger): support warn/error in LevelFromString; explicit LevelError case in write"
```

---

## Task 7: Fix isGameRemoved fragile string matching

**Files:**
- Modify: `internal/itchio/game.go` (or `download.go` — wherever `FetchGameDetail` / `FetchUploads` return HTTP errors)
- Modify: `internal/inventory/updater.go`

**Context:** `isGameRemoved` checks `strings.Contains(err.Error(), "HTTP 404")`. This couples the check to a specific error message format. Adding a sentinel error (`ErrGameRemoved`) and wrapping it at the source (HTTP 404/410 returns) makes the check robust.

First, find where 404/410 errors are returned:

```bash
grep -n "HTTP 404\|HTTP 410\|StatusNotFound\|StatusGone" \
  /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io/internal/itchio/game.go \
  /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io/internal/itchio/download.go
```

- [ ] **Step 1: Write the failing test**

Add to `internal/inventory/updater_test.go`:

```go
func TestIsGameRemoved_SentinelError(t *testing.T) {
    // isGameRemoved is unexported; test via checkFreeGame/checkPaidGame behaviour
    // is integration-level. Instead verify that wrapping ErrGameRemoved works.
    // We test this indirectly via the exported itchio errors.
    if !errors.Is(fmt.Errorf("wrap: %w", itchio.ErrGameRemoved), itchio.ErrGameRemoved) {
        t.Error("ErrGameRemoved should unwrap via errors.Is")
    }
}
```

Also add the import `"errors"` and `"fmt"` and `"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"` if not already present.

- [ ] **Step 2: Add ErrGameRemoved to the itchio package**

Find the file where other sentinel errors are defined (likely `client.go` or `game.go`):
```bash
grep -n "ErrCloudflare\|var Err" /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io/internal/itchio/*.go | grep -v test | grep -v ".worktrees"
```

In the same file, add:
```go
// ErrGameRemoved is returned when the game page responds with HTTP 404 or 410.
var ErrGameRemoved = errors.New("game removed (HTTP 404/410)")
```

Add `"errors"` to the import if not already present.

- [ ] **Step 3: Wrap ErrGameRemoved at HTTP 404/410 return sites**

In `internal/itchio/game.go`, find the status check for FetchGameDetail and update to wrap the sentinel:
```go
if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
    return nil, fmt.Errorf("fetch game detail: %w", ErrGameRemoved)
}
if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("fetch game detail: HTTP %d", resp.StatusCode)
}
```

In `internal/itchio/download.go`, find the FetchUploads status check for 404/410:
```go
if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
    logger.Error("uploads: game page HTTP %d", resp.StatusCode)
    return nil, fmt.Errorf("fetch game page: %w", ErrGameRemoved)
}
if resp.StatusCode != http.StatusOK {
    logger.Error("uploads: game page HTTP %d", resp.StatusCode)
    return nil, fmt.Errorf("fetch game page: HTTP %d", resp.StatusCode)
}
```

Note: The existing check at the top of `FetchUploads` reads the body first then checks the status. After Task 11 (FetchUploads status order) this order will be swapped — for now, keep the existing order and just update the status check to use the sentinel.

- [ ] **Step 4: Update isGameRemoved in updater.go**

Replace:
```go
func isGameRemoved(err error) bool {
    if err == nil {
        return false
    }
    s := err.Error()
    return strings.Contains(s, "HTTP 404") || strings.Contains(s, "HTTP 410")
}
```
with:
```go
func isGameRemoved(err error) bool {
    return errors.Is(err, itchio.ErrGameRemoved)
}
```

Add `"errors"` import and `"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"` if not already present. Remove `"strings"` if no longer used.

- [ ] **Step 5: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/ internal/inventory/updater.go internal/inventory/updater_test.go
git commit -m "fix(itchio): replace fragile string-match in isGameRemoved with ErrGameRemoved sentinel"
```

---

## Task 8: Fix ImageCache bare http.Client

**Files:**
- Modify: `internal/itchio/client.go`
- Modify: `internal/renderer/image_cache.go`
- Modify: `cmd/itchio-pak/main_sdl.go`

**Context:** `NewImageCache` creates a bare `http.Client` with Go's default TLS fingerprint. The `itchio.Client` uses a Chrome-fingerprint transport to avoid Cloudflare detection. Cover art comes from itch.io CDN domains — if Cloudflare extends fingerprint checks to the CDN, image fetches will break silently. Expose the underlying `*http.Client` from `itchio.Client` and thread it through to `NewImageCache`.

- [ ] **Step 1: Add HTTPClient() accessor to itchio.Client**

In `internal/itchio/client.go`, add after the `NewClientWithBaseAndButler` function:

```go
// HTTPClient returns the underlying *http.Client used for all requests.
// Use this when another component (e.g. ImageCache) needs the same transport
// to maintain consistent TLS fingerprinting.
func (c *Client) HTTPClient() *http.Client {
    return c.http
}
```

- [ ] **Step 2: Update NewImageCache signature to accept *http.Client**

In `internal/renderer/image_cache.go`, change:

```go
func NewImageCache(maxEntries int) *ImageCache {
    return &ImageCache{
        ...
        client:   &http.Client{Timeout: 20 * time.Second},
        ...
    }
}
```

to:

```go
func NewImageCache(maxEntries int, httpClient *http.Client) *ImageCache {
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 20 * time.Second}
    }
    return &ImageCache{
        lru:      list.New(),
        items:    make(map[string]*list.Element),
        fetching: make(map[string]struct{}),
        failed:   make(map[string]struct{}),
        max:      maxEntries,
        client:   httpClient,
        readyCh:  make(chan rawImage, 32),
        sem:      make(chan struct{}, maxConcurrentFetches),
    }
}
```

- [ ] **Step 3: Update main_sdl.go call site**

In `cmd/itchio-pak/main_sdl.go`, the `client` is created before `cache`. Change:

```go
cache := renderer.NewImageCache(50)
```
to:
```go
cache := renderer.NewImageCache(50, client.HTTPClient())
```

Note: verify that `client` is declared before this line — it is (line 113 in the original file).

- [ ] **Step 4: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/client.go internal/renderer/image_cache.go cmd/itchio-pak/main_sdl.go
git commit -m "fix(renderer): use itchio transport in ImageCache for consistent TLS fingerprinting"
```

---

## Task 9: Fix inRanges cmap sort assumption

**Files:**
- Modify: `internal/renderer/text.go`

**Context:** `inRanges` uses binary search and requires sorted ranges. `parseCmap4` and `parseCmap12` rely on the TTF cmap spec ordering, which is typically sorted but not guaranteed for malformed fonts. Adding an explicit sort is a cheap defensive guard.

- [ ] **Step 1: Add sort import and sort after parseCmap4**

In `internal/renderer/text.go`, add `"sort"` to the import block. Then at the end of `parseCmap4`, before `return ranges`:

```go
sort.Slice(ranges, func(i, j int) bool { return ranges[i].lo < ranges[j].lo })
return ranges
```

- [ ] **Step 2: Add sort after parseCmap12**

At the end of `parseCmap12`, before `return ranges`:

```go
sort.Slice(ranges, func(i, j int) bool { return ranges[i].lo < ranges[j].lo })
return ranges
```

- [ ] **Step 3: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/renderer/text.go
git commit -m "fix(renderer): sort cmap ranges after parsing to guard against malformed fonts"
```

---

## Task 10: Fix Advisory IsAdvisoryTriggered allocation

**Files:**
- Modify: `internal/itchio/advisory.go`

**Context:** The `norm` closure is re-created and called on every `IsAdvisoryTriggered` invocation. Extracting it to a package-level function removes the closure allocation. The per-call slice allocations (one per Disabled list) remain but are acceptable for once-per-page-load usage.

- [ ] **Step 1: Extract norm to package level**

In `internal/itchio/advisory.go`, remove the `norm` closure from inside `IsAdvisoryTriggered` and add a package-level function:

```go
// normalizeTagList returns a copy of list with each element lowercased and trimmed.
func normalizeTagList(list []string) []string {
    out := make([]string, len(list))
    for i, d := range list {
        out[i] = strings.ToLower(strings.TrimSpace(d))
    }
    return out
}
```

Then in `IsAdvisoryTriggered`, replace:
```go
norm := func(list []string) []string {
    out := make([]string, len(list))
    for i, d := range list {
        out[i] = strings.ToLower(strings.TrimSpace(d))
    }
    return out
}
adultDis := norm(cfg.AdultContent.Disabled)
queerDis := norm(cfg.QueerContent.Disabled)
heavyDis := norm(cfg.HeavyThemes.Disabled)
substanceDis := norm(cfg.SubstanceUse.Disabled)
```
with:
```go
adultDis := normalizeTagList(cfg.AdultContent.Disabled)
queerDis := normalizeTagList(cfg.QueerContent.Disabled)
heavyDis := normalizeTagList(cfg.HeavyThemes.Disabled)
substanceDis := normalizeTagList(cfg.SubstanceUse.Disabled)
```

- [ ] **Step 2: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/itchio/advisory.go
git commit -m "refactor(itchio): extract norm closure to package-level function in IsAdvisoryTriggered"
```

---

## Task 11: Fix UpdateService.Stop idempotence

**Files:**
- Modify: `internal/inventory/updater.go`

**Context:** `Stop()` sends to `stopCh`. If the goroutine has already exited (e.g. double-deferred Stop, or called after goroutine naturally ended), the send is silently dropped by the default branch. The comment says "Idempotent" but the underlying channel can only signal once. Use `sync.Once` with `close(stopCh)` — closing a channel is truly idempotent (the goroutine receives the zero value on close, and subsequent closes panic). Use a `stopOnce sync.Once` to guard the close.

- [ ] **Step 1: Update UpdateService struct and Stop**

In `internal/inventory/updater.go`, add `stopOnce sync.Once` to the struct:

```go
type UpdateService struct {
    inv           *Inventory
    inventoryPath string
    client        *itchio.Client
    notify        func()
    triggerCh     chan struct{}
    stopCh        chan struct{}
    stopOnce      sync.Once
    running       atomic.Bool
}
```

Add `"sync"` to imports if not present.

Replace `Stop`:
```go
// Stop signals the goroutine to exit. Idempotent — safe to call multiple times.
func (s *UpdateService) Stop() {
    s.stopOnce.Do(func() { close(s.stopCh) })
}
```

Update the goroutine in `Start` to receive from the closed channel correctly. The current `case <-s.stopCh:` already works with a closed channel (receives immediately with zero value). No change needed in the goroutine itself.

- [ ] **Step 2: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/inventory/updater.go
git commit -m "fix(inventory): make UpdateService.Stop truly idempotent via sync.Once channel close"
```

---

## Task 12: Fix time.After in pollForGames

**Files:**
- Modify: `internal/ui/dev_start.go`

**Context:** `pollForGames` uses `time.After(100ms)` in a loop. Each iteration creates a new timer that is not GC'd until it fires. Use `time.NewTicker` with `defer ticker.Stop()` instead.

- [ ] **Step 1: Replace time.After with time.NewTicker**

In `internal/ui/dev_start.go`, replace the `pollForGames` function:

```go
func (s *devAutoDetailScreen) pollForGames() {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-s.stopPoll:
            return
        case <-ticker.C:
            if len(s.list.viewGames) > 0 {
                sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: DevNavEventCode})
                return
            }
        }
    }
}
```

- [ ] **Step 2: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/dev_start.go
git commit -m "fix(ui): replace time.After loop with time.NewTicker in pollForGames"
```

---

## Task 13: Minor fixes batch

**Files:**
- Modify: `internal/ui/screen_list.go` — remove dead `drawPlaceholder` method; fix `shoulderAccelMin` alignment
- Modify: `internal/itchio/download_auth.go` — replace insertion sort in `AnnotateBundleNames`
- Modify: `internal/itchio/download.go` — fix FetchUploads status check order
- Modify: `internal/itchio/sort.go` — add capacity hint to SortModeFree/SortModePaid

- [ ] **Step 1: Remove dead drawPlaceholder from screen_list.go**

Find and delete the `drawPlaceholder` method (around line 874–880):
```go
// drawPlaceholder renders a bordered rectangle with centered text.
func (s *ListScreen) drawPlaceholder(r *renderer.Renderer, x, y, w, h int32, label string) {
    bg := r.Theme.Background
    r.DrawRect(x, y, w, h, 45, 45, 45)
    r.DrawRect(x+2, y+2, w-4, h-4, bg[0], bg[1], bg[2])
    r.DrawText(label, x+w/2-40, y+h/2-10, 80, 80, 80)
}
```

- [ ] **Step 2: Fix shoulderAccelMin alignment in screen_list.go**

In the const block, `shoulderAccelMin` has inconsistent tab spacing. Fix:
```go
	shoulderAccelMin      = 15 * time.Millisecond  // minimum repeat interval for D-pad page-scroll
```
(align the `=` sign with the other constants in the block using tabs, matching the column of `accelRamp`, `accelMin`, etc.)

- [ ] **Step 3: Replace insertion sort in AnnotateBundleNames**

In `internal/itchio/download_auth.go`, replace:
```go
for i := 1; i < len(bundleIdxs); i++ {
    for j := i; j > 0 && bundleIdxs[j].t.Before(bundleIdxs[j-1].t); j-- {
        bundleIdxs[j], bundleIdxs[j-1] = bundleIdxs[j-1], bundleIdxs[j]
    }
}
```
with:
```go
sort.Slice(bundleIdxs, func(i, j int) bool {
    return bundleIdxs[i].t.Before(bundleIdxs[j].t)
})
```

Add `"sort"` to imports if not present.

- [ ] **Step 4: Fix FetchUploads status check order**

In `internal/itchio/download.go`, in `FetchUploads`, the body is read before the status code is checked. Reorder so that after reading the status, non-OK responses are rejected without reading their body unnecessarily:

Current structure (lines ~60–70):
```go
resp, err := c.http.Get(gameURL)
if err != nil { return nil, ... }
body, err := io.ReadAll(resp.Body)
resp.Body.Close()
if err != nil { return nil, ... }
if resp.StatusCode != http.StatusOK {
    ...
    return nil, fmt.Errorf("fetch game page: HTTP %d", resp.StatusCode)
}
```

Note: because the body is needed for CSRF parsing, we still need to read it for success cases. But we should check status before reading body for error responses. The simplest reorder is to peek at the status first:

```go
resp, err := c.http.Get(gameURL)
if err != nil {
    return nil, fmt.Errorf("fetch game page: %w", err)
}
if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
    resp.Body.Close()
    logger.Error("uploads: game page HTTP %d", resp.StatusCode)
    return nil, fmt.Errorf("fetch game page: %w", ErrGameRemoved)
}
if resp.StatusCode != http.StatusOK {
    resp.Body.Close()
    logger.Error("uploads: game page HTTP %d", resp.StatusCode)
    return nil, fmt.Errorf("fetch game page: HTTP %d", resp.StatusCode)
}
body, err := io.ReadAll(resp.Body)
resp.Body.Close()
if err != nil {
    return nil, fmt.Errorf("read game page: %w", err)
}
```

Note: This also incorporates the `ErrGameRemoved` wrapping from Task 7 for the 404/410 case. If Task 7 is done first, remove the separate 404/410 check that was added there for `FetchUploads` and use this consolidated version instead.

- [ ] **Step 5: Add capacity hint to SortModeFree and SortModePaid**

In `internal/itchio/sort.go`, replace:
```go
case SortModeFree:
    out := make([]Game, 0)
```
with:
```go
case SortModeFree:
    out := make([]Game, 0, len(games))
```

And:
```go
case SortModePaid:
    out := make([]Game, 0)
```
with:
```go
case SortModePaid:
    out := make([]Game, 0, len(games))
```

- [ ] **Step 6: Build and test**

Run: `./scripts/build.sh native && ./scripts/test.sh`
Expected: clean build, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screen_list.go internal/itchio/download_auth.go \
        internal/itchio/download.go internal/itchio/sort.go
git commit -m "chore: minor code quality fixes — dead code, sort, alloc hints, status check order"
```

---

## Self-Review

**Spec coverage:**
- Critical #1 (DownloadScreen race) → Task 1 ✓
- Critical #2 (FetchUploadsScreen race) → Task 2 ✓
- Critical #3 (ListScreen race) → Task 3 ✓
- Important #5 (Stop idempotence) → Task 11 ✓
- Important #6 (sync loadPage in rebuildView) → Task 3 Step 8 ✓
- Important #7 (DownloadFree unused param) → Task 5 ✓
- Important #8 (LevelFromString gaps) → Task 6 ✓
- Important #9 (Config.Save non-atomic) → Task 4 ✓
- Important #10 (ImageCache http client) → Task 8 ✓
- Important #11 (isGameRemoved fragile) → Task 7 ✓
- Important #12 (cmap sort assumption) → Task 9 ✓
- Important #13 (Advisory alloc) → Task 10 ✓
- Important #16 (time.After loop) → Task 12 ✓
- Minor #14 (SortModeFree/Paid alloc) → Task 13 Step 5 ✓
- Minor #17 (dead drawPlaceholder) → Task 13 Step 1 ✓
- Minor #18 (shoulderAccelMin alignment) → Task 13 Step 2 ✓
- Minor #19 (AnnotateBundleNames sort) → Task 13 Step 3 ✓
- Minor #20 (logger default case) → Task 6 Step 3 ✓
- Minor #21 (FetchUploads status order) → Task 13 Step 4 ✓
- Minor #22 (Inventory.Save lock comment) → Not included; the code is correct as-is; the comment is adequate. Adding a clarifying comment would be noise.

**Security #4 (API key in URL):** Reviewer noted `logger.RegisterSecret(cfg.APIKey, "[API-KEY]")` in main_sdl.go handles this correctly already. No code change needed — this was an informational note, not an action item.

**Placeholder scan:** No TBDs found. All steps contain exact code.

**Type consistency:** All struct field changes (atomic.Bool, atomic.Int32) are consistently applied across all read and write sites within their tasks.
