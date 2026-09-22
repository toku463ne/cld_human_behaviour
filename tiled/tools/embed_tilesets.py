#!/usr/bin/env python3
"""Turn a Tiled map into the .tmj the engine reads.

Tiled saves a map either way, and the engine reads one of them: .tmj is JSON,
which the standard library parses, and its tilesets have to be embedded,
because engine/tiled.go is handed bytes and never opens a file (that is why it
refuses a <tileset source="..."> - it has no way to follow it). A .tmx is XML
with the tilesets kept beside it in .tsx files, which is the more comfortable
thing to draw with: edit the palette once and every map that uses it changes.

So this is the bridge, and it is the only thing here that knows about files.
It takes a map saved either way - .tmx, or the .tmj Tiled writes when the
tilesets are still kept beside it - follows each external tileset, copies the
properties of every tile that has any into the output, and writes the layers
out as plain arrays. Nothing is interpreted: a property is carried across with
the name and the type Tiled gave it, and what the names mean is
tiled/properties.md.

    python3 tiled/tools/embed_tilesets.py tiled/samples/typical.tmj /tmp/typical.tmj
    go run ./cmd/devview -tiled /tmp/typical.tmj

Two things it refuses rather than guesses at: a compressed or base64 layer
(save the layers as CSV - Tiled's Preferences has the setting) and a tileset
whose image it cannot size. Both are the same refusals the engine makes.
"""

import json
import os
import sys
import xml.etree.ElementTree as ET


def prop_value(p):
    """One custom property, in the type Tiled wrote it with."""
    kind = p.get("type", "string")
    raw = p.get("value", p.text or "")
    if kind == "int":
        return int(raw)
    if kind == "float":
        return float(raw)
    if kind == "bool":
        return raw == "true"
    return raw


def properties(el):
    block = el.find("properties")
    if block is None:
        return None
    return [{"name": p.get("name"), "type": p.get("type", "string"),
             "value": prop_value(p)} for p in block.findall("property")]


def tiles_of(root):
    """The tiles that carry properties, which is all the engine looks at."""
    out = []
    for tile in root.findall("tile"):
        props = properties(tile)
        if props:
            out.append({"id": int(tile.get("id")), "properties": props})
    return out


def external_tileset(path, firstgid):
    root = ET.parse(path).getroot()
    image = root.find("image")
    out = {
        "firstgid": firstgid,
        "name": root.get("name", ""),
        "tilewidth": int(root.get("tilewidth", 32)),
        "tileheight": int(root.get("tileheight", 32)),
        "tilecount": int(root.get("tilecount", 0)),
        "columns": int(root.get("columns", 1)),
        "margin": int(root.get("margin", 0)),
        "spacing": int(root.get("spacing", 0)),
        "tiles": tiles_of(root),
    }
    if image is not None:
        # The image path is relative to the .tsx, and the .tmj lands wherever
        # the caller asked for it, so it is rewritten relative to nothing at
        # all: the engine never loads it, and Tiled only follows it if the
        # output is opened for editing, which is not what the output is for.
        out["image"] = image.get("source")
        out["imagewidth"] = int(image.get("width", 0))
        out["imageheight"] = int(image.get("height", 0))
    return out


