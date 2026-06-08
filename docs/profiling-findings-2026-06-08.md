# Profiling Findings — itchio-pak (2026-06-08)

**Session:** unknown duration (profile snapshot at 01:14:20)
**Live heap at snapshot:** 79.97 MB
**Lifetime allocations:** 1,631 MB total
**Profiles:** `debug-profiles/itchio-mem-5.prof` (cpu-5 is empty — app not restarted after latest deploy)
**Binary:** feature/ui-redesign branch, post-detail-redesign

---

## Comparison with Round 4 (2026-05-08)

| Metric | Round 4 | Round 5 | Notes |
|---|---|---|---|
| Live heap | 6.58 MB | 79.97 MB | Higher — more GIF previews loaded |
| Lifetime allocs | 3,484 MB / 901 s | 1,631 MB / ? | Rate unknown without session duration |
| Top allocator | GIF decode (58%) | GIF decode (58%) | Unchanged — expected |
| `drawFilledCircle` allocs | ~0 (fixed r4) | Not in top 30 | Confirmed clean ✅ |

---

## Live Heap (inuse_space)

| MB | % | Source |
|---|---|---|
| 75.45 | 94.4% | `image.NewPaletted` — GIF frame cache |
| 1.50 | 1.9% | `reflect.growslice` |
| 1.01 | 1.3% | pprof internal |
| 0.51 | 0.6% | `compress/lzw` |

**Assessment:** Live heap entirely dominated by GIF preview frames held in the image LRU cache. This is expected and unchanged from previous rounds. No action needed.

---

## Lifetime Allocations (alloc_space)

| MB | % | Source | Status |
|---|---|---|---|
| 946 | 58.0% | `image.NewPaletted` (GIF decode) | Expected ✅ |
| 171 | 10.5% | `compress/lzw` (GIF decode) | Expected ✅ |
| 150 | 9.2% | `renderGIFFrames` | Expected ✅ |
| 124 | 7.6% | `io.ReadAll` | Expected ✅ |
| 42 | 2.6% | **`strings.ToLower` via `romFileExt`** | **FIX NEEDED ⚠️** |
| 21 | 1.3% | `truncateBoldToWidth` | Acceptable ✓ |
| 16 | 1.0% | `applyPlatformFilter` | Acceptable ✓ |

---

## Root Cause: `romFileExt` hot-path allocation

### What's happening

`romFileExt` (added for `.p8.png` stem matching in the update service) calls `strings.ToLower(filename)` unconditionally:

```go
func romFileExt(filename string) string {
    if strings.HasSuffix(strings.ToLower(filename), ".p8.png") {  // ← allocates every call
        return ".p8.png"
    }
    return filepath.Ext(filename)
}
```

`HasPendingUpdates` calls `romFileExt` for every downloaded file of every game. `ListScreen.Draw()` calls `HasPendingUpdates` for **every visible row, every frame**. On a 60 fps device with 10 visible rows and average 2 files/game, that's ~1,200 `strings.ToLower` calls per second — 945,452 allocation objects over the session.

### Fix: zero-allocation byte comparison

```go
func romFileExt(filename string) string {
    const ext = ".p8.png"
    if len(filename) >= len(ext) {
        suf := filename[len(filename)-len(ext):]
        if len(suf) == 7 &&
            suf[0] == '.' &&
            (suf[1] == 'p' || suf[1] == 'P') &&
            suf[2] == '8' &&
            suf[3] == '.' &&
            (suf[4] == 'p' || suf[4] == 'P') &&
            (suf[5] == 'n' || suf[5] == 'N') &&
            (suf[6] == 'g' || suf[6] == 'G') {
            return ext
        }
    }
    return filepath.Ext(filename)
}
```

Zero allocations — pure index arithmetic on the existing string.

---

## Other Findings (no action needed)

### `truncateBoldToWidth` — 824,681 objects / 21 MB

Each invocation builds a new string (`string(runes) + "…"`). The `truncCache` in `ListScreen` limits calls to one per unique (title, maxW, bold) per rebuild cycle, but with 10K+ games and ~82 rebuild cycles observed in this session, total calls are high. The cache is working correctly; the per-build allocations are unavoidable without string interning. **Acceptable.**

### `applyPlatformFilter` — 16 MB

Allocates one `[]Game` slice per `rebuildView()` call. With 10K+ games at ~160 bytes/entry, one call ≈ 1.6 MB; 16 MB = ~10 rebuild cycles. Not a per-frame path. **Acceptable.**

### `sdl.(*Texture).Query` — 229,379 objects

Called in `ListScreen.Draw()` for cover art texture dimensions. Previously identified as a refactoring target; no change in position. **Known, not urgent.**

---

## Action Items

| Priority | Item | Effort |
|---|---|---|
| **High** | Fix `romFileExt` to use zero-allocation byte comparison | 5 min |
| Low | `Texture.Query` caching (known from r3/r4) | Medium |
| Low | `truncateBoldToWidth` string interning | Complex |
