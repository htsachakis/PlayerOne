#!/usr/bin/env python3
"""Generate PlayerOne's application icons.

The mark is a dark rounded tile holding a play triangle beside a list of rows --
the player and its "In this video" panel, which is the whole idea of the
application in one shape.

Icons are generated rather than checked in as opaque binaries so the design can
be adjusted in one place and every size stays consistent. Run it after changing
anything here:

    python build/branding/make_icons.py

Outputs:
    build/appicon.png          1024x1024, used by Wails and the NSIS installer
    build/windows/icon.ico     multi-resolution 16-256, the executable's icon
    build/branding/logo.png    1024x1024 wordmark lockup, for the README
"""

from __future__ import annotations

import os
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

# Everything is drawn at SCALE times the target size and then reduced, which is
# what gives clean edges without hand-written anti-aliasing.
SCALE = 4
SIZE = 256
CANVAS = SIZE * SCALE

ROOT = Path(__file__).resolve().parents[2]
BUILD = ROOT / "build"

# Palette, matching the application's dark theme.
TILE_TOP = (24, 31, 42)
TILE_BOTTOM = (10, 13, 18)
GLOW = (47, 155, 255)
SCREEN = (13, 17, 23)
PLAY_TOP = (110, 226, 255)
PLAY_BOTTOM = (74, 123, 255)
ROW_DIM = (44, 54, 68)
ROW_ACTIVE = (47, 155, 255)
BAR_TRACK = (48, 58, 72)
BAR_FILL = (47, 123, 255)
KNOB = (60, 200, 255)


