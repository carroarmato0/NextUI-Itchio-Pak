# Cloudflare Bypass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the itch.io HTTP client against Cloudflare bot-protection 403 responses by mimicking Chrome's TLS fingerprint, adding browser-like HTTP headers, returning a typed error on 403, and surfacing a helpful user-facing message.

**Architecture:** Three layers of defence are added to `internal/itchio/client.go`: (1) the existing `uaTransport` is extended with browser-standard HTTP headers, (2) a `utls`-based `DialTLSContext` replaces Go's default TLS stack with a Chrome-compatible TLS fingerprint, (3) a sentinel error `ErrCloudflareBlocked` is defined so the two UI screens that display feed errors can show a human-readable explanation instead of the raw error string.

**Tech Stack:** Go 1.22, `github.com/refraction-networking/utls` (pure Go — cross-compiles cleanly to ARM), existing `net/http`, SDL2 UI screens.

---

## File Map

| Action | File | Purpose |
|--------|------|---------|
| **Create** | `internal/itchio/errors.go` | Sentinel error `ErrCloudflareBlocked` |
| **Modify** | `internal/itchio/client.go` | Browser headers in `uaTransport` + utls `DialTLSContext` |
| **Modify** | `internal/itchio/feed.go` | Detect HTTP 403, wrap with `ErrCloudflareBlocked` |
| **Modify** | `internal/itchio/feed_test.go` | Tests: 403 → typed error, headers are sent |
| **Modify** | `internal/ui/screen_list.go` | Show Cloudflare-specific message on 403 |
| **Modify** | `internal/ui/screen_cache_refresh.go` | Show Cloudflare-specific message on 403 |
| **Modify** | `README.md` | Known Limitations: Cloudflare blocking explained |

---

## Task 1: Define the sentinel error

**Files:**
- Create: `internal/itchio/errors.go`

- [ ] **Step 1: Create the error file**

```go
package itchio

import "errors"

// ErrCloudflareBlocked is returned when itch.io responds with HTTP 403,
// indicating Cloudflare bot-protection rejected the request.
var ErrCloudflareBlocked = errors.New("Cloudflare blocked the request (HTTP 403)")
```

- [ ] **Step 2: Verify it compiles**

```bash
cd internal/itchio && go build ./...
```
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/itchio/errors.go
git commit -m "feat(itchio): define ErrCloudflareBlocked sentinel error"
```

---

## Task 2: Return ErrCloudflareBlocked on HTTP 403

**Files:**
- Modify: `internal/itchio/feed.go` — `FetchGamesFromURL` (line 101) and `FetchTotalGames` (line 207)

- [ ] **Step 1: Write failing tests**

Add to `internal/itchio/feed_test.go`, inside the existing `package itchio_test` block:

```go
func TestFetchGamesFromURL_403ReturnsCloudflareError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	_, err := c.FetchGamesFromURL(srv.URL + "/games/made-with-gb-studio.xml?page=1")
	if !errors.Is(err, itchio.ErrCloudflareBlocked) {
		t.Fatalf("expected ErrCloudflareBlocked, got %v", err)
	}
}
```

Add the `"errors"` import to the import block in `feed_test.go`.

- [ ] **Step 2: Run the test to confirm it fails**

```bash
make test
```
Expected: FAIL — `FetchGamesFromURL_403ReturnsCloudflareError` fails because the error is currently `"fetch feed: HTTP 403"`, not `ErrCloudflareBlocked`.

- [ ] **Step 3: Update FetchGamesFromURL in feed.go**

Replace lines 101–104 in `internal/itchio/feed.go`:

```go
	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: HTTP %d from %s", resp.StatusCode, url)
		return nil, fmt.Errorf("fetch feed: HTTP %d", resp.StatusCode)
	}
```

With:

```go
	if resp.StatusCode == http.StatusForbidden {
		logger.Error("feed: HTTP 403 from %s (Cloudflare bot-protection)", url)
		return nil, fmt.Errorf("fetch feed: %w", ErrCloudflareBlocked)
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: HTTP %d from %s", resp.StatusCode, url)
		return nil, fmt.Errorf("fetch feed: HTTP %d", resp.StatusCode)
	}
```

- [ ] **Step 4: Update FetchTotalGames in feed.go**

Replace lines 207–210 in `internal/itchio/feed.go`:

```go
	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: total-games HTTP %d", resp.StatusCode)
		return 0, fmt.Errorf("fetch total games: HTTP %d", resp.StatusCode)
	}
