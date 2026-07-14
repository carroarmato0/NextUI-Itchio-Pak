# Smooth Scroll + Screenshot Pre-fetch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make L1/R1 list scrolling visually smooth (single-step at higher speed instead of page jumps), and pre-fetch all game screenshots when a detail screen opens so the user doesn't see "Loading..." when cycling through them.

**Architecture:** Two independent one-file changes in `internal/ui/`. Task 1 changes two lines in `screen_list.go`. Task 2 adds a warm loop in the `NewDetailScreen` fetch goroutine in `screen_detail.go`. Neither touches the renderer, cache internals, or network layer.

**Tech Stack:** Go, `internal/ui` package, `./scripts/test.sh` for headless tests, `./scripts/build.sh tg5040` + `./scripts/deploy.sh` for device verification.

---

### Task 1: L1/R1 single-step smooth scroll

**Files:**
- Modify: `internal/ui/screen_list.go:38` — `shoulderAccelMin` constant
- Modify: `internal/ui/screen_list.go:233` — jump expression in `processAutoRepeat`

**Context for the implementer:**

`screen_list.go` has two auto-repeat state machines — one for D-pad (`heldDir`) and one for shoulder buttons (`heldShoulderDir`). The shoulder handler lives in `processAutoRepeat` at around line 226:

```go
if s.heldShoulderDir != 0 {
    elapsed := now.Sub(s.heldShoulderSince)
    interval := currentRepeatInterval(elapsed - repeatDelay)
    if interval < shoulderAccelMin {
        interval = shoulderAccelMin
    }
    if elapsed >= repeatDelay && now.Sub(s.lastShoulderRepeat) >= interval {
        s.jumpCursor(s.heldShoulderDir * s.lastVisibleRows)
        s.lastShoulderRepeat = now
    }
}
```

The problem: `* s.lastVisibleRows` makes each repeat jump ~10 rows at once. The fix is to jump 1 row per repeat (same as D-pad's `moveCursor`) but allow the interval to go much lower so L1/R1 is still faster than D-pad overall.

The `shoulderAccelMin` constant is at line 38:
```go
shoulderAccelMin = 150 * time.Millisecond
```

These are the only two lines to change. No other logic, no new state, no new functions.

- [ ] **Step 1: Verify the current test suite passes as baseline**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io/.worktrees/perf/renderer-text-perf
./scripts/test.sh
```

Expected: all 9 packages pass.

- [ ] **Step 2: Change `shoulderAccelMin` from 150ms to 15ms**

In `internal/ui/screen_list.go`, find line 38:

```go
	shoulderAccelMin = 150 * time.Millisecond // minimum repeat interval for L1/R1 page jumps
```

Replace with:

```go
	shoulderAccelMin = 15 * time.Millisecond // minimum repeat interval for L1/R1 (one row per frame at 60fps)
```

- [ ] **Step 3: Change the jump expression from page-size to single-step**

In `internal/ui/screen_list.go`, find line 233 (inside `processAutoRepeat`):

```go
			s.jumpCursor(s.heldShoulderDir * s.lastVisibleRows)
```

Replace with:

```go
			s.jumpCursor(s.heldShoulderDir)
```

- [ ] **Step 4: Run the full headless test suite**

```bash
./scripts/test.sh
```

Expected: all 9 packages pass. (No unit tests directly cover `processAutoRepeat`, but the build step catches compilation errors.)

- [ ] **Step 5: Build for device**

```bash
./scripts/build.sh tg5040
```

Expected: `Built: bin/tg5040/itchio-pak (...)` with no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): smooth L1/R1 scroll — single-step at 15ms floor instead of page jumps

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Detail screen screenshot pre-fetch

**Files:**
- Modify: `internal/ui/screen_detail.go:131-133` — add `Warm` loop after ScreenshotURLs assembled

**Context for the implementer:**

`NewDetailScreen` launches a goroutine that calls `FetchGameDetail` and assembles `ScreenshotURLs`. Currently screenshots are fetched on-demand via `cache.Get()` when each index is displayed. The fix is to call `cache.Warm(url)` for every URL immediately after the slice is assembled, before `s.loading = false`.

The relevant section of the goroutine (lines 124–160):

```go
d, err := client.FetchGameDetail(game.URL)
if err != nil {
    logger.Error("detail: FetchGameDetail: %v", err)
} else {
    logger.Info("detail: %d screenshots", len(d.ScreenshotURLs))
    // Prepend cover art as the first image so it's shown by default
    if game.CoverURL != "" {
        d.ScreenshotURLs = append([]string{game.CoverURL}, d.ScreenshotURLs...)
    }
}
s.detail = d
s.err = err
```

The warm loop goes immediately after the `if game.CoverURL != ""` block (still inside the `else` branch, while `d != nil` is guaranteed). `cache.Warm(url)` is goroutine-safe, non-blocking, and idempotent — calling it on the cover URL (index 0) is a no-op since it's already cached from the list screen warm window.

`maxConcurrentFetches = 2` in the image cache throttles downloads automatically; the warm calls just queue the URLs.

- [ ] **Step 1: Add the `Warm` loop after ScreenshotURLs is assembled**

In `internal/ui/screen_detail.go`, find this block (around lines 127–132):

```go
	} else {
		logger.Info("detail: %d screenshots", len(d.ScreenshotURLs))
		// Prepend cover art as the first image so it's shown by default
		if game.CoverURL != "" {
			d.ScreenshotURLs = append([]string{game.CoverURL}, d.ScreenshotURLs...)
		}
	}
```

Replace with:

```go
	} else {
		logger.Info("detail: %d screenshots", len(d.ScreenshotURLs))
		// Prepend cover art as the first image so it's shown by default
		if game.CoverURL != "" {
			d.ScreenshotURLs = append([]string{game.CoverURL}, d.ScreenshotURLs...)
		}
		logger.Debug("detail: warming %d screenshot URLs", len(d.ScreenshotURLs))
		for _, u := range d.ScreenshotURLs {
			cache.Warm(u)
		}
	}
```

- [ ] **Step 2: Run the full headless test suite**

```bash
./scripts/test.sh
```

Expected: all 9 packages pass.

- [ ] **Step 3: Build for device**

```bash
./scripts/build.sh tg5040
```

Expected: `Built: bin/tg5040/itchio-pak (...)` with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat(ui): pre-fetch all screenshots when detail screen opens

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Deploy and smoke-test on device

- [ ] **Step 1: Deploy to connected device**

```bash
./scripts/deploy.sh
```

Expected: binary pushed, no ADB errors.

- [ ] **Step 2: Verify L1/R1 scroll feel**

Hold L1 or R1 for several seconds on the list screen. The list should scroll smoothly row-by-row, accelerating progressively, without any visible content teleporting. At full speed it should be noticeably faster than holding a D-pad direction.

- [ ] **Step 3: Verify screenshot pre-fetch**

Open any game's detail screen. While "Loading..." is shown (detail fetch in progress), wait for it to complete. Then press R1 to cycle through screenshots — the first few should appear instantly (or nearly so) rather than showing "Loading...".

Optionally stream the log to confirm warm calls fire:

```bash
./scripts/debug.sh logs
```

Look for lines like:
```
detail: warming N screenshot URLs
```
