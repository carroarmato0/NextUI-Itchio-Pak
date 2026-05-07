# OWNED Sort Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new OWNED sort mode that surfaces the user's purchased itch.io library, and show an OWNED badge on purchased-but-not-yet-downloaded games in every sort view.

**Architecture:** A new `owned_cache.go` file persists owned game URLs to disk (written after API key validation, loaded at startup). The list screen receives an `onOwnedReady` callback threaded from KeyTestScreen through SettingsScreen; updates flow back via a channel + SDL event, mirroring the existing `cacheUpdateCh` pattern. The badge switch in Draw gains one new case; the sort cycle skips OWNED when the owned map is empty.

**Tech Stack:** Go 1.22, SDL2 via go-sdl2, `encoding/json` for the cache file.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/itchio/owned_cache.go` | OwnedCache struct, Save/Load helpers |
| Create | `internal/itchio/owned_cache_test.go` | Round-trip + missing-file tests |
| Modify | `internal/itchio/sort.go` | SortModeOwned constant, SortModes slice, SortModeBadge, ApplySort (add `owned` param + OWNED case) |
| Modify | `internal/itchio/sort_test.go` | Add `, nil` to all existing ApplySort calls; add OWNED tests; update SortModeBadge + cycle tests |
| Modify | `internal/ui/screen_list.go` | New fields + constructor changes + channel drain + nextSortMode helper + rebuildView + badge |
| Modify | `internal/ui/screen_settings.go` | Thread `onOwnedReady` param through to KeyTestScreen |
| Modify | `internal/ui/screen_apikey_check.go` | Accept + call `onOwnedReady` on success |
| Modify | `cmd/itchio-pak/main_sdl.go` | Add `ownedCachePath`, pass to `NewListScreen` |

---

## Task 1: Owned cache file

**Files:**
- Create: `internal/itchio/owned_cache.go`
- Create: `internal/itchio/owned_cache_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/itchio/owned_cache_test.go`:

```go
package itchio_test

import (
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestOwnedCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned_cache.json")
	urls := []string{"https://a.itch.io/game1", "https://b.itch.io/game2"}

	if err := itchio.SaveOwnedCache(path, urls); err != nil {
		t.Fatalf("SaveOwnedCache: %v", err)
	}
	got, err := itchio.LoadOwnedCache(path)
	if err != nil {
		t.Fatalf("LoadOwnedCache: %v", err)
	}
	if len(got) != len(urls) {
		t.Fatalf("got %d URLs, want %d", len(got), len(urls))
	}
	for i, u := range urls {
		if got[i] != u {
			t.Errorf("url[%d]: got %q, want %q", i, got[i], u)
		}
	}
}

func TestLoadOwnedCache_MissingFile(t *testing.T) {
	got, err := itchio.LoadOwnedCache(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slice for missing file, got %v", got)
	}
}

func TestSaveOwnedCache_EmptySlice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned_cache.json")
	if err := itchio.SaveOwnedCache(path, []string{}); err != nil {
		t.Fatalf("SaveOwnedCache empty: %v", err)
	}
	got, err := itchio.LoadOwnedCache(path)
	if err != nil {
		t.Fatalf("LoadOwnedCache after empty save: %v", err)
	}
	// empty array round-trips to empty (not nil)
	if len(got) != 0 {
		t.Errorf("expected 0 URLs, got %v", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|owned_cache"
```
Expected: compile error — `itchio.SaveOwnedCache` / `itchio.LoadOwnedCache` undefined.

- [ ] **Step 3: Create `internal/itchio/owned_cache.go`**

```go
package itchio

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// OwnedCache is the on-disk representation of the user's owned game URLs.
type OwnedCache struct {
	SavedAt time.Time `json:"saved_at"`
	URLs    []string  `json:"urls"`
}

// SaveOwnedCache writes urls to path atomically (write to .tmp then rename).
func SaveOwnedCache(path string, urls []string) error {
	cache := OwnedCache{SavedAt: time.Now(), URLs: urls}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("marshal owned cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write owned cache tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename owned cache: %w", err)
	}
	return nil
}

// LoadOwnedCache reads the cache at path and returns the URL list.
// Returns a nil slice (not an error) when the file does not exist.
func LoadOwnedCache(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read owned cache: %w", err)
	}
	var cache OwnedCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse owned cache: %w", err)
	}
	return cache.URLs, nil
}
```

- [ ] **Step 4: Run tests — all should pass**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/owned_cache.go internal/itchio/owned_cache_test.go
git commit -m "feat(itchio): add OwnedCache save/load helpers"
```

