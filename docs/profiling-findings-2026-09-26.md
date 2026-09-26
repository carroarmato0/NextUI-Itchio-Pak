# Profiling Findings — v1.0.24 vs 1.1 (2026-09-26)

**Device:** TrimUI Brick (tg5040, NextUI 20260719-0, kernel 4.9.191), 1024×768
**Compared:** the published **v1.0.24** release (Go 1.22.12) against
`release/1.1.0` at `9c26cb4` (Go 1.27.1)
**Profiles:** CPU and heap, one run per build, via `.profile-flags`. The files
are kept locally in `debug-profiles/2026-09-26/{v1024,v110}/` (git-ignored).

This round has what round 5 lacked: a known session length, a CPU profile, and
a baseline run on the same device with the same steps. That means the numbers
can be compared as rates, not just totals.

---

## Method

Each build ran on the Brick, one after the other, about 20 minutes apart:

1. Launch from the NextUI menu. The startup update check runs here.
2. Scroll the game list from top to bottom and back.
3. Open the game pages of Tobu Tobu Girl Deluxe, Glory Hunters and Fall from
   Space, and page through each one's screenshots (GIFs included).
4. Settings → Refresh Game List, and wait for it to finish.
5. Download Tobu Tobu Girl Deluxe.
6. Quit with B. The profiles are written only on a normal exit.

Each build had its own matching data. v1.0.24 used the pre-1.1 backups (the
config with its API key, plus inventory and owned cache). 1.1 used its own data,
signed in with the QR code. Everything was snapshotted beforehand and restored
afterwards.

