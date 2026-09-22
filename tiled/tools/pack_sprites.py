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

The viewer keeps several sets of art side by side and switches between them,
so the last OUT_DIR of a run is cmd/devview/assets/<set name> rather than
assets itself, and the name goes in assets/sets.json. An existing set is a
perfectly good --carry: building the next one on top of the last is how a
redraw of half the sheets keeps the half nobody redrew.
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

# The young and the old, from a sheet of their own. Four bodies across every
# row - a boy, a girl, an old man, an old woman - and six rows, which is what
# the first sheet was missing: it had them standing and nothing else, so a
# child that started walking turned into an adult drawn small.
#
# Two things had to be decided here rather than measured.
#
# The old are normalised to a standing ADULT, not to what they were drawn at.
# This render draws them at a child's size - the old man 115 against the boy's
# 112 and the reference adult's 141 - and a bald child is a worse lie than an
# old man who has not stooped. Age is said by the white hair and the bald
# head, which is what the first sheet decided too, and for the same reason.
#
# The children keep the proportion they were drawn at, 0.80 of the reference
# adult, which is the only thing in the sheet that says "child" at all once
# the picture is 24 pixels tall.
#
# Everything else is the drift, measured: one body is drawn 7% taller walking
# than standing and 6% shorter eating, and the numbers below are what a
# standing adult measures on each clip's own scale, which is the reference
# (141) moved by however much that row was drawn bigger or smaller. Sitting
# and wading keep their drawn proportion against the same body standing, so
# they are given the standing number and come out lower.
CHILDREN = [
    ("child.idle",     0, [0, 1], 141),
    ("child.walk",     1, [0, 1], 152),
    ("child.eat",      2, [0, 1], 141),
    ("child.fight",    3, [0, 1], 143),
    ("child.hurt",     4, [0, 1], 144),
    ("child.swim",     5, [0, 1], 141),
    ("child.f.idle",   0, [2, 3], 141),
    ("child.f.walk",   1, [2, 3], 147),
    ("child.f.eat",    2, [2, 3], 141),
    ("child.f.fight",  3, [2, 3], 138),
    ("child.f.hurt",   4, [2, 3], 141),
    ("child.f.swim",   5, [2, 3], 141),
    ("old.idle",       0, [4, 5], 116),
    ("old.walk",       1, [4, 5], 118),
    ("old.eat",        2, [4, 5], 116),
    ("old.fight",      3, [4, 5], 110),
    ("old.hurt",       4, [4, 5], 109),
    ("old.swim",       5, [4, 5], 116),
    ("old.f.idle",     0, [6, 7], 129),
    ("old.f.walk",     1, [6, 7], 135),
    ("old.f.eat",      2, [6, 7], 129),
    ("old.f.fight",    3, [6, 7], 119),
    ("old.f.hurt",     4, [6, 7], 122),
    ("old.f.swim",     5, [6, 7], 129),
]

# The spirits (2026-09-22). A sheet of one cast drawn white, with the colour
# left out on purpose: these bodies are tinted as they are stamped, so that
# which line of descent a body belongs to is something the viewer can say and
# the art does not have to. The people before them could not be tinted at all
# - brown hair and skin share a hue exactly, so multiplying by a colour
# stains the body rather than colouring it - and this sheet came back with a
# saturation of 6 in the median. White with a dark outline multiplies into
# any colour there is.
#
# Two things the art says instead of colour, because colour is spoken for:
# the sex is the shape of the head (ears against a flame), and age is the
# shape of the body (this sheet is the grown ones; the young and the old are
# a sheet of their own).
#
# Every clip is scaled so that the two sexes end the same height overall -
# the tuft counts, the way hair did. The numbers are the drawn height of the
# body in that row, so walking, striking and being struck come out at the
# standing height even though the spirit crouches a little to do them: a body
# that shrinks a tenth when a fight starts is the artefact the eye cannot
# miss, and this sheet draws the walk 7% shorter than the stand. The numbers
# are the body's own height in the render, not the box's: what matters is
# what comes out 30 pixels tall, and a pose whose box holds a mark as well as
# a body (a punch, a blow landing) is taller in the render than the body in
# it is. Eating,
# wading and dying keep the standing number instead, so they come out as low
# as they were drawn, which is what they are for.
SPIRITS = [
    ("human.idle",    0, [0, 1], 119),
    ("human.walk",    1, [0, 1], 111),
    ("human.eat",     2, [0, 1], 119),
    ("human.fight",   3, [0, 1], 117),
    ("human.hurt",    4, [0, 1], 117),
    ("human.swim",    5, [0, 1], 119),
    ("human.dead",    6, [0, 1], 119),
    ("human.f.idle",  0, [2, 3], 134),
    ("human.f.walk",  1, [2, 3], 129),
    ("human.f.eat",   2, [2, 3], 134),
    ("human.f.fight", 3, [2, 3], 129),
    ("human.f.hurt",  4, [2, 3], 121),
    ("human.f.swim",  5, [2, 3], 134),
    # Not bodies: the light that says whose line a body belongs to. Scaled
    # to fill the cell like a beast, because what it has to match is the
    # cell rather than a height, and the viewer draws it larger than the
    # body it goes behind.
    ("aura.ring",     7, [0, 1], "aura"),
    ("aura.motes",    7, [2, 3], "aura"),
    ("aura.ripple",   7, [4, 5], "aura"),
]

