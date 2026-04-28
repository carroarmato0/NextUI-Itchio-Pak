# Inventory Update Tracking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the inventory system to detect new upstream ROM files and removed games in the background, surface update state via `[UP]`/`[!]` badges in the game list, and let the user dismiss updates with a single button press.

**Architecture:** A new `UpdateService` in `internal/inventory/updater.go` runs `runCheck()` on startup and on manual trigger; it repairs missing cover art, detects 404'd games, and diffs upstream file lists against `KnownUpstreamFiles` in each `Entry`. Sort logic in `ApplySort` floats `[UP]`/`[!]` games to the top of the `[DL]` filter. UI changes are confined to `screen_list.go` (badges, pill overlay, X-dismiss) and `screen_settings.go` (new "Update Inventory" entry).

**Tech Stack:** Go 1.22+, SDL2 via go-sdl2, standard `net/http/httptest` for tests, `sync/atomic` for `IsRunning` state.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Modify | `internal/inventory/inventory.go` | `UpstreamFile` type; 6 new `Entry` fields; `HasPendingUpdates`, `IsRemoved`, `DismissUpdate`, `DismissRemoval` methods |
| Modify | `internal/inventory/inventory_test.go` | Tests for the four new methods |
| Create | `internal/inventory/updater.go` | `UpdateService` struct, lifecycle, `runCheck()` |
| Create | `internal/inventory/updater_test.go` | Tests for cover art repair, 404 detection, file diff |
| Modify | `internal/itchio/sort.go` | Extend `ApplySort` signature; add `[UP]`/`[!]` grouping inside `SortModeDL` |
| Modify | `internal/itchio/sort_test.go` | Tests for new DL grouping; update signature in existing tests |
| Modify | `internal/ui/screen_list.go` | `updateSvc` field; `[UP]`/`[!]` row badges; cover art pill overlay; section separators; X-dismiss; footer hints; updated `rebuildView()`; updated `NewListScreen` signature |
| Modify | `internal/ui/screen_settings.go` | `updateSvc` field; `sItemUpdateInventory`; timestamp; `checking…` state; updated `NewSettingsScreen` signature |
| Modify | `internal/ui/screen_download.go` | Pass `IsFree: game.IsFree` in `inv.Add()` call |
| Modify | `cmd/itchio-pak/main_sdl.go` | Construct `UpdateService`, wire `Start()`/`Stop()`, pass to `NewListScreen` |

---

## Task 1: Data model — `UpstreamFile`, extended `Entry`, new methods

**Files:**
- Modify: `internal/inventory/inventory.go`
- Modify: `internal/inventory/inventory_test.go`
- Modify: `internal/ui/screen_download.go`

- [ ] **Step 1.1 — Write failing tests for new inventory methods**

Add to `internal/inventory/inventory_test.go` (after the existing `TestCoverArtPath_*` tests):

```go
// ── HasPendingUpdates ────────────────────────────────────────────────────────

func TestHasPendingUpdates_NoEntry(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false for missing entry")
	}
}

func TestHasPendingUpdates_NoUpstreamFiles(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false when no upstream files recorded")
	}
}

func TestHasPendingUpdates_AllDownloaded(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.SetUpstreamFiles("https://dev.itch.io/game",
		[]inventory.UpstreamFile{{Filename: "g.gb", UploadID: "1", SeenAt: time.Now()}})
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false when all upstream files are downloaded")
	}
}

func TestHasPendingUpdates_NewFile(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g.gb", UploadID: "1", SeenAt: time.Now()},
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: time.Now()},
	})
	if !inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want true when upstream has file not in downloaded set")
	}
}

func TestHasPendingUpdates_DismissedFile(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	seenAt := time.Now().Add(-time.Hour) // file was seen an hour ago
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g.gb", UploadID: "1", SeenAt: seenAt},
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: seenAt},
	})
	inv.DismissUpdate("https://dev.itch.io/game") // dismisses all files seen before now
	if inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want false after dismiss")
	}
}

func TestHasPendingUpdates_NewFileAfterDismiss(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	oldSeenAt := time.Now().Add(-time.Hour)
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: oldSeenAt},
	})
	inv.DismissUpdate("https://dev.itch.io/game")

	// New file appears after dismiss
	newSeenAt := time.Now().Add(time.Hour)
	inv.SetUpstreamFiles("https://dev.itch.io/game", []inventory.UpstreamFile{
		{Filename: "g-v2.gb", UploadID: "2", SeenAt: oldSeenAt},
		{Filename: "g-v3.gb", UploadID: "3", SeenAt: newSeenAt},
	})
	if !inv.HasPendingUpdates("https://dev.itch.io/game") {
		t.Error("HasPendingUpdates: want true when new file appears after dismiss")
	}
}

// ── IsRemoved ────────────────────────────────────────────────────────────────

func TestIsRemoved_NoEntry(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false for missing entry")
	}
}

func TestIsRemoved_NotRemoved(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false when GameRemovedAt is zero")
	}
}

func TestIsRemoved_Removed(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	if !inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want true after MarkRemoved")
	}
}

func TestIsRemoved_DismissedRemoval(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	inv.DismissRemoval("https://dev.itch.io/game")
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false after DismissRemoval")
	}
}

func TestIsRemoved_MarkRemovedIdempotent(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	inv.DismissRemoval("https://dev.itch.io/game")
	// Calling MarkRemoved again when already set must NOT overwrite (idempotent).
	inv.MarkRemoved("https://dev.itch.io/game")
	// Since GameRemovedAt was already set (before DismissRemoval), it stays unchanged —
	// RemovalDismissedAt is still after it, so IsRemoved stays false.
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: MarkRemoved must be idempotent; badge must stay suppressed after dismiss")
	}
}

func TestMarkRemovedThenReachable_ClearsBoth(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game", inventory.Entry{Title: "G"},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	inv.MarkRemoved("https://dev.itch.io/game")
	inv.MarkReachable("https://dev.itch.io/game")
	if inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want false after MarkReachable")
	}
	// And MarkRemoved again should trigger the badge fresh.
	inv.MarkRemoved("https://dev.itch.io/game")
	if !inv.IsRemoved("https://dev.itch.io/game") {
		t.Error("IsRemoved: want true after fresh MarkRemoved post-MarkReachable")
	}
}

// ── IsFree persistence ───────────────────────────────────────────────────────

func TestAdd_PreservesIsFree(t *testing.T) {
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game",
		inventory.Entry{Title: "G", IsFree: true},
		inventory.DownloadedFile{Filename: "g.gb", DestPath: "/g.gb"})
	e, ok := inv.Lookup("https://dev.itch.io/game")
	if !ok {
		t.Fatal("entry not found")
	}
	if !e.IsFree {
		t.Error("IsFree should be true after Add with IsFree:true")
	}
}
```

- [ ] **Step 1.2 — Run to confirm failures**

