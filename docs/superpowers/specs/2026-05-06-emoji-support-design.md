# Emoji Support Design

**Date:** 2026-05-06
**Status:** Approved

## Summary

Add monochrome emoji rendering to the Itch.io Pak by bundling NotoEmoji-Regular.ttf as a fallback font. Emoji are rendered everywhere in the UI but stripped from ROM filenames and from A-Z/Z-A sort keys. Implementation requires no changes to the renderer's font-loading infrastructure — the existing `font_fallback_*.ttf` glob picks up the new asset automatically.

## Background

Game titles on Itch.io frequently contain emoji. The renderer currently strips all emoji codepoints before passing text to SDL2_ttf, producing blank gaps in titles. The Noto font family is already in use (Noto Sans as primary, Noto Sans JP and Noto Sans Arabic/Hebrew/Thai/Devanagari as fallbacks), making NotoEmoji-Regular a natural addition.

SDL2_ttf renders font outlines only — it cannot decode colour emoji (CBDT/CBLC tables). Monochrome outline rendering is therefore the only viable approach without replacing the text rendering pipeline. Emoji glyphs are tinted to match surrounding text colour, consistent with all other glyphs.

## Rendering

NotoEmoji-Regular.ttf is bundled as `assets/font_fallback_emoji.ttf`. The renderer's existing init loop globs `assets/font_fallback_*.ttf` alphabetically and loads each file as a fallback font, parsing its cmap to determine which codepoints it covers. No init-code changes are required.

Emoji codepoints route through `fontIndex` to NotoEmoji exactly as Arabic or Thai codepoints route to their respective fallbacks. The `sanitizeText` and `isEmoji` functions in `internal/renderer/text.go` are removed entirely — stripping is no longer needed at render time. The existing floppy-disk special-case (`U+1F4BE`) is removed with them; NotoEmoji covers that codepoint and renders it correctly.

If the font file fails to load, the existing fallback-loading loop logs a warning and continues. Emoji fall through to the primary font and render as tofu — degraded but not a crash.

## Unrenderable Emoji

NotoEmoji-Regular does not cover every Unicode emoji codepoint. Rather than showing tofu for uncovered emoji, the renderer silently drops them. This is implemented by extending `fontIndex` to return a sentinel value (-1) when a rune is in an emoji range but no loaded font covers it:

- If the rune is in an emoji range (`text.IsEmoji`, an additional exported function in `internal/text`) AND the primary font does not cover it AND no fallback covers it → return -1.
- `splitTextRuns` skips runes whose font index is -1; they produce no text run and therefore no glyph.
- The per-rune cache stores -1 for skipped runes (same lazy-evaluation pattern as covered runes).

Non-emoji runes that fall through to the primary font without coverage continue to render as tofu (the existing behaviour for unsupported scripts with no loaded fallback). Only emoji-range runes are silently dropped.

`internal/text` exports a second function alongside `StripEmoji`:

```go
// IsEmoji reports whether r is an emoji codepoint.
func IsEmoji(r rune) bool
```

The renderer imports `internal/text` solely for this check. The range table remains in `internal/text` as the single source of truth.

## Shared `internal/text` Package

A new package `internal/text` provides a single exported function:

```go
// StripEmoji returns s with all emoji codepoints removed.
func StripEmoji(s string) string
```

An unexported `isEmoji(r rune) bool` helper contains the Unicode range table (moved verbatim from `internal/renderer/text.go`). The package has no dependencies on SDL2, CGo, or any other internal package.

This package is the single source of truth for the emoji range table. `roms` and `itchio` import it for stripping; the renderer imports it for the unrenderable-emoji drop check (see below).

## Filename Sanitisation

`roms.SanitiseFilename` calls `text.StripEmoji(title)` as its first step, before the existing filesystem-unsafe character stripping. An emoji-only title produces an empty string after stripping; the existing early-return on empty already handles that by signalling the caller to use the upstream filename instead.

## Sort Key

`itchio.ApplySort` introduces a local helper:

```go
func sortKey(s string) string {
    return strings.ToLower(text.StripEmoji(s))
}
```

The A-Z and Z-A cases use `sortKey` instead of bare `strings.ToLower`. All other sort modes (RSS, New, DL, Free, Paid) are unaffected — they do not compare by title.

Two titles that differ only in their emoji prefix produce the same sort key. `sort.SliceStable` preserves their original relative (RSS feed) order in that case.

## ZWJ Sequences

SDL2_ttf does not perform glyph substitution, so ZWJ sequences (e.g. `👨‍💻`) render as independent glyphs rather than a combined image. `StripEmoji` strips each emoji codepoint in the sequence; the ZWJ character itself (U+200D) is not in the emoji range table and may survive as a zero-width invisible character. This is harmless in both filenames (zero-width, no visual impact) and sort keys (no effect on ordering).

## Testing

| Location | What is tested |
|---|---|
| `internal/text/text_test.go` | Pure ASCII passthrough; mixed emoji+text; emoji-only → empty; `U+1F4BE` stripped; non-BMP scripts (CJK, Arabic) untouched; `IsEmoji` returns true for emoji ranges, false for Latin/CJK |
| `internal/roms/sanitise_test.go` | Leading emoji stripped from title before filename built; embedded emoji stripped; emoji-only title returns `""` |
| `internal/itchio/sort_test.go` | In A-Z mode, `"🎮 Zelda"` sorts with `"Z…"` entries, not at end of list |
| `internal/renderer/text_test.go` | `splitTextRuns` with a mock `fontIndex` that returns -1 for a rune omits that rune from all runs |

## Font Asset

- **File:** NotoEmoji-Regular.ttf from the [Noto Emoji GitHub repository](https://github.com/googlefonts/noto-emoji)
- **Stored as:** `assets/font_fallback_emoji.ttf`
- **License:** SIL Open Font License 1.1 (same as other bundled Noto fonts)
- **Approximate size:** ~9 MB uncompressed; adds ~4–6 MB to the compressed pak.zip (estimated 17–19 MB total, up from 13 MB)
- **License file:** `assets/OFL-1.1-NotoEmoji.txt` (to be added alongside the font)