# The young and the old of the spirits, from a sheet of their own. Five
# groups across every row, and the first of them - a grown one - is swallowed
# whole as the reference column, because its two frames were drawn close
# enough to read as one block. That is no loss: the grown ones came in the
# sheet before this.
#
# Eating did not arrive. Nothing is put in its place here: what is asked for
# and not drawn falls back to the nearest thing the set has (standIn), which
# for a child is the grown spirit's own eating pose drawn small - and that is
# this cast rather than the brown-haired people the old sets are made of. See
# --drop, which is how those are kept out.
#
# All four are normalised to their own standing height, so all four come out
# the same size and the viewer does the shrinking (AgeFactor). That is what
# the brief asked for and what they drew: the boy 0.91 of the grown one, the
# old man 0.89, the old woman 0.99, and the girl 1.09 - the tuft counts, and
# without this she would be the tallest body in the world.
SPIRIT_YOUNG = [
    ("child.idle",     0, [0, 1], 116),
    ("child.walk",     1, [0, 1], 116),
    ("child.fight",    2, [0],    116),
    ("child.hurt",     3, [0, 1], 116),
    ("child.swim",     4, [0, 1], 116),
    ("child.f.idle",   0, [2, 3], 140),
    ("child.f.walk",   1, [2, 3], 140),
    ("child.f.fight",  2, [1],    140),
    ("child.f.hurt",   3, [2, 3], 140),
    ("child.f.swim",   4, [2, 3], 140),
    ("old.idle",       0, [4, 5], 114),
    ("old.walk",       1, [4, 5], 114),
    ("old.fight",      2, [2],    114),
    ("old.hurt",       3, [4, 5], 114),
    ("old.swim",       4, [4, 5], 114),
    ("old.f.idle",     0, [6, 7], 127),
    ("old.f.walk",     1, [6, 7], 127),
    ("old.f.fight",    2, [3],    127),
    ("old.f.hurt",     3, [6, 7], 127),
    ("old.f.swim",     4, [6, 7], 127),
]

# The one row of the young and the old that did not arrive with the rest:
# eating. Asked for again on its own, with the standing row beside it, which
# is the only way a sheet of one row can be scaled at all - the numbers below
# are that sheet's own standing heights, and they are not the other sheet's
# (this render drew everything half again as large).
SPIRIT_EAT = [
    ("child.eat",   1, [0, 1], 147),
    ("child.f.eat", 1, [2, 3], 177),
    ("old.eat",     1, [4, 5], 150),
    ("old.f.eat",   1, [6, 7], 177),
]

# The beasts of the same world. Four builds across every row in the order the
# brief asked for them - heavy, light, water, winged - and this time the row
# of one being struck arrived, which it had not in the sheet before.
#
# Scaled by build, like the beasts always have been: what is on the screen is
# read off the budget the body was drawn from, so every build fills its cell
# the same way and the difference between them comes from the world.
SPIRIT_BEASTS = [
    ("enemy.big.idle",    0, [0, 1], "big"),
    ("enemy.big.walk",    1, [0, 1], "big"),
    ("enemy.big.eat",     2, [0, 1], "big"),
    ("enemy.big.fight",   3, [0, 1], "big"),
    ("enemy.big.hurt",    4, [0, 1], "big"),
    ("enemy.big.dead",    5, [0, 1], "big"),
    ("enemy.small.idle",  0, [2, 3], "small"),
    ("enemy.small.walk",  1, [2, 3], "small"),
    ("enemy.small.eat",   2, [2, 3], "small"),
    ("enemy.small.fight", 3, [2, 3], "small"),
    ("enemy.small.hurt",  4, [2, 3], "small"),
    ("enemy.small.dead",  5, [2, 3], "small"),
    ("enemy.water.idle",  0, [4, 5], "water"),
    ("enemy.water.walk",  1, [4, 5], "water"),
    ("enemy.water.eat",   2, [4, 5], "water"),
    ("enemy.water.fight", 3, [4, 5], "water"),
    ("enemy.water.hurt",  4, [4, 5], "water"),
    ("enemy.water.dead",  5, [4, 5], "water"),
    ("enemy.fly.idle",    0, [6, 7], "fly"),
    ("enemy.fly.walk",    1, [6, 7], "fly"),
    ("enemy.fly.eat",     2, [6, 7], "fly"),
    ("enemy.fly.fight",   3, [6, 7], "fly"),
    ("enemy.fly.hurt",    4, [6, 7], "fly"),
    ("enemy.fly.dead",    5, [6, 7], "fly"),
    ("enemy.fly.air",     6, [0, 1], "air"),
    ("enemy.fly.flap",    6, [2, 3], "air"),
    ("remains",           7, [0],    "bone"),
]