---

## Task 2: SortModeOwned in `sort.go`

**Files:**
- Modify: `internal/itchio/sort.go`
- Modify: `internal/itchio/sort_test.go`

> Note: `ApplySort` gains a new `owned map[string]bool` sixth parameter. All existing call sites in `sort_test.go` are updated to pass `nil`. The one call site in `screen_list.go` is updated in Task 3. **Do not run tests until both files are edited** — the package will not compile in between.

- [ ] **Step 1: Write the new OWNED tests and update all existing ApplySort calls in `sort_test.go`**

In `sort_test.go`, every existing `ApplySort(...)` call currently has 5 arguments. Add `, nil` as the 6th argument to **every** call. There are 16 such calls — use search/replace:

```
itchio.ApplySort(games, itchio.SortMode → itchio.ApplySort(games, itchio.SortMode
```

Specifically, change every occurrence of the form:
```go
itchio.ApplySort(..., nil, nil, nil)
```
to:
```go
itchio.ApplySort(..., nil, nil, nil, nil)
```

Also update `TestApplySort_ReturnsNewSlice` — its loop body still works but calls `ApplySort` with 5 args.

Then append these new tests at the bottom (before the `// helpers` section):

```go
func TestApplySort_Owned_FiltersToOwnedOnly(t *testing.T) {
	games := []itchio.Game{
		{Title: "Zelda", URL: "https://a.itch.io/zelda"},
		{Title: "Asteroids", URL: "https://b.itch.io/asteroids"},
		{Title: "Metroid", URL: "https://c.itch.io/metroid"},
		{Title: "Pong", URL: "https://d.itch.io/pong"}, // not owned
	}
	owned := map[string]bool{
		"https://a.itch.io/zelda":     true,
		"https://b.itch.io/asteroids": true,
		"https://c.itch.io/metroid":   true,
	}
	result := itchio.ApplySort(games, itchio.SortModeOwned, nil, nil, nil, owned)
	if len(result) != 3 {
		t.Fatalf("expected 3 owned games, got %d", len(result))
	}
	for _, g := range result {
		if g.Title == "Pong" {
			t.Error("Pong (not owned) should not appear in OWNED sort")
		}
	}
}

func TestApplySort_Owned_PendingUpdatesFirst(t *testing.T) {
	games := []itchio.Game{
		{Title: "Zelda", URL: "https://a.itch.io/zelda"},
		{Title: "Asteroids", URL: "https://b.itch.io/asteroids"},
		{Title: "Metroid", URL: "https://c.itch.io/metroid"},
	}
	owned := map[string]bool{
		"https://a.itch.io/zelda":     true,
		"https://b.itch.io/asteroids": true,
		"https://c.itch.io/metroid":   true,
	}
	pendingUpdates := map[string]bool{
		"https://c.itch.io/metroid": true,
	}
	result := itchio.ApplySort(games, itchio.SortModeOwned, nil, pendingUpdates, nil, owned)
	if len(result) != 3 {
		t.Fatalf("expected 3 games, got %d", len(result))
	}
	if result[0].Title != "Metroid" {
		t.Errorf("pending-update game should be first, got %q", result[0].Title)
	}
	// Remaining games sorted A-Z
	if result[1].Title != "Asteroids" {
		t.Errorf("expected Asteroids second (A-Z), got %q", result[1].Title)
	}
	if result[2].Title != "Zelda" {
		t.Errorf("expected Zelda third (A-Z), got %q", result[2].Title)
	}
}

func TestApplySort_Owned_NilMap(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeOwned, nil, nil, nil, nil)
	if len(result) != 0 {
		t.Errorf("nil owned map: expected 0 games, got %d", len(result))
	}
}

func TestApplySort_Owned_AZWithinGroup(t *testing.T) {
	games := []itchio.Game{
		{Title: "Zelda", URL: "https://a.itch.io/zelda"},
		{Title: "Asteroids", URL: "https://b.itch.io/asteroids"},
		{Title: "Metroid", URL: "https://c.itch.io/metroid"},
	}
	owned := map[string]bool{
		"https://a.itch.io/zelda":     true,
		"https://b.itch.io/asteroids": true,
		"https://c.itch.io/metroid":   true,
	}
	result := itchio.ApplySort(games, itchio.SortModeOwned, nil, nil, nil, owned)
	want := []string{"Asteroids", "Metroid", "Zelda"}
	if !equalTitles(result, want) {
		t.Errorf("OWNED A-Z: got %v, want %v", titles(result), want)
	}
}
```

