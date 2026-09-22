#!/bin/sh
# Single source of truth for what Itch-io builds and ships.
#
# Sourced by build.sh, release.sh, deploy.sh, debug.sh and the Makefile.  Adding
# support for another custom firmware should be a one-row change here plus a
# packaging rule in release.sh — never a hunt through five scripts.
#
# Row format:  firmware:device:toolchain:bundle_sdl:source:max_glibc
#
#   firmware    custom firmware the artifact targets (nextui, muos, ...)
#   device      hardware / library variant within that firmware
#   toolchain   LoveRetro toolchain image code used to compile it
#   bundle_sdl  yes = ship SDL2 .so files harvested from the toolchain sysroot
#               no  = link the firmware's own system SDL2 at runtime
#   source      empty = compile this target
#               <firmware>/<device> = copy that target's binary instead
#   max_glibc   highest glibc symbol version the binary may require, or empty
#               for no constraint.  Only meaningful for binaries that have to
#               run somewhere other than the device they were built for.
#
# Why muos/arm64 is a copy rather than a compile: the ARM64 binary needs only
# GLIBC_2.17, and every muOS device ships glibc >= 2.40 with its own patched
# SDL2 2.30 + SDL2_ttf 2.22.  One binary therefore covers every muOS device, and
# building against the tg5040 toolchain (oldest SDL2 headers, 2.26) gives the
# widest forward compatibility.  Do not "fix" this into a separate compile.
#
# nextui/tg5040 is the portable build: it is the binary shipped inside
# Itch-io.NextUI.*.pak.zip for every NextUI device, and the one muOS copies.  It
# therefore carries a hard GLIBC_2.17 ceiling, enforced by build.sh, so it can
# never quietly acquire a symbol some device's libc lacks.  The tg5050 and my355
# builds only ever run on the hardware they were compiled for, so they are left
# unconstrained — tg5050's toolchain legitimately emits GLIBC_2.32.

# nextui/h700 is a copy for the same reason, plus one of its own.  H700 is
# AArch64 Cortex-A53 on glibc 2.35, so the portable tg5040 binary satisfies it
# with room to spare, and NextUI installs its own mali-fbdev SDL2 in
# .system/h700/lib — the pak must ship none, per the H700 pak porting contract.
#
# A dedicated ghcr.io/loveretro/h700-toolchain image does exist, and its sysroot
# is the same TrimUI SDK (SDK_usr_tg5040_a133p.tgz, glibc 2.33) with the same
# SDL2_ttf.  The only difference that reaches a build is SDL2 itself, prebaked
# into PREFIX_LOCAL=/opt/nextui.  We link libSDL2-2.0.so.0 by SONAME and bundle
# no SDL2, so the device loads its own copy either way, and we use no post-2.26
# API.  Compiling separately would therefore produce an equivalent binary while
# adding a second one to the single zip the Pak Store fetches — roughly +6MB
# paid by every TrimUI and Miyoo user for nothing.
#
# Switch to a real compile when H700 needs an SDL2 API only the fork provides,
# or when a tester finds a divergence this arrangement cannot explain.  Then the
# row becomes:
#     nextui:h700:h700:no::2.17
# and docker/Dockerfile.toolchain needs, for h700 only:
#     ENV CGO_CFLAGS=-I/opt/nextui/include/SDL2
#     ENV CGO_LDFLAGS=-L/opt/nextui/lib
# Those explicit flags are required because the image sets
# PKG_CONFIG_SYSROOT_DIR=$SYSROOT while putting /opt/nextui/lib/pkgconfig first,
# so pkg-config would emit $SYSROOT/opt/nextui/... — a path that does not exist.
# That switch also has to gate build.sh's SDL2 harvest on target_bundles_sdl:
# toolchain_libdir is keyed by toolchain, so a compiled h700 would otherwise
# harvest into an unused lib/h700.  Today every compiled target bundles SDL2 and
# every non-bundling one is a copy, so build_target never sees the case.

TARGETS='nextui:tg5040:tg5040:yes::2.17
nextui:tg5050:tg5050:yes::
nextui:my355:my355:yes::
nextui:h700:tg5040:no:nextui/tg5040:2.17
muos:arm64:tg5040:no:nextui/tg5040:2.17'

# Name of the executable.  "Pak" is a NextUI packaging concept, not part of the
# application's identity, so the binary is plain "itchio" on every firmware.
BIN_NAME="itchio"

# The released version, read from the Pak Store manifest.  pak.json stays the
# single place a version is written even though it is NextUI-flavoured; the
# muOS artifact carries the same string in a plain version.txt.
pak_version() {
    grep '"version"' pak.json | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# _target_row <firmware/device> -> the raw manifest row, or empty if unknown.
_target_row() {
    printf '%s\n' "$TARGETS" | while IFS=: read -r fw dev tc sdl src glibc; do
        [ "$fw/$dev" = "$1" ] && printf '%s:%s:%s:%s:%s:%s\n' "$fw" "$dev" "$tc" "$sdl" "$src" "$glibc"
    done
}

# _target_field <firmware/device> <field-number>
_target_field() {
    _target_row "$1" | cut -d: -f"$2"
}

target_exists()    { [ -n "$(_target_row "$1")" ]; }
target_firmware()  { _target_field "$1" 1; }
target_device()    { _target_field "$1" 2; }
target_toolchain() { _target_field "$1" 3; }
target_source()    { _target_field "$1" 5; }
target_max_glibc() { _target_field "$1" 6; }

# True when this target ships SDL2 alongside the binary.
target_bundles_sdl() { [ "$(_target_field "$1" 4)" = "yes" ]; }

# All targets, in manifest order: "nextui/tg5040 nextui/tg5050 ... muos/arm64"
all_targets() {
    printf '%s\n' "$TARGETS" | while IFS=: read -r fw dev _tc _sdl _src _g; do
        printf '%s/%s\n' "$fw" "$dev"
    done
}

# Targets belonging to one firmware, e.g. targets_for nextui
targets_for() {
    printf '%s\n' "$TARGETS" | while IFS=: read -r fw dev _tc _sdl _src _g; do
        [ "$fw" = "$1" ] && printf '%s/%s\n' "$fw" "$dev"
    done
}

# Every firmware named in the manifest, deduplicated.
all_firmwares() {
    printf '%s\n' "$TARGETS" | cut -d: -f1 | sort -u
}

# Targets that are actually compiled (source field empty).
compiled_targets() {
    printf '%s\n' "$TARGETS" | while IFS=: read -r fw dev _tc _sdl src _g; do
        [ -z "$src" ] && printf '%s/%s\n' "$fw" "$dev"
    done
}

# Toolchain images that must exist before a full build, deduplicated.
all_toolchains() {
    printf '%s\n' "$TARGETS" | while IFS=: read -r fw dev tc _sdl src _g; do
        [ -z "$src" ] && printf '%s\n' "$tc"
    done | sort -u
}

# Where a target's binary lives in the repo.
target_bindir() { printf 'bin/%s\n' "$1"; }
target_binary() { printf 'bin/%s/%s\n' "$1" "$BIN_NAME"; }

# Where a toolchain's harvested SDL2 libraries live.  Keyed by toolchain rather
# than firmware: the .so files are a property of the sysroot they came from, and
# the shipped pak layout (lib/<device>/) mirrors this directly.
toolchain_libdir() { printf 'lib/%s\n' "$1"; }