```bash
go test -tags headless ./internal/inventory/ -run "TestHasPendingUpdates|TestIsRemoved|TestMarkRemoved|TestAdd_PreservesIsFree" -v 2>&1 | head -40
```

Expected: compilation errors or test failures (methods not defined yet).

- [ ] **Step 1.3 — Add `UpstreamFile` type and new `Entry` fields to `inventory.go`**

Add after the `DownloadedFile` struct definition:

```go
type UpstreamFile struct {
	Filename string    `json:"filename"`
	UploadID string    `json:"upload_id"`
	SeenAt   time.Time `json:"seen_at"`
}
```

Extend the `Entry` struct (add after `VerifiedAt`):

```go
IsFree             bool           `json:"is_free,omitempty"`
KnownUpstreamFiles []UpstreamFile `json:"known_upstream_files,omitempty"`
UpdateCheckedAt    time.Time      `json:"update_checked_at,omitempty"`
UpdateDismissedAt  time.Time      `json:"update_dismissed_at,omitempty"`
GameRemovedAt      time.Time      `json:"game_removed_at,omitempty"`
RemovalDismissedAt time.Time      `json:"removal_dismissed_at,omitempty"`
```

- [ ] **Step 1.4 — Update `Add()` to persist `IsFree`**

In `Add()`, change the `entry := &Entry{...}` initialisation to:

```go
entry := &Entry{
    GameURL:  gameURL,
    Title:    e.Title,
    Author:   e.Author,
    CoverURL: e.CoverURL,
    IsFree:   e.IsFree,
}
```

- [ ] **Step 1.5 — Add new methods to `inventory.go`**

Add after `VerifyAndClean`:

```go
// HasPendingUpdates returns true when any UpstreamFile for gameURL has a
// filename not in the downloaded set and was seen after UpdateDismissedAt.
func (inv *Inventory) HasPendingUpdates(gameURL string) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	downloaded := make(map[string]bool, len(e.Files))
	for _, f := range e.Files {
		downloaded[f.Filename] = true
	}
	for _, u := range e.KnownUpstreamFiles {
		if !downloaded[u.Filename] && u.SeenAt.After(e.UpdateDismissedAt) {
			return true
		}
	}
	return false
}

// IsRemoved returns true when the game was detected as 404 upstream and the
// user has not yet dismissed the warning (or the warning reappeared after a
// subsequent removal).
func (inv *Inventory) IsRemoved(gameURL string) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return false
	}
	return !e.GameRemovedAt.IsZero() &&
		(e.RemovalDismissedAt.IsZero() || e.GameRemovedAt.After(e.RemovalDismissedAt))
}

// DismissUpdate sets UpdateDismissedAt to now, suppressing [UP] for all
// upstream files seen before this moment.
func (inv *Inventory) DismissUpdate(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.UpdateDismissedAt = time.Now()
}

// DismissRemoval sets RemovalDismissedAt to now, suppressing [!] until the
// game is re-detected as removed after reappearing upstream.
func (inv *Inventory) DismissRemoval(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.RemovalDismissedAt = time.Now()
}

// MarkRemoved sets GameRemovedAt to now only on the first detection
// (idempotent: does nothing if GameRemovedAt is already set).
func (inv *Inventory) MarkRemoved(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok || !e.GameRemovedAt.IsZero() {
		return
	}
	e.GameRemovedAt = time.Now()
}

// MarkReachable clears GameRemovedAt and RemovalDismissedAt, returning the
// entry to a clean slate when a previously-removed game becomes reachable again.
func (inv *Inventory) MarkReachable(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.GameRemovedAt = time.Time{}
	e.RemovalDismissedAt = time.Time{}
}

// SetUpstreamFiles replaces KnownUpstreamFiles for gameURL and sets
// UpdateCheckedAt to now. Call this after each successful file-list scrape.
func (inv *Inventory) SetUpstreamFiles(gameURL string, files []UpstreamFile) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	e, ok := inv.Entries[gameURL]
	if !ok {
		return
	}
	e.KnownUpstreamFiles = files
	e.UpdateCheckedAt = time.Now()
}
```

- [ ] **Step 1.6 — Run tests and confirm they pass**

```bash
go test -tags headless ./internal/inventory/ -run "TestHasPendingUpdates|TestIsRemoved|TestMarkRemoved|TestMarkReachable|TestAdd_PreservesIsFree" -v
```

Expected: all new tests PASS.

- [ ] **Step 1.7 — Update `screen_download.go` to pass `IsFree`**

In `internal/ui/screen_download.go`, change the `inv.Add(...)` call to:

```go
s.inv.Add(game.URL, inventory.Entry{
    GameURL:  game.URL,
    Title:    game.Title,
    Author:   game.Author,
    CoverURL: game.CoverURL,
    IsFree:   game.IsFree,
}, inventory.DownloadedFile{
    Filename:     upload.Filename,
    DestPath:     dest,
    DownloadedAt: time.Now(),
})
```

- [ ] **Step 1.8 — Run all inventory tests**

```bash
go test -tags headless ./internal/inventory/ -v
```

Expected: all tests PASS.

- [ ] **Step 1.9 — Commit**

```bash
git add internal/inventory/inventory.go internal/inventory/inventory_test.go internal/ui/screen_download.go
git commit -m "feat(inventory): add UpstreamFile type, update fields, and update-tracking methods"
```

---

## Task 2: Sort logic — `ApplySort` signature + `[DL]` grouping

**Files:**
- Modify: `internal/itchio/sort.go`
- Modify: `internal/itchio/sort_test.go`
- Modify: `internal/ui/screen_list.go` (call site only — `rebuildView`)

- [ ] **Step 2.1 — Write failing tests for new DL grouping**

Add to `internal/itchio/sort_test.go` (before the helpers section):

