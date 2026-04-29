# Power Control Integration — Design Spec

**Date:** 2026-04-28  
**Status:** Approved for implementation

---

## Context

When a user presses the power button while the Itch.io Pak is running, the current behaviour is immediate exit via `SDL_QUIT`. This is wrong for two reasons:

1. Background tasks (ROM downloads, inventory checks, game cache builds) may be mid-flight, risking data loss or a corrupted inventory file.
2. On NextUI devices, deep sleep preserves process state — if the app exits before the device sleeps, the user wakes to the launcher instead of resuming the app where they left it.

The goal is to match the emulator experience: press power → device sleeps → press power again → app resumes exactly where it was.

---

## Mechanism

### Sleep and shutdown signals on NextUI

| Action | Mechanism |
|---|---|
| Deep sleep | `$SYSTEM_PATH/bin/suspend` — synchronous call; blocks until device wakes |
| Shutdown | `touch /tmp/poweroff` — NextUI detects the file and initiates clean shutdown |

Neither `poweroff` command nor minui-power-control is used. Both sleep and shutdown are triggered directly from Go after tasks complete.

### Power button detection

Rather than relying on `SDL_QUIT` (which carries no duration and conflates with SIGTERM), we read the power button directly from the Linux input subsystem via evdev. We scan `/dev/input/event*` at startup for a device advertising `EV_KEY` + `KEY_POWER` capability — this is platform-agnostic and works across tg5040, tg5050, and my355 without hardcoding device paths.

Press duration determines the action:
- **< 2 seconds (release):** sleep
- **≥ 2 seconds (held):** shutdown

### User experience on power press

When the power button is pressed while tasks are running:

> Full-screen overlay: "Please wait…" / "Finishing up before sleep" (or "…before shutdown")

No user input is accepted while the overlay is showing. When all tasks are done, the action fires automatically (sleep or shutdown). The user chose this — no confirmation or cancellation button.

---

## Architecture

### New package: `internal/power`

Single file `power.go` (build tag `!headless`).

```
Manager
  notify func(Action)   — callback injected by main; pushes SDL UserEvent

Action
  ActionSleep
  ActionShutdown
```

**Responsibilities:**
- Find the power button input device by scanning `/dev/input/event*` for `KEY_POWER` capability
- Read evdev events in a goroutine; measure press duration
- On short release: call `notify(ActionSleep)`
- On 2-second hold: call `notify(ActionShutdown)`
- Cooldown of 1 second after each action (prevents duplicate triggers)
- If no device found: log warning and return silently (app still works, just no graceful sleep)

**Dependency:** `github.com/holoplot/go-evdev` — pure Go, no CGo.

### `BusyChecker` interface — `internal/ui/screen.go`

```go
// BusyChecker is implemented by screens that block safe shutdown.
type BusyChecker interface {
    IsBusy() bool
}
```

Not added to the `Screen` interface. The main loop uses a type assertion. Only three screens implement it:

| Screen | `IsBusy()` returns true when |
|---|---|
| `DownloadScreen` | `s.state == dlDownloading` |
| `CacheRefreshScreen` | state is `refreshCacheLoading` |
| `ListScreen` | `s.cacheBuilding.Load() == true` |

`ListScreen` gains a new `cacheBuilding atomic.Bool` field, set at the start of `buildCache()` / `refreshCacheIfStale()` and cleared on return.

### Main loop changes — `cmd/itchio-pak/main_sdl.go`

New state:
```go
pendingQuit   bool
pendingAction power.Action  // Sleep or Shutdown
```

**SDL UserEvent codes** (defined as constants in `main_sdl.go`):
```
code 0 — inventory update complete (existing)
code 1 — power sleep requested
code 2 — power shutdown requested
```

The `power.Manager` is constructed with a notify callback that pushes a `sdl.UserEvent` with the appropriate code.

**Event loop changes:**

```
*sdl.UserEvent:
  code 0 → existing behaviour (redraw trigger)
  code 1 → pendingQuit = true, pendingAction = Sleep,  updateSvc.Stop()
  code 2 → pendingQuit = true, pendingAction = Shutdown, updateSvc.Stop()

*sdl.QuitEvent (SIGTERM from NextUI):
  if pendingQuit → ignore (already handling power event)
  else           → break loop (clean exit, existing behaviour)
```

**Per-frame check (after event polling):**

```
if pendingQuit:
  busy = current.(BusyChecker).IsBusy()  // false if screen doesn't implement it
  if !busy && !updateSvc.IsRunning():
    execute action:
      Sleep    → exec $SYSTEM_PATH/bin/suspend (blocks until wake)
                 after return: clear pendingQuit, resume loop normally
      Shutdown → os.WriteFile("/tmp/poweroff", nil, 0644)
                 loop forever (system kills us)
```

**Draw:**

```
if pendingQuit → drawPowerPendingOverlay(r)   // replaces current screen
else           → current.Draw(r)
```

`drawPowerPendingOverlay` is a function in `main_sdl.go`:
- `r.Clear(20, 20, 20)`
- `r.DrawTextCentered("Please wait…", …)`
- `r.DrawSmallTextCentered("Finishing up before sleep" or "…before shutdown", …)`
- `r.Present()`

### Cleanup: remove dead `QuitEvent` handlers

All 17 `case *sdl.QuitEvent:` branches across 15 screen files are removed. They are unreachable once the main loop intercepts `SDL_QUIT` before forwarding to screens.

---

## Data flow

```
[power button pressed]
       │
       ▼
internal/power goroutine
  measures duration
       │
       ▼
notify(ActionSleep / ActionShutdown)
  → sdl.PushEvent(UserEvent code 1 or 2)
       │
       ▼
main event loop wakes
  sets pendingQuit + pendingAction
  calls updateSvc.Stop()
       │
       ▼
each frame: check BusyChecker + updateSvc.IsRunning()
show overlay while busy
       │
       ▼
all tasks done
       ├─ Sleep    → exec $SYSTEM_PATH/bin/suspend ──► [device sleeps]
       │                                               [device wakes]
       │                                               clear pendingQuit
       │                                               ◄─ resume event loop
       │
       └─ Shutdown → touch /tmp/poweroff ──► [NextUI shuts down]
```

---

## Error handling

| Condition | Behaviour |
|---|---|
| No power button device found | Warning logged; sleep/shutdown not available; app still runs normally |
| `$SYSTEM_PATH` not set | Sleep falls back to clean exit (break loop) |
| `$SYSTEM_PATH/bin/suspend` missing | Same fallback; warning logged |
| `/tmp/poweroff` write fails | Error logged; shutdown attempt abandoned |

---

## What does NOT change

- `launch.sh` — no changes needed; no minui-power-control binary required
- The `Screen` interface — `BusyChecker` is a separate optional interface
- `UpdateService` — `Stop()` and `IsRunning()` are already the right API
- `DownloadScreen` event loop — the download goroutine continues uninterrupted during the wait

---

## Testing

- **Unit:** `internal/power` device-scanning logic is testable with a fake `/dev/input` path; press duration logic is testable without hardware
- **Device:** ADB into a live device; press power short → overlay appears, tasks complete, device sleeps, press power → app resumes at same screen; hold power 2s → overlay, then device shuts down
- **Edge cases:**
  - Power press during active download: overlay shown, download completes, then sleep
  - Power press during inventory check: overlay shown, check completes, then sleep
  - Power press with no tasks running: overlay flashes briefly (or not at all), sleep fires immediately
  - SIGTERM while not in pendingQuit: exits cleanly as before