LAYOUTS = {"humans": HUMANS, "enemies": ENEMIES, "children": CHILDREN,
           "spirits": SPIRITS, "spirit_young": SPIRIT_YOUNG,
           "spirit_eat": SPIRIT_EAT, "spirit_beasts": SPIRIT_BEASTS}

# How many things are in each row, for the sheets where morphology cannot
# say.
#
# bodies() glues what is drawn loose - the marks off a punch - to the body it
# came from, by dilating before labelling. That works while the loose part is
# a few pixels away from a body, and it does not work for this sheet: the
# spirits scatter into motes when they die and the light is nothing BUT
# motes, so one row came back as sixteen things. The row knows how many it
# holds, though, so the blobs are cut into that many groups at the widest
# gaps between them and each group becomes one box. Bigger dilation would
# have been the other way and it is the wrong one: enough to gather a cloud
# of motes is enough to gather the body beside it.
# Which layouts draw art that is MEANT to be coloured as it is stamped.
#
# The rule this replaces was the size of the clip: 16 pixels meant a grey
# placeholder and 32 meant a drawn body, and a drawn body must never be
# multiplied by a colour because it stains rather than tints. The spirits
# broke that rule by being drawn at 32 and drawn white, on purpose, so that
# the viewer can say with colour what the art deliberately does not: whose
# line a body belongs to.
TINTED = {"spirits", "spirit_young", "spirit_eat"}

# The beasts are turned inside out instead: dark where the art is white, and
# a bright edge where the art is dark.
#
# They are not coloured at the moment of drawing, because nothing about a
# beast is a line of descent to say - and because two casts drawn the same
# way and told apart only by colour would be two casts told apart by nothing
# at all on a screen where colour already means kin. Dark with a bright rim
# is the other direction entirely: it reads as a thing that is not made of
# the same light as the people, at any size, without a colour of its own.
SHADOW = {"spirit_beasts": ("enemy.", (30, 27, 36), (198, 206, 224))}

COUNTS = {
    "spirits": [4, 4, 4, 4, 4, 4, 4, 6],
    # Striking is one frame per body on this sheet rather than two, which is
    # why its row is four and not eight.
    "spirit_young": [8, 8, 4, 8, 8],
    "spirit_eat": [8, 8],
    # The last row is one thing that is nothing but motes, so it says one.
    "spirit_beasts": [8, 8, 8, 8, 8, 8, 4, 1],
}

# --- the ground ------------------------------------------------------------
#
# A tile is not a body and almost nothing above applies to it. A body is cut
# out of its background, normalised against the height of a standing adult and
# dropped into its cell with its feet on the floor; a tile IS its cell, edge to
# edge, and what it has to match is not the other tiles but the grid. So the
# ground has a path of its own: find the square, cut the magenta fringe off it,
# and resample it to the cell. Nothing is keyed out and nothing is normalised,
# because there is no figure and no ground - the picture is the ground.
#
# Three things measured on the sheet these tables were written for:
#
#   - The tiles came back 147 to 184 pixels square, differing by row. It does
#     not matter: each one is resampled to the cell on its own, and unlike a
#     body it has no neighbour to be compared against. What differs by 22% is
#     the size of the grain - a pebble, a leaf - and at 32 pixels that is a
#     pebble either way.
#   - The magenta bleeds about 3 pixels into every tile (10 to 26% of the
#     outermost ring was pink). Trimming a fixed 3% of the tile takes it off
#     whatever size the tile came back at.
#   - The generator will not draw two frames of one cell. Asked for "two
#     frames", it draws two pictures: the first water sheet came back with a
#     boulder in one frame and not the other. Asked to copy the frame and move
#     only the foam, it complied - the difference in composition between
#     frames fell from 11.0 to 4.7 (measured blurred, so that only the layout
#     counts and the foam does not). That is why the water
#     is packed from a second sheet.
TERRAIN = [
    ("ground.flat.a",     0, [0], None),
    ("ground.flat.b",     0, [1], None),
    ("ground.flat.c",     0, [2], None),
    ("ground.rough.a",    1, [0], None),
    ("ground.rough.b",    1, [1], None),
    ("ground.rough.c",    1, [2], None),
    # Two frames of one cell, alternated. The deep pair is the one that came
    # back as an animation rather than as two pictures.
    ("ground.water.a",    2, [0, 1], None),
    ("ground.water.b",    2, [2, 3], None),
    ("ground.bank.side",  3, [0], None),
    ("ground.bank.outer", 3, [1], None),
    ("ground.bank.inner", 3, [2], None),
    # The steps run across the climb, so turning the tile turns the ramp. The
    # fourth is the one drawn without a direction, for a cell where the map
    # does not say which way is up.
    ("ground.slope.a",    4, [0], None),
    ("ground.slope.b",    4, [1], None),
    ("ground.slope.c",    4, [2], None),
    ("ground.slope.free", 4, [3], None),
    ("ground.high.a",     5, [0], None),
    ("ground.high.b",     5, [1], None),
    ("ground.high.c",     5, [2], None),
    # What a body cannot climb: the trunk of a great tree and the head of a
    # boulder. They are faces of a block several cells across, so they are
    # drawn to carry on past their own edges, and the rim of the block is the
    # roots below.
    ("ground.block.a",    6, [0], None),
    ("ground.block.b",    6, [1], None),
    ("ground.block.rock", 6, [2], None),
    ("ground.cliff.side", 7, [0], None),
    ("ground.cliff.outer", 7, [1], None),
    ("ground.cliff.inner", 7, [2], None),
    # The same three pictures again, in the floor's light rather than the
    # ledge's. They are what lies at the foot of a boulder, which stands on
    # the floor: rimming one in the ledge's pale green put a bright halo
    # round every rock in the world. One drawing, two lights - which is the
    # whole of what PALETTE does - rather than another row to ask for.
    ("ground.scree.side", 7, [0], None),
    ("ground.scree.outer", 7, [1], None),
    ("ground.scree.inner", 7, [2], None),
    ("ground.root.side",  8, [0], None),
    ("ground.root.outer", 8, [1], None),
    ("ground.root.inner", 8, [2], None),
    ("ground.nest",       9, [0], None),
]

