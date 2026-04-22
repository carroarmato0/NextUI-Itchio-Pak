# Unicode Text Rendering Support

**Date:** 2026-04-22
**Status:** Approved

## Problem

`assets/font.ttf` is DejaVu Sans (270KB), which has no CJK glyphs and no emoji glyphs. Game titles in Japanese (e.g. `彼は私の中の少女を犯し尽くした`, `かぞくロボット`, `夏戸市`) render as empty boxes. Emoji characters in titles and descriptions (e.g. `B🎳wling`, `🦾`) also render as boxes.

Research across ~720 sampled GB Studio games on itch.io identified the following scripts in use:

| Script | Frequency | Notes |
|--------|-----------|-------|
| Latin + accented (Spanish, Portuguese, French, Swedish…) | Very common | Already covered by DejaVu Sans |
| Japanese (Hiragana, Katakana, Kanji) | ~6–7 titles | Primary gap — fully unreadable titles |
| Traditional Chinese (CJK Unified) | 1 title | Covered by Japanese CJK font due to Unicode unification |
| Cyrillic | 1–2 titles | DejaVu Sans already includes Cyrillic |
| Emoji | Occasional in titles and descriptions | Handled by stripping |

## Design

### 1. Font replacement

Replace `assets/font.ttf` with **Noto Sans JP Regular**.

- Covers: Latin, Latin Extended, Cyrillic, Hiragana, Katakana, CJK Unified Ideographs (Japanese Kanji + most Traditional/Simplified Chinese)
- Size: ~3.7MB (vs. 270KB for DejaVu Sans)
- License: SIL Open Font License 1.1 — compatible with the pak's distribution
- No renderer code changes required; same `ttf.OpenFont` + `RenderUTF8Blended` path

The `go-sdl2/ttf` bindings (v0.4.40) do not expose `TTF_AddFallbackFont`, so font chaining is not available without custom CGo. A single comprehensive font is the correct approach.

### 2. Emoji stripping

Add `sanitizeText(s string) string` in `internal/renderer/text.go`.

Strips characters in the following Unicode ranges before any text reaches SDL2_ttf:

| Range | Block |
|-------|-------|
| U+2600–U+26FF | Miscellaneous Symbols |
| U+2700–U+27BF | Dingbats |
| U+1F300–U+1F5FF | Misc Symbols and Pictographs |
| U+1F600–U+1F64F | Emoticons |
| U+1F650–U+1F67F | Ornamental Dingbats |
| U+1F680–U+1F6FF | Transport and Map |
| U+1F700–U+1FFFF | Various supplementary emoji blocks |

Stripping is silent — no replacement character. `"B🎳wling"` → `"Bwling"`.

`sanitizeText` is called at the entry point of every text rendering and measurement function:
- `DrawText`
- `DrawSmallText`
- `DrawWrappedText`
- `WrapText`
- `TextSize`
- `SmallTextSize`

Stripping happens at render time, not at data fetch/cache time, so stored game data is unmodified.

## What is not changing

- No changes to `internal/itchio/` scraping or caching
- No font fallback infrastructure
- `WrapText` correctness is maintained because `sanitizeText` runs before line-breaking

## Files affected

| File | Change |
|------|--------|
| `assets/font.ttf` | Replace with Noto Sans JP Regular |
| `internal/renderer/text.go` | New file — `sanitizeText` helper |
| `internal/renderer/renderer.go` | Call `sanitizeText` in all text functions |
