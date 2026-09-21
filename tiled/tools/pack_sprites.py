#!/usr/bin/env python3
"""Turn a generated character sheet into the tile sheet cmd/devview reads.

The art for this viewer is generated, and a generator returns a picture of
pixel art rather than pixel art: one body in the sheet this was written for
holds 32,718 colours and not one row or column of it repeats. So nothing here
extracts anything. The render is resampled down to the resolution the art was
drawn at - about one art pixel per 13.7 - and the noise averages out on the
way down.

Three rules, each of them measured on a real sheet rather than guessed:

  - The background is keyed on hue, not on brightness. An earlier sheet had a
    white-haired head whose pixels were 55% "near white"; keying on brightness
    eats white hair and pale skin along with the background.
  - Frames of one clip are scaled together, never each to its own bounding
    box. Two frames of a standing body differ by a pixel in 460, and
    normalising each on its own turns that into a body that grows and shrinks
    as it breathes.
  - A clip that is a body standing on the ground is normalised to one height.
    The generator drifts towards chibi proportions down the sheet - measured
    at 90, 83, 81, 79 pixels tall for standing, walking, punching and being
    punched, with the head going the other way, 52 to 56 - and a body that
    shrinks by a tenth when a fight starts is the one artefact of that drift
    the eye cannot miss. Poses that are meant to be lower (sitting, wading,
    a child, a corpse) keep their drawn proportion instead.

What this file does not do is decide anything about the simulation. It knows
which picture goes in which cell of a sheet; which body gets which picture is
cmd/devview/tiles.go, and the engine has never heard of either.

Usage:
    python3 tiled/tools/pack_sprites.py SHEET.png OUT_DIR --layout NAME [--carry DIR]

SHEET.png is a generated sheet, laid out as docs/sprites.md asks for it: one
clip per row, a standing reference body alone in the leftmost column of every
row. --layout names the table that says what is in it. OUT_DIR gets
tiles.<hash>.png and manifest.json. --carry names a directory holding an
existing sheet and manifest, and any clip in it that this sheet does not
provide is copied across - which is how several sheets and the grey
placeholders end up in one texture, by running this once per sheet and
carrying the last result forward.
"""

import argparse
import hashlib
import json
import os
import sys

try:
    import numpy as np
    from PIL import Image
    from scipy import ndimage
except ImportError as exc:  # pragma: no cover - a developer's tool, not a build step
    sys.exit("pack_sprites needs numpy, pillow and scipy: %s" % exc)

TILE = 32   # one cell of the sheet
BODY = 30   # how tall a standing adult is inside its cell
FEET = 1    # rows left under the feet, so a body is not flush with the edge

# Which bodies of which row make which clip, per sheet.
#
# A table belongs to the sheet it was read off: a fresh render has its own
# rows, its own drift and its own idea of how big things are, so whoever packs
# one runs --dump first and checks it against this. --layout picks the table.
#
# The fourth field is how the clip is scaled, and the two sheets need
# different answers because the two subjects are shaped differently.
#
# A person is taller than wide and stands on the ground, so what has to match
# between clips is height: a body that shrank a tenth when a fight started
# would be the one artefact of the generator's drift nobody could miss. A
# number there is the height, in that render, of a standing body in that row.
# None means the pose is meant to be lower - sitting, wading, a child, a
# corpse - and keeps its drawn proportion against a standing adult.
#
# A beast is wider than tall, and its size on screen does not come from its
# picture at all: the viewer reads that off the budget its body was drawn
# from, so a brute is drawn big because it IS big. Making the art half again a
# person as well would count the same fact twice. So a beast's clips are
# scaled to fill the cell the same way, by a name shared with every other clip
# of the same build - one scale for the build, taken from its widest frame, so
# that nothing changes size between standing and pouncing.
HUMANS = [
    ("human.idle",    0, [0, 1], 90),
    ("human.walk",    1, [0, 1], 83),
    ("human.fight",   3, [0, 1], 81),
    ("human.hurt",    4, [0, 1], 79),
    ("human.eat",     2, [0, 1], None),
    ("human.swim",    5, [0, 1], None),
    ("human.dead",    6, [0, 1], None),
    ("human.f.idle",  0, [4, 5], 90),
    ("human.f.walk",  1, [4, 5], 83),
    ("human.f.fight", 3, [4, 5], 80),
    ("human.f.hurt",  4, [4, 5], 78),
    ("human.f.eat",   2, [4, 5], None),
    ("human.f.swim",  5, [4, 5], None),
    ("child.idle",    7, [0, 1], None),
    ("child.f.idle",  7, [4, 5], None),
    # The old stand like everyone else, so they are normalised like everyone
    # else. The render had the old man at 82 and the old woman at 92 against
    # an adult's 90, and an old woman taller than every other body in the
    # world is a worse lie than an old man who has not shrunk. Age is said by
    # the white hair and the bald head, which nothing else in the sheet says.
    ("old.idle",      8, [0, 1], 82),
    ("old.f.idle",    8, [4, 5], 92),
    ("hair",          9, [0, 1, 2, 3, 4, 5, 6], 90),
    ("item",         10, [0, 1, 2, 3, 4, 5, 6, 7], None),
]