```

With:

```go
	if resp.StatusCode == http.StatusForbidden {
		logger.Error("feed: total-games HTTP 403 (Cloudflare bot-protection)")
		return 0, fmt.Errorf("fetch total games: %w", ErrCloudflareBlocked)
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: total-games HTTP %d", resp.StatusCode)
		return 0, fmt.Errorf("fetch total games: HTTP %d", resp.StatusCode)
	}
```

- [ ] **Step 5: Run tests to confirm they pass**

```bash
make test
```
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/feed.go internal/itchio/feed_test.go
git commit -m "feat(itchio): return ErrCloudflareBlocked on HTTP 403"
```

---

## Task 3: Add browser-like HTTP headers to uaTransport

**Files:**
- Modify: `internal/itchio/client.go`
- Modify: `internal/itchio/feed_test.go`

> **Why no Accept-Encoding?** Go's `net/http` automatically adds `Accept-Encoding: gzip` and decompresses the response — but only when the request does *not* already have that header. Setting it manually would skip automatic decompression, requiring manual gunzip in every reader. Omitting it lets Go handle encoding transparently; Cloudflare's header checks are less strict than its TLS fingerprint checks.

- [ ] **Step 1: Write a failing header test**

Add to `internal/itchio/feed_test.go`:

```go
func TestFetchGamesFromURL_sendsBrowserHeaders(t *testing.T) {
	var gotUA, gotAccept, gotLang, gotFetchMode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		gotLang = r.Header.Get("Accept-Language")
		gotFetchMode = r.Header.Get("Sec-Fetch-Mode")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`))
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	c.FetchGamesFromURL(srv.URL + "/games/made-with-gb-studio.xml?page=1")

	if gotUA == "" {
		t.Error("User-Agent header not sent")
	}
	if gotAccept == "" {
		t.Error("Accept header not sent")
	}
	if gotLang == "" {
		t.Error("Accept-Language header not sent")
	}
	if gotFetchMode == "" {
		t.Error("Sec-Fetch-Mode header not sent")
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
make test
```
Expected: FAIL — `Sec-Fetch-Mode` (and others besides User-Agent) are not currently sent.

- [ ] **Step 3: Add a helper and extend uaTransport.RoundTrip in client.go**

Replace the entire `uaTransport` type and its `RoundTrip` method in `internal/itchio/client.go`:

```go
// uaTransport injects browser-compatible headers on every outbound request
// that does not already have them, then delegates to the wrapped RoundTripper.
type uaTransport struct {
	wrapped http.RoundTripper
}

func setDefaultHeader(req *http.Request, key, value string) {
	if req.Header.Get(key) == "" {
		req.Header.Set(key, value)
	}
}

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	setDefaultHeader(req, "User-Agent", userAgent)
	setDefaultHeader(req, "Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	setDefaultHeader(req, "Accept-Language", "en-US,en;q=0.9")
	setDefaultHeader(req, "sec-ch-ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`)
	setDefaultHeader(req, "sec-ch-ua-mobile", "?0")
	setDefaultHeader(req, "sec-ch-ua-platform", `"Windows"`)
	setDefaultHeader(req, "Sec-Fetch-Dest", "document")
	setDefaultHeader(req, "Sec-Fetch-Mode", "navigate")
	setDefaultHeader(req, "Sec-Fetch-Site", "none")
	setDefaultHeader(req, "Sec-Fetch-User", "?1")
	setDefaultHeader(req, "Cache-Control", "max-age=0")
	return t.wrapped.RoundTrip(req)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
make test
```
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/client.go internal/itchio/feed_test.go
git commit -m "feat(itchio): add browser-compatible HTTP headers to all outbound requests"
```

---

## Task 4: Add utls TLS fingerprint mimicry

**Files:**
- Modify: `internal/itchio/client.go`

> **What this does:** Go's default `crypto/tls` produces a JA3 fingerprint that is instantly recognisable as a Go HTTP client. Cloudflare scores it negatively. `utls` replaces the TLS handshake with an exact byte-for-byte replica of Chrome's ClientHello (cipher suites, extensions, elliptic curves). This is the most impactful change.
>
> **HTTP/2 note:** `NextProtos: []string{"http/1.1"}` is set in the utls config to prevent h2 negotiation. If h2 were negotiated at the TLS layer but Go's transport sent HTTP/1.1 frames, the server would reject the connection. HTTP/1.1 is sufficient for RSS XML fetching. The ALPN extension is still present in the ClientHello (matching Chrome structure); only the advertised value differs.
>
> **Cross-compilation:** `utls` is pure Go — no CGO required. It compiles cleanly for ARM targets (`tg5040`, `tg5050`, `my355`).

- [ ] **Step 1: Add the utls dependency**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
go get github.com/refraction-networking/utls@latest
go mod tidy
```
Expected: `go.mod` and `go.sum` updated; no errors.

- [ ] **Step 2: Add the DialTLSContext function and update newHTTPClient in client.go**

Add a new import to `internal/itchio/client.go`:

```go
import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"

	utls "github.com/refraction-networking/utls"
)
```

Add `dialTLS` function after the `uaTransport` block:

```go
// dialTLS dials a TCP connection and upgrades it with a Chrome-compatible TLS
// handshake using utls. This replaces Go's default crypto/tls fingerprint
// (which Cloudflare bot-protection flags) with Chrome's ClientHello.
func dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	uconn := utls.UClient(conn, &utls.Config{
		ServerName: host,
		NextProtos: []string{"http/1.1"},
	}, utls.HelloChrome_Auto)
	if err := uconn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return uconn, nil
}
```

Update `newHTTPClient` to use `dialTLS`:

```go
func newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		DialTLSContext: dialTLS,
	}
	return &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		Transport: &uaTransport{wrapped: transport},
	}
}
```

- [ ] **Step 3: Run tests to confirm nothing regresses**

```bash
make test
```
Expected: all tests PASS. (Tests use plain HTTP via `httptest.NewServer` — `DialTLSContext` is only invoked for `https://` URLs, so existing tests are unaffected.)

