# Cover Art Download — Design Spec

**Date:** 2026-04-25

## Summary

When a ROM download completes, automatically fetch the game's cover art from Itch.io and save it into the `.media/` subdirectory alongside the ROM. NextUI uses this directory to display box art in the game list. The operation is best-effort and silent: any failure is logged at Warn level and does not affect the download result shown to the user.

---

## Architecture

A new method `(c *Client) DownloadCoverArt(coverURL, romDestPath string) error` is added to the `itchio` package in a new file `internal/itchio/cover_art.go`. It is called from the existing download goroutine in `screen_download.go` immediately after the ROM file is fully written, before transitioning to `dlDone`.

The method uses the shared `Client` receiver, which carries the UA/TLS transport (Cloudflare-resistant) already used for all other Itch.io requests.

---

## Path Derivation

Given `romDestPath`, the cover art destination is computed as follows:

1. `dir = filepath.Dir(romDestPath)` — the ROM's containing directory
2. `mediaDir = filepath.Join(dir, ".media")` — created with `os.MkdirAll` if absent
3. `base = strings.TrimSuffix(filepath.Base(romDestPath), filepath.Ext(romDestPath))` — ROM filename without extension
4. `ext` — derived from `filepath.Ext(path.Base(parsedCoverURL.Path))`; falls back to `.png` if empty or unrecognised
5. `artPath = filepath.Join(mediaDir, base+ext)`

**Example:**

| Input | Value |
|-------|-------|
| `romDestPath` | `/mnt/SDCARD/Roms/Game Boy Color (GBC)/Wario Land II.gbc` |
| `coverURL` | `https://img.itch.zone/aW1nLzEyMzQ1Njc4.jpg/315x250%23c/abc.jpg` |
| `mediaDir` | `/mnt/SDCARD/Roms/Game Boy Color (GBC)/.media/` |
| `artPath` | `/mnt/SDCARD/Roms/Game Boy Color (GBC)/.media/Wario Land II.jpg` |

This works for both the auto destination (`roms.DestinationDir` + filename) and user-chosen paths from `LocationPickerScreen`.

---

## Image Format

Cover images are downloaded and written as-is from Itch.io's CDN — no format conversion. The file extension is preserved from the source URL. Itch.io typically serves JPEG images.

NextUI's box art support is confirmed for PNG (used by the ScrapeGoat Pak). Whether it also loads JPEG or other formats from `.media/` is untested and will be verified on device. If only PNG is supported, a follow-up can add a decode/re-encode step using Go's standard `image/png` + `image/jpeg` packages (no new dependencies).

---

## Call Site

`internal/ui/screen_download.go` — inside the download goroutine, after the ROM write succeeds:

```go
if artErr := client.DownloadCoverArt(game.CoverURL, dest); artErr != nil {
    logger.Warn("cover-art: %v", artErr)
}
s.state = dlDone
```

`dlDone` is always reached regardless of cover art outcome.

---

## Error Handling

| Condition | Behaviour |
|-----------|-----------|
| `coverURL` is empty | `logger.Debug`, return `nil` (no file created) |
| `coverURL` cannot be parsed | return error (caller logs Warn) |
| `os.MkdirAll` fails | return error |
| HTTP status ≠ 200 | return error with status code |
| File write error | return error |
| Success | `logger.Info` with byte count and path |

---

## Tests

`internal/itchio/cover_art_test.go` using `httptest.NewServer` (same pattern as existing download tests):

| Test case | Assertion |
|-----------|-----------|
| Empty `coverURL` | returns `nil`, no file created |
| HTTP 404 from server | returns non-nil error |
| Successful fetch | `.media/` dir created; file written with correct name and bytes |
| URL with no file extension | falls back to `.png` extension |
| ROM path with no extension | base name used as-is |

---

## Files Changed

| File | Change |
|------|--------|
| `internal/itchio/cover_art.go` | New — `DownloadCoverArt` method |
| `internal/itchio/cover_art_test.go` | New — unit tests |
| `internal/ui/screen_download.go` | Call `DownloadCoverArt` after ROM write, log error |
