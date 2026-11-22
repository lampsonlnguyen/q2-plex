# v0.7 Native Q2 Pod Playback Integration Audit

Status: in progress; physical-device verification pending.

## Verified source-level handoff

- q2-plex generates `/tmp/q2-plex.urls` with the Q2 URL queue header, selected start index, song URL or local card path, title, artist, album, cover URL, duration, Plex ID and favorite state.
- q2-plex atomically replaces the queue before writing the `/tmp/q2-plex.play` sequence trigger. Q2 Pod imports the queue into its stock Tidal/native player.
- q2-plex's loopback HTTP relay on `127.0.0.1:2328` fetches authenticated original-quality Plex audio and album covers. It does **not** decode audio or receive hardware playback commands.
- Q2 Pod handles decoding, output routing, seek, pause, next/previous and player screen for these queues. Its optional `now=1` status file reports real playback state and position; q2-plex reads it for Plex timeline and saved queue state. The app does not synthesize player position.
- Downloaded tracks are handed to the same native player as local `/mnt/mmc/PlexMusic/music/` paths, bypassing the Plex stream relay.
- Q2SPOT/1 owns browser rows, selection, interaction, navigation and list cursors. The Up Next list now pages all queued tracks in windows of 40 instead of hiding entries past 40. Pressing a queued row submits a new native queue starting at its absolute index.
- Plex sign-in, metadata, auth-scoped relay, Plex favorite-event sync and server timeline calls remain app responsibilities.

## Intentional limits

- The app's `playqueue` action regenerates a native queue to jump to a selected track. There is no documented card-app API to seek directly inside an existing Q2 stock queue by absolute index; do not implement synthetic button presses.
- The Plex relay remains required to attach credentials while keeping tokens out of URL queue files. Do not remove it merely because Q2 Pod decodes the audio.
- `now`, `fav`, `art`, `progress` and similar firmware extensions may be absent on older Q2 Pod firmware. Verify firmware capabilities before modifying fallback behavior.
- Exact-second resume is not implemented; restoring the last queue currently starts at the saved track's beginning. Do not misrepresent this as exact seek resume.
- Streaming entry remains **Streaming → Plex Music**; no firmware modifications, power-on/off functions, local decoder, or custom Now Playing screen.

## v0.7 verification checklist

- [x] Trace queue creation, relay, native player handoff, status reader and optional offline local paths at source level.
- [x] Replace the Up Next 40-track truncation with native Q2SPOT pagination.
- [x] Add regression tests for all 250 queue entries, absolute track indexing and invalid page cursors.
- [ ] CI: formatting, Go tests and Q2 MIPS packaging.
- [ ] Hardware: full-length FLAC album, next/previous, pause, seek, cover loading and playlist duplicates.
- [ ] Hardware: disconnect/reconnect Wi-Fi during streaming, optional offline playback and Plex timeline accuracy.
- [ ] Record physical RAM/CPU/battery observations before claiming a v0.7 stable beta.

References:
- [Q2 Pod native streaming queue implementation](https://github.com/DiamondBond/q2-pod/blob/main/docs/internals.md)
- [Q2 Pod card app browsing protocol](https://github.com/DiamondBond/q2-pod/blob/main/docs/librespot.md)