Also add `SortModeOwned` to the `TestSortModeBadge` cases:
```go
{itchio.SortModeOwned, "OWNED"},
```

The `TestNextSortMode_Cycle` test will pass automatically once `SortModeOwned` is in `SortModes` — no change needed.

- [ ] **Step 2: Update `internal/itchio/sort.go`**

**2a.** Add the new constant (after `SortModePaid`):
```go
SortModePaid  SortMode = "paid"
SortModeOwned SortMode = "owned"
```

**2b.** Append `SortModeOwned` to `SortModes`:
```go
var SortModes = []SortMode{
	SortModeRSS, SortModeAZ, SortModeZA, SortModeNew,
	SortModeDL, SortModeFree, SortModePaid, SortModeOwned,
}
```

**2c.** Add `"OWNED"` case to `SortModeBadge`:
```go
case SortModeOwned:
    return "OWNED"
```

**2d.** Update `ApplySort` signature — add `owned map[string]bool` as the sixth parameter:
```go
func ApplySort(games []Game, mode SortMode, downloaded, pendingUpdates, removed, owned map[string]bool) []Game {
```

**2e.** Add `SortModeOwned` case inside `ApplySort` (before the `default` case):
```go
case SortModeOwned:
    var updates, rest []Game
    for _, g := range games {
        if !owned[g.URL] {
            continue
        }
        if pendingUpdates[g.URL] {
            updates = append(updates, g)
        } else {
            rest = append(rest, g)
        }
    }
    sort.SliceStable(rest, func(i, j int) bool {
        return sortKey(rest[i].Title) < sortKey(rest[j].Title)
    })
    out := make([]Game, 0, len(updates)+len(rest))
    out = append(out, updates...)
    out = append(out, rest...)
    return out
```

- [ ] **Step 3: Run tests — all should pass**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```
Expected: all `ok`. The `internal/ui` package will fail to compile (ApplySort call site not yet updated) — that is expected and fixed in Task 3.

Actually — `internal/ui` has the `!headless` build tag so the test runner only compiles the headless packages. Confirm with:
```bash
./scripts/test.sh 2>&1
```
Expected: all `ok` (UI screens are excluded from CI).

- [ ] **Step 4: Commit**

```bash
git add internal/itchio/sort.go internal/itchio/sort_test.go
git commit -m "feat(itchio): add SortModeOwned with pending-updates-first A-Z sort"
```

---

## Task 3: ListScreen owned infrastructure

**Files:**
- Modify: `internal/ui/screen_list.go`

This task adds the owned-data plumbing to the list screen. All changes are in SDL-only code (build tag `!headless`) — verified by running the existing test suite after each sub-step.

- [ ] **Step 1: Add new fields to `ListScreen` struct**

Locate the struct definition (around line 75). After the existing `cacheUpdateCh` field, add:

```go
// ownedUpdateCh carries owned-URL map updates from the background goroutine
// (post-API-key-validation) to the SDL thread. Capacity 1: stale updates
// are silently discarded, same as cacheUpdateCh.
ownedUpdateCh  chan map[string]bool
ownedURLs      map[string]bool
ownedCachePath string

