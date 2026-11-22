#!/usr/bin/env python3
"""Package a static MIPS Go binary for Q2 Pod (nan2008 ELF flag)."""
import pathlib
import struct
import sys
import zipfile

def main():
    src, dst = map(pathlib.Path, sys.argv[1:3])
    binary = src / "q2-plex"
    data = bytearray(binary.read_bytes())
    if data[:7] != b"\x7fELF\x01\x01\x01":
        raise SystemExit("expected a 32-bit little-endian ELF")
    machine = struct.unpack_from("<H", data, 18)[0]
    if machine != 8:
        raise SystemExit("expected MIPS ELF")
    flags = struct.unpack_from("<I", data, 36)[0]
    struct.pack_into("<I", data, 36, flags | 0x400) # EF_MIPS_NAN2008
    binary.write_bytes(data)
    with zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as z:
        for name in ["q2app", "run", "cacert.pem", "q2-plex"]:
            info = zipfile.ZipInfo(".plex/" + name, (2026, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = (0o100000 | (0o755 if name in ("run", "q2-plex") else 0o644)) << 16
            z.writestr(info, (src / name).read_bytes())
    print(dst)

if __name__ == "__main__":
    main()