# The beasts. Four builds across every row, in the order the brief asked for
# them: heavy, light, water, winged. The row of a body being struck was asked
# for and did not come back, which costs nothing because nothing asks for it,
# and the two rows of a winged one in the air did, which nothing asks for
# either - they wait in the sheet the way the humans' spare poses do.
ENEMIES = [
    ("enemy.big.idle",    0, [0, 1], "big"),
    ("enemy.big.walk",    1, [0, 1], "big"),
    ("enemy.big.eat",     2, [0, 1], "big"),
    ("enemy.big.fight",   3, [0, 1], "big"),
    ("enemy.big.dead",    4, [0, 1], "big"),
    ("enemy.small.idle",  0, [2, 3], "small"),
    ("enemy.small.walk",  1, [2, 3], "small"),
    ("enemy.small.eat",   2, [2, 3], "small"),
    ("enemy.small.fight", 3, [2, 3], "small"),
    ("enemy.small.dead",  4, [2, 3], "small"),
    ("enemy.water.idle",  0, [4, 5], "water"),
    ("enemy.water.walk",  1, [4, 5], "water"),
    ("enemy.water.eat",   2, [4, 5], "water"),
    ("enemy.water.fight", 3, [4, 5], "water"),
    ("enemy.water.dead",  4, [4, 5], "water"),
    ("enemy.fly.idle",    0, [6, 7], "fly"),
    ("enemy.fly.walk",    1, [6, 7], "fly"),
    ("enemy.fly.eat",     2, [6, 7], "fly"),
    ("enemy.fly.fight",   3, [6, 7], "fly"),
    ("enemy.fly.dead",    4, [6, 7], "fly"),
    # In the air, which nothing draws yet. Its own scale because a spread
    # wing is half again as wide as a folded one, and sharing the ground
    # build's scale would push it off both sides of its cell.
    ("enemy.fly.air",     5, [0, 1], "air"),
    ("enemy.fly.flap",    5, [2, 3], "air"),
    ("remains",           6, [0],    "bone"),
]

LAYOUTS = {"humans": HUMANS, "enemies": ENEMIES}

# The height a standing adult is drawn at in the human render, which every
# clip of it that is not normalised in its own right is measured against.
STANDING = 90.0


# --- reading the render ----------------------------------------------------

def key(a):
    """Foreground mask. The background is magenta: red and blue up, green down."""
    r, g, b = a[:, :, 0], a[:, :, 1], a[:, :, 2]
    return ~((r > 175) & (b > 175) & (g < 120))


def despill(rgb, fg):
    """Take the background's colour back out of the edges it bled into.

    The key is all or nothing, so a pixel the edge only half covers keeps a
    pink cast, and a pink cast reads as a halo once the body is on a dark map.
    Where green is the smallest channel by a wide margin the pixel is part
    background, and capping the other two near green takes the cast out
    without touching anything that is honestly pink - a dress, a mouth.
    """
    out = rgb.astype(float).copy()
    r, g, b = out[:, :, 0], out[:, :, 1], out[:, :, 2]
    spill = fg & (r > g + 40) & (b > g + 40)
    out[:, :, 0] = np.where(spill, np.minimum(r, g + 40), r)
    out[:, :, 2] = np.where(spill, np.minimum(b, g + 40), b)
    return out


