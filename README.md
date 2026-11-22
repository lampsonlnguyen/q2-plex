# q2-plex

**Experimental v0.3.1** — music-only Plex client for the Shanling Q2 with Q2 Pod firmware.

This is an initial implementation, **not a tested production release**. Inspired by Q2 Pod's generic card app API, A4Link/q2-jellyfin (MIT), and the Plex authentication and library APIs. No video playback or video decoding.

## Included in the first milestone
- Plex account login using a 4-character PIN on **https://plex.tv/link** and Ed25519-signed device JWT.
- Saving the device identity and access token on the microSD card; proactive token refresh after five days.
- Finding a connected Plex Media Server and listing **music** libraries, albums, artists, tracks.
- Q2 Pod's native list/selection protocol and direct FLAC/MP3 streaming with bounded album queues through a loopback proxy.
- A 48 MiB Go heap target; server paging is 25 rows per API request. No audio decoding inside this app.

### Known limitations
- No physical Q2 hardware testing yet. This beta may require changes to device login and server discovery.
- No offline downloads or playback reporting yet. Search, playlists, five-star Favorites and cover art are experimental and require device testing.
- Server connections prefer local endpoints; connections selected from Plex discovery allow plain HTTP **only** to private/loopback IP addresses.
- Credential file `.plex/auth.json` includes a Plex access token and signing key. **Anyone with read access to the card can extract them.** Don't share this file. Future work will add encryption/device-bound storage.
- The Q2's system clock needs to be reasonably accurate for TLS validation and signed JWTs. The build includes a CA certificate bundle; it does not bypass TLS verification.
- On old Q2 Pod firmware without URL queue support, streaming will not work.

## Build and install

Requires Go 1.22+, Python 3, curl, and an internet connection for the CA bundle.

```sh
./contrib/shanlingq2/build.sh
```

Unzip `dist/q2-plex-v0.3.1.zip` into the **root** of the Q2's microSD card, preserving the hidden `.plex` folder. After replacing an existing version, restart the Q2 so the old executable is not still running. In Q2 Pod select **Streaming → Plex Music**.

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
- **Power:** Q2 Pod firmware controls the device. The app handles SIGTERM and Q2 Pod controls system power.
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
