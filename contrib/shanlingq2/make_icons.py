#!/usr/bin/env python3
"""Create round Plexamp app icons using only Python's standard library.

Q2 Pod expects 52px icon.png and 40px icon-40.png. Input must be
non-interlaced 8-bit RGB or RGBA PNG; unsupported formats fail explicitly.
"""
import pathlib
import struct
import sys
import zlib

SIGNATURE = b"\x89PNG\r\n\x1a\n"

def chunks(data):
    if not data.startswith(SIGNATURE):
        raise ValueError("not a PNG")
    offset = len(SIGNATURE)
    while offset < len(data):
        size = struct.unpack_from(">I", data, offset)[0]
        tag = data[offset + 4:offset + 8]
        payload = data[offset + 8:offset + 8 + size]
        yield tag, payload
        offset += 12 + size
        if tag == b"IEND":
            break

def read_png(path):
    width = height = channels = None
    packed = bytearray()
    for tag, chunk in chunks(path.read_bytes()):
        if tag == b"IHDR":
            width, height, depth, kind, compression, filtering, interlace = struct.unpack(">IIBBBBB", chunk)
            if depth != 8 or kind not in (2, 6) or compression or filtering or interlace:
                raise ValueError("expected non-interlaced 8-bit RGB/RGBA PNG")
            channels = 3 if kind == 2 else 4
        elif tag == b"IDAT":
            packed.extend(chunk)
    if not width or not height:
        raise ValueError("missing PNG dimensions")
    decoded = zlib.decompress(packed)
    stride = width * channels
    rows, previous, offset = [], bytearray(stride), 0
    for _ in range(height):
        filter_type = decoded[offset]
        offset += 1
        raw = decoded[offset:offset + stride]
        if len(raw) != stride:
            raise ValueError("truncated PNG scanline")
        offset += stride
        row = bytearray(raw)
        for j in range(stride):
            left = row[j - channels] if j >= channels else 0
            up = previous[j]
            upper_left = previous[j - channels] if j >= channels else 0
            if filter_type == 1:
                predictor = left
            elif filter_type == 2:
                predictor = up
            elif filter_type == 3:
                predictor = (left + up) // 2
            elif filter_type == 4:
                p = left + up - upper_left
                distances = (abs(p - left), abs(p - up), abs(p - upper_left))
                predictor = (left, up, upper_left)[distances.index(min(distances))]
            elif filter_type == 0:
                predictor = 0
            else:
                raise ValueError("unsupported PNG filter")
            row[j] = (row[j] + predictor) & 255
        rows.append(row)
        previous = row
    return width, height, channels, rows

def chunk(tag, payload):
    content = tag + payload
    return struct.pack(">I", len(payload)) + content + struct.pack(">I", zlib.crc32(content) & 0xffffffff)

def write_icon(width, height, channels, rows, size, output):
    """Downsample to a padded circular RGBA icon with anti-aliased edges.

    Keep the source aspect ratio by cropping to its centered square. The PNG
    canvas stays 52x52 / 40x40 as required by Q2 Pod; only the outer ring and
    corners are transparent. Keep the source's alpha as well.
    """
    import math

    side = min(width, height)
    if side < size:
        raise ValueError("Plexamp artwork is too small to resize cleanly")
    left = (width - side) // 2
    top = (height - side) // 2
    radius = size * 0.46  # about two transparent pixels of padding per side
    out = bytearray()
    for y in range(size):
        out.append(0)  # PNG filter: None
        y0 = top + y * side // size
        y1 = top + (y + 1) * side // size
        for x in range(size):
            x0 = left + x * side // size
            x1 = left + (x + 1) * side // size
            count = (x1 - x0) * (y1 - y0)
            pixel = [
                (sum(rows[sy][sx * channels + c] for sy in range(y0, y1)
                     for sx in range(x0, x1)) + count // 2) // count
                for c in range(channels)
            ]
            if channels == 3:
                pixel.append(255)
            # 1-pixel antialias band around the circular edge.
            distance = math.hypot(x + 0.5 - size / 2, y + 0.5 - size / 2)
            coverage = max(0.0, min(1.0, radius + 0.5 - distance))
            pixel[3] = round(pixel[3] * coverage)
            out.extend(pixel)
    image = (SIGNATURE +
             chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)) +
             chunk(b"IDAT", zlib.compress(bytes(out), 9)) +
             chunk(b"IEND", b""))
    pathlib.Path(output).write_bytes(image)

def main():
    source, outdir = map(pathlib.Path, sys.argv[1:3])
    outdir.mkdir(parents=True, exist_ok=True)
    width, height, channels, rows = read_png(source)
    for size, filename in ((52, "icon.png"), (40, "icon-40.png")):
        write_icon(width, height, channels, rows, size, outdir / filename)

if __name__ == "__main__":
    main()