The two runs were not the same length, so the comparison uses per-second
figures. The network does different work in the two builds on purpose (see
[Network](#network-behaviour)), and itch.io's response times vary. Treat
network timings as indicative, not measured.

---

## Summary

| | v1.0.24 (Go 1.22) | 1.1 (Go 1.27) |
|---|---|---|
| Session | 475 s | 435 s |
| CPU | 159 s — **33.4%** | 145 s — **33.4%** |
| Allocated | 2.10 GB — **4.41 MB/s** | 1.93 GB — **4.43 MB/s** |
| In use at quit | 15.3 MB | 18.5 MB |
| GC (background mark) | 2.75 s | 1.98 s |
| Fetching 378 feed pages (CPU) | 3.33 s | 2.95 s |
| TLS read (CPU) | 0.92 s (uTLS) | 1.04 s (crypto/tls) |
| Startup update check | 27 s, **24 `download_url` POSTs** | 17 s, **none** — 24 games via the API |
| Refresh Game List | **failed** after 4,596 games | 11,785 games in 70 s |
| HTTP 429 from itch.io | 12 | 3 |
| Errors in the log | 11 | 0 |

**Overall, CPU and memory cost for the same work is unchanged.** About 70% of
CPU goes to drawing the screen, which neither build changed:

- **SDL calls:** `runtime.cgocall` is about 57% of CPU in both builds.
- **GIFs:** decoding and compositing are about 18% of CPU and more than half of
  all allocations.

The gains are elsewhere:

- **Network and reliability:** covered in the next section.
- **Garbage collection:** about 28% less CPU on Go 1.27.
- **Feed fetching:** about 11% cheaper.
- **TLS:** moving from uTLS to `crypto/tls` made no measurable difference.

In-use memory at quit rose by 3 MB. That is within normal variation for
whichever cover images happen to be cached at exit, not a trend.

---

## Network behaviour

This is the reason for the release, and the log confirms it:

- **Download handshake gone.** 1.1's startup check sent no `download_url` POSTs.
  v1.0.24 sent 24, one browser-style download handshake per installed free
  game, which itch.io asked us to stop (issue #4). 1.1 lists files through the
  API when signed in and never starts a download.
- **Refresh finishes.** v1.0.24's full refresh failed at
  `tag-pico-8.xml?page=201`: a 404 past the end of the feed, retried and then
  treated as fatal (fixed in v1.0.25). 1.1 treats the 404 as the end of that
  feed and finishes.
- **Fewer 429s** (3 against 12), thanks to the per-host back-off in v1.0.25.
- **1.1 ran the update check twice.** The second run follows a successful
  refresh. v1.0.24's refresh failed, so its second check never happened.
  1.1's check also does more per game than v1.0.24's: it looks at paid games'
  files too.

---

## CPU — top functions (flat)

| v1.0.24 | | 1.1 | |
|---|---|---|---|
| 57.1% | `runtime.cgocall` (SDL) | 57.6% | `runtime.cgocall` (SDL) |
| 7.3% | `compositePalettedOver` (GIF) | 7.1% | `compositePalettedOver` (GIF) |
| 6.1% | `compress/lzw` decode (GIF) | 6.1% | `compress/lzw` decode (GIF) |
| 1.5% | `strings.Map` | 1.5% | `strings.Map` |
| 1.2% | `unicode.ToLower` | 1.4% | `unicode.ToLower` |

The `strings.Map` and `unicode.ToLower` lines are the finding below.

## Allocations — top (alloc_space)

| v1.0.24 | | 1.1 | |
|---|---|---|---|
| 36.8% | `image.NewPaletted` (GIF frames) | 39.3% | `image.NewPaletted` (GIF frames) |
| 15.7% | `strings.(*Builder).grow` | 16.5% | `bytealg.MakeNoZero` |
| 11.1% | `io.ReadAll` | 12.0% | `renderGIFFrames` |
| 10.5% | `renderGIFFrames` | 6.7% | `compress/lzw.newReader` |

`Builder.grow` and `bytealg.MakeNoZero` are the same allocation. Go 1.27 moved
where `strings.Builder` allocates. Both lead back to the finding below.

---

## Finding: the game page re-parsed its description on every frame — fixed

| | v1.0.24 | 1.1 before the fix |
|---|---|---|
| `DrawFormattedText`, CPU (cumulative) | 10.7 s — 6.8% | 10.3 s — 7.1% |
| `DrawFormattedText`, allocations (cumulative) | 390 MB — 18.6% | 367 MB — 19.0% |

**What it did:** `Renderer.DrawFormattedText` parsed the description's HTML on
every frame. For every paragraph, heading and list item, it ran
`strings.ToLower` over *the whole rest of the description* to find the closing
tag. That is quadratic in the description's length, and it ran about 60 times a
second whenever a game page was on screen.

**Fix** (`c509d2c`):

- **Parse once.** `parseDescription` (pure Go, unit-tested) turns the markup
  into blocks one time. The renderer caches the blocks per description, keeping
  eight, and a frame now only draws them.
- **No lowercased copy.** Closing tags are found with an ASCII case-insensitive
  search. Besides being cheaper, this removes a latent bug: `strings.ToLower`
  can change the byte length of some non-ASCII text, which would have shifted
  offsets against the original string.
- **Output unchanged.** The drawing arithmetic did not change. Renders of the
  fixture description, and of a new scene using every supported tag
  (`detail-rich-description`), are **pixel-identical** to the old code at
  1024×768, 640×480 and 720×720, in dark and light palettes.
- **Cost now.** One parse of a 2.7 KB description takes about 26 µs and 19 KB
  on the host (`BenchmarkParseDescription_real`), once per page instead of
  every frame.

**Expected effect:** almost all of the 7% CPU and 19% allocation share goes
away while a game page is open. Long descriptions should scroll more smoothly
on slower devices. **Not yet measured on the device.** The next profiling round
should confirm it.

---

## Other observations (no action)

- **GIF decoding** is still the largest allocator (about 890 MB per session in
  both builds). That is expected: animated screenshots are decoded into frames
  the LRU image cache holds. Same as rounds 4 and 5.
- **`romFileExt`**, round 5's high-priority item (fixed in `016f7b4`), no longer shows in the top
  lists.
- **`Texture.Query`** and **`truncateBoldToWidth`**, round 5's low-priority
  items, were not investigated this round.

---

## Action items

| Priority | Item | Status |
|---|---|---|
| High | Parse descriptions once, not every frame | **Done** (`c509d2c`) |
| Medium | Re-profile on the Brick to confirm the description fix | Next round |
| Low | `Texture.Query` caching (from r3/r4/r5) | Open |
| Low | `truncateBoldToWidth` string interning | Open |