```go
func TestApplySort_DL_PendingUpdatesFirst(t *testing.T) {
	games := testGames() // Banana(paid,DL), Apple(free,DL+UP), Cherry(free,DL)
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
		"https://b.itch.io/apple":  true,
		"https://c.itch.io/cherry": true,
	}
	pendingUpdates := map[string]bool{
		"https://b.itch.io/apple": true,
	}
	result := itchio.ApplySort(games, itchio.SortModeDL, downloaded, pendingUpdates, nil)
	if len(result) != 3 {
		t.Fatalf("DL grouping: want 3 games, got %d", len(result))
	}
	if result[0].Title != "Apple" {
		t.Errorf("DL grouping: [UP] game should be first, got %q", result[0].Title)
	}
}

func TestApplySort_DL_RemovedSecond(t *testing.T) {
	games := testGames()
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
		"https://b.itch.io/apple":  true,
		"https://c.itch.io/cherry": true,
	}
	removed := map[string]bool{
		"https://c.itch.io/cherry": true,
	}
	result := itchio.ApplySort(games, itchio.SortModeDL, downloaded, nil, removed)
	if len(result) != 3 {
		t.Fatalf("want 3 games, got %d", len(result))
	}
	if result[0].Title != "Cherry" {
		t.Errorf("DL grouping: [!] game should be first when no [UP] games, got %q", result[0].Title)
	}
}

func TestApplySort_DL_UpdateBeforeRemoved(t *testing.T) {
	games := testGames()
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
		"https://b.itch.io/apple":  true,
		"https://c.itch.io/cherry": true,
	}
	pendingUpdates := map[string]bool{"https://b.itch.io/apple": true}
	removed := map[string]bool{"https://c.itch.io/cherry": true}
	result := itchio.ApplySort(games, itchio.SortModeDL, downloaded, pendingUpdates, removed)
	if result[0].Title != "Apple" {
		t.Errorf("want [UP] Apple first, got %q", result[0].Title)
	}
	if result[1].Title != "Cherry" {
		t.Errorf("want [!] Cherry second, got %q", result[1].Title)
	}
	if result[2].Title != "Banana" {
		t.Errorf("want [DL] Banana third, got %q", result[2].Title)
	}
}
```

- [ ] **Step 2.2 — Run to confirm failures**

```bash
go test -tags headless ./internal/itchio/ -run "TestApplySort_DL_" -v 2>&1 | head -20
```

Expected: compile error (wrong number of arguments to `ApplySort`).

- [ ] **Step 2.3 — Extend `ApplySort` signature and update `SortModeDL` case**

In `internal/itchio/sort.go`, change the function signature:

```go
func ApplySort(games []Game, mode SortMode, downloaded, pendingUpdates, removed map[string]bool) []Game {
```

Replace the `SortModeDL` case:

```go
case SortModeDL:
    // Collect downloaded games then sort into three groups:
    // 1 — pending updates, 2 — removed from store, 3 — up-to-date
    var g1, g2, g3 []Game
    for _, g := range games {
        if !downloaded[g.URL] {
            continue
        }
        switch {
        case pendingUpdates[g.URL]:
            g1 = append(g1, g)
        case removed[g.URL]:
            g2 = append(g2, g)
        default:
            g3 = append(g3, g)
        }
    }
    out := make([]Game, 0, len(g1)+len(g2)+len(g3))
    out = append(out, g1...)
    out = append(out, g2...)
    out = append(out, g3...)
    return out
```

- [ ] **Step 2.4 — Fix existing tests for new signature**

In `internal/itchio/sort_test.go`, update every existing call to `itchio.ApplySort` that uses the old 3-argument form. Change all occurrences of:

```go
itchio.ApplySort(games, mode, downloaded)
itchio.ApplySort(testGames(), itchio.SortModeDL, downloaded)
itchio.ApplySort(testGames(), itchio.SortModeDL, map[string]bool{})
itchio.ApplySort(testGames(), itchio.SortModeDL, nil)
itchio.ApplySort(games, mode, nil)
```

to the 5-argument form, appending `, nil, nil`:

```go
itchio.ApplySort(games, mode, downloaded, nil, nil)
itchio.ApplySort(testGames(), itchio.SortModeDL, downloaded, nil, nil)
itchio.ApplySort(testGames(), itchio.SortModeDL, map[string]bool{}, nil, nil)
itchio.ApplySort(testGames(), itchio.SortModeDL, nil, nil, nil)
itchio.ApplySort(games, mode, nil, nil, nil)
```

Also update `TestApplySort_ReturnsNewSlice` which iterates `SortModes`:

```go
func TestApplySort_ReturnsNewSlice(t *testing.T) {
	games := testGames()
	for _, mode := range itchio.SortModes {
		result := itchio.ApplySort(games, mode, nil, nil, nil)
		if result == nil {
			t.Errorf("mode %q: ApplySort returned nil", mode)
		}
	}
}
```

- [ ] **Step 2.5 — Run sort tests**

```bash
go test -tags headless ./internal/itchio/ -v
```

Expected: all tests PASS.

- [ ] **Step 2.6 — Fix `rebuildView()` call site in `screen_list.go`**

In `internal/ui/screen_list.go`, find `rebuildView()` and update it to build and pass the two new maps. Replace the entire `rebuildView` function body:

```go
func (s *ListScreen) rebuildView() {
	downloaded := make(map[string]bool)
	pendingUpdates := make(map[string]bool)
	removed := make(map[string]bool)
	for _, g := range s.cachedGames {
		if s.inv.IsPresent(g.URL) {
			downloaded[g.URL] = true
		}
		if s.inv.HasPendingUpdates(g.URL) {
			pendingUpdates[g.URL] = true
		}
		if s.inv.IsRemoved(g.URL) {
			removed[g.URL] = true
		}
	}
	s.viewGames = itchio.ApplySort(s.cachedGames, s.sortMode, downloaded, pendingUpdates, removed)
	s.totalGames = len(s.viewGames)
	s.totalPages = (s.totalGames + itchio.PerPage - 1) / itchio.PerPage
	s.page = 1
	s.loadPage(1, "")
	logger.Debug("sort: view rebuilt — %d games visible (mode=%s)", len(s.viewGames), itchio.SortModeBadge(s.sortMode))
}
```

- [ ] **Step 2.7 — Build check**

```bash
go build -tags headless ./...
```

Expected: no errors.

- [ ] **Step 2.8 — Commit**

```bash
git add internal/itchio/sort.go internal/itchio/sort_test.go internal/ui/screen_list.go
git commit -m "feat(sort): extend ApplySort for [UP]/[!] grouping in [DL] mode"
```

---

## Task 3: `UpdateService` — struct, lifecycle, cover art repair

**Files:**
- Create: `internal/inventory/updater.go`
- Create: `internal/inventory/updater_test.go`

- [ ] **Step 3.1 — Write failing test for cover art repair**

Create `internal/inventory/updater_test.go`:

```go
package inventory_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// minimalPNG returns the bytes of a 1x1 white PNG.
func minimalPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestUpdateService_RepairsMissingCoverArt(t *testing.T) {
	pngData := minimalPNG()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cover.png" {
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngData)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	if err := os.WriteFile(romPath, []byte("ROM"), 0644); err != nil {
		t.Fatal(err)
	}

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	client := itchio.NewClientWithBase(srv.URL)
	svc := inventory.NewUpdateService(inv, invPath, client, nil)

	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	if _, err := os.Stat(artPath); err != nil {
		t.Errorf("cover art not created at %s: %v", artPath, err)
	}
}

func TestUpdateService_SkipsCoverArtIfPresent(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cover.png" {
			callCount++
			w.Header().Set("Content-Type", "image/png")
			w.Write(minimalPNG())
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)

	// Pre-create the cover art so it already exists
	mediaDir := filepath.Join(dir, ".media")
	os.MkdirAll(mediaDir, 0755)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add("https://dev.itch.io/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	// Also set upstream files so FetchUploads doesn't go out (no game page endpoint)
	// — for this test we care only about cover art, so use a paid game to skip FetchUploads.
	inv2 := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv2.Add("https://dev.itch.io/paid",
		inventory.Entry{Title: "Paid", IsFree: false, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	invPath2 := filepath.Join(dir, "inventory2.json")
	inv2.Save(invPath2)

	client := itchio.NewClientWithBase(srv.URL)
	svc := inventory.NewUpdateService(inv2, invPath2, client, nil)
	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	if callCount != 0 {
		t.Errorf("cover art HTTP GET called %d times, want 0 (art already present)", callCount)
	}
}
```

