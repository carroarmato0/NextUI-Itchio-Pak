# H700 support — design

**Date:** 2026-08-16
**Branch:** `feature/h700-support` (from `dev`)
**Ships as:** `v1.0.23-rc3`

## Summary

Add `nextui/h700` as a supported target so Itch-io runs on the Anbernic RG XX
family. H700 is a single NextUI platform covering eleven SKUs, added upstream by
[LoveRetro/NextUI#807](https://github.com/LoveRetro/NextUI/pull/807), which is
open at the time of writing.

The work is small in code and large in unknowns: nobody on this project owns an
H700 device, so the design is shaped around what can be proven offline and what
has to be handed to testers with an instrument attached.

## Constraints

- **No hardware.** Every H700-specific claim in this document is derived from
  upstream source or the porting contract, not measured. The design must make
  each derivation falsifiable from a tester's log.
- **Upstream is not merged, but that does not gate the release.** An earlier
  draft of this spec held the RC until H700 landed in mainstream NextUI, on the
  reasoning that testers would otherwise have no firmware. That reasoning was
  wrong: the H700 port ships its own beta releases, people are already running
  them, and the Pak Store works on those devices — so advertising `h700` in
  `pak.json` reaches real testers today. The RC ships when the code is ready.
- **The pak must not ship SDL2 for H700.** NextUI installs its own mali-fbdev
  build into `.system/h700/lib`, and the porting contract forbids bundling a
  generic SDL2 alongside it.

## What H700 is

One `PLATFORM=h700` across eleven SKUs, distinguished by `$DEVICE`:

| `$DEVICE` | Panel |
|---|---|
| `rg35xxplus`, `rg35xxh`, `rg35xxsp`, `rg35xxpro`, `rg40xxv`, `rg40xxh` | 640×480 |
| `rg34xx`, `rg34xxsp`, `rgsp` | 720×480 |
| `rgcubexx` | 720×720 |
| `rg28xx` | 640×480, via `SDL_ROTATION` on a physical 480×640 panel |

AArch64 Cortex-A53 — the same silicon family as tg5040 — on a glibc 2.35
userland. Three host userlands are supported upstream (Anbernic stock, StockMod,
BaseOS), which is why nothing here may assume systemd, `bash`, or a given set of
command-line tools.

## Decisions

### Build: copy the tg5040 binary, do not compile a fourth time

One row in `scripts/targets.sh`:

```
nextui:h700:tg5040:no:nextui/tg5040:2.17
```

Compiled by nobody, copied from the portable tg5040 build, ships no SDL2 — the
same shape as `muos/arm64`.

A dedicated `h700-toolchain` build was considered and rejected **for now**. The
published image (`ghcr.io/loveretro/h700-toolchain:latest`) is a fork of
`tg5040-toolchain`: identical GCC 8.3, identical `SYSROOT` (the TrimUI
`SDK_usr_tg5040_a133p.tgz`, glibc 2.33), identical SDL2_ttf. The only difference
that reaches a build is SDL2 itself, prebaked into `PREFIX_LOCAL=/opt/nextui`
from `JohnnyonFlame/SDL-malifbdev-rot` @ `d4a7d75` plus `sdl2-h700.patch`.

That difference does not reach the shipped artifact. We link
`libSDL2-2.0.so.0` by SONAME and bundle no SDL2, so the device loads NextUI's
own copy whichever library was on the linker command line, and we use no
post-2.26 API. The same binary already runs on muOS against muOS's patched SDL2
2.30 — on this very silicon, since muOS's core devices are RG35XX/RG40XX. So
compiling separately would produce an equivalent binary while costing a second
multi-gigabyte image, a fourth parallel compile, a `PKG_CONFIG_SYSROOT_DIR`
workaround, and — because the Pak Store fetches exactly one file — a **second
binary inside the single pak zip**, roughly +6 MB paid by every TrimUI and Miyoo
user for an H700 benefit that currently rounds to zero.

**Escape hatch.** If H700 ever needs its own binary, the row becomes
`nextui:h700:h700:no::2.17` and `docker/Dockerfile.toolchain` gains, for h700
only, `CGO_CFLAGS=-I/opt/nextui/include/SDL2` and `CGO_LDFLAGS=-L/opt/nextui/lib`.
Those explicit flags are required because the image sets
`PKG_CONFIG_SYSROOT_DIR=$SYSROOT` while putting `/opt/nextui/lib/pkgconfig`
first, so pkg-config would otherwise emit `$SYSROOT/opt/nextui/...` — a path
that does not exist. This trigger and fix go in the `targets.sh` comment so the
option stays cheap instead of being rediscovered.

The `2.17` glibc ceiling is kept even though the device ships 2.35. The h700
sysroot *is* the TG5040 SDK, so the check passes for free, and it insures
against BaseOS shipping something older than Anbernic stock.

### Runtime: resolve SDL2 from `$SYSTEM_PATH/lib` first

`launch.sh` currently probes `/usr/trimui/lib`, `/usr/miyoo/lib`, `/usr/lib`,
`/usr/local/lib` and prepends the first directory holding `libSDL2-2.0.so.0`.
On H700 that matches `/usr/lib` — stock Anbernic's SDL2 **2.0.12**, built with
only the mali and dummy video backends, with no `libSDL2_ttf` beside it. We
would override NextUI's own SDL2 with the wrong library and fail at startup or
render nothing.

`$SYSTEM_PATH/lib` goes to the head of the search list. That is
`.system/h700/lib`, where NextUI installs its bundled `libSDL2*` — authoritative
by definition when present. On tg5040, tg5050 and my355 that directory holds no
`libSDL2` (those platforms use the stock OS's), so the probe falls through and
behaviour is unchanged. **This is the one edit that can break a currently
working device, so it is verified on real TrimUI hardware before merge.**

That verification happened over ADB against real tg5040 and tg5050 hardware
at firmware `NextUI-20260719-0`: `$SYSTEM_PATH/lib` was confirmed to hold no
`libSDL2` on either, so the probe falls through exactly as it did before this
change. It was **not** verified on my355 — no Miyoo Flip was available — so
my355 behaving the same way is assumed rather than checked; `launch.sh`'s
comment on the search order and `scripts/launch_test.sh` both flag this.

Bundled-library selection keys on `$PLATFORM` when set, keeping the
`/usr/miyoo` and `/proc/cpuinfo` probes as fallback — that part is unchanged
and is not h700-specific. What is new: a filesystem check can then override
that selection on any platform, not just h700. If `$SYSTEM_PATH/lib` holds a
*complete* SDL2 pair — both `libSDL2-2.0.so.0` and `libSDL2_ttf-2.0.so.0` —
the bundled directory drops out of `LD_LIBRARY_PATH` entirely, because a
firmware shipping the whole pair makes our copy redundant and the porting
contract asks us not to ship one alongside it. A partial pair (SDL2 without
`libSDL2_ttf`, or vice versa) does not trigger this: the bundled directory
stays, ordered after `$SYSTEM_PATH/lib` on `LD_LIBRARY_PATH`, where it
supplies only whatever the firmware's copy is missing. `h700` is expected to
hit the override in practice, since NextUI's mali-fbdev build ships both
files in `.system/h700/lib`, but the code contains no h700-specific case —
any platform whose firmware starts shipping a complete pair would hit it the
same way. The inherited `$LD_LIBRARY_PATH` continues to be appended rather
than replaced, as the porting contract requires.

`launch.sh` is already `#!/bin/sh` with no bashisms, so the `&>` trap that
catches TrimUI launchers under dash does not apply — but it stays on the review
checklist.

### Firmware: identity only, no new capabilities

`internal/firmware/nextui.go` learns `h700`. Because one platform covers eleven
SKUs, the device label resolves from `$DEVICE` (`rg40xxv` → "Anbernic RG40XX V"),
falling back to `Anbernic H700 (<DEVICE>)` and then `Anbernic H700`. Startup
logs `PLATFORM`, `DEVICE` and `RGXX_MODEL`: with no hardware the log is the only
instrument, and `DEVICE` is the sole thing distinguishing one tester's report
from ten other SKUs.

Nothing else changes. H700's `skeleton/EXTRAS/Emus/h700/` set was compared
against tg5040's and is the same thirty paks, `MGBA` and `P8` included, so every
`Caps` flag stays true: palette, MinUI save formats, save/state sync, GBA
emulator choice, Pico-8 core choice. ROM folder names, palette directories and
shared userdata all follow the existing NextUI layout under `$SYSTEM_PATH`.

Power needs no work. Shutdown already writes `/tmp/poweroff`, which is exactly
what the H700 contract mandates, and suspend already degrades to a clean exit
when `$SYSTEM_PATH/bin/suspend` is absent.

### Input: a third face-button arrangement

`firmware.FaceMapping` is currently two-valued — all-swapped or all-direct.
Neither is correct for H700.

Comparing NextUI's own joystick indices across platforms:

| Platform | `JOY_A` | `JOY_B` | `JOY_X` | `JOY_Y` |
|---|---|---|---|---|
| tg5040 / tg5050 | 1 | 0 | 3 | 2 |
| **h700** | **0** | **1** | 3 | 2 |

On TrimUI we know from hardware that the shell's A arrives as
`CONTROLLER_BUTTON_B` (=1) and its X as `BUTTON_Y` (=3) — a four-for-four match
with the `JOY_*` indices, establishing that SDL's controller index equals
NextUI's `JOY_` index. H700's `platform.h` sets `CODE_A=304` (`BTN_SOUTH`),
`CODE_B=305`, `CODE_Y=306`, `CODE_X=307`, which SDL's evdev classification maps
to `b0`–`b3` in ascending order — consistent with its `JOY_*` values. Applying
the validated model: **A and B are direct, X and Y are swapped.**

`FaceMapping` therefore gains a third value, and `SetFaceMapping` becomes a
small `{A,B,X,Y}` table rather than an if/else — three arrangements is where a
boolean-shaped branch stops paying. X and Y are load-bearing: delete,
unified-naming toggle, clear-filters, keyboard backspace, and API-key edit.

This is a derivation, not a measurement. It is confirmed or refuted by the first
tester log, since `logControllerButton` already records every button press.

### Layout: two predicates, replacing the width test

H700 introduces two panel geometries the app has never rendered: 720×480 and
720×720. `LayoutFor` splits on `w <= 640`, so 720×480 would take the roomy
1024×768 layout on a 480-tall panel — 100-pixel overlay margins and padding
sized for a screen with 60% more height.

`narrowScreenW` also turns out to be consulted **directly at eight sites** — six
in `screen_detail.go`, two in `screen_list.go` — independently of `LayoutFor`.
Height-keying `LayoutFor` alone would leave those eight on the wrong branch, so
the plan going in was a single shared predicate replacing the width test and
all eight call sites:

> **compact** unless the panel is roomy in *both* dimensions — `!(w > 640 && h > 480)`

That single predicate shipped first, then broke: `devshot` rendering every
scene at 720×720 with `--audit` showed footer hints and a right-aligned page
indicator drawn on top of each other, in one colour. 720×720 has ample
*vertical* room, so the single conjunction correctly classed it as roomy —
but roomy means full-length text, and full hints plus the page indicator do
not fit across 720 pixels of *width* regardless of height. One predicate
could not be both "spacing depends on width and height together" and "text
fit depends on width alone" at the same time, because those two things turned
out not to be the same question.

The fix (commit `50208d6`) splits the one predicate into two, replacing all
eight call sites with whichever question they were actually asking:

> **`compact(w, h)`** — spacing only: header/row/footer padding, content gap,
> cover-art column width, overlay margins. Unchanged from the original plan:
> `!(w > 640 && h > 480)`.
>
> **`abbreviate(w)`** — text fit only: footer hints, the QR column width/label.
> Width alone, since horizontal budget doesn't depend on height: `w < 1024`.

| Panel | Device | `compact` | `abbreviate` |
|---|---|---|---|
| 640×480 | Miyoo Flip, most RG XX | yes *(unchanged)* | yes |
| 720×480 | RG34XX, RG34XX SP, RG SP | yes *(the original fix)* | yes |
| 720×720 | RG Cube XX | no | yes *(the correction)* |
| 480×640 | RG28XX, if `SDL_ROTATION` misses our window | yes | yes |
| 1024×768, 1280×720 | TrimUI | no *(unchanged)* | no |

The conjunction in `compact` rather than a plain height test is deliberate: it
keeps RG28XX safe in the case where rotation does not reach us. `abbreviate`
needs no such conjunction — it only ever consulted width.

### Packaging

`pak.json` gains `h700` in `platforms` and a `targets` entry labelled for the
Anbernic RG XX family. The single Pak Store zip is unchanged in shape — it
already carries one portable binary for every NextUI device, and H700 uses that
same binary. The `.pakz` bundle gains `Tools/h700/Itch-io.pak`, with no `lib/`
directory inside it.

## Verification

**Provable offline:**

- `devshot` renders every scene at 720×480 and 720×720 across the palette set
  with `--audit` for contrast. This deliberately departs from CLAUDE.md's
  "render at 1024×768" rule, which exists because pill padding and `LayoutFor`
  constants are fixed pixels — so the rule is amended rather than quietly
  violated, and `palette-audit.sh` covers the new geometries.
- Unit tests: the `compact(w, h)` and `abbreviate(w)` tables across the
  geometries above, `$DEVICE` → label resolution including the two fallbacks,
  the three face arrangements, and structural assertions that the `.pakz`
  contains `Tools/h700/Itch-io.pak` and that no artifact ships `lib/h700/`.

**Verified on hardware we do have:** the `launch.sh` library-order change is
regression-tested on the TrimUI (NextUI) and muOS devices — the log must show
SDL resolving exactly as it does today.

**New diagnostic:** log SDL's *runtime* version at startup. On H700 that single
line separates "loaded NextUI's 2.28.5 from `.system/h700/lib`" from "loaded
stock Anbernic's 2.0.12 from `/usr/lib`" — the failure mode most expected here,
and otherwise invisible in a bug report.

**Left to testers**, each answered by an existing log line: the face-button
arrangement (`logControllerButton`), real panel geometry (`display: WxH`), and
ROM writes landing correctly (`ROMDirs()`). A wrong face mapping is annoying
rather than bricking — both buttons still do something, so a tester can always
exit and send the log.

## Release

Merge to `dev` when complete and regression-tested, then cut `v1.0.23-rc3`. The
RC covers both muOS and H700, continuing the v1.0.23 testing cycle rather than
opening a second one.

No upstream gate. H700 testers are already running the port's own beta firmware,
and the Pak Store works there, so `h700` in `pak.json` reaches them as soon as
the release is published. Their reports are what turn this branch's derived
claims — the face-button arrangement above all — into measured ones.

Docs to update: README's platform table, CLAUDE.md's build-target table and
devshot sizing note, and the `itchio-pak-build` and `itchio-pak-project` skills.

## Out of scope

- HDMI output (1280×720 already falls in the roomy class; nothing is tuned for it).
- Per-SKU behaviour beyond the device label — LEDs, lid sensors and analog
  sticks are not used by this app.
- Any change to muOS support, which already reaches this hardware independently.