- [ ] **Step 4: Verify cross-compilation still works**

```bash
make build-all
```
Expected: binaries produced for all platforms (`tg5040`, `tg5050`, `my355`) without errors.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/client.go go.mod go.sum
git commit -m "feat(itchio): mimic Chrome TLS fingerprint with utls to reduce Cloudflare 403s"
```

---

## Task 5: Show a Cloudflare-specific message in the List screen

**Files:**
- Modify: `internal/ui/screen_list.go` — `Draw` method, error branch (around line 210)

- [ ] **Step 1: Add the errors import to screen_list.go**

The file already imports `"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"`. Add `"errors"` to the import block.

- [ ] **Step 2: Replace the generic error display with a Cloudflare-aware one**

Replace lines 210–213 in `internal/ui/screen_list.go`:

```go
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), 20, r.H/2, 200, 50, 50)
		r.Present()
		return
	}
```

With:

```go
	if s.err != nil {
		_, fontH := r.TextSize("Ag")
		mid := r.H / 2
		if errors.Is(s.err, itchio.ErrCloudflareBlocked) {
			r.DrawTextCentered("Cloudflare blocked the request (HTTP 403)", 0, mid-fontH-4, r.W, 200, 100, 50)
			r.DrawWrappedText("Visit itch.io in a browser on the same WiFi, then press A to retry.", 20, mid+4, r.W-40, fontH+4, 200, 160, 100)
		} else {
			r.DrawText("Error: "+s.err.Error(), 20, mid, 200, 50, 50)
		}
		r.Present()
		return
	}
```

> `r.TextSize("Ag")` returns `(w, h int32)` — use `fontH` for line spacing. `r.DrawTextCentered` and `r.DrawWrappedText` are both present in the existing renderer API (already used in `screen_cache_refresh.go`).

- [ ] **Step 3: Add a retry action for the Cloudflare error**

Find the `HandleEvent` method in `screen_list.go`. Locate the controller `A` button handler where page loading occurs. Add a retry path when `s.err` is a Cloudflare block:

Find the block that handles `sdl.CONTROLLER_BUTTON_A` in `HandleEvent`. If there is no error-state A handler, add one at the top of the controller button handler:

```go
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		// Allow retrying when the feed is blocked.
		if s.err != nil && ev.Button == sdl.CONTROLLER_BUTTON_A {
			go s.loadPage(s.page, "")
			return s
		}
		// ... existing button handling follows
```

- [ ] **Step 4: Build the UI target to confirm it compiles**

```bash
make build-native 2>&1 | tail -20
```
Expected: no errors. (If `build-native` requires local SDL2 dev libs and those aren't present, run `make test` instead — the `!headless` tag means UI code is excluded from test builds, but the import graph is still validated by the headless build.)

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): show Cloudflare-specific guidance on HTTP 403 in game list"
```

---