- [ ] **Step 3.2 — Run to confirm failures**

```bash
go test -tags headless ./internal/inventory/ -run "TestUpdateService" -v 2>&1 | head -20
```

Expected: compile error — `inventory.NewUpdateService` does not exist.

- [ ] **Step 3.3 — Create `internal/inventory/updater.go`**

```go
package inventory

import (
	"os"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// UpdateService checks each inventory entry for missing cover art, removed
// games, and new upstream files. It runs once at startup and re-runs each time
// TriggerNow is called.
type UpdateService struct {
	inv           *Inventory
	inventoryPath string
	client        *itchio.Client
	triggerCh     chan struct{} // buffered(1): absorbs duplicate triggers
	stopCh        chan struct{}
	running       atomic.Bool
}

// NewUpdateService constructs an UpdateService. notify (may be nil) is called
// after each runCheck completes; use it to push an SDL UserEvent from the
// caller without importing SDL here.
func NewUpdateService(inv *Inventory, inventoryPath string, client *itchio.Client, notify func()) *UpdateService {
	return &UpdateService{
		inv:           inv,
		inventoryPath: inventoryPath,
		client:        client,
		triggerCh:     make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
	}
}

// Start launches the background goroutine and runs the first check immediately.
// onDone is called after the first check completes (for tests; may be nil).
func (s *UpdateService) Start(onDone func()) {
	go func() {
		s.running.Store(true)
		s.runCheck()
		s.running.Store(false)
		if onDone != nil {
			onDone()
		}
		for {
			select {
			case <-s.triggerCh:
				s.running.Store(true)
				s.runCheck()
				s.running.Store(false)
				if onDone != nil {
					onDone()
				}
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop signals the goroutine to exit. Idempotent.
func (s *UpdateService) Stop() {
	select {
	case s.stopCh <- struct{}{}:
	default:
	}
}

// TriggerNow queues a re-check. Non-blocking; a pending check absorbs the signal.
func (s *UpdateService) TriggerNow() {
	select {
	case s.triggerCh <- struct{}{}:
		logger.Info("update-svc: manual check triggered")
	default:
		logger.Debug("update-svc: trigger ignored (check already queued)")
	}
}

// IsRunning reports whether runCheck is currently executing.
func (s *UpdateService) IsRunning() bool {
	return s.running.Load()
}

func (s *UpdateService) runCheck() {
	s.inv.mu.Lock()
	urls := make([]string, 0, len(s.inv.Entries))
	for url := range s.inv.Entries {
		urls = append(urls, url)
	}
	s.inv.mu.Unlock()

	logger.Info("update-svc: checking %d inventory entries", len(urls))

	for _, gameURL := range urls {
		s.checkEntry(gameURL)
		if err := s.inv.Save(s.inventoryPath); err != nil {
			logger.Error("update-svc: save after entry %s: %v", gameURL, err)
		}
	}

	logger.Info("update-svc: check complete")
}

func (s *UpdateService) checkEntry(gameURL string) {
	s.inv.mu.Lock()
	entry, ok := s.inv.Entries[gameURL]
	if !ok {
		s.inv.mu.Unlock()
		return
	}
	// Snapshot what we need without holding the lock during I/O.
	coverURL := entry.CoverURL
	isFree := entry.IsFree
	files := append([]DownloadedFile(nil), entry.Files...)
	s.inv.mu.Unlock()

	// 1. Cover art repair.
	for _, f := range files {
		artPath := CoverArtPath(coverURL, f.DestPath)
		if artPath == "" {
			continue
		}
		if _, err := os.Stat(artPath); err == nil {
			logger.Debug("update-svc: cover art present for %s", f.Filename)
			continue
		}
		logger.Info("update-svc: repairing cover art for %s", f.Filename)
		if err := s.client.DownloadCoverArt(coverURL, f.DestPath); err != nil {
			logger.Error("update-svc: cover art repair failed for %s: %v", f.Filename, err)
		}
	}

	// 2 & 3. Upstream file check (handled in Task 4).
	_ = isFree
}
```

Note: the `notify` parameter is stored in Task 4 when the full `runCheck` is wired. For now `Start` accepts it but the struct doesn't store it yet — add a `notify func()` field to the struct:

```go
type UpdateService struct {
	inv           *Inventory
	inventoryPath string
	client        *itchio.Client
	notify        func()
	triggerCh     chan struct{}
	stopCh        chan struct{}
	running       atomic.Bool
}
```

And store it in `NewUpdateService`:

```go
func NewUpdateService(inv *Inventory, inventoryPath string, client *itchio.Client, notify func()) *UpdateService {
	return &UpdateService{
		inv:           inv,
		inventoryPath: inventoryPath,
		client:        client,
		notify:        notify,
		triggerCh:     make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
	}
}
```

Call `notify` at the end of the goroutine (replace `onDone` calls):

```go
go func() {
    s.running.Store(true)
    s.runCheck()
    s.running.Store(false)
    if onDone != nil {
        onDone()
    }
    if s.notify != nil {
        s.notify()
    }
    for {
        select {
        case <-s.triggerCh:
            s.running.Store(true)
            s.runCheck()
            s.running.Store(false)
            if onDone != nil {
                onDone()
            }
            if s.notify != nil {
                s.notify()
            }
        case <-s.stopCh:
            return
        }
    }
}()
```

- [ ] **Step 3.4 — Run cover art repair tests**

```bash
go test -tags headless ./internal/inventory/ -run "TestUpdateService_Repairs|TestUpdateService_Skips" -v
```

Expected: both tests PASS.

- [ ] **Step 3.5 — Commit**

```bash
git add internal/inventory/updater.go internal/inventory/updater_test.go
git commit -m "feat(inventory): add UpdateService with cover art repair"
```

---

## Task 4: `UpdateService` — 404 detection and file diff

**Files:**
- Modify: `internal/inventory/updater.go`
- Modify: `internal/inventory/updater_test.go`

- [ ] **Step 4.1 — Write failing tests**

Add to `internal/inventory/updater_test.go`:

