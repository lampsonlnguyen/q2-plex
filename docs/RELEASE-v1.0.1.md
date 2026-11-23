# Plex Music v1.0.1 — Round Icon Polish

**Unofficial Plex music client for Shanling Q2 running Q2 Pod.** This independent project is not affiliated with or endorsed by Plex, Inc. It is not the official Plexamp app.

## Changes since v1.0.0

- The community Plexamp-style artwork now appears as a **round icon with transparent, anti-aliased corners** and a small transparent margin, matching other Q2 Pod Streaming icons.
- Both `icon.png` (**52 × 52**) and `icon-40.png` (**40 × 40**) are generated as RGBA PNG images.
- Added automated icon-generation tests and install-package validation to check transparency, size, layout and MIPS binary metadata.
- **No music playback, networking, account, offline or Q2 power-management behavior has changed.** The app remains **Streaming → Plex Music**.

## Installation / upgrade

1. Download `q2-plex-v1.0.1.zip` from this GitHub release's assets (not the GitHub source archive).
2. Unzip it at the **root** of the Q2 microSD card, replacing only the files included under the hidden `.plex/` directory.
3. Preserve existing `.plex/auth.json`, `.plex/last-queue.json`, `.plex/fav.log`, and the entire `PlexMusic/` folder. They are not included in the ZIP.
4. Eject the card safely and restart the Q2. Open **Streaming → Plex Music** to see the circular icon.
5. Optionally verify the ZIP using the attached `SHA256SUMS`.

## Known limitations (unchanged from v1.0.0)

Physical-Q2 validation remains incomplete for long-running album playback, seeking, Wi-Fi recovery, offline downloads and battery life. Some functionality depends on your Q2 Pod firmware's optional card-app capabilities. Offline music may require Wi-Fi to *launch* via Streaming; downloaded tracks remain accessible in the native Folder view. Last Queue can restore the song but not its exact playback second.

## Attribution

Plexamp-style icon artwork: [selfh.st/icons](https://selfh.st/icons/) (CC BY 4.0). Plex and Plexamp are trademarks of Plex, Inc. No affiliation or endorsement is implied.

Source: https://github.com/lampsonlnguyen/q2-plex