// onOwnedReady is called by KeyTestScreen after a successful key validation.
// It saves the owned cache to disk and sends the new map to ownedUpdateCh.
onOwnedReady func([]itchio.OwnedGame)
```

- [ ] **Step 2: Update `NewListScreen` signature and constructor body**

**2a.** Add `ownedCachePath string` as the last parameter (after `onThemeToggle func(bool)`):

```go
func NewListScreen(
    client *itchio.Client,
    cfg *settings.Config,
    cfgPath string,
    cache *renderer.ImageCache,
    cachePath string,
    inv *inventory.Inventory,
    inventoryPath string,
    updateSvc UpdateServicer,
    nextUITheme theme.Theme,
    defaultTheme theme.Theme,
    themeAvailable bool,
    onThemeToggle func(bool),
    ownedCachePath string,
) *ListScreen {
```

**2b.** In the constructor body, after the existing field assignments, add:

```go
s.ownedCachePath = ownedCachePath
s.ownedUpdateCh = make(chan map[string]bool, 1)
s.ownedURLs = make(map[string]bool)

// Load owned cache from disk (populated on previous API key validation).
if urls, err := itchio.LoadOwnedCache(ownedCachePath); err == nil && len(urls) > 0 {
    for _, u := range urls {
        s.ownedURLs[u] = true
    }
    logger.Info("owned: loaded %d owned game URL(s) from cache", len(s.ownedURLs))
} else if err != nil {
    logger.Warn("owned: failed to load owned cache: %v", err)
}

// Build the callback invoked by KeyTestScreen after a successful key validation.
// Runs on a background goroutine — must not write to s.ownedURLs directly.
s.onOwnedReady = func(owned []itchio.OwnedGame) {
    urls := make([]string, len(owned))
    for i, g := range owned {
        urls[i] = g.URL
    }
    if err := itchio.SaveOwnedCache(s.ownedCachePath, urls); err != nil {
        logger.Warn("owned: failed to save owned cache: %v", err)
    }
    m := make(map[string]bool, len(urls))
    for _, u := range urls {
        m[u] = true
    }
    // Non-blocking drain so a stale update is replaced, then send.
    select {
    case <-s.ownedUpdateCh:
    default:
    }
    s.ownedUpdateCh <- m
    sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
    logger.Info("owned: %d owned game URL(s) received from key validation", len(m))
}
```

- [ ] **Step 3: Drain `ownedUpdateCh` in `Draw`**

In `Draw`, the existing `cacheUpdateCh` drain is at the very top of the function (around line 407):
```go
select {
case games := <-s.cacheUpdateCh:
    ...
default:
}
```

Add a second drain block immediately after it:
```go
select {
case newOwned := <-s.ownedUpdateCh:
    s.ownedURLs = newOwned
    s.rebuildView()
default:
}
```

- [ ] **Step 4: Add `nextSortMode` helper and replace the two call sites**

Add this method after `stopAlphaHold`:
```go
// nextSortMode returns the next mode in the cycle, skipping SortModeOwned
// when no owned data is loaded (empty ownedURLs map).
func (s *ListScreen) nextSortMode() itchio.SortMode {
    m := itchio.NextSortMode(s.sortMode)
    if m == itchio.SortModeOwned && len(s.ownedURLs) == 0 {
        m = itchio.NextSortMode(m)
    }
    return m
}
```

Find the two occurrences of `itchio.NextSortMode(s.sortMode)` in `HandleEvent` (keyboard SELECT and controller BACK, both around lines 979 and 1069) and replace each with `s.nextSortMode()`.

- [ ] **Step 5: Update `rebuildView` to pass `s.ownedURLs` to `ApplySort`**

Find the line (around line 1196):
```go
s.viewGames = itchio.ApplySort(s.cachedGames, s.sortMode, downloaded, pendingUpdates, removed)
```
Change it to:
```go
s.viewGames = itchio.ApplySort(s.cachedGames, s.sortMode, downloaded, pendingUpdates, removed, s.ownedURLs)
```

- [ ] **Step 6: Pass `s.onOwnedReady` to both `NewSettingsScreen` calls**

The two calls are around lines 981 and 1064. Currently:
```go
return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle)
```
Change both to:
```go
return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc, s.nextUITheme, s.defaultTheme, s.themeAvailable, s.onThemeToggle, s.onOwnedReady)
```

- [ ] **Step 7: Run tests — all should pass**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```
Expected: all `ok` (UI not compiled under headless tag, so the signature mismatch in screen_settings.go does not break CI yet).

- [ ] **Step 8: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): add owned cache infrastructure to ListScreen"
```

---

## Task 4: OWNED badge in `Draw`

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Add `isOwned` lookup and new badge case**

In `Draw`, locate the per-row badge section (around line 592):
```go
isPendingUpdate := s.inv.HasPendingUpdates(g.URL)
isRemovedGame := s.inv.IsRemoved(g.URL)
isPresent := s.inv.IsPresent(g.URL)

// Badge: update/removed/downloaded state or price.
var badgeLabel string
var badgeR, badgeG, badgeB uint8
switch {
case isPendingUpdate:
    ...
case isRemovedGame:
    ...
case isPresent:
    badgeLabel = "DL"
    badgeR, badgeG, badgeB = 80, 200, 220
case g.IsFree:
    ...
default:
    ...
}
```

Add `isOwned` after the existing three bool declarations:
```go
isOwned := s.ownedURLs[g.URL]
```

Insert the new badge case between `isPresent` and `g.IsFree`:
```go
case isPresent:
    badgeLabel = "DL"
    badgeR, badgeG, badgeB = 80, 200, 220
case isOwned:
    badgeLabel = "OWNED"
    badgeR, badgeG, badgeB = 60, 200, 120