```go
// freeGameServer builds an httptest.Server that mimics a free itch.io game page
// with the given upload filenames. Pass status 404 to simulate a removed game.
func freeGameServer(t *testing.T, status int, filenames []string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()

	if status != http.StatusOK {
		mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", status)
		})
		srv = httptest.NewServer(mux)
		return srv
	}

	// GET /game — game page with CSRF token
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			http.Error(w, "use /game/download_url", http.StatusBadRequest)
			return
		}
		w.Write([]byte(`<html><head><meta name="csrf_token" value="TESTCSRF"/></head></html>`))
	})

	// POST /game/download_url — returns signed URL
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		import_json_encoder := struct{ URL string }{}
		import_json_encoder.URL = srv.URL + "/dl/page"
		w.Header().Set("Content-Type", "application/json")
		import_enc, _ := json.Marshal(map[string]string{"url": srv.URL + "/dl/page"})
		w.Write(import_enc)
	})

	// GET /dl/page — signed download page with upload list
	mux.HandleFunc("/dl/page", func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		body.WriteString(`<html><head><meta name="csrf_token" value="DLCSRF"/></head><body>`)
		for i, fn := range filenames {
			body.WriteString(`<div class="upload"><div class="info_column"><div class="upload_name">`)
			body.WriteString(`<strong class="name" title="` + fn + `">` + fn + `</strong></div></div>`)
			body.WriteString(`<div class="actions"><a class="button download_btn" href="javascript:void(0);" data-upload_id="`)
			body.WriteString(fmt.Sprintf("%d", 100+i))
			body.WriteString(`">Download</a></div></div>`)
		}
		body.WriteString(`</body></html>`)
		w.Write(body.Bytes())
	})

	srv = httptest.NewServer(mux)
	return srv
}

