# Accelerated Scrolling — Design Spec

**Date:** 2026-05-03
**Status:** Approved

## Problem

Both the game list screen and the game detail screen use a fixed repeat interval (40 ms) once the D-pad hold delay expires. The cursor/scroll accelerates from zero to max speed instantly, which feels abrupt and mechanical.

## Goal

Replace the fixed interval with an ease-out acceleration ramp so that the cursor starts slow, accelerates quickly, and plateaus smoothly at max speed — without any abrupt transitions. Inspired by the `1 − (1 − t)³` ease-out curve introduced in the NextUI commit `d10f433`.

## Easing Formula

```
easeOut(t) = 1 − (1 − t)³       t ∈ [0, 1]

interval(t) = accelStart + (accelMin − accelStart) × easeOut(t)
```

- At `t = 0`: interval = `accelStart` (slow)
- At `t = 1`: interval = `accelMin` (max speed)
- Shape: interval drops fast early (rapid acceleration), then plateaus smoothly

`t` is the normalised progress through the ramp:
```
t = clamp(repeatPhaseElapsed / accelRamp, 0, 1)
repeatPhaseElapsed = (now − heldSince) − repeatDelay
```

## Constants

Defined in `internal/ui/screen_list.go`. The old `repeatInterval = 40ms` constant is removed.

| Constant | Value | Notes |
|---|---|---|
| `repeatDelay` | 300 ms | unchanged — pause before repeat begins |
| `accelStart` | 180 ms | interval when repeat first fires |
| `accelMin` | 30 ms | interval at full speed |
| `accelRamp` | 1 500 ms | time to reach `accelMin` from `accelStart` |

## Shared Helper

```go
// currentRepeatInterval returns the repeat interval for a held button.
// elapsed is the time since the repeat phase began (after repeatDelay).
func currentRepeatInterval(elapsed time.Duration) time.Duration {
    t := float64(elapsed) / float64(accelRamp)
    if t > 1 {
        t = 1
    }
    eased := 1.0 - math.Pow(1.0-t, 3)
    return accelStart - time.Duration(float64(accelStart-accelMin)*eased)
}
```

Lives in `screen_list.go` alongside the constants. Requires `"math"` added to that file's imports.

## Affected Files

### `internal/ui/screen_list.go`

1. Remove `repeatInterval = 40 * time.Millisecond` from the constants block.
2. Add `accelStart`, `accelMin`, `accelRamp` to the constants block.
3. Add `"math"` to the import list.
4. Add `currentRepeatInterval` helper function.
5. Update `processAutoRepeat`: replace `repeatInterval` check with `currentRepeatInterval(elapsed - repeatDelay)`.

### `internal/ui/screen_detail.go`

1. Update `processAutoScroll`: replace `repeatInterval` check with `currentRepeatInterval(elapsed - repeatDelay)`.
   - `elapsed` is already computed as `now.Sub(s.heldSince)`.
   - The scroll step size (`viewportH / 20`) is unchanged.

## What Does Not Change

- The 300 ms initial hold delay before any repeat fires.
- `startHold` / `stopHold` / `startScrollHold` / `stopScrollHold` — release stops movement immediately (no momentum).
- The detail screen's scroll step size.
- The horizontal title scroll and vertical tag scroll (auto-scrolling, not user-driven).
- All other screen behaviour.