# The redrawn water, from a sheet of its own. Only the shallow pair is taken:
# the deep pair came back worse than the one
# already in hand (composition 9.0 against 6.3), and a pair that is worse is not an improvement for having
# been asked for.
WATER = [
    ("ground.water.b", 0, [2, 3], None),
]

GROUND_LAYOUTS = {"terrain": TERRAIN, "water": WATER}

# What the water has to look like, because it is the one thing in this sheet
# that meets art from another one: the bank tiles hold water at their top
# edge, and they were drawn beside the water that shipped in the first sheet.
# The redraw came back a bright cyan - mean (25,117,141) and (68,136,132),
# saturation 118 and 77, against the sheet's own 31 to 36 - so it is matched
# back, channel by channel, to what it is replacing: the mean and spread of
# the old shallow pair, measured at the size it will be drawn.
#
# Water is the one thing that CAN be recoloured this way. Hair and skin cannot
# (their hues overlap exactly, docs/sprites.md section 1), and that is why
# nothing else here is touched.
WATER_MATCH = {
    "ground.water.b": ((65.4, 93.3, 86.3), (25.3, 21.7, 20.6)),
}

# What each sort of ground is tinted by, and why a tint at all.
#
# The sheet came back beautiful and nearly one colour. Measured over the whole
# of each family, the mean colours were 10.6 apart for level ground against
# broken ground and 9.3 apart against a trunk, on a scale where the channels
# run to 255 - and their brightnesses were 77.0, 78.7 and 79.1. At the size a
# cell is drawn, a difference of ten in the mean is no difference at all: the
# first thing anybody said on seeing the world drawn was that they could not
# tell a ledge from a field or broken ground from either.
#
# So the families are pushed apart here, once, rather than at every draw. A
# gain per channel, which multiplies what is there and leaves the texture
# alone; the picture is still the picture, in a drier or a shadier light. It
# is done at the size the tile will be drawn, after the water has been matched
# to the sheet it has to join.
#
# Brightness carries ONE fact and carries it in two steps: the floor of the
# world is dark and what is up on a ledge is light. It is not a scale - a
# ledge two levels up is no brighter than one - because the only thing the eye
# has to get from brightness is which of the two it is looking at, and a
# gradient of four made all four of them middling. Everything else is told
# apart by hue at the same brightness: broken country is dry and red, a trunk
# is brown and darker still, stone is grey and cool, the stream is blue.
# (Which level a ledge is on is said by the line round the drop, ground.go.)
#
# What is NOT pushed apart is everything that touches level ground inside its
# own tile - the shore, the roots, the ramp, the mouth of a nest - so they
# take the floor's own figure: a tile that is half field has to be the same
# green as the field beside it or the join shows.
#
# After: brightness 66 for the floor, 66 for broken ground, 124 on a ledge, 49
# for a trunk, 72 for stone; and 26.6 apart in mean colour from floor to
# broken ground, 98.2 to a ledge, 30.1 to a trunk. One pixel in ten thousand
# clips.
LOW = (0.86, 0.86, 0.86)  # the floor of the world, a step darker than drawn
PALETTE = {
    "ground.flat":  LOW,
    "ground.root":  LOW,   # a trunk's foot, and half of it is floor
    "ground.bank":  LOW,   # the shore, and half of it is floor
    "ground.slope": LOW,   # a ramp runs from the floor to the ledge
    "ground.nest":  LOW,
    "ground.scree": LOW,   # the cliff's own rocks, at the foot of a boulder
    "ground.water": LOW,   # so that the stream matches the shore that holds it
    "ground.rough": (1.053, 0.750, 0.553),  # dry and red, as dark as the floor
    "ground.high":  (1.404, 1.429, 1.557),  # up in the light, and cooler
    "ground.cliff": (1.404, 1.429, 1.557),  # the same ledge, seen at its edge
    "ground.block": (0.792, 0.618, 0.467),  # a mass, in its own shade
    # ...except the one mass that is not wood. Pushing stone towards brown
    # with the trunks turned a boulder into a heap of earth; it is left grey
    # and pushed cool instead, which is the other half of "which way each one
    # is pushed is what the ground IS".
    "ground.block.rock": (0.792, 0.827, 1.008),
}

