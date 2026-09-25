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

## Game details: data.json first, the page for the rest (1.1.0+)

`GET https://{author}.itch.io/{game}/data.json` — public, no sign-in, added to
by itch.io for this app. `FetchGameData` / `applyGameData` take from it:
`id` (GameID), `tags`, `screenshots` (347x500, as the page), and pricing —
`price` absent = free, `"$0.00"` = name-your-own-price, else paid (plus
`original_price` and `sale` during a sale) — and `suggested_price`, which
replaced the old `/purchase` page scrape. 404 = removed; a renamed game
redirects and `links.self` is its new address.

`FetchGameDetail` fetches data.json and the page in parallel. The page is
still needed for the description, bundle names, browser-only detection and
the CSRF token of the web download flow. If data.json fails, the page's own
scraped values are kept, so a data.json outage degrades nothing.

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

## Signing in: QR device grant (internal/itchio/oauth.go)

The token used below comes from QR sign-in, not a typed API key (1.1.0+).
`POST /oauth/device` (client_id, scope, PKCE S256) → show a QR of
`verification_uri_complete` + `user_code` → `POST /oauth/device/poll` every
`interval`s, after each answer (429 = slow down, double it) → on `approved`,
`POST /oauth/token` (code, code_verifier, redirect_uri=urn:itchio:poll,
device_info). 404 at the start = client not approved for QR login. Scope is
`profile:me profile:owned game:view:uploads` — never `itch`. Tokens do not
expire; a 401/403 from /profile means revoked (ErrTokenRejected). Never log
the device code, verifier, approval code or token.

## Authenticated Download Flow (API v2, key in the header)

Every call goes to `api.itch.io` with `Authorization: Bearer {api_key}`. The key
is never put in a URL (the old `itch.io/api/1/{key}/...` endpoints were dropped
at itch.io's request, issue #4).

```
1. GET https://api.itch.io/profile/owned-keys?game_ids={id,id,...}&page=N
   → game_ids: comma-separated, max 50 per request (scanOwnedKeys enforces
     it). NOT game_id — the singular is ignored and returns the whole library.
     Still filtered client-side. Last page is {"owned_keys":{}} (object).
   → BundleSize = distinct games per purchase_id over the WHOLE library; a
     filtered answer uses the counts cached by the startup ValidateAPIKey scan.

2. GET https://api.itch.io/games/{game_id}/uploads[?download_key_id={key_id}]
   → {"uploads":[{id, filename, size, md5_hash, build_id?, updated_at, type, traits}]}
   → download_key_id is optional: free and name-your-own-price games list
     without it, so signed-in users skip the web flow. A paid game without
     it lists only its demo uploads.
   → "traits" is {} when empty and an array otherwise — do not decode it.

3. POST https://api.itch.io/games/{game_id}/download-sessions  [download_key_id]
   → {"uuid": "..."} — ONE per install (roms.DownloadSession, shared by every
     upload of one listing), created lazily on the first resolve. Failure is
     non-fatal: the download proceeds ungrouped.

4. GET https://api.itch.io/uploads/{upload_id}/download?download_key_id=&uuid=
   → 302 to a signed R2 URL. Do NOT follow it (CheckRedirect →
     ErrUseLastResponse): the caller needs the URL for ZIP range reads and the
     magic-byte probe, and the Authorization header must not reach the CDN.

5. GET {cdn_url}  →  stream to file (resolve right before streaming; it expires)
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