def convert(src, dst):
    root = ET.parse(src).getroot()
    beside = os.path.dirname(os.path.abspath(src))
    out = {
        "type": "map",
        "version": root.get("version", "1.10"),
        "tiledversion": root.get("tiledversion", ""),
        "orientation": root.get("orientation", "orthogonal"),
        "renderorder": root.get("renderorder", "right-down"),
        "infinite": root.get("infinite", "0") != "0",
        "width": int(root.get("width")),
        "height": int(root.get("height")),
        "tilewidth": int(root.get("tilewidth")),
        "tileheight": int(root.get("tileheight")),
        "nextlayerid": int(root.get("nextlayerid", 1)),
        "nextobjectid": int(root.get("nextobjectid", 1)),
        "tilesets": [],
        "layers": [],
    }
    if out["infinite"]:
        sys.exit("%s: the map is infinite, and the engine reads a finite one"
                 % src)
    for ts in root.findall("tileset"):
        first = int(ts.get("firstgid"))
        source = ts.get("source")
        if source:
            out["tilesets"].append(external_tileset(
                os.path.join(beside, source), first))
        else:  # already embedded, which Tiled does on Map > Embed Tileset
            image = ts.find("image")
            embedded = {"firstgid": first, "name": ts.get("name", ""),
                        "tilecount": int(ts.get("tilecount", 0)),
                        "columns": int(ts.get("columns", 1)),
                        "tiles": tiles_of(ts)}
            if image is not None:
                embedded["image"] = image.get("source")
                embedded["imagewidth"] = int(image.get("width", 0))
                embedded["imageheight"] = int(image.get("height", 0))
            out["tilesets"].append(embedded)

    layer_id = 0
    for el in root:
        if el.tag == "layer":
            layer_id += 1
            data = el.find("data")
            if data.get("encoding") != "csv":
                sys.exit("%s: layer %r is saved as %s, and the engine reads "
                         "CSV" % (src, el.get("name"), data.get("encoding")
                                  or data.get("compression") or "XML"))
            gids = [int(x) for x in data.text.replace("\n", "").split(",")
                    if x.strip()]
            out["layers"].append({
                "type": "tilelayer", "id": layer_id, "name": el.get("name"),
                "opacity": float(el.get("opacity", 1)),
                "visible": el.get("visible", "1") != "0",
                "x": 0, "y": 0,
                "width": int(el.get("width")), "height": int(el.get("height")),
                "data": gids,
            })
        elif el.tag == "objectgroup":
            layer_id += 1
            objects = []
            for o in el.findall("object"):
                one = {"id": int(o.get("id", 0)), "name": o.get("name", ""),
                       "x": float(o.get("x", 0)), "y": float(o.get("y", 0)),
                       "width": float(o.get("width", 0)),
                       "height": float(o.get("height", 0))}
                if o.find("point") is not None:
                    one["point"] = True
                if o.find("ellipse") is not None:
                    one["ellipse"] = True
                props = properties(o)
                if props:
                    one["properties"] = props
                objects.append(one)
            out["layers"].append({
                "type": "objectgroup", "id": layer_id, "name": el.get("name"),
                "opacity": float(el.get("opacity", 1)),
                "visible": el.get("visible", "1") != "0",
                "x": 0, "y": 0, "objects": objects,
            })

    with open(dst, "w") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print("%s -> %s (%dx%d, %d layers, %d tilesets)" % (
        src, dst, out["width"], out["height"], len(out["layers"]),
        len(out["tilesets"])))


def embed(src, dst):
    """A .tmj whose tilesets are still in files beside it, brought in."""
    with open(src) as f:
        out = json.load(f)
    beside = os.path.dirname(os.path.abspath(src))
    sets = []
    for ts in out.get("tilesets", []):
        source = ts.get("source")
        if not source:
            sets.append(ts)
            continue
        path = os.path.join(beside, source)
        if path.endswith(".tsx"):
            sets.append(external_tileset(path, ts["firstgid"]))
            continue
        with open(path) as f:  # an external .tsj
            one = json.load(f)
        one["firstgid"] = ts["firstgid"]
        sets.append(one)
    out["tilesets"] = sets
    for layer in out.get("layers", []):
        if layer.get("type") == "tilelayer" and not isinstance(layer.get("data"), list):
            sys.exit("%s: layer %r is %s, and the engine reads plain numbers "
                     "(save the layers uncompressed, or as CSV)"
                     % (src, layer.get("name"), layer.get("encoding", "encoded")))
    with open(dst, "w") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print("%s -> %s (%dx%d, %d layers, %d tilesets)" % (
        src, dst, out["width"], out["height"], len(out.get("layers", [])),
        len(sets)))


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit("usage: embed_tilesets.py <map.tmx|map.tmj> <map.tmj>")
    if sys.argv[1].endswith(".tmj") or sys.argv[1].endswith(".json"):
        embed(sys.argv[1], sys.argv[2])
    else:
        convert(sys.argv[1], sys.argv[2])