# How much of a tile's edge is the background bleeding in.
FRINGE = 0.03

# The tiles a Tiled tileset is built from, in the order the placeholder set
# used before this - so a map drawn against that one keeps meaning what it
# meant, and anything new is appended rather than inserted.
#
# Each one carries the properties the engine reads off it (tiled/properties.md)
# and a class, which is the name Tiled shows the person painting with it.
#
# Two ramps rather than one. A ramp joins the level it is on to the one below,
# so a terrace two levels up needs a ramp of its own to be reachable at all -
# the first version of this tileset had only the first, and the sample map
# drawn with it had a second terrace nobody could stand on.
TILED_SET = [
    ("ground.flat.a",     "field",    {"kind": "flat"}),
    ("ground.rough.a",    "broken",   {"kind": "rough"}),
    ("ground.water.a",    "stream",   {"kind": "water"}),
    ("ground.slope.a",    "ramp 1",   {"kind": "slope", "height": 1}),
    ("ground.high.a",     "ledge 1",  {"kind": "high", "height": 1}),
    ("ground.high.b",     "ledge 2",  {"kind": "high", "height": 2}),
    ("ground.block.a",    "tree",     {"kind": "high", "height": 3}),
    ("ground.block.rock", "boulder",  {"kind": "high", "height": 5}),
    ("ground.slope.b",    "ramp 2",   {"kind": "slope", "height": 2}),
]


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


def grouped(boxes, want):
    """Cut a row's boxes into `want` groups, and merge each into one box.

    What is in a row is known - the table says so - and two different things
    make more boxes than that, so there are two ways of gathering them.

    Where the row holds `want` big things, the big ones ARE the groups and
    everything loose joins whichever it is nearest: that is a punch's marks
    going with the body that threw them, and it is right even when the marks
    fly further than the gap to the next body, which is where cutting at the
    widest gaps got it wrong (one row of this sheet came back with two
    bodies merged into one box 316 wide).

    Where there is no such core - the row of light is motes and nothing else
    - the widest gaps are all there is to go on, and they are enough,
    because the groups in such a row are evenly spread by construction.
    """
    if len(boxes) <= want:
        return boxes

    def area(b):
        return (b[2] - b[0]) * (b[3] - b[1])

    def merge(part):
        return (min(b[0] for b in part), min(b[1] for b in part),
                max(b[2] for b in part), max(b[3] for b in part))

    big = [b for b in boxes if area(b) >= 0.4 * max(area(x) for x in boxes)]
    if len(big) == want:
        groups = [[b] for b in big]
        for b in boxes:
            if b in big:
                continue
            mid = (b[0] + b[2]) / 2
            near = min(range(len(big)),
                       key=lambda j: abs((big[j][0] + big[j][2]) / 2 - mid))
            core = big[near]
            # ...unless it is further off than that body is wide, in which
            # case it is not part of this row at all. Bands are cut between
            # rows and a row of fliers leaves the tips of its wings in the
            # band below: gathering those into what is down there stretched
            # one clip across the whole sheet.
            if max(core[0] - b[2], b[0] - core[2], 0) > core[2] - core[0]:
                continue
            groups[near].append(b)
        return sorted(merge(g) for g in groups)

    gaps = sorted((boxes[i + 1][0] - boxes[i][2], i) for i in range(len(boxes) - 1))
    cuts = sorted(i for _, i in gaps[-(want - 1):])
    out, start = [], 0
    for end in cuts + [len(boxes) - 1]:
        out.append(merge(boxes[start:end + 1]))
        start = end + 1
    return out


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


def shadowed(frame, dark, light):
    """Turn a white body with a dark outline into a dark one with a bright rim.

    A multiply cannot do this: it can only take light away, so white stays the
    brightest thing in the picture whatever colour it is given. The map here
    runs the other way - the brightest pixel becomes `dark` and the darkest
    becomes `light` - which keeps every edge the artist drew and swaps which
    side of it is lit.
    """
    a = np.asarray(frame).astype(float)
    lum = (0.299 * a[:, :, 0] + 0.587 * a[:, :, 1] + 0.114 * a[:, :, 2]) / 255.0
    for c in range(3):
        a[:, :, c] = light[c] + (dark[c] - light[c]) * lum
    return Image.fromarray(np.clip(a, 0, 255).astype("uint8"))


