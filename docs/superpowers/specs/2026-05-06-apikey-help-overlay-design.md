# API Key Help Overlay — Design Spec

**Date:** 2026-05-06
**Status:** Approved

## Problem

When no API key is configured, pressing A on the "API Key: (not set)" row in Settings does nothing. Users have no indication of how to add a key.

## Solution

Show an opaque centered overlay panel when the user presses A on the API Key row and no key is set. The panel explains the requirement and displays a QR code linking to the setup guide. Any button press dismisses it.

When a key IS set, pressing A continues to open the existing key-test screen — no change to that path.

## Architecture

The overlay lives entirely within `SettingsScreen`. A `showAPIKeyHelp bool` flag gates rendering and input routing. No new screen type, no screen stack navigation, no renderer changes.

## Renderer

No changes. The overlay uses only existing primitives: `DrawRect`, `DrawPill`, `DrawText`, `DrawWrappedText`, `DrawSmallText`, `DrawTextureAt`, `QRTexture`.

## SettingsScreen Changes

### New fields

```go
showAPIKeyHelp bool
apiKeyHelpQR   *sdl.Texture // lazy-generated, destroyed on dismiss
```

### `activate()`

Add a case for `sItemAPIKey`:

```go
case sItemAPIKey:
    if s.cfg.APIKey == "" {
        s.showAPIKeyHelp = true
    }
```

The existing check in `HandleEvent` for the non-empty key path (navigate to key-test screen) is unaffected — it runs before `activate()` is reached.

### `HandleEvent()`

Prepend an early-return guard at the top of the method body:

```go
if s.showAPIKeyHelp {
    dismiss := false
    switch ev := e.(type) {
    case *sdl.KeyboardEvent:
        dismiss = ev.Type == sdl.KEYDOWN
    case *sdl.ControllerButtonEvent:
        dismiss = ev.Type == sdl.CONTROLLERBUTTONDOWN
    }
    if dismiss {
        s.showAPIKeyHelp = false
        if s.apiKeyHelpQR != nil {
            s.apiKeyHelpQR.Destroy()
            s.apiKeyHelpQR = nil
        }
    }
    return s
}
```

This swallows all input while the overlay is visible.

### `Draw()`

The existing `r.Present()` call at the end of `Draw()` is moved to be the absolute last line. The overlay is drawn between the footer hints and `r.Present()`:

```go
r.DrawFooterHints(hints, ftrY)
if s.showAPIKeyHelp {
    s.drawAPIKeyHelpOverlay(r)
}
r.Present()
```

`drawAPIKeyHelpOverlay` does not call `r.Present()` itself — there is exactly one `Present()` per frame.

### Footer hint

When cursor is on the API Key row and no key is set, change the hint from "Select" to "Setup guide":

```go
if s.cursor == sItemAPIKey {
    if s.cfg.APIKey != "" {
        hints[1].Text = "Test API key"
    } else {
        hints[1].Text = "Setup guide"
    }
}
```

### `drawAPIKeyHelpOverlay(r *renderer.Renderer)`

New private method. Renders:

1. **Panel background** — centered solid rect using theme background color, with a 2px accent-colored border drawn via four `DrawRect` calls
2. **Title** — "API Key Setup" in main text color
3. **Body text** — "Paid games require an Itch.io API key. Scan the QR code below for instructions on how to add it." — wrapped to fit panel width
4. **QR code** — lazily generated into `s.apiKeyHelpQR` using `r.QRTexture(apiKeySetupURL, size)`; cached for the lifetime of the overlay; destroyed on dismiss
5. **Caption** — "Scan to open setup guide" in hint text color

**QR URL:** `https://github.com/carroarmato0/NextUI-Itchio-Pak#adding-the-key-to-the-pak`

**Panel sizing** — scales to screen dimensions so it works across all three device resolutions (640×480, 1024×768, 1280×720). Target: ~60% screen width, content-height plus padding.

**QR size** — capped so it fits within the panel. Minimum 80px (scannable floor). Maximum one-third of screen width.

## Logging

Following project logging standards:
- `logger.Info("settings: API key help overlay shown")` when overlay opens
- `logger.Debug("settings: API key help overlay dismissed")` when dismissed

## Out of Scope

- Virtual keyboard input — the device doesn't support in-app text entry; the config file is the intended input method
- Overlay animation — static panel only