def vertical_gradient(size: tuple[int, int], top: tuple[int, int, int],
                      bottom: tuple[int, int, int]) -> Image.Image:
    """Builds a vertical gradient by drawing one line per row."""
    width, height = size
    gradient = Image.new("RGB", (1, height))
    pixels = gradient.load()
    for y in range(height):
        t = y / max(1, height - 1)
        pixels[0, y] = tuple(round(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
    return gradient.resize((width, height), Image.Resampling.BILINEAR)


def rounded_mask(size: tuple[int, int], radius: int) -> Image.Image:
    mask = Image.new("L", size, 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, size[0] - 1, size[1] - 1], radius, fill=255)
    return mask


def draw_icon() -> Image.Image:
    icon = Image.new("RGBA", (CANVAS, CANVAS), (0, 0, 0, 0))

    margin = int(CANVAS * 0.055)
    tile_box = (margin, margin, CANVAS - margin, CANVAS - margin)
    tile_size = (tile_box[2] - tile_box[0], tile_box[3] - tile_box[1])
    radius = int(tile_size[0] * 0.235)

    # The outer glow: the tile silhouette, blurred, tinted blue. Drawn first so
    # everything else sits on top of it.
    glow = Image.new("RGBA", (CANVAS, CANVAS), (0, 0, 0, 0))
    ImageDraw.Draw(glow).rounded_rectangle(tile_box, radius, fill=GLOW + (190,))
    glow = glow.filter(ImageFilter.GaussianBlur(int(CANVAS * 0.026)))
    icon.alpha_composite(glow)

    # The tile itself.
    tile = vertical_gradient(tile_size, TILE_TOP, TILE_BOTTOM).convert("RGBA")
    tile.putalpha(rounded_mask(tile_size, radius))
    icon.alpha_composite(tile, (tile_box[0], tile_box[1]))

    # A bright inner edge, which is what makes the tile read as lit rather than
    # flat at small sizes.
    edge = ImageDraw.Draw(icon)
    edge.rounded_rectangle(tile_box, radius, outline=GLOW + (235,), width=max(2, int(CANVAS * 0.008)))

    inner = int(tile_size[0] * 0.075)
    content = (tile_box[0] + inner, tile_box[1] + inner, tile_box[2] - inner, tile_box[3] - inner)
    content_w = content[2] - content[0]
    content_h = content[3] - content[1]

    # The screen: the left two thirds, holding the play triangle.
    screen_w = int(content_w * 0.62)
    screen_h = int(content_h * 0.74)
    screen_box = (content[0], content[1], content[0] + screen_w, content[1] + screen_h)
    screen_radius = int(screen_w * 0.16)
    ImageDraw.Draw(icon).rounded_rectangle(screen_box, screen_radius, fill=SCREEN + (255,))

    draw_play(icon, screen_box)
    draw_rows(icon, (content[0] + screen_w + int(content_w * 0.055), content[1],
                     content[2], content[1] + screen_h))
    draw_progress(icon, (content[0], content[1] + screen_h + int(content_h * 0.11),
                         content[2], content[3]))

    return icon.resize((SIZE * 4, SIZE * 4), Image.Resampling.LANCZOS)


def draw_play(icon: Image.Image, box: tuple[int, int, int, int]) -> None:
    """The play triangle, filled with a gradient through a triangular mask."""
    x0, y0, x1, y1 = box
    width, height = x1 - x0, y1 - y0

    cx = x0 + width * 0.47
    cy = y0 + height * 0.5
    size = min(width, height) * 0.62

    # A slightly rounded triangle: three points, then a blur-and-threshold pass
    # would over-complicate it, so the corner rounding comes from the supersample.
    points = [
        (cx - size * 0.42, cy - size * 0.56),
        (cx + size * 0.62, cy),
        (cx - size * 0.42, cy + size * 0.56),
    ]

    mask = Image.new("L", icon.size, 0)
    ImageDraw.Draw(mask).polygon(points, fill=255)

    gradient = vertical_gradient(icon.size, PLAY_TOP, PLAY_BOTTOM).convert("RGBA")
    gradient.putalpha(mask)
    icon.alpha_composite(gradient)


def draw_rows(icon: Image.Image, box: tuple[int, int, int, int]) -> None:
    """The side panel: rows of chapter entries, one of them current."""
    x0, y0, x1, y1 = box
    width = x1 - x0
    if width <= 0:
        return

    draw = ImageDraw.Draw(icon)

    count = 5
    active = 2
    gap = (y1 - y0) / count
    row_h = gap * 0.42
    radius = row_h * 0.5

    marker_w = width * 0.26
    line_x = x0 + marker_w + width * 0.12

    for index in range(count):
        top = y0 + gap * index + (gap - row_h) / 2
        colour = ROW_ACTIVE if index == active else ROW_DIM

        draw.rounded_rectangle([x0, top, x0 + marker_w, top + row_h], radius, fill=colour + (255,))
        draw.rounded_rectangle([line_x, top + row_h * 0.22, x1, top + row_h * 0.78],
                               radius * 0.6, fill=colour + (255,))


def draw_progress(icon: Image.Image, box: tuple[int, int, int, int]) -> None:
    """The seek bar across the bottom."""
    x0, y0, x1, y1 = box
    draw = ImageDraw.Draw(icon)

    height = (y1 - y0) * 0.36
    top = y0 + ((y1 - y0) - height) / 2
    radius = height / 2

    draw.rounded_rectangle([x0, top, x1, top + height], radius, fill=BAR_TRACK + (255,))

    played = x0 + (x1 - x0) * 0.46
    draw.rounded_rectangle([x0, top, played, top + height], radius, fill=BAR_FILL + (255,))

    knob = height * 1.9
    draw.ellipse([played - knob / 2, top + height / 2 - knob / 2,
                  played + knob / 2, top + height / 2 + knob / 2], fill=KNOB + (255,))


def write_icons(icon: Image.Image) -> None:
    BUILD.mkdir(parents=True, exist_ok=True)
    (BUILD / "windows").mkdir(parents=True, exist_ok=True)

    appicon = icon.resize((1024, 1024), Image.Resampling.LANCZOS)
    appicon.save(BUILD / "appicon.png")

    # Windows picks the closest size from the icon; supplying every standard one
    # avoids the blurry rescale that a single large entry produces in Explorer.
    sizes = [(n, n) for n in (16, 24, 32, 48, 64, 128, 256)]
    icon.resize((256, 256), Image.Resampling.LANCZOS).save(
        BUILD / "windows" / "icon.ico", format="ICO", sizes=sizes
    )


def write_wordmark(icon: Image.Image) -> None:
    """The mark beside the name, for the README and the installer header."""
    width, height = 1400, 420
    canvas = Image.new("RGBA", (width, height), (13, 17, 23, 255))

    mark = icon.resize((300, 300), Image.Resampling.LANCZOS)
    canvas.alpha_composite(mark, (70, 60))

    draw = ImageDraw.Draw(canvas)
    name_font = load_font(120)
    tag_font = load_font(40)

    draw.text((410, 120), "PlayerOne", font=name_font, fill=(241, 241, 241, 255))
    draw.text((416, 258), "WATCH   LEARN   EXPLORE", font=tag_font, fill=(110, 194, 255, 255))

    canvas.convert("RGB").save(BUILD / "branding" / "logo.png")


def load_font(size: int) -> ImageFont.FreeTypeFont:
    """Finds a usable UI font, falling back to Pillow's built-in bitmap font."""
    candidates = [
        r"C:\Windows\Fonts\segoeuib.ttf",
        r"C:\Windows\Fonts\seguisb.ttf",
        r"C:\Windows\Fonts\arialbd.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    ]
    for path in candidates:
        if os.path.exists(path):
            return ImageFont.truetype(path, size)
    return ImageFont.load_default(size)


def main() -> None:
    icon = draw_icon()
    write_icons(icon)
    write_wordmark(icon)
    print(f"wrote {BUILD / 'appicon.png'}")
    print(f"wrote {BUILD / 'windows' / 'icon.ico'}")
    print(f"wrote {BUILD / 'branding' / 'logo.png'}")


if __name__ == "__main__":
    main()