def load(path):
    src = Image.open(path).convert("RGB")
    a = np.asarray(src).astype(int)
    fg = key(a)
    rgba = np.dstack([despill(a, fg), np.where(fg, 255, 0)]).astype("uint8")
    return Image.fromarray(rgba), fg


def runs(v, least=1):
    out, start = [], None
    for i, c in enumerate(v):
        if c > 0 and start is None:
            start = i
        elif c == 0 and start is not None:
            out.append((start, i))
            start = None
    if start is not None:
        out.append((start, len(v)))
    return [r for r in out if r[1] - r[0] >= least]


def rows_of(fg):
    """Find the rows, and where the bodies in them start, from the reference column.

    The leftmost thing in the picture is the column of reference bodies, one
    per row, which is what makes the rows findable without anybody typing
    coordinates: they are wherever that column has something in it. The bands
    are cut halfway between one reference and the next rather than at the
    reference's own edges, because a row's bodies can stand taller than the
    reference beside them.
    """
    columns = runs(fg.sum(0), least=10)
    if not columns:
        raise SystemExit("nothing in this picture")
    x0, x1 = columns[0]
    refs = runs(fg[:, x0:x1].sum(1), least=10)
    bands, height = [], fg.shape[0]
    for i, (top, bottom) in enumerate(refs):
        up = 0 if i == 0 else (refs[i - 1][1] + top) // 2
        down = height if i == len(refs) - 1 else (bottom + refs[i + 1][0]) // 2
        bands.append((up, down, bottom - top))
    return bands, x1


def bodies(fg, band, x_from, least=200):
    """Bounding boxes of the bodies in one band, left to right.

    Dilated before labelling so that the loose parts of a body - the marks
    flying off a punch, the splash around a waist - come away with the body
    they belong to instead of as sprites of their own.
    """
    up, down = band
    strip = fg[up:down, x_from:]
    labels, _ = ndimage.label(ndimage.binary_dilation(strip, np.ones((3, 7))))
    out = []
    for where in ndimage.find_objects(labels):
        ys, xs = where
        rows_, _ = np.nonzero(strip[ys, xs])
        if len(rows_) < least:
            continue
        out.append((xs.start + x_from, up + ys.start + rows_.min(),
                    xs.stop + x_from, up + ys.start + rows_.max() + 1))
    out.sort()
    return out


# --- writing the sheet -----------------------------------------------------