func TestUpdateService_Marks404AsRemoved(t *testing.T) {
	srv := freeGameServer(t, http.StatusNotFound, nil)
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	// Pre-create cover art so repair is skipped.
	mediaDir := filepath.Join(dir, ".media")
	os.MkdirAll(mediaDir, 0755)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add(srv.URL+"/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	done := make(chan struct{})
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	if !inv.IsRemoved(srv.URL + "/game") {
		t.Error("IsRemoved: want true after 404 from upstream")
	}
}

func TestUpdateService_DiffAddsNewFile(t *testing.T) {
	srv := freeGameServer(t, http.StatusOK, []string{"game.gb", "game-v2.gb"})
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	mediaDir := filepath.Join(dir, ".media")
	os.MkdirAll(mediaDir, 0755)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add(srv.URL+"/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	done := make(chan struct{})
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	if !inv.HasPendingUpdates(srv.URL + "/game") {
		t.Error("HasPendingUpdates: want true after new file detected upstream")
	}
}

func TestUpdateService_DiffPrunesVanishedFile(t *testing.T) {
	// Game page now only has game.gb — game-v2.gb has vanished.
	srv := freeGameServer(t, http.StatusOK, []string{"game.gb"})
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	mediaDir := filepath.Join(dir, ".media")
	os.MkdirAll(mediaDir, 0755)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add(srv.URL+"/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	// Seed a previously-known file that has now vanished.
	inv.SetUpstreamFiles(srv.URL+"/game", []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "100", SeenAt: time.Now().Add(-time.Hour)},
		{Filename: "game-v2.gb", UploadID: "101", SeenAt: time.Now().Add(-time.Hour)},
	})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	done := make(chan struct{})
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	e, _ := inv.Lookup(srv.URL + "/game")
	for _, u := range e.KnownUpstreamFiles {
		if u.Filename == "game-v2.gb" {
			t.Error("vanished file game-v2.gb should have been pruned from KnownUpstreamFiles")
		}
	}
}
```

Also add the missing imports to the test file (`"bytes"`, `"encoding/json"`, `"fmt"` are needed for `freeGameServer`). Add them to the import block.

- [ ] **Step 4.2 — Run to confirm failures**

```bash
go test -tags headless ./internal/inventory/ -run "TestUpdateService_Marks|TestUpdateService_Diff" -v 2>&1 | head -30
```

Expected: compile errors or test failures.

- [ ] **Step 4.3 — Complete `checkEntry()` in `updater.go`**

Add a helper to `updater.go`:

```go
import "strings"

// isGameRemoved reports whether err indicates a 404 or 410 HTTP response.
func isGameRemoved(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 404") || strings.Contains(s, "HTTP 410")
}
```

Replace the `// 2 & 3.` placeholder at the end of `checkEntry()` with:

```go
	// 2. Upstream check.
	if isFree {
		s.checkFreeGame(gameURL)
	} else {
		s.checkPaidGame(gameURL)
	}
```

Add `checkFreeGame` and `checkPaidGame` methods:

```go
func (s *UpdateService) checkFreeGame(gameURL string) {
	uploads, err := s.client.FetchUploads(gameURL)
	if err != nil {
		if isGameRemoved(err) {
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: game removed (404) %s", gameURL)
		} else {
			logger.Warn("update-svc: transient error for %s: %v", gameURL, err)
		}
		return
	}
	// Game is reachable — clear any stale removal state.
	s.inv.MarkReachable(gameURL)

	// Build new upstream file list from scraped uploads.
	newFiles := make([]UpstreamFile, 0, len(uploads))
	for _, u := range uploads {
		newFiles = append(newFiles, UpstreamFile{
			Filename: u.Filename,
			UploadID: u.UploadID,
			SeenAt:   time.Now(),
		})
	}
	s.inv.SetUpstreamFiles(gameURL, newFiles)
	logger.Debug("update-svc: %s — %d upstream file(s) recorded", gameURL, len(newFiles))
}

func (s *UpdateService) checkPaidGame(gameURL string) {
	_, err := s.client.FetchGameDetail(gameURL)
	if err != nil {
		if isGameRemoved(err) {
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: paid game removed (404) %s", gameURL)
		} else {
			logger.Warn("update-svc: transient error for paid game %s: %v", gameURL, err)
		}
		return
	}
	s.inv.MarkReachable(gameURL)

	// Update UpdateCheckedAt without touching KnownUpstreamFiles.
	s.inv.mu.Lock()
	if e, ok := s.inv.Entries[gameURL]; ok {
		e.UpdateCheckedAt = time.Now()
	}
	s.inv.mu.Unlock()
	logger.Debug("update-svc: paid game %s reachable, no file diff", gameURL)
}
```

Add `"time"` to imports in `updater.go` if not already present.

- [ ] **Step 4.4 — Run updater tests**

```bash
go test -tags headless ./internal/inventory/ -run "TestUpdateService" -v
```

Expected: all tests PASS.

- [ ] **Step 4.5 — Run full inventory suite**

```bash
go test -tags headless ./internal/inventory/ -v
```

Expected: all tests PASS.

- [ ] **Step 4.6 — Commit**

```bash
git add internal/inventory/updater.go internal/inventory/updater_test.go
git commit -m "feat(inventory): implement runCheck with 404 detection and file diff"
```

---

## Task 5: List screen — row badges + cover art pill + section separators

**Files:**
- Modify: `internal/ui/screen_list.go`

There are no unit tests for SDL2 rendering. Verify visually on device or via the headless build check.

- [ ] **Step 5.1 — Update row badge logic in `Draw()`**

In `screen_list.go`, find the block that determines the badge label (currently `isPresent → "[DL]"`, price, free). Replace it:

```go
// Determine badge for this row.
isPendingUpdate := s.inv.HasPendingUpdates(g.URL)
isRemovedGame := s.inv.IsRemoved(g.URL)
isPresent := s.inv.IsPresent(g.URL)

var badgeLabel string
var badgeR, badgeG, badgeB uint8
switch {
case isPendingUpdate:
    badgeLabel = "[UP]"
    badgeR, badgeG, badgeB = 240, 160, 40
case isRemovedGame:
    badgeLabel = "[!]"
    badgeR, badgeG, badgeB = 200, 60, 60
case isPresent:
    badgeLabel = "[DL]"
    badgeR, badgeG, badgeB = 80, 200, 220
case g.IsFree:
    badgeLabel = "Free"
    badgeR, badgeG, badgeB = 80, 200, 80
default:
    badgeLabel = fmt.Sprintf("$%.2f", g.Price)
    badgeR, badgeG, badgeB = 220, 180, 60
}
```

- [ ] **Step 5.2 — Add cover art pill badge overlay**

In `Draw()`, after `r.DrawTextureAt(tex, imgX, imgY, dw, dh)` and after the `else if s.cache.Failed(...)` block (i.e., after the cover art is drawn but still inside the `if s.cursor < len(s.games)` block), add the pill overlay:

```go
// Pill badge overlay — drawn after texture so it appears above animated GIFs.
g := s.games[s.cursor]
isPendingUpdate := s.inv.HasPendingUpdates(g.URL)
isRemovedGame := s.inv.IsRemoved(g.URL)
if isPendingUpdate || isRemovedGame {
    var pillLabel string
    var pillR, pillG, pillB uint8
    var shadowR, shadowG, shadowB uint8
    var textR, textG, textB uint8
    if isPendingUpdate {
        pillLabel = "UPDATE"
        pillR, pillG, pillB = 240, 160, 40
        shadowR, shadowG, shadowB = 160, 96, 16
        textR, textG, textB = 20, 20, 20
    } else {
        pillLabel = "REMOVED"
        pillR, pillG, pillB = 200, 60, 60
        shadowR, shadowG, shadowB = 122, 16, 16
        textR, textG, textB = 255, 255, 255
    }
    lw, lh := r.SmallTextSize(pillLabel)
    pad := int32(4)
    pillW := lw + pad*2
    pillH := lh + pad
    pillX := imgX + dw - pillW - 5
    pillY := imgY + 5
    // Shadow layer (1px offset down-right)
    r.DrawRect(pillX+1, pillY+1, pillW, pillH, shadowR, shadowG, shadowB)
    // Main pill
    r.DrawRect(pillX, pillY, pillW, pillH, pillR, pillG, pillB)
    // Label
    r.DrawSmallText(pillLabel, pillX+pad, pillY+pad/2, textR, textG, textB)
}
```

Note: `r.SmallTextSize` must return `(int32, int32)`. Check the `renderer.go` signature and adjust if the return types differ.

- [ ] **Step 5.3 — Add section separators in `[DL]` mode**

In `Draw()`, inside the game list row loop, just before drawing the first row's text, add separator rendering. Place this logic before the `for i, g := range s.games` loop — compute which rows need a separator:

```go
// In [DL] mode, compute where group transitions occur for separator lines.
var dlSepAfterUpdates int = -1 // row index after which to draw "— downloaded —" separator
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

Then inside the row rendering loop, after drawing a row at index `i`, check:

```go
if i == dlSepAfterUpdates {
    sepY := rowTop + rowH
    r.DrawRect(0, sepY, leftW, 1, 50, 50, 50)
    sepLabelY := sepY + 2
    r.DrawSmallText("— downloaded —", 10, sepLabelY, 80, 80, 80)
    // Rows below must account for the extra separator height.
    // This is achieved by adjusting contentTop before the loop or by
    // rendering separators as fixed-height items — simplest approach:
    // accept that the separator overlaps the next row slightly on narrow screens.
}
```

> **Note:** the separator is a cosmetic overlay; it does not consume a row slot. On very full pages it may slightly overlap the next row. A full slot-based separator can be implemented as a follow-up.

- [ ] **Step 5.4 — Build check**

```bash
go build -tags headless ./...
```

Expected: no errors.

- [ ] **Step 5.5 — Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): add [UP]/[!] row badges, cover art pill overlay, and DL section separators"
```

---

## Task 6: List screen — X button dismiss + contextual footer hints

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 6.1 — Add X button handler in `HandleEvent()`**

In `HandleEvent()`, add to the `*sdl.ControllerButtonEvent` switch (after the existing `CONTROLLER_BUTTON_BACK` case):

```go
case sdl.CONTROLLER_BUTTON_X:
    if s.cursor < len(s.games) {
        g := s.games[s.cursor]
        if s.inv.HasPendingUpdates(g.URL) {
            s.inv.DismissUpdate(g.URL)
            logger.Info("update-svc: update dismissed for game=%q", g.Title)
            if err := s.inv.Save(s.inventoryPath); err != nil {
                logger.Warn("inventory: save after dismiss: %v", err)
            }
            s.rebuildView()
        } else if s.inv.IsRemoved(g.URL) {
            s.inv.DismissRemoval(g.URL)
            logger.Info("update-svc: removal dismissed for game=%q", g.Title)
            if err := s.inv.Save(s.inventoryPath); err != nil {
                logger.Warn("inventory: save after dismiss: %v", err)
            }
            s.rebuildView()
        }
    }
    return s
```

Also add for the keyboard path (for desktop testing):

```go
case sdl.K_x:
    if s.cursor < len(s.games) {
        g := s.games[s.cursor]
        if s.inv.HasPendingUpdates(g.URL) {
            s.inv.DismissUpdate(g.URL)
            if err := s.inv.Save(s.inventoryPath); err != nil {
                logger.Warn("inventory: save after dismiss: %v", err)
            }
            s.rebuildView()
        } else if s.inv.IsRemoved(g.URL) {
            s.inv.DismissRemoval(g.URL)
            if err := s.inv.Save(s.inventoryPath); err != nil {
                logger.Warn("inventory: save after dismiss: %v", err)
            }
            s.rebuildView()
        }
    }
    return s
```

- [ ] **Step 6.2 — Add contextual footer hints in `Draw()`**

Replace the footer `hints` string construction. Currently it looks like:

```go
var hints string
if r.W <= narrowScreenW {
    if s.cacheReady {
        hints = "A:sel  L/R  SEL:sort  B:exit  ⚙"
    } else {
        hints = "A:sel  L/R  B:exit  ⚙"
    }
} else {
    if s.cacheReady {
        hints = "A:select  L/R:page  SELECT:sort  B:exit  Start:settings"
    } else {
        hints = "A:select  L/R:page  B:exit  Start:settings"
    }
}
```

Replace with:

```go
var dismissHint string
if s.cursor < len(s.games) {
    g := s.games[s.cursor]
    if s.inv.HasPendingUpdates(g.URL) {
        if r.W <= narrowScreenW {
            dismissHint = "  X:dismiss"
        } else {
            dismissHint = "  X:dismiss update"
        }
    } else if s.inv.IsRemoved(g.URL) {
        if r.W <= narrowScreenW {
            dismissHint = "  X:dismiss"
        } else {
            dismissHint = "  X:dismiss warning"
        }
    }
}

var hints string
if r.W <= narrowScreenW {
    if s.cacheReady {
        hints = "A:sel  L/R  SEL:sort  B:exit  ⚙" + dismissHint
    } else {
        hints = "A:sel  L/R  B:exit  ⚙" + dismissHint
    }
} else {
    if s.cacheReady {
        hints = "A:select  L/R:page  SELECT:sort  B:exit  Start:settings" + dismissHint
    } else {
        hints = "A:select  L/R:page  B:exit  Start:settings" + dismissHint
    }
}
```

- [ ] **Step 6.3 — Build check**

```bash
go build -tags headless ./...
```

Expected: no errors.

- [ ] **Step 6.4 — Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): add X-button dismiss for [UP]/[!] games with contextual footer hints"
```

---

## Task 7: Settings screen — "Update Inventory" entry

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 7.1 — Add `updateSvc` field and new item constant**

In `screen_settings.go`, add `updateSvc *inventory.UpdateService` to `SettingsScreen`:

```go
type SettingsScreen struct {
	client         *itchio.Client
	cfg            *settings.Config
	cfgPath        string
	cursor         settingsItem
	prev           Screen
	onRefreshGames func(Screen) Screen
	updateSvc      *inventory.UpdateService

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time
}
```

Add the new constant after `sItemRefreshCache`:

```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemROMLocation
	sItemLogLevel
	sItemClearCache
	sItemRefreshCache
	sItemUpdateInventory // NEW
	sItemContentModeration
	sItemAbout
	sItemCount
)
```

- [ ] **Step 7.2 — Update `NewSettingsScreen` signature**

```go
func NewSettingsScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, prev Screen, onRefreshGames func(Screen) Screen, updateSvc *inventory.UpdateService) *SettingsScreen {
	s := &SettingsScreen{
		client:         client,
		cfg:            cfg,
		cfgPath:        cfgPath,
		prev:           prev,
		onRefreshGames: onRefreshGames,
		updateSvc:      updateSvc,
	}
	// ... rest unchanged
```

Fix the two call sites that pass `NewSettingsScreen` — both are in `screen_list.go`:

```go
// K_s / CONTROLLER_BUTTON_START handler:
return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc)
```

- [ ] **Step 7.3 — Add "Update Inventory" row to `Draw()`**

In `Draw()`, replace the `items` slice:

```go
items := []string{
    "API Key: ",
    "ROM Selection: " + s.cfg.ROMSelection,
    "ROM Location: " + s.cfg.ROMLocation,
    "Log Level: " + logLevelLabel,
    "Clear Image Cache",
    "Refresh Game List",
    "Update Inventory",
    "Content Moderation >",
    "About",
}
```

After the main `for i, label := range items` loop adds the label, append a right-aligned annotation for the `sItemUpdateInventory` row. Add inside the loop body, after the `sItemAPIKey` block:

```go
if settingsItem(i) == sItemUpdateInventory && s.updateSvc != nil {
    annotation := updateInventoryAnnotation(s.updateSvc)
    aw, _ := r.SmallTextSize(annotation)
    ax := r.W - aw - 20
    var aR, aG, aB uint8
    if s.updateSvc.IsRunning() {
        aR, aG, aB = 240, 160, 40
    } else {
        aR, aG, aB = 100, 100, 100
    }
    r.DrawSmallText(annotation, ax, y+(fontH-0)/2, aR, aG, aB)
}
```

Add the helper function (outside the method, in the same file):

```go
// updateInventoryAnnotation returns a short right-aligned label for the
// "Update Inventory" settings row.
func updateInventoryAnnotation(svc *inventory.UpdateService) string {
	if svc.IsRunning() {
		return "checking…"
	}
	// Find the most recent UpdateCheckedAt across all entries.
	// Access via the public Entries field is not thread-safe; use a snapshot method.
	// For simplicity, use the service's LastCheckedAt if we add that, or
	// just show "last: unknown" until a dedicated method is added.
	// See Task 8 for wiring the timestamp.
	return "last: unknown"
}
```

> **Note:** The real timestamp logic requires a new `LatestCheckedAt() time.Time` method on `*Inventory`. Add it to `inventory.go`:

```go
// LatestCheckedAt returns the most recent UpdateCheckedAt across all entries,
// or the zero time if no checks have run.
func (inv *Inventory) LatestCheckedAt() time.Time {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	var latest time.Time
	for _, e := range inv.Entries {
		if e.UpdateCheckedAt.After(latest) {
			latest = e.UpdateCheckedAt
		}
	}
	return latest
}
```

Then update `updateInventoryAnnotation` to use `svc`'s inventory. Since `UpdateService` doesn't expose the inventory directly, add a `LatestCheckedAt() time.Time` method to `UpdateService`:

```go
// LatestCheckedAt delegates to the inventory's LatestCheckedAt.
func (s *UpdateService) LatestCheckedAt() time.Time {
	return s.inv.LatestCheckedAt()
}
```

Now update `updateInventoryAnnotation`:

```go
func updateInventoryAnnotation(svc *inventory.UpdateService) string {
	if svc.IsRunning() {
		return "checking…"
	}
	t := svc.LatestCheckedAt()
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "last: just now"
	case d < time.Hour:
		return fmt.Sprintf("last: %dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("last: %dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("last: %dd ago", int(d.Hours()/24))
	}
}
```

Add `"fmt"` and `"time"` to imports if not already present.

- [ ] **Step 7.4 — Add activate handler for `sItemUpdateInventory`**

In `activate()`:

```go
case sItemUpdateInventory:
    if s.updateSvc != nil {
        s.updateSvc.TriggerNow()
        logger.Info("settings: Update Inventory triggered manually")
    }
