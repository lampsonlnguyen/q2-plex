# Plex Music v1.0.0

**Unofficial, independent Plex music client for Shanling Q2 running Q2 Pod.** Not affiliated with, endorsed by, or developed by Plex, Inc. This is **not** the official Plexamp app.

This is the first public v1.0.0 package. Automated compilation and tests do not establish full physical-device stability; see known limitations below.

## Features

- Sign in to Plex through the phone-assisted `plex.tv/link` PIN flow and browse Plex **music** libraries, artists, albums, tracks, search and playlists.
- Stream music through the Q2 Pod **native player**, with album/playlist queues, original-quality Plex media and native hardware controls.
- Optionally download original-quality tracks, albums and playlists to `PlexMusic/` on the microSD card for local playback.
- Five-star Plex favorites and playback timeline reporting on Q2 Pod firmware exposing the appropriate capabilities.
- Last Queue restoration (song-level; see resume limitation below), bounded artwork caching, automatic Plex reconnection and opt-in CPU/RAM/battery diagnostics.
- The Plexamp-style icon and **Plex Music** display name under **Streaming**.

## Installation

1. Download **`q2-plex-v1.0.0.zip`** from the assets below. Do **not** install the release source-code ZIP.
2. Unzip it **at the root of the Q2 microSD card**. The archive contains a hidden `.plex/` directory.
3. When upgrading, **preserve** `.plex/auth.json`, `.plex/last-queue.json`, `.plex/fav.log`, `.plex/fav.log.taking`, and the `PlexMusic/` directory. Do not delete existing downloaded media.
4. Eject the SD card safely and restart the Q2 to replace the running binary.
5. Open **Streaming → Plex Music**, then follow the Plex sign-in instructions.

Use `SHA256SUMS` to verify the ZIP before installation if desired.

## Compatibility and known limitations

- Requires Q2 Pod firmware with card-app support and URL queue playback. Older firmware may lack artwork, heart synchronization, timeline reporting and other optional capabilities.
- The Streaming launcher may require Wi-Fi even for offline music. Downloaded files can also be played from Q2 Pod's native Folder view.
- **Actual physical-Q2 testing remains incomplete** for uninterrupted full-length album streaming, seeking, next/previous, long FLAC playback, full download/resume cycles, Wi-Fi loss/recovery and battery lifetime. Report issues with the firmware version and steps to reproduce.
- Restoring the last queue starts the saved song from its beginning; precise second-level seek-on-restore is not yet supported by the documented Q2 card-app API.
- Plex connectivity is required for new streaming, downloads, timeline updates and server favorite sync. Offline listening history is not backfilled.
- No video features, video decoding or separate audio engine: Q2 Pod handles audio playback and device power.
- `.plex/auth.json` stores sensitive Plex credentials on the removable SD card. Do not share it or upload it to a public issue. For diagnostics, use `/tmp/q2-plex.log`, without publishing tokens or private server details.
- Plexamp-on-phone relay remains outside this release.

## Icon credits

Uses community Plexamp artwork from [selfh.st/icons](https://selfh.st/icons/) under CC BY 4.0, adapted to 52px and 40px Q2 Pod app icon assets. Plex and Plexamp are trademarks of Plex, Inc.; the artwork does not imply official affiliation.

Source: https://github.com/lampsonlnguyen/q2-plex