case g.IsFree:
```

- [ ] **Step 2: Run tests**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```
Expected: all `ok`.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): show OWNED badge for purchased-but-not-downloaded games"
```

---

## Task 5: Thread `onOwnedReady` through SettingsScreen and KeyTestScreen

**Files:**
- Modify: `internal/ui/screen_settings.go`
- Modify: `internal/ui/screen_apikey_check.go`

- [ ] **Step 1: Update `SettingsScreen`**

**1a.** Add field to the `SettingsScreen` struct (after `onThemeToggle`):
```go
onOwnedReady func([]itchio.OwnedGame)
```

**1b.** Add parameter to `NewSettingsScreen` (after `onThemeToggle func(bool)`):
```go
onOwnedReady func([]itchio.OwnedGame),
```

**1c.** Assign in constructor body:
```go
s.onOwnedReady = onOwnedReady
```

**1d.** Pass `s.onOwnedReady` to both `NewKeyTestScreen` calls (lines 367 and 398):
```go
// Before:
return NewKeyTestScreen(s.client, s.cfg, s)
// After:
return NewKeyTestScreen(s.client, s.cfg, s, s.onOwnedReady)
```

- [ ] **Step 2: Update `KeyTestScreen`**

**2a.** Add field to the `KeyTestScreen` struct:
```go
onOwnedReady func([]itchio.OwnedGame)
```

**2b.** Add parameter to `NewKeyTestScreen` (after `prev Screen`):
```go
func NewKeyTestScreen(client *itchio.Client, cfg *settings.Config, prev Screen, onOwnedReady func([]itchio.OwnedGame)) *KeyTestScreen {
```

**2c.** Assign in constructor:
```go
s := &KeyTestScreen{client: client, cfg: cfg, prev: prev, onOwnedReady: onOwnedReady, state: keyTestRunning}
```

**2d.** Call the callback in the success branch of the goroutine (after setting `s.ownedCount`):
```go
s.username = username
s.ownedCount = len(owned)
s.state = keyTestOK
if s.onOwnedReady != nil {
    s.onOwnedReady(owned)
}
```

- [ ] **Step 3: Run tests**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```
Expected: all `ok`. The `main_sdl.go` call site will not compile until Task 6 — but it is excluded from the headless test runner.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_settings.go internal/ui/screen_apikey_check.go
git commit -m "feat(ui): thread onOwnedReady callback through Settings and KeyTest screens"
```

---

## Task 6: Wire `ownedCachePath` in `main_sdl.go`

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`

- [ ] **Step 1: Add `ownedCachePath` and pass it to `NewListScreen`**

Find the block where `cachePath` and `inventoryPath` are declared (around line 29):
```go
cachePath := filepath.Join(filepath.Dir(cfgPath), "games_cache.json")
```

Add immediately after:
```go
ownedCachePath := filepath.Join(filepath.Dir(cfgPath), "owned_cache.json")
```

Find the `NewListScreen` call (around line 131):
```go
listScreen := ui.NewListScreen(client, cfg, cfgPath, cache, cachePath, inv, inventoryPath, updateSvc, nextUITheme, defaultTheme, themeAvailable, onThemeToggle)
```
Change to:
```go
listScreen := ui.NewListScreen(client, cfg, cfgPath, cache, cachePath, inv, inventoryPath, updateSvc, nextUITheme, defaultTheme, themeAvailable, onThemeToggle, ownedCachePath)
```

- [ ] **Step 2: Build for the host to confirm everything compiles**

```bash
./scripts/build.sh native 2>&1 | tail -20
```
Expected: build succeeds with no errors.

- [ ] **Step 3: Run full test suite**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```
Expected: all `ok`.

- [ ] **Step 4: Commit**

```bash
git add cmd/itchio-pak/main_sdl.go
git commit -m "feat(main): wire ownedCachePath into ListScreen"
```

---

## Task 7: Cross-compile and smoke-test

- [ ] **Step 1: Cross-compile for all targets**

```bash
./scripts/build.sh all 2>&1 | tail -20
```
Expected: three binaries produced, no errors.

- [ ] **Step 2: Verify OWNED sort badge colours are visually distinct**

Run the dev-screenshot tool against the list screen to confirm OWNED badge (teal-green) is distinct from Free (green) and DL (cyan):

```bash
./scripts/dev-screenshot.sh --all --out-dir /tmp/itchio-screenshots
```
Review the output images in `/tmp/itchio-screenshots/`.

- [ ] **Step 3: Final commit (if any outstanding changes)**

```bash
git status
# If clean, nothing to do. Otherwise:
git add -A
git commit -m "chore: final cleanup for OWNED sort mode"
```