```

- [ ] **Step 7.5 — Build check**

```bash
go build -tags headless ./...
```

Expected: no errors.

- [ ] **Step 7.6 — Commit**

```bash
git add internal/ui/screen_settings.go internal/inventory/inventory.go internal/inventory/updater.go
git commit -m "feat(ui): add Update Inventory settings entry with timestamp and checking state"
```

---

## Task 8: Integration wiring

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`
- Modify: `internal/ui/screen_list.go` (add `updateSvc` field + `NewListScreen` signature)

- [ ] **Step 8.1 — Add `updateSvc` field to `ListScreen`**

In `screen_list.go`, add to the `ListScreen` struct:

```go
updateSvc     *inventory.UpdateService
```

- [ ] **Step 8.2 — Update `NewListScreen` signature**

```go
func NewListScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, cachePath string, inv *inventory.Inventory, inventoryPath string, updateSvc *inventory.UpdateService) *ListScreen {
	s := &ListScreen{
		client:        client,
		cfg:           cfg,
		cache:         cache,
		page:          1,
		cfgPath:       cfgPath,
		cachePath:     cachePath,
		inv:           inv,
		inventoryPath: inventoryPath,
		updateSvc:     updateSvc,
	}
	// ... rest unchanged
```

- [ ] **Step 8.3 — Wire `TriggerNow()` into cache refresh callback**