## Task 6: Show a Cloudflare-specific message in the Cache Refresh screen

**Files:**
- Modify: `internal/ui/screen_cache_refresh.go` — `Draw` method, `refreshCacheError` case (lines 112–114)

- [ ] **Step 1: Add the errors import to screen_cache_refresh.go**

The file already imports `"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"`. Add `"errors"` to the import block.

- [ ] **Step 2: Replace the generic error display**

Replace the `refreshCacheError` case in `Draw` in `internal/ui/screen_cache_refresh.go`:

```go
	case refreshCacheError:
		r.DrawTextCentered("Refresh failed:", 0, mid-fontH-8, r.W, 200, 60, 60)
		r.DrawWrappedText(s.err.Error(), 20, mid, r.W-40, fontH+4, 200, 100, 100)
```

With:

```go
	case refreshCacheError:
		if errors.Is(s.err, itchio.ErrCloudflareBlocked) {
			r.DrawTextCentered("Cloudflare blocked the request (HTTP 403)", 0, mid-fontH-8, r.W, 200, 100, 50)
			r.DrawWrappedText("Visit itch.io in a browser on the same WiFi, then retry the refresh.", 20, mid, r.W-40, fontH+4, 200, 160, 100)
		} else {
			r.DrawTextCentered("Refresh failed:", 0, mid-fontH-8, r.W, 200, 60, 60)
			r.DrawWrappedText(s.err.Error(), 20, mid, r.W-40, fontH+4, 200, 100, 100)
		}
```

- [ ] **Step 3: Build to confirm it compiles**

```bash
make test
```
Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_cache_refresh.go
git commit -m "feat(ui): show Cloudflare-specific guidance on HTTP 403 in cache refresh"
```

---

## Task 7: Document the issue in README.md

**Files:**
- Modify: `README.md` — add to the **Known Limitations / To-Do** section

- [ ] **Step 1: Add the Cloudflare entry**

In `README.md`, under `## Known Limitations / To-Do`, add the following block **before** the existing first bullet:

```markdown
- **Cloudflare may block RSS feed requests (HTTP 403).** itch.io is protected by
  Cloudflare, which uses browser fingerprinting to detect bot traffic. Despite the
  app mimicking Chrome headers and TLS fingerprint, Cloudflare may still issue a
  temporary block — especially on networks or IP ranges it has not seen before.

  **What you will see:** An error message on the game list or cache-refresh screen
  that reads *"Cloudflare blocked the request (HTTP 403)"*.

  **What you can do:**
  1. **Visit itch.io in a browser on the same WiFi network.** Your device and your
     phone or laptop share the same public IP address. If Cloudflare presents a
     human-verification challenge in the browser and you pass it, the IP is marked
     as human traffic. Return to the Pak and press **A** (game list) or retry the
     refresh — it will often succeed immediately afterwards.
  2. **Wait a few minutes and retry.** Cloudflare challenges are sometimes
     temporary. The Pak retries the request each time you press **A** on the error
     screen.
  3. **Try a different network.** Switching WiFi networks (e.g. a mobile hotspot)
     gives the Pak a fresh public IP that may not be challenged.

```

- [ ] **Step 2: Verify the markdown renders correctly**

Preview the file in any markdown renderer or run:

```bash
grep -A 20 "Cloudflare may block" README.md
```
Expected: the block appears with correct formatting.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document Cloudflare 403 blocking and user workarounds in README"
```

---

## Self-Review

### Spec coverage

| Requirement | Task |
|-------------|------|
| Add browser-like HTTP headers | Task 3 |
| Add utls TLS fingerprint mimicry | Task 4 |
| Typed error on 403 | Tasks 1 & 2 |
| User-friendly UI message on 403 | Tasks 5 & 6 |
| README explanation | Task 7 |
| Both `FetchGamesFromURL` and `FetchTotalGames` wrap 403 | Task 2 (both sites handled) |
| Retry action in list screen | Task 5 (A button retry) |

### Placeholder scan

No TBDs, no "add appropriate handling", no forward references.

### Type/method consistency

- `ErrCloudflareBlocked` defined in Task 1, referenced in Tasks 2, 5, 6 — name is consistent throughout.
- `dialTLS` is a package-level function in `client.go` — not exported, not referenced from tests.
- `r.DrawTextCentered` and `r.DrawWrappedText` are used in the existing `screen_cache_refresh.go` — confirmed present in the renderer API.
- `setDefaultHeader` is a package-level helper in `client.go` — not exported.
