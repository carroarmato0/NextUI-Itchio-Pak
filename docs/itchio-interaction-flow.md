# itch.io Interaction Flow

This document describes how the Pak interacts with itch.io's website and
undocumented web API. There is no official public API for anonymous free
downloads — everything here was derived by observing browser network traffic
and confirmed through trial and error.

All HTTP logic lives in `internal/itchio/`.

---

## Game list (browse)

**Source:** `feed.go`

itch.io publishes an RSS/XML feed for tag-filtered searches:

```
GET https://itch.io/games/made-with-gb-studio.xml?page=N&q=QUERY
```

Each `<item>` contains title, link, description, image URL, and price. The
`<title>` field may include `[Tag]` brackets (e.g. `[GBC]`) which are stripped
from the display title but parsed as tags. Price is a free-text string; `$0.00`
or an empty/zero value means the game is free.

The total result count is scraped from the HTML browse page
(`https://itch.io/games/made-with-gb-studio`) by matching a
`"N results"` pattern, since the XML feed does not include it.

---

## Game detail (metadata + screenshots)

**Source:** `game.go` — `FetchGameDetail`

```
GET https://{author}.itch.io/{game}
```

The HTML page is scraped with regexes for:

| Field | Source in HTML |
|---|---|
| Game ID | `<meta name="itch:path" content="games/NNNN">` |
| CSRF token | `<meta name="csrf_token" value="...">` or `<input name="csrf_token" value="...">` |
| Screenshots | `<img class="screenshot..." src="...">` |
| Description | `<div class="formatted_description">...</div>` (converted to plain text) |

The CSRF token extracted here is used in the download flow (Step 2 below).

---

## Free game download flow

**Source:** `download.go` — `FetchUploads` + `DownloadFree`

There are six steps. The same `*Client` (and its cookie jar) is used
throughout, so cookies set in early steps are available in later ones.

### Step 1 — GET game page → CSRF token

```
GET https://{author}.itch.io/{game}
```

The page-level CSRF token is extracted from the HTML. It authenticates the
next request.

### Step 2 — POST download_url → signed download page URL

```
POST https://{author}.itch.io/{game}/download_url
Content-Type: application/x-www-form-urlencoded

csrf_token={GAME_PAGE_CSRF}&suggested_amount=0
```

Response (JSON):

```json
{ "url": "https://{author}.itch.io/{game}/download/{DOWNLOAD_KEY_JWT}" }
```

`suggested_amount=0` signals "pay nothing" for pay-what-you-want games.
If the game requires purchase and no key is present, `url` is empty — the
flow stops with an error.

### Step 3 — Extract download key from signed URL

The signed URL's last path segment is the download key. It is a
dot-separated signed token:

```
base64({"id": NNNN, "expires": UNIX_TIMESTAMP}).base64(HMAC_SIGNATURE)
```

Example (decoded):
```
{"id": 2661299, "expires": 1776640021}
```

**Key extraction note:** the key may contain `/` characters (base64 alphabet),
which itch.io percent-encodes as `%2F` in the URL path. To avoid splitting on
them, the code uses `url.EscapedPath()` (not `url.Path`) before splitting on
`/`, then `url.PathUnescape()` on the final segment.

### Step 4 — GET signed download page → upload list + CSRF token

```
GET https://{author}.itch.io/{game}/download/{DOWNLOAD_KEY_JWT}
```

The page lists all available downloads. It is parsed as HTML. For each
`<div class="upload">` block:

- **Filename** — from `<strong class="name" title="filename.gb">` (title
  attribute preferred; text content as fallback)
- **Upload ID** — from `data-upload_id="NNNNNN"` on the `<a class="download_btn">` element

Only `.gb` and `.gbc` uploads are kept.

The page also has its own CSRF token (distinct from the game page token):

```html
<meta name="csrf_token" value="..."/>
```

This CSRF token is required in Step 5.

**Why the download button has `href="javascript:void(0)"`:** itch.io's
frontend resolves the real CDN URL via JavaScript at click time. There is no
pre-baked `href` to a CDN URL in the HTML.

### Step 5 — POST file resolver → CDN URL

For each upload, a resolver URL of the form
`{gameURL}/file/{UPLOAD_ID}?key={JWT}&csrf={PAGE_CSRF}` is stored on the
`Upload` struct (constructed after Step 4). When the user selects a file,
`DownloadFree` makes:

```
POST https://{author}.itch.io/{game}/file/{UPLOAD_ID}
Content-Type: application/x-www-form-urlencoded

csrf_token={SIGNED_PAGE_CSRF}&download_key_id={NUMERIC_ID}
```

Two non-obvious details:

1. **`download_key_id` is the numeric ID** from the JWT payload (e.g. `2661299`),
   **not** the full JWT string. Sending the raw JWT as `key=...` returns
   `{"errors":["invalid key"]}`.

2. **`csrf_token` must be from the signed download page** (Step 4), not the
   game page (Step 1). These tokens differ per request.

Response (JSON):

```json
{ "url": "https://cdn-files.itch.zone/.../{filename}" }
```

### Step 6 — GET CDN URL → stream file

```
GET {CDN_URL}
```

The file is streamed directly to the destination path on disk. The `Content-Length`
header is used to track progress.

---

## Paid game download (API key path)

**Source:** `download_auth.go` — `DownloadAuth` / `CheckOwnership`

**Status: incomplete.** The infrastructure exists but paid downloads are not
yet functional end-to-end (see Known Limitations below).

For paid games, the user supplies an itch.io API key via the Settings screen.

**Ownership check** (`CheckOwnership`):

```
GET https://itch.io/api/1/{API_KEY}/game/{GAME_ID}/download_keys
```

Returns `{"download_keys": [...]}`. Non-empty list means the user owns the game.

**Authenticated download** (`DownloadAuth`):

```
GET {UPLOAD_URL}
Authorization: Bearer {API_KEY}
```

The `DownloadScreen` (`internal/ui/screen_download.go`) calls `CheckOwnership`
and, if the user owns the game, dispatches to `DownloadAuth` instead of
`DownloadFree`. However, this path is currently unreachable for paid games:
`FetchUploads` (Step 2) fails before `DownloadScreen` is ever reached, because
itch.io returns an empty URL in the `download_url` POST for games that require
purchase. A complete paid path would need to use itch.io's authenticated upload
API to obtain valid upload URLs independently of the free download flow.

---

## CSRF token format

itch.io CSRF tokens are WTFkit-style signed tokens:

```
base64([nonce, timestamp, session_id]).base64(HMAC_signature)
```

They are short-lived and tied to the current session. The game page and the
signed download page each issue their own token.

---

## Known limitations and fragility

- **HTML scraping is brittle.** itch.io can change its page structure without
  notice. The download button detection relies on `<div class="upload">`,
  `<strong class="name">`, and `data-upload_id` — any of these changing would
  break file listing.

- **No official API.** Everything in the free download path uses itch.io's
  internal web API. It is undocumented and unsupported.

- **CSRF tokens expire.** If the user leaves the ROM picker open for a long
  time before selecting a file, the CSRF token from Step 4 may expire, causing
  the resolver to return `{"errors":["invalid key"]}`.

- **`suggested_amount=0`** only works for truly free or PWYW games. A game
  with a mandatory minimum price will return an empty URL in Step 2.
