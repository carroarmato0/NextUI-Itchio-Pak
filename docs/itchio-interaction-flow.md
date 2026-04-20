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
| Game ID | `<meta content="games/NNNN" name="itch:path">` |
| CSRF token | `<meta name="csrf_token" value="...">` or `<input name="csrf_token" value="...">` |
| Screenshots | `<img class="screenshot..." src="...">` |
| Description | `<div class="formatted_description">...</div>` (converted to plain text) |

The CSRF token extracted here is used in the free download flow (Step 2 below).

**Game ID extraction note:** itch.io places `content=` before `name=` in the
`itch:path` meta element. The code uses a two-step approach — match the whole
`<meta>` element containing `itch:path`, then extract the numeric ID from its
`content` attribute — to be resilient to attribute ordering.

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

**Source:** `download_auth.go` — `FetchAuthUploads` + `DownloadAuthUpload`

For paid games the user already owns, the pak uses a combination of the
butler-style `api.itch.io` endpoint (to retrieve the buyer's download key)
and the public v1 API (to list uploads and resolve CDN URLs). This path is
taken automatically when all three conditions are true:

- `game.IsFree == false`
- `cfg.APIKey != ""`
- `detail.GameID != ""`

### Step 1 — GET buyer's download key ID

```
GET https://api.itch.io/profile/owned-keys?game_id={GAME_ID}
Authorization: Bearer {API_KEY}
```

Response (JSON):

```json
{
  "owned_keys": [
    { "id": 153412711, "game_id": 4228927, "purchase_id": 35928998, ... }
  ]
}
```

The `id` field is the buyer's **download key ID** — a numeric identifier tied
to their purchase. This is distinct from the API key. It is required to
authenticate upload listing and CDN URL resolution in subsequent steps.

If `owned_keys` is empty, the game is not owned by this user and an error
is surfaced.

**Why `api.itch.io` and not `itch.io/api/1/KEY/game/GAME_ID/download_keys`?**
The v1 `download_keys` endpoint is a **creator** endpoint — it lists keys a
game developer has issued to others. It returns `{"errors":["invalid game_id"]}`
for any game the API key owner did not create. The butler-style
`api.itch.io/profile/owned-keys` is the buyer-side equivalent.

### Step 2 — List uploads

```
GET https://itch.io/api/1/{API_KEY}/game/{GAME_ID}/uploads?download_key_id={KEY_ID}
```

Returns all uploads for the game, authenticated by the download key. Only
`.gb` and `.gbc` uploads are kept. The upload ID (`id` field) is stored
on each `Upload` struct alongside the download key ID.

### Step 3 — Resolve CDN URL

```
GET https://itch.io/api/1/{API_KEY}/upload/{UPLOAD_ID}/download?download_key_id={KEY_ID}
```

Response (JSON):

```json
{ "url": "https://itchio-mirror.{hash}.r2.cloudflarestorage.com/upload2/..." }
```

The signed CDN URL expires quickly (60 seconds). It is resolved immediately
before streaming, not cached.

### Step 4 — Stream file

```
GET {CDN_URL}
```

The file is streamed directly to the destination path on disk, identical to
the free download flow. The `Content-Length` header drives the progress bar.

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
  notice. The free download button detection relies on `<div class="upload">`,
  `<strong class="name">`, and `data-upload_id` — any of these changing would
  break file listing for free games.

- **No official free-download API.** Everything in the free download path uses
  itch.io's internal web API. It is undocumented and unsupported.

- **CSRF tokens expire.** If the user leaves the ROM picker open for a long
  time before selecting a file, the CSRF token from Step 4 may expire, causing
  the resolver to return `{"errors":["invalid key"]}`.

- **`suggested_amount=0`** only works for truly free or PWYW games. A game
  with a mandatory minimum price will return an empty URL in Step 2 of the free
  flow — this is expected; those games use the paid API path instead.

- **CDN URLs are short-lived.** The signed URL returned by Step 3 of the paid
  path expires in ~60 seconds. The URL is resolved immediately before streaming
  so this is not normally an issue, but a very slow or stalled connection could
  cause it to expire mid-transfer.

- **API key entry is not available in-app.** The Settings screen shows whether
  an API key is configured but does not provide a keyboard for entering one.
  The key must be set by editing `config.json` directly (see README).