def pack_bodies(img, picks, found):
    """The bodies half of the tool, as it has always worked."""
    # A build's scale, for the clips that share one: the widest frame any of
    # them uses, so every pose of that build fits its cell and none of them is
    # shrunk again by fits() and left smaller than its neighbours.
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
        boxes = [found[row][i] for i in frames]
        scale = fits(img, boxes, scale)
        made.append((name, [cell(img, b, scale) for b in boxes], TILE, TILE))
    return made


# --- the ground ------------------------------------------------------------

def ground_bands(fg, x_from):
    """The rows of tiles, from the tiles rather than from the reference bodies.

    rows_of cuts a body's band halfway to the next reference, because a body
    can stand taller than the reference beside it and its band has to hold all
    of it. Tiles do not overhang anything: they are squares in a grid with
    magenta between the rows, so the rows are simply where there is something
    to the right of the reference column. Cutting them the bodies' way merges
    a row with its neighbour - the first sheet came back with rows 172 pixels
    tall and bands 180, which glued two tiles into one 340-wide blob.
    """
    return [(up, down) for up, down in runs(fg[:, x_from:].sum(1), least=16)]


def squares(fg, band, x_from):
    """The tiles in one band, left to right.

    Simpler than bodies(): a tile is a solid block of picture with magenta
    either side of it, so the columns that have anything in them are the
    tiles, and nothing has to be dilated or labelled. The gaps came back 9 to
    261 pixels wide on the sheet this was written for, and the narrowest of
    them - the two frames of one animation, deliberately close together - is
    still five times what any fringe is.
    """
    up, down = band
    out = []
    for x0, x1 in runs(fg[up:down, x_from:].sum(0), least=16):
        strip = fg[up:down, x_from + x0:x_from + x1]
        ys = np.nonzero(strip.sum(1))[0]
        out.append((x_from + x0, up + ys[0], x_from + x1, up + ys[-1] + 1))
    return out


def square(src, box):
    """One tile, trimmed of the fringe and resampled to the cell.

    Opaque on purpose. Everything else in the sheet is cut out of its
    background and carries alpha; a tile is the background, and a hole in it
    would be a hole in the world.
    """
    x0, y0, x1, y1 = box
    trim = max(1, round(min(x1 - x0, y1 - y0) * FRINGE))
    cut = src.crop((x0 + trim, y0 + trim, x1 - trim, y1 - trim)).convert("RGB")
    out = cut.resize((TILE, TILE), Image.BOX).convert("RGBA")
    out.putalpha(255)
    return out


def matched(frames, target):
    """Move a clip's colour onto another one's, channel by channel.

    Mean and spread, which is enough for a texture and is the whole of what
    was wrong with the redrawn water: it came back the right shapes in the
    wrong palette. Every frame of the clip is moved by the SAME figures, taken
    over all of them together - matching frames one at a time would make the
    colour flicker as the animation ran, which is the fault this was called in
    to fix.
    """
    want_mean, want_std = target
    a = [np.asarray(f).astype(float) for f in frames]
    pool = np.concatenate([x[:, :, :3].reshape(-1, 3) for x in a])
    mean, std = pool.mean(axis=0), pool.std(axis=0)
    out = []
    for x in a:
        rgb = (x[:, :, :3] - mean) / np.maximum(std, 1e-6) * np.array(want_std) + np.array(want_mean)
        x[:, :, :3] = np.clip(rgb, 0, 255)
        out.append(Image.fromarray(x.astype("uint8")))
    return out


def tinted(frame, gain):
    """One tile in a drier or a shadier light: a gain on each channel.

    Multiplied rather than shifted, so that what was dark stays dark and the
    texture keeps its shape. See PALETTE for why this is done at all.
    """
    a = np.asarray(frame).astype(float)
    a[:, :, :3] = np.clip(a[:, :, :3] * np.array(gain), 0, 255)
    return Image.fromarray(a.astype("uint8"))


def pack_ground(src, picks, found):
    """The ground half of the tool: one clip per named square."""
    made = []
    for name, row, frames, _ in picks:
        if row >= len(found):
            sys.exit("%s wants row %d and the sheet has %d" % (name, row, len(found)))
        cells = []
        for i in frames:
            if i >= len(found[row]):
                sys.exit("%s wants tile %d of row %d and the row has %d"
                         % (name, i, row, len(found[row])))
            cells.append(square(src, found[row][i]))
        if name in WATER_MATCH:
            cells = matched(cells, WATER_MATCH[name])
        # The whole name first, so that one tile can step out of its
        # family, then the family.
        gain = PALETTE.get(name) or PALETTE.get(".".join(name.split(".")[:2]))
        if gain:
            cells = [tinted(c, gain) for c in cells]
        made.append((name, cells, TILE, TILE))
    return made


