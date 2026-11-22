#!/usr/bin/env python3
"""Verify the exact Q2 Pod install archive before publication."""
import pathlib
import struct
import sys
import zipfile

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
        raise SystemExit("usage: verify_release.py dist/q2-plex-v1.0.0.zip")
    check(pathlib.Path(sys.argv[1]))
