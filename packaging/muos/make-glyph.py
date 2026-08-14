#!/usr/bin/env python3
"""Render the muOS menu glyph.

muOS glyphs are greyscale+alpha PNGs drawn as solid silhouettes with detail cut
out as transparency — the frontend tints the opaque pixels to the theme colour,
so anything drawn in outline disappears against a matching background.

The design is deliberately our own: a download arrow landing in a tray. itch.io's
mark is a trademark with no stated licence, and their press kit asks that it not
be redistorted or recoloured, which is exactly what fitting it into a 22px
monochrome silhouette would require.

Proportions are matched to the glyphs muOS ships rather than chosen by eye. The
stock set draws on a 24x24 viewBox and lands, at 34px, in a ~28x28 box with
about 3px of margin on every side, at roughly a third ink coverage:

    app 28x27 43%   ppsspp 30x23 47%   portmaster 30x24 36%   music 26x23 26%

This one is 28x28 with 3px margins at 33%. The first version was a solid tile at
59% — the same size as the others but visually much heavier, which a tester on an
RG35XX H reported as the icon looking too large. Weight matters as much as
dimensions here, so verify_proportions() keeps that honest.

Run after editing:  python3 packaging/muos/make-glyph.py
"""

from PIL import Image, ImageDraw

# Rendered large and downscaled on the device to whatever the active theme uses
# — 22, 26, 34 and 47 px are all in the wild — so it has to stay crisp when
# reduced by roughly 10x.
SIZE = 256
OUT = "packaging/muos/glyph/itchio.png"

# Laid out on muOS's 34px grid, the size the stock theme renders at 1280x720.
GRID = 34.0

SHAFT_W = 6.0
HEAD_W = 18.4
TRAY_W = 4.2
TRAY_X0, TRAY_X1 = 3.4, 30.6


def render(size=SIZE):
    s = size / GRID

    def p(v):
        return v * s

    img = Image.new("LA", (size, size), (255, 0))
    d = ImageDraw.Draw(img)
    ink = (255, 255)

    # Arrow shaft.
    d.rounded_rectangle(
        [p(17 - SHAFT_W / 2), p(3.4), p(17 + SHAFT_W / 2), p(16.5)],
        radius=p(1.3), fill=ink,
    )
    # Arrowhead.
    d.polygon(
        [(p(17 - HEAD_W / 2), p(14.0)), (p(17 + HEAD_W / 2), p(14.0)), (p(17), p(25.0))],
        fill=ink,
    )
    # Tray: an open-topped U, so the arrow reads as landing in it.
    w = p(TRAY_W)
    d.rounded_rectangle([p(TRAY_X0), p(21.0), p(TRAY_X0) + w, p(30.4)], radius=p(1.3), fill=ink)
    d.rounded_rectangle([p(TRAY_X1) - w, p(21.0), p(TRAY_X1), p(30.4)], radius=p(1.3), fill=ink)
    d.rounded_rectangle([p(TRAY_X0), p(30.4) - w, p(TRAY_X1), p(30.4)], radius=p(1.3), fill=ink)

    return img


def verify_proportions(img):
    """Fail loudly if the glyph drifts out of the range the stock set occupies."""
    a = img.convert("RGBA").resize((34, 34), Image.LANCZOS).split()[-1]
    box = a.point(lambda v: 255 if v > 64 else 0).getbbox()
    w, h = box[2] - box[0], box[3] - box[1]
    ink = sum(1 for v in a.get_flattened_data() if v > 40) / (34 * 34) * 100

    problems = []
    if not (24 <= w <= 30 and 22 <= h <= 30):
        problems.append(f"ink box {w}x{h} outside the stock range (24-30 x 22-30)")
    if min(box[0], box[1], 34 - box[2], 34 - box[3]) < 2:
        problems.append(f"margins {box[0]},{box[1]},{34 - box[2]},{34 - box[3]} too tight (stock keeps 2-6)")
    if not (24 <= ink <= 50):
        problems.append(f"ink coverage {ink:.0f}% outside the stock range (26-47%)")

    if problems:
        raise SystemExit("glyph proportions off:\n  " + "\n  ".join(problems))
    return w, h, ink


def main():
    img = render()
    w, h, ink = verify_proportions(img)
    img.save(OUT, optimize=True)
    print(f"wrote {OUT} ({SIZE}x{SIZE} {img.mode}) — {w}x{h} ink box, {ink:.0f}% coverage at 34px")


if __name__ == "__main__":
    main()