def write_tiled(directory, made):
    """A Tiled tileset of the same pictures, so a map is drawn in the art it
    will be played in.

    The placeholder set before this was flat colours, and somebody picking
    between seven coloured squares is guessing at what the world will look
    like. The order is that set's order, so a map drawn against it still means
    what it meant.

    Written twice, as .tsj and as .tsx, because Tiled reads both and which one
    a given build offers in its file chooser is not worth finding out from
    here. They are the same tileset; adding either gives the same tiles.

    The properties are the map's half of the bargain (decision #133): the tile
    carries what a piece of ground IS and where it is, and nothing about what
    it is worth. What a level means - climbable up to two, a thing rather than
    a place from three - is cmd/devview's reading of the same numbers, and it
    is written down in tiled/properties.md rather than in here.
    """
    have = {name: frames for name, frames, _, _ in made}
    sheet = Image.new("RGBA", (TILE * len(TILED_SET), TILE), (0, 0, 0, 0))
    tiles = []
    for i, (clip, klass, props) in enumerate(TILED_SET):
        if clip not in have:
            sys.exit("the Tiled set wants %s and this sheet has not got it" % clip)
        sheet.paste(have[clip][0], (i * TILE, 0))
        tiles.append({"id": i, "class": klass, "properties": [
            {"name": k, "type": "int" if isinstance(v, int) else "string", "value": v}
            for k, v in sorted(props.items())]})
    os.makedirs(directory, exist_ok=True)
    sheet.save(os.path.join(directory, "terrain.png"))
    head = {"columns": len(TILED_SET), "image": "terrain.png",
            "imageheight": TILE, "imagewidth": TILE * len(TILED_SET),
            "margin": 0, "name": "mononoke terrain", "spacing": 0,
            "tilecount": len(TILED_SET), "tiledversion": "1.12.2",
            "tileheight": TILE, "tilewidth": TILE}
    with open(os.path.join(directory, "terrain.tsj"), "w") as f:
        json.dump(dict(head, tiles=tiles, type="tileset", version="1.10"),
                  f, indent=2)
        f.write("\n")
    with open(os.path.join(directory, "terrain.tsx"), "w") as f:
        f.write('<?xml version="1.0" encoding="UTF-8"?>\n')
        f.write('<tileset version="1.10" tiledversion="%s" name="%s" '
                'tilewidth="%d" tileheight="%d" tilecount="%d" columns="%d">\n'
                % (head["tiledversion"], head["name"], TILE, TILE,
                   len(TILED_SET), len(TILED_SET)))
        f.write(' <image source="terrain.png" width="%d" height="%d"/>\n'
                % (head["imagewidth"], head["imageheight"]))
        for t in tiles:
            f.write(' <tile id="%d" class="%s">\n  <properties>\n' % (t["id"], t["class"]))
            for prop in t["properties"]:
                f.write('   <property name="%s" type="%s" value="%s"/>\n'
                        % (prop["name"], prop["type"], prop["value"]))
            f.write('  </properties>\n </tile>\n')
        f.write('</tileset>\n')
    print("%s  %d tiles  (.tsj and .tsx)"
          % (os.path.join(directory, "terrain.png"), len(TILED_SET)))


# --- writing the sheet -----------------------------------------------------

def wide(img, box):
    """How wide the body in this frame is, as against how wide the frame is.

    What is drawn flying off a body - the marks off a punch, the red off a
    blow landing - is part of the picture and was glued to the body on the way
    in, because a mark that came away as a sprite of its own would be counted
    as another body in the row. It is not part of the body, though, and on
    this sheet a punch throws its marks half again as far as the body is wide:
    130 pixels against a body of 68. Fitting the cell to that shrinks the body
    to make room for a mark.

    So the body is the largest blob in the frame, found without the dilation
    that glued the loose parts on. Everything the eye reads a size off is one
    blob - a body standing, a body lying down, a beast with its wings out, a
    body up to its waist in water it overlaps - and everything that is thrown
    off one is a small separate one.
    """
    a = np.asarray(img.crop(box))[:, :, 3] > 0
    labels, n = ndimage.label(a)
    if n == 0:
        return box[2] - box[0]
    biggest = 1 + np.argmax(ndimage.sum(a, labels, range(1, n + 1)))
    columns = np.nonzero((labels == biggest).sum(0))[0]
    return columns[-1] - columns[0] + 1


def tall(img, box):
    """How tall the body in this frame is, as against how tall the frame is.

    The same reading as wide(), for the same reason and found the same way:
    the marks off a punch and the shock off a blow landing are part of the
    picture, and this sheet throws them over the body's head. Measuring the
    picture there and fitting THAT to the cell steps the whole clip down, so
    a spirit came out 7% shorter on the two frames where it fights and is
    fought - which is exactly the artefact the width was fixed to avoid.
    """
    a = np.asarray(img.crop(box))[:, :, 3] > 0
    labels, n = ndimage.label(a)
    if n == 0:
        return box[3] - box[1]
    biggest = 1 + np.argmax(ndimage.sum(a, labels, range(1, n + 1)))
    rows_ = np.nonzero((labels == biggest).sum(1))[0]
    return rows_[-1] - rows_[0] + 1


