---
name: itchio-pak-itch-scraping
description: Use when working on the internal/itchio package of the Itch.io NextUI Pak — implementing or debugging the RSS feed fetcher, game page scraper, free download flow, authenticated download flow, or testdata fixtures.
---

# Itch.io Pak — Itch.io Scraping Reference

## RSS Feed

```
GET https://itch.io/games/made-with-gb-studio.xml?page=N
```
- 40 entries per page, ~33 pages total (~1,299 games)
- Each `<item>` contains: title, link (game URL), description (with cover img tag), price, guid
- Cover image URL is embedded in the `<description>` HTML — parse with regex or XML
- Author derived from subdomain: `https://{author}.itch.io/{game}`

## Free Game Download Flow (5 steps)

```
1. GET https://{user}.itch.io/{game}
   Extract: data-game_id (attribute on page), csrf_token (hidden input or meta tag)

2. POST https://{user}.itch.io/{game}/download_url
   Content-Type: application/x-www-form-urlencoded
   Body: csrf_token={token}&suggested_amount=0
   → JSON response: { "url": "https://{user}.itch.io/{game}/download/{token}" }

3. GET {url from step 2}
   → HTML page listing downloadable uploads

4. Parse upload rows from HTML via data-upload_id attributes + <strong class="name"> text.
   Collect .gb, .gbc, and .zip uploads; skip .pocket and other formats.
   For each accepted upload, build a resolver URL:
     https://{user}.itch.io/{game}/file/{upload_id}?key={download_key}
   (The download key is embedded as a hidden input on the signed download page.)

5. POST {resolver_url}
   Body: csrf_token={token}
   → JSON { "url": "..." } with a short-lived CDN URL
   GET {cdn_url} → stream response body to destination file
```

HTTP client needs: cookie jar (session persists across requests), redirect following.

## Authenticated Download Flow (API key)

```
1. GET https://api.itch.io/profile/owned-keys?page=N
   Header: Authorization: Bearer {api_key}
   → paginates all owned keys (50/page), filters client-side for game_id
   → last page returns {"owned_keys":{}} (object, NOT array) — detect with RawMessage check

2. GET https://itch.io/api/1/{key}/game/{game_id}/uploads?download_key_id={key_id}
   → list .gb/.gbc uploads for the game

3. GET https://itch.io/api/1/{key}/upload/{upload_id}/download?download_key_id={key_id}
   → JSON {"url": "..."} with signed CDN URL (expires ~60s, resolve immediately before streaming)

4. GET {cdn_url}  →  stream to file
```

game_id comes from `data-game_id` on the game page (same as free flow step 1).

**owned-keys last-page quirk:** when no more keys exist, `owned_keys` is `{}` (empty object),
not `[]`. Decoding `{}` into `[]struct` causes Go's JSON decoder to return `UnmarshalTypeError`,
which would discard all keys already collected from earlier pages. Use `json.RawMessage` and
check `raw[0] == '['` before unmarshaling — same pattern as `ValidateAPIKey` in `auth_validate.go`.

## Fallback Behaviour
If any step fails (network error, unexpected HTML structure, paid gate):
- **Never fail silently**
- Show QR code for the game's store URL + human-readable error message
- Log error details to `$HOME/itchio-pak.log` for debugging

## Testdata Fixtures (`testdata/`)

| File | Used by |
|------|---------|
| `rss_page1.xml` | `feed_test.go` — RSS parsing |
| `game_page_free.html` | `game_test.go` — game_id / csrf extraction |
| `game_page_paid.html` | `game_test.go` — paid gate detection |
| `download_page.html` | `download_test.go` — upload link parsing |

All tests in `internal/itchio/` use these fixtures via `httptest.NewServer` — no live network calls in tests.

## Key Selectors (may need updating if Itch.io changes structure)

```go
// game_id: data attribute on the page
// e.g. <div class="game_download" data-game_id="850892">
gameIDRegex = regexp.MustCompile(`data-game_id="(\d+)"`)

// csrf_token: hidden input OR meta tag — supports both content= and value= attributes
csrfRegex = regexp.MustCompile(`name="csrf_token"\s+(?:content|value)="([^"]+)"`)

// upload rows on the signed download page are parsed via HTML tokeniser, not regex:
//   upload ID  → data-upload_id attribute on the upload row element
//   filename   → text inside <strong class="name">
// See ParseDownloadPage in game.go.
```

## ROM Selection Logic

```go
// Auto mode: score uploads, pick highest
func scoreUpload(filename string) int {
    switch filepath.Ext(strings.ToLower(filename)) {
    case ".gbc": return 2
    case ".gb":  return 1
    default:     return 0  // skip
    }
}
```

Ask mode: present filtered list via SDL2 file-picker screen before downloading.