def cell(img, box, scale):
    """One body, scaled and dropped into its cell with its feet on the floor.

    Capped by width as well as height because a body lying down is wider than
    a cell at the scale a standing one fits: it is 36 to 38 pixels across when
    a standing body is 30 tall. Nothing reads a corpse's size, so the corpse
    gives up the tenth rather than the grid give up its square cells.

    Feet on the floor rather than centred: a body centred in its cell rises
    off the ground whenever a pose changes its outline.
    """
    c = img.crop(box)
    w, h = round(c.width * scale), round(c.height * scale)
    if w > TILE:
        h = max(1, round(h * TILE / w))
        w = TILE
    if h > TILE - FEET:
        w = max(1, round(w * (TILE - FEET) / h))
        h = TILE - FEET
    small = c.resize((max(1, w), max(1, h)), Image.BOX)
    out = Image.new("RGBA", (TILE, TILE), (0, 0, 0, 0))
    out.paste(small, ((TILE - small.width) // 2, TILE - FEET - small.height), small)
    return out


def carried(directory):
    """Clips from an existing sheet, by name, as (image, clip) pairs.

    Art nobody has redrawn yet - the grey placeholders the enemies are still
    drawn with - lives here. It comes across at whatever size it already is:
    a clip carries its own w and h, and the viewer scales a body to the size
    it should be on screen, so a 16-pixel clip and a 32-pixel one sit in the
    same sheet and draw the same size.
    """
    manifest = json.load(open(os.path.join(directory, "manifest.json")))
    sheet = Image.open(os.path.join(directory, manifest["sheet"])).convert("RGBA")
    out = {}
    for c in manifest["clips"]:
        frames = [sheet.crop((c["x"] + i * c["w"], c["y"],
                              c["x"] + (i + 1) * c["w"], c["y"] + c["h"]))
                  for i in range(c["frames"])]
        out[c["name"]] = (frames, c["w"], c["h"])
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("sheet")
    ap.add_argument("out_dir")
    ap.add_argument("--layout", default="humans", choices=sorted(LAYOUTS),
                    help="which table says what is in this sheet")
    ap.add_argument("--carry", help="a directory whose unclaimed clips come across")
    ap.add_argument("--dump", action="store_true",
                    help="print what was found and stop, for checking the table")
    args = ap.parse_args()
    picks = LAYOUTS[args.layout]

    img, fg = load(args.sheet)
    bands, x_from = rows_of(fg)
    found = [bodies(fg, (up, down), x_from) for up, down, _ in bands]

    if args.dump:
        for i, (row, (_, _, ref)) in enumerate(zip(found, bands)):
            print("row %2d  reference %3d  bodies %2d  wxh %s"
                  % (i, ref, len(row),
                     [(b[2] - b[0], b[3] - b[1]) for b in row]))
        return

    # A build's scale, for the clips that share one: the widest frame any of
    # them uses, so every pose of that build fits its cell and none of them is
    # shrunk again by fit() and left smaller than its neighbours.
    widest = {}
    for name, row, frames, how in picks:
        if not isinstance(how, str):
            continue
        if row >= len(found):
            continue
        for i in frames:
            if i < len(found[row]):
                x0, _, x1, _ = found[row][i]
                widest[how] = max(widest.get(how, 0), x1 - x0)

    made = []
    for name, row, frames, how in picks:
        if row >= len(found):
            sys.exit("%s wants row %d and the sheet has %d" % (name, row, len(found)))
        if isinstance(how, str):
            scale = TILE / widest[how]
        else:
            scale = BODY / (how if how else STANDING)
        made.append((name, [cell(img, found[row][i], scale) for i in frames], TILE, TILE))

    claimed = {name for name, _, _, _ in made}
    if args.carry:
        for name, (frames, w, h) in sorted(carried(args.carry).items()):
            if name not in claimed:
                made.append((name, frames, w, h))

    width = max(len(f) * w for _, f, w, _ in made)
    height = sum(h for _, _, _, h in made)
    sheet = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    clips, y = [], 0
    for name, frames, w, h in made:
        for i, f in enumerate(frames):
            sheet.paste(f, (i * w, y))
        clips.append({"name": name, "x": 0, "y": y, "w": w, "h": h,
                      "frames": len(frames),
                      # Grey art has to be given a colour to be visible at
                      # all; drawn art must not be, because multiplying a
                      # drawn body by a colour turns it into a stain.
                      "tint": w != TILE})
        y += h

    os.makedirs(args.out_dir, exist_ok=True)
    raw = os.path.join(args.out_dir, "tiles.tmp.png")
    sheet.save(raw)
    digest = hashlib.sha256(open(raw, "rb").read()).hexdigest()[:8]
    final = os.path.join(args.out_dir, "tiles.%s.png" % digest)
    for old in os.listdir(args.out_dir):
        if old.startswith("tiles.") and old.endswith(".png"):
            os.remove(os.path.join(args.out_dir, old))
    sheet.save(final)
    with open(os.path.join(args.out_dir, "manifest.json"), "w") as f:
        json.dump({"sheet": os.path.basename(final), "tile": TILE, "clips": clips},
                  f, indent=2)
        f.write("\n")
    print("%s  %dx%d  %d clips  %d frames"
          % (os.path.basename(final), width, height, len(clips),
             sum(c["frames"] for c in clips)))


if __name__ == "__main__":
    main()