def fits(img, boxes, scale):
    """The scale a clip is actually drawn at: its own, unless a frame overflows.

    A cell is square and some poses are not. A body lying down is 36 to 38
    pixels across at the scale a standing one is 30 tall, and nothing reads a
    corpse's size, so the corpse gives up the tenth rather than the grid give
    up its square cells. What overflows and is not a body - the far end of a
    punch's mark - is cut off at the cell's edge instead, because clipping the
    tip of a mark is not something the eye has anything to compare against and
    a body that flinches a tenth smaller on the frame it lands its punch is.

    Whatever has to give, it cannot be one frame giving it and not the next:
    the overflow is measured across every frame of the clip and the whole clip
    is stepped down together. Scaling each frame to fit its own outline is how
    two frames of one punch end up two sizes.
    """
    for box in boxes:
        scale = min(scale, TILE / wide(img, box), (TILE - FEET) / tall(img, box))
    return scale


def cell(img, box, scale):
    """One body, scaled and dropped into its cell with its feet on the floor.

    Feet on the floor rather than centred: a body centred in its cell rises
    off the ground whenever a pose changes its outline.
    """
    c = img.crop(box)
    w, h = max(1, round(c.width * scale)), max(1, round(c.height * scale))
    small = c.resize((w, h), Image.BOX)
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
        # The flag comes across with the clip. It used to be worked out again
        # from the clip's size, which was fine while size and flag meant the
        # same thing and wrong the moment a drawn body was meant to be
        # coloured: a spirit carried through three sheets arrived grey.
        out[c["name"]] = (frames, c["w"], c["h"], c.get("tint", c["w"] != TILE))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("sheet")
    ap.add_argument("out_dir")
    ap.add_argument("--layout", default="humans",
                    choices=sorted(list(LAYOUTS) + list(GROUND_LAYOUTS)),
                    help="which table says what is in this sheet")
    ap.add_argument("--carry", help="a directory whose unclaimed clips come across")
    ap.add_argument("--dump", action="store_true",
                    help="print what was found and stop, for checking the table")
    ap.add_argument("--tiled", help="also write a Tiled tileset of the ground here")
    ap.add_argument("--drop", default="",
                    help="clips NOT to carry across, comma separated. For art "
                         "a new cast has replaced everywhere but one pose: the "
                         "old picture would be carried into the new set and "
                         "drawn beside the new one, and a body from another "
                         "world is worse than the nearest pose of this one")
    args = ap.parse_args()
    ground = args.layout in GROUND_LAYOUTS
    picks = GROUND_LAYOUTS[args.layout] if ground else LAYOUTS[args.layout]

    img, fg = load(args.sheet)
    src = Image.open(args.sheet)
    bands, x_from = rows_of(fg)
    if ground:
        bands = [(up, down, 0) for up, down in ground_bands(fg, x_from)]
        found = [squares(fg, (up, down), x_from) for up, down, _ in bands]
    else:
        found = [bodies(fg, (up, down), x_from) for up, down, _ in bands]
        if counts := COUNTS.get(args.layout):
            found = [grouped(row, want) if i < len(counts) else row
                     for i, (row, want) in enumerate(zip(found, counts))]

    if args.dump:
        for i, (row, (_, _, ref)) in enumerate(zip(found, bands)):
            print("row %2d  reference %3d  found %2d  wxh %s"
                  % (i, ref, len(row),
                     [(b[2] - b[0], b[3] - b[1]) for b in row]))
        return

    if ground:
        made = pack_ground(src, picks, found)
    else:
        made = pack_bodies(img, picks, found)

    if shadow := SHADOW.get(args.layout):
        prefix, dark, light = shadow
        made = [(name, [shadowed(f, dark, light) for f in frames] if
                 name.startswith(prefix) else frames, w, h)
                for name, frames, w, h in made]

    # Whether each clip is to be given a colour when it is stamped. The
    # layout says, for what it made; what is carried keeps what it had.
    paint = {name: args.layout in TINTED for name, _, _, _ in made}

    claimed = {name for name, _, _, _ in made}
    claimed |= {n.strip() for n in args.drop.split(",") if n.strip()}
    carry = carried(args.carry) if args.carry else {}
    for name, (frames, w, h, tinted) in sorted(carry.items()):
        if name not in claimed:
            made.append((name, frames, w, h))
            paint[name] = tinted

    width = max(len(f) * w for _, f, w, _ in made)
    height = sum(h for _, _, _, h in made)
    sheet = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    clips, y = [], 0
    for name, frames, w, h in made:
        for i, f in enumerate(frames):
            sheet.paste(f, (i * w, y))
        clips.append({"name": name, "x": 0, "y": y, "w": w, "h": h,
                      "frames": len(frames),
                      # Whether this one is given a colour when it is
                      # stamped. Grey art has to be, to be visible at all;
                      # art drawn in its own colours must not be, because
                      # multiplying those by a colour stains rather than
                      # tints; and the spirits are drawn white BECAUSE they
                      # are to be coloured (TINTED).
                      "tint": paint.get(name, w != TILE)})
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
    if args.tiled:
        write_tiled(args.tiled, made)


if __name__ == "__main__":
    main()
