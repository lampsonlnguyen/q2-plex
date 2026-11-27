#!/usr/bin/env python3
"""Regression tests for circular, transparent Q2 Plex Music icons."""
import pathlib
import tempfile
import unittest

import make_icons


class RoundIconTests(unittest.TestCase):
    def test_sizes_alpha_and_antialias(self):
        # RGB source is promoted to RGBA; no image libraries are required.
        width, height = 112, 112
        rows = [bytearray([32, 44, 66] * width) for _ in range(height)]
        with tempfile.TemporaryDirectory() as temp:
            for size in (40, 52):
                output = pathlib.Path(temp) / f"icon-{size}.png"
                make_icons.write_icon(width, height, 3, rows, size, output)
                w, h, channels, decoded = make_icons.read_png(output)
                self.assertEqual((w, h, channels), (size, size, 4))
                alpha = lambda x, y: decoded[y][x * 4 + 3]
                for x, y in ((0, 0), (size - 1, 0),
                             (0, size - 1), (size - 1, size - 1),
                             (0, size // 2), (size - 1, size // 2)):
                    self.assertEqual(alpha(x, y), 0)
                self.assertEqual(alpha(size // 2, size // 2), 255)
                self.assertTrue(any(0 < decoded[y][x * 4 + 3] < 255
                                    for y in range(size) for x in range(size)),
                                "round edge should be anti-aliased")

    def test_pane(self):
        with tempfile.TemporaryDirectory() as temp:
            output = pathlib.Path(temp) / "pane.png"
            make_icons.write_pane(output)
            w, h, channels, rows = make_icons.read_png(output)
            self.assertEqual((w, h, channels), (96, 96, 4))
            pixels = [row[x:x + 4] for row in rows for x in range(0, len(row), 4)]
            self.assertTrue(any(p[3] == 255 for p in pixels))
            self.assertTrue(any(0 < p[3] < 255 for p in pixels))
            self.assertTrue(all(p[:3] == b"\xff\xff\xff" for p in pixels if p[3]))
            self.assertEqual(rows[0][3], 0)
            self.assertEqual(rows[95][95 * 4 + 3], 0)

    def test_preserves_source_alpha(self):
        width, height = 96, 96
        rows = [bytearray([20, 30, 40, 93] * width) for _ in range(height)]
        with tempfile.TemporaryDirectory() as temp:
            output = pathlib.Path(temp) / "icon.png"
            make_icons.write_icon(width, height, 4, rows, 52, output)
            _, _, channels, decoded = make_icons.read_png(output)
            self.assertEqual(channels, 4)
            self.assertEqual(decoded[26][26 * 4 + 3], 93)
            self.assertEqual(decoded[0][3], 0)

    def test_center_crops_rectangular_artwork(self):
        width, height = 128, 96
        rows = []
        for _ in range(height):
            row = bytearray()
            for x in range(width):
                row.extend((10, 190, 40) if 16 <= x < 112 else (240, 10, 10))
            rows.append(row)
        with tempfile.TemporaryDirectory() as temp:
            output = pathlib.Path(temp) / "icon.png"
            make_icons.write_icon(width, height, 3, rows, 52, output)
            _, _, _, decoded = make_icons.read_png(output)
            self.assertEqual(list(decoded[26][26 * 4:26 * 4 + 3]),
                             [10, 190, 40])

    def test_source_too_small_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(ValueError, "too small"):
                make_icons.write_icon(24, 24, 3,
                                      [bytearray([0, 0, 0] * 24) for _ in range(24)],
                                      52, pathlib.Path(temp) / "icon.png")


if __name__ == "__main__":
    unittest.main()
