#!/usr/bin/env python3
"""Verify the exact Q2 Pod install archive before publication."""
import pathlib
import struct
import sys
import zipfile
import zlib

EXPECTED = {
    ".plex/q2app", ".plex/run", ".plex/cacert.pem", ".plex/q2-plex",
    ".plex/icon.png", ".plex/icon-40.png",
}
PNG_SIG = b"\x89PNG\r\n\x1a\n"

def check(path):
    with zipfile.ZipFile(path) as z:
        entries = z.namelist()
        if set(entries) != EXPECTED or len(entries) != len(EXPECTED):
            raise ValueError("wrong install contents: " + repr(entries))
        for name in EXPECTED:
            if z.getinfo(name).file_size == 0:
                raise ValueError("empty file " + name)
            if not name.endswith((".png", ".pem")):
                if z.getinfo(name).file_size > 20 * 1024 * 1024:
                    raise ValueError("unexpectedly large " + name)
        q2app = z.read(".plex/q2app").decode("utf-8")
        if "name=Plex Music\n" not in q2app or "row=streaming\n" not in q2app:
            raise ValueError("wrong Q2 app name or menu placement")
        for name, dim in ((".plex/icon.png", 52), (".plex/icon-40.png", 40)):
            content = z.read(name)
            if not content.startswith(PNG_SIG) or content[12:16] != b"IHDR":
                raise ValueError("invalid PNG " + name)
            w, h = struct.unpack_from(">II", content, 16)
            if (w, h) != (dim, dim):
                raise ValueError(f"icon {name}: {w}x{h}, expected {dim}x{dim}")
            if content[24:26] != b"\\x08\\x06":
                raise ValueError(f"{name}: expected 8-bit RGBA")
            # The icon exporter uses one or more non-interlaced, unfiltered
            # IDAT scanlines. Verify genuine transparent corners, not merely
            # a square bitmap with the same dimensions.
            offset = 8
            compressed = bytearray()
            while offset < len(content):
                length = struct.unpack_from(">I", content, offset)[0]
                tag = content[offset + 4:offset + 8]
                if tag == b"IDAT":
                    compressed.extend(content[offset + 8:offset + 8 + length])
                offset += length + 12
                if tag == b"IEND":
                    break
            pixels = zlib.decompress(compressed)
            stride = 1 + dim * 4
            if len(pixels) != stride * dim:
                raise ValueError(f"{name}: wrong decoded pixel count")
            if any(pixels[y * stride] != 0 for y in range(dim)):
                raise ValueError(f"{name}: expected unfiltered icon rows")
            alpha = lambda x, y: pixels[y * stride + 1 + x * 4 + 3]
            if any(alpha(x, y) != 0 for x, y in
                   ((0, 0), (0, dim - 1), (dim - 1, 0),
                    (dim - 1, dim - 1), (0, dim // 2))):
                raise ValueError(f"{name}: corners and outer padding must be transparent")
            if not any(alpha(x, y) for y in range(dim) for x in range(dim)):
                raise ValueError(f"{name}: icon must contain visible artwork")
        binary = z.read(".plex/q2-plex")
        if binary[:7] != b"\x7fELF\x01\x01\x01":
            raise ValueError("not a 32-bit little endian ELF")
        if struct.unpack_from("<H", binary, 18)[0] != 8:
            raise ValueError("wrong ELF machine (not MIPS)")
        if struct.unpack_from("<I", binary, 36)[0] & 0x400 == 0:
            raise ValueError("MIPS NaN-2008 ABI flag missing")
        if z.getinfo(".plex/run").external_attr >> 16 & 0o111 == 0:
            raise ValueError("launcher is not executable")
    print(f"Verified Q2 Plex install archive: {path}")

if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: verify_release.py dist/q2-plex-v1.0.1.zip")
    check(pathlib.Path(sys.argv[1]))