In `newCacheRefreshScreen()`, add the trigger after `rebuildView()`:

```go
return NewCacheRefreshScreen(s.client, s.cachePath, prev, func(games []itchio.Game) {
    s.cachedGames = games
    s.cacheReady = true
    s.rebuildView()
    if s.updateSvc != nil {
        s.updateSvc.TriggerNow()
    }
})
```

Also add it to the end of `buildCache()` (background auto-refresh path):

```go
// At the end of buildCache(), after rebuildView():
if s.updateSvc != nil {
    s.updateSvc.TriggerNow()
}
```

- [ ] **Step 8.4 — Update `NewSettingsScreen` calls in `HandleEvent()`**

Both `K_s` / `CONTROLLER_BUTTON_START` paths become:

```go
return NewSettingsScreen(s.client, s.cfg, s.cfgPath, s, s.newCacheRefreshScreen, s.updateSvc)
```

- [ ] **Step 8.5 — Wire everything in `main_sdl.go`**

```go
func runSDL() {
	cfgPath := os.Getenv("HOME") + "/config.json"
	cachePath := filepath.Join(filepath.Dir(cfgPath), "games_cache.json")
	cfg, _ := settings.Load(cfgPath)

	logger.SetLevel(logger.LevelFromString(cfg.LogLevel))
	logger.RegisterSecret(cfg.APIKey, "[API-KEY]")

	inventoryPath := filepath.Join(filepath.Dir(cfgPath), "inventory.json")
	inv, _ := inventory.Load(inventoryPath)
	inv.VerifyAndClean(inventoryPath)

	level := cfg.LogLevel
	if level == "" {
		level = "info"
	}
	logger.Info("platform=%s nextui=%s log_level=%s",
		readPlatform(), readNextUIVersion(), level)

	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_JOYSTICK | sdl.INIT_GAMECONTROLLER); err != nil {
		logger.Error("sdl pre-init: %v", err)
		os.Exit(1)
	}

	for i := 0; i < sdl.NumJoysticks(); i++ {
		if sdl.IsGameController(i) {
			if gc := sdl.GameControllerOpen(i); gc != nil {
				defer gc.Close()
			}
		} else {
			if js := sdl.JoystickOpen(i); js != nil {
				defer js.Close()
			}
		}
	}

	w, h := int32(1024), int32(768)
	if dm, err := sdl.GetCurrentDisplayMode(0); err == nil {
		w, h = dm.W, dm.H
	}
	logger.Info("display: %dx%d", w, h)

	r, err := renderer.New("Itch.io", int(w), int(h))
	if err != nil {
		logger.Error("renderer init: %v", err)
		os.Exit(1)
	}
	defer r.Close()

	cache := renderer.NewImageCache(50)
	defer cache.Clear()

	client := itchio.NewClient()

	updateSvc := inventory.NewUpdateService(inv, inventoryPath, client, func() {
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	})
	updateSvc.Start(nil)
	defer updateSvc.Stop()

	var current ui.Screen = ui.NewListScreen(client, cfg, cfgPath, cache, cachePath, inv, inventoryPath, updateSvc)

	platform := readPlatform()
	var pressedScancodes map[sdl.Scancode]bool
	if platform == "my355" {
		pressedScancodes = make(map[sdl.Scancode]bool)
	}

	for current != nil {
		cache.ProcessPending(r)
		for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
			if pressedScancodes != nil {
				if kev, ok := e.(*sdl.KeyboardEvent); ok {
					sc := kev.Keysym.Scancode
					if kev.Type == sdl.KEYDOWN {
						if pressedScancodes[sc] {
							continue
						}
						pressedScancodes[sc] = true
					} else if kev.Type == sdl.KEYUP {
						delete(pressedScancodes, sc)
					}
				}
			}
			current = current.HandleEvent(e)
			if current == nil {
				break
			}
		}
		if current != nil {
			current.Draw(r)
		}
		sdl.Delay(16)
	}
}
```

- [ ] **Step 8.6 — Full build check**

```bash
go build -tags headless ./...
```

Expected: no errors.

- [ ] **Step 8.7 — Run full test suite**

```bash
go test -tags headless ./... 2>&1
```

Expected: all tests PASS.

- [ ] **Step 8.8 — Commit**

```bash
git add cmd/itchio-pak/main_sdl.go internal/ui/screen_list.go internal/ui/screen_settings.go
git commit -m "feat: wire UpdateService through main, ListScreen, and SettingsScreen"
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Covered by |
|---|---|
| UpstreamFile type + Entry fields | Task 1 |
| HasPendingUpdates, IsRemoved, DismissUpdate, DismissRemoval | Task 1 |
| IsFree in Add() | Task 1 step 1.4 + 1.7 |
| MarkRemoved idempotent / MarkReachable | Task 1 |
| UpdateService struct + lifecycle | Task 3 |
| Cover art repair | Task 3 |
| 404 detection (free + paid) | Task 4 |
| File diff (add new, prune vanished) | Task 4 |
| SetUpstreamFiles + UpdateCheckedAt | Task 1 (method) + Task 4 (caller) |
| ApplySort [DL] grouping | Task 2 |
| Row badges [UP]/[!] | Task 5 |
| Cover art pill overlay | Task 5 |
| Section separators | Task 5 |
| X button dismiss | Task 6 |
| Footer hints wide/narrow | Task 6 |
| Settings "Update Inventory" entry | Task 7 |
| Timestamp annotation | Task 7 |
| TriggerNow from Settings | Task 7 |
| TriggerNow post-cache-refresh | Task 8 |
| UpdateService.Start/Stop in main | Task 8 |
| notify → sdl.UserEvent | Task 8 |

All spec requirements covered. No gaps found.
