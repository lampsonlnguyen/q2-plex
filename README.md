# q2-plex <img width="52" height="52" alt="icon" src="https://github.com/user-attachments/assets/dd6630b4-9860-4373-a015-12652338f960" />


**Unofficial Plex client** — music-only third-party card app for Shanling Q2 with Q2 Pod firmware. Not developed, sponsored, or endorsed by Plex, Inc. Plex account login and music playback engine were developed referencing Plex’s API [documentation](https://developer.plex.tv/pms/). The app is named **Plex Music** and uses Q2 Pod's native player; it is not the official Plexamp application.

**Current release: v1.0.2.** This visual-only patch adds the Q2 Pod 1.9.6 Home shortcut preview icon while retaining the circular list icons. See [v1.0.2 release notes](docs/RELEASE-v1.0.2.md) for installation and unchanged hardware limitations.

More hardware testing is still needed, particularly for uninterrupted full-length playback, Wi-Fi recovery, offline downloads and battery performance. Inspired by Q2 Pod's generic card app API, A4Link/q2-jellyfin (MIT), and the Plex authentication and library APIs. No video playback or video decoding.

## App identity and icon

- Display name: **Plex Music**, under **Streaming**.
- Uses community Plexamp artwork by [selfh.st/icons](https://selfh.st/icons/) ([Plexamp icon](https://dashboardicons.com/icons/external/plexamp), CC BY 4.0). The build masks that artwork into **circular icons with transparent anti-aliased edges** at `icon.png` (52 × 52) and `icon-40.png` (40 × 40) for Q2 Pod.
- Plex and Plexamp names and logos are trademarks of their respective owner, Plex, Inc. This is an **independent, unofficial Plex-compatible client**, not an official Plex or Plexamp release.
- The icon source is downloaded during packaging, so producing a package requires network access. Redistribution should retain this attribution.

## Artist release browsing

**Artists → [Artist]** lists Plex albums, EPs and singles from the selected music library using the artist's Plex ID, rather than relying on the artist `/children` view, which can omit releases on some Plex servers. Artist pages use Q2 Pod's existing native card-app list UI, click-wheel and touch navigation, pagination and cover handling; no custom screen or audio player is added. If an album belongs to a different *album artist* in Plex metadata, Plex may still exclude it from this artist filter.

## Included in the first milestone
- Plex account login using a 4-character PIN on **https://plex.tv/link** and Ed25519-signed device JWT.
- Saving the device identity and access token on the microSD card; proactive token refresh after five days.
- Finding a connected Plex Media Server and listing **music** libraries, albums, artists, tracks.
- Q2 Pod's native list/selection protocol and direct FLAC/MP3 streaming with bounded album queues through a loopback proxy.
- A 48 MiB Go heap target; server paging is 25 rows per API request. No audio decoding inside this app.

## Credential safety and automated secret scanning

Plex access tokens and the device signing key are stored **only on the Q2 microSD card** in `.plex/auth.json`. Never commit, attach, publish, or paste this file. `.gitignore` excludes the runtime `.plex/` directory, `PlexMusic/` downloads, build artifacts, developer `.env` files, and private-key formats.

GitHub Actions runs **Gitleaks v3** on every push and pull request, on manual request, and weekly on Mondays. It fetches the complete Git history, uses Gitleaks' default rules plus Plex-specific token detection, and blocks the app build if scanning fails. The workflow avoids posting suspected secrets to pull-request comments or uploading secret-containing reports.

Automated scanning cannot guarantee that every secret is caught, and deleting a committed secret does not erase it from Git history. **If a real Plex token or private key is exposed, revoke or rotate it immediately**, even if you later remove the file from the repository.

### Known limitations
- Plex login and music browsing have been confirmed on a physical Q2. The v0.6 optimizations still require hardware validation.
- Playback timeline reporting requires compatible Q2 Pod firmware. Search, playlists, five-star Favorites and artwork need physical-device regression testing.
- Server connections prefer local endpoints; connections selected from Plex discovery allow plain HTTP **only** to private/loopback IP addresses.
- Credential file `.plex/auth.json` includes a Plex access token and signing key. **Anyone with read access to the card can extract them.** Don't share this file. Future work will add encryption/device-bound storage.
- The Q2's system clock needs to be reasonably accurate for TLS validation and signed JWTs. The build includes a CA certificate bundle; it does not bypass TLS verification.
- On old Q2 Pod firmware without URL queue support, streaming will not work.

## Build and install

Requires Go 1.22+, Python 3, curl, and an internet connection for the CA bundle.

```sh
./contrib/shanlingq2/build.sh
```

Unzip `dist/q2-plex-v1.0.2.zip` into the **root** of the Q2's microSD card, preserving the hidden `.plex` folder. After replacing an existing version, restart the Q2 so the old executable is not still running. In Q2 Pod select **Streaming → Plex Music**. The app stays in Streaming, as in previous releases.

1. Select **Sign In with Plex**.
2. On your phone open https://plex.tv/link and enter the code shown on the Q2.
3. Once the app finds the server, browse **Albums**, **Artists**, **Playlists**, **Favorites**, or **Search**.
4. Select **Play Album** to stream the album in track order, or press a song to begin from that track.
5. Open **Play Queue** to jump to a queued song. Q2 Pod's hardware player handles skip and seek.

For diagnostics inspect `/tmp/q2-plex.log` on the Q2. Never paste `.plex/auth.json` into a public issue.

## Design
- **Plex API:** HTTPS PIN/JWK login, server discovery, music-only library filtering, metadata paging.
- **Q2 integration:** a Unix domain socket at `/tmp/q2-plex.browse.sock` using Q2SPOT/1.
- **Playback:** local relay `127.0.0.1:2328` with `/tmp/q2-plex.urls` + `/tmp/q2-plex.play`. Tokens are passed in HTTPS/HTTP request headers to PMS and **not** in song URLs.
- **Power:** Q2 Pod firmware controls the device. During downloads the app writes a temporary `hold=1` state to prevent standby, then clears it.
- No changes to the core Q2 Pod firmware are required.

## References
- [Q2 Pod card apps and Q2SPOT/1](https://github.com/DiamondBond/q2-pod/blob/main/docs/librespot.md)
- [q2-jellyfin](https://github.com/A4Link/q2-jellyfin)
- [Ampcast's Plex adapter](https://github.com/rekkyrosso/ampcast/tree/main/src/services/plex)
- [Plex developer API](https://developer.plex.tv/pms/)

## License
MIT. Source written for this project. Protocol details follow Q2 Pod's public documentation.

## v0.3.1 playback
- Album track queues are capped at 250 items to bound memory. Q2 Pod's native player handles next, previous, play/pause and seeks.
- Plex playback requests are proxied through the existing loopback relay; its HTTP client has no 9-second total request timeout for long tracks.
- The album track selected starts at its original position; subsequent tracks play automatically.
- Hardware playback, login, and FLAC compatibility still require validation on the physical Q2.

## v0.3.1 Music Library
- Search is available through the Q2 T9 search UI; queries use Plex hubs, limited to music artists, albums and tracks.
- Audio-only Plex playlists can be browsed and played in track order, with bounded queues (max 250 tracks).
- **Hold Play/Pause on a song row** to toggle five-star favorite status (Plex user rating 10); the Favorites page lists five-star tracks. This changes the Plex rating, not a separate Plexamp-only flag.
- Album/artist/playlist list covers use 64px JPEGs in a bounded /tmp cache. Now Playing art uses the existing token-free loopback relay.
- Existing login state in `.plex/auth.json` is preserved when replacing only bundled app files.
- **Known**: full library search result relevance depends on PMS; playlist duplicates resolve to the first matching track for Play-from-here; hardware tests still needed.

## v0.3.1 Plex connection troubleshooting
- Confirmed login can succeed while Plex server discovery fails. The app now indicates **Connecting to Plex** immediately and retains a concise error message on failure.
- Discovery checks at most eight endpoints with a four-second per-endpoint deadline, selects an actual **music** library, and reports connection failures without leaking tokens.
- **Manual Server** allows direct connection to `http://192.168.x.x:32400` for local Plex servers or a verified HTTPS URL. The Plex account must already be signed in and authorized for that server.
- Q2 firmware expects a reply within 15 seconds; discovery runs in a separate goroutine and the UI immediately returns a pending state.
- To diagnose a failed connect, inspect `/tmp/q2-plex.log` on your device. Do not share `.plex/auth.json`.

## v0.4.0 Offline Music

- Plex Music **stays under Streaming**. Q2 Pod currently requires Wi-Fi to launch a Streaming app. If you open the app while connected, downloaded audio plays from the SD card without requesting music from Plex. For use with Wi-Fi completely off from boot, browse the files under **PlexMusic/music** in Q2 Pod’s native Folder view; initial startup through Streaming may still wait for Wi-Fi (firmware limitation). Do not set `wifi=1` in the manifest.
- Open an online **Album → Download Album to SD**, or **Playlist → Download Playlist to SD**. Downloads are processed one at a time, in original FLAC/MP3/etc. quality with no transcoding.
- Check **Downloads** for progress or cancel. A partial collection is playable immediately; selecting Download again resumes incomplete files when Plex supports byte ranges, and skips already saved tracks when their source sizes match.
- Browse **Offline Music** for saved albums and playlists, play the entire collection or start at a selected song. Q2 Pod's native player reads local SD-card paths and handles previous/next/pause/seek. No Plex connection or streaming proxy is needed for those songs.
- **Remove Download...** requires an explicit confirmation and deletes only media no longer used by any other downloaded collection.
- All downloaded audio and the versioned manifest are under the visible **`PlexMusic/`** folder on the microSD card; **preserve the `PlexMusic/` folder and `.plex/auth.json` during upgrades**. Do not delete or reformat the card while downloading. No media files are ever included in release ZIPs.
- Downloads use bounded 32 KiB buffers, an atomic `.part` → completed-file rename, track-size limit of 512 MiB and a minimum 256 MiB free-space reserve to protect Q2 Pod's own database and updates.
- The application does not turn Q2 hardware power on/off; firmware does. An active download is canceled on process shutdown and its next attempt can resume partial content when the Plex source size is known.
- Current beta scope: download individual albums or audio playlists (up to 250 tracks each), direct original-quality files, offline playback and removal. No scheduled/automatic sync, re-encoding, offline artwork or cross-device position sync yet.
- If Plex authentication expires while offline, previously downloaded music remains available via **Offline Music**. Reconnect once online to download more music.

## v0.5.0 Playback synchronization and last queue

- The application **remains in Streaming → Plex Music**, and streaming remains the default. No device power-control changes.
- Where Q2 Pod firmware supports `now`, q2-plex adds `now=1` to native playback queues, reads `/tmp/q2-plex.now`, and forwards *actual* play/pause/stop and positions to Plex's `/:/timeline` approximately every ten seconds. Plex decides the resulting history and play counts; no duplicate direct scrobbles are sent.
- On firmware with `fav`, tapping Now Playing's heart writes a durable `.plex/fav.log` record that q2-plex applies to Plex five-star track ratings. The app also advertises `favdir=PlexMusic` so native local favorites can sync. Events not accepted by Plex remain queued to retry when connected.
- `Last Queue` stores track details, local paths, last song, and reported playback position in `.plex/last-queue.json`. Select **Last Queue → Restore Last Queue** to rebuild the native player queue from the last song.
- **Exact-second resume is not available** through the current documented Q2 card-app interface: the saved song restarts from its beginning. The previous position is displayed for reference.
- Older firmware without the new `now` or `fav` capabilities continues to play music and download albums, but cannot automatically report live progress or heart changes.
- Offline history backfill and automatic ratings reconciliation from other Plex clients are not included in this beta. A Plex connection is required to report listening history; downloaded music still plays locally.
- Preserve `.plex/auth.json`, `.plex/last-queue.json`, `.plex/fav.log`, and `PlexMusic/` while upgrading.

## v0.6.0 Stability, efficiency, and Wi-Fi recovery

- **Still under Streaming → Plex Music.** Existing audio streaming, native FLAC/MP3 decoding, downloaded music, account login, and `PlexMusic/` storage remain unchanged.
- **Lower idle overhead:** playback status is checked every **5 seconds** while playing or paused, every **30 seconds** when stopped/idle, and a new queue wakes the status monitor immediately. Unchanged Q2 status files are not re-parsed.
- **Fewer SD writes:** the last playback bookmark is saved on track/state changes, backwards seeks, or after **30 seconds** of progress instead of approximately every five seconds. The final bookmark is saved during normal Q2 app shutdown. Plex timeline reports are bounded to roughly every **30 seconds** during uninterrupted playback, plus immediate state transitions.
- **Album art:** one background cover worker instead of two, at most 16 pending artwork requests, bounded temporary cache and failed-request records. Caching remains RAM/temp backed and 64-pixel for list covers.
- **Wi-Fi recovery:** idempotent Plex metadata GETs retry once after transient network/HTTP 429/5xx failures. Interrupted original-quality downloads retry at most twice using safe `.part` file resume. Plex authentication, ratings, and playback-event writes are **not** automatically replayed.
- **Connection recovery:** startup tries the previously authorized Plex server before discovering through the cloud. If a connection fails, automatic reconnect backs off **30, 60, 120, 240, then 300 seconds maximum**, with a manual **Connection → Reconnect Now** command. There is no continuous server scanning while connected.
- **UI:** streaming libraries appear first, followed by Offline Music, Downloads, Last Queue, and a compact Connection menu with status/manual server/sign-out. Firmware controls font size; v0.6 does not patch the native theme.
- **Bounded work:** downloading still uses a 32 KiB transfer buffer and now throttles on-screen progress notifications to reduce per-chunk overhead. Existing 48 MiB heap goal and music-only scope stay intact.
- **Device testing required:** actual RAM/RSS, CPU time, standby power, Wi-Fi dropouts during streaming, and battery life have **not** been benchmarked on physical Q2 hardware. A live audio stream already in progress cannot be transparently resumed after a broken TCP response; Q2 native player retry/skip behavior must be observed. This milestone improves recovery, not a guarantee of gapless playback across Wi-Fi loss.
