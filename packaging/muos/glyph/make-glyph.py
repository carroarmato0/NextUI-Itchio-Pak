#!/usr/bin/env python3
"""Render the muOS menu glyph.

muOS glyphs are greyscale+alpha PNGs drawn as solid silhouettes with detail cut
out as transparency — the frontend tints the opaque pixels to the theme colour,
so anything drawn in outline disappears against a matching background.

The design is deliberately our own: a store tile with a download arrow knocked
out of it. itch.io's own mark is a trademark with no stated licence, and their
press kit asks that it not be redistorted or recoloured, which is exactly what
fitting it into a 26px monochrome silhouette would require.

Run after editing:  python3 packaging/muos/glyph/make-glyph.py
"""

from PIL import Image, ImageDraw

# Rendered large and downscaled on the device to whatever the active theme uses
# (26, 34 and 47 px in the stock theme alone), so it has to stay crisp when
# reduced by roughly 10x.
SIZE = 256
OUT = "packaging/muos/glyph/itchio.png"

S = SIZE / 34.0  # design was laid out on muOS's 34px grid


def px(v):
    return v * S


def main():
    img = Image.new("LA", (SIZE, SIZE), (255, 0))
    d = ImageDraw.Draw(img)

    # Solid tile. Opaque white; the frontend recolours it.
    d.rounded_rectangle(
        [px(3), px(3), px(31), px(31)], radius=px(5.5), fill=(255, 255)
    )

    # Download arrow, knocked out as transparency.
    hole = (255, 0)
    d.rectangle([px(15.1), px(8.5), px(18.9), px(18.5)], fill=hole)
    d.polygon(
        [(px(10.6), px(16.6)), (px(23.4), px(16.6)), (px(17.0), px(24.4))],
        fill=hole,
    )

    # Tray the arrow lands in.
    d.rounded_rectangle(
        [px(10.0), px(25.6), px(24.0), px(27.6)], radius=px(1.0), fill=hole
    )

    img.save(OUT, optimize=True)
    print(f"wrote {OUT} ({SIZE}x{SIZE} {img.mode})")


if __name__ == "__main__":
    main()
