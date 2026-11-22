# q2-plex Development Roadmap

Updated: 2026-11-22

## Guiding architecture

**q2-plex is a music-only Plex client; Q2 Pod is the audio player.** q2-plex handles Plex sign-in, music library browsing/search, playlists, favorites, source authorization, track metadata, loopback stream relay when necessary, optional SD-card downloads, and Plex-specific timeline/rating synchronization. The Q2 Pod firmware owns audio decoding, actual play/pause/seek/skip, hardware controls, audio output, Now Playing UI and supported native queues. Avoid duplicating native player behavior.

Keep the app in **Streaming → Plex Music**. Streaming stays the default; offline downloads remain optional. Do not modify core firmware to implement Plex-specific logic, power controls, or a second decoder.

Reference contract: [Q2 Pod card apps / Q2SPOT/1](https://github.com/DiamondBond/q2-pod/blob/main/docs/librespot.md). Check the installed firmware's capabilities before relying on optional features.

## Completed baseline (v0.2.0–v0.6.0)

- Album streaming, track-order queues and native playback integration.
- Music search, playlists, five-star favorites, and artwork.
- Optional original-quality album/playlist downloads and offline playback.
- Plex timeline reporting, persisted last queue and favorite-event journal, with firmware capability requirements.
- Bounded retries, lower idle polling, fewer SD writes and connection recovery.

Automated builds passed previously, but real-device validation remains incomplete. A successful Plex login and visible music library do **not** imply that full-length playback is proven.

## v0.7.0 — Native Q2 player integration audit (BETA MERGED)

1. **Trace the playback path:** inspect q2-plex's generated `/tmp/q2-plex.urls` and `/tmp/q2-plex.play`, local token-free relay, track metadata, artwork paths, and `Q2SPOT/1` browsing commands against the current Q2 Pod implementation. Document where the firmware owns each behavior.
2. **Keep responsibilities narrow:** delegate playback, queue transitions, gapless behavior where native-supported, pause, seek, previous/next, Now Playing and hardware controls to Q2 Pod. Remove only demonstrably duplicated or conflicting app-side behavior. Retain the relay when it is needed for authenticated Plex streams.
3. **Respect capability detection:** gate `now`, `fav`, `art`, `pages`, `progress` and other optional card-app behavior on firmware support; retain fallbacks for older releases. Do not assume every native feature is exposed to all card apps.
4. **Protect existing behavior:** retain Plex timeline reporting, favorite synchronization, last-queue recovery, offline files, downloads, login and original-quality streaming.
5. **Test and verify:** unit/regression tests, Go formatting, MIPS packaging and CI; then physical-Q2 testing of a complete FLAC album, next/previous, play/pause, seeking, artwork, long streams, local downloads and Wi-Fi interruption/recovery.

**Beta status:** Native playback ownership documented; full 250-track queue pagination implemented. PR #8 merged. The offline-queue build call was corrected after CI detected a compilation failure. New CI and physical-device validation remain to be confirmed. Production-ready status is deferred; v0.8 will address observed hardware defects.

## v0.8.0 — Hardware-found bug fixes and UI polish

Improve browse hierarchy, music search, list pagination, artwork responsiveness, loading/error states and click-wheel ergonomics using Q2 Pod's existing card-app UI. Prefer native Q2 Pod visual conventions; avoid a custom player screen.

## v0.9.0 — Performance and battery validation (IN PROGRESS)

- Added opt-in **Connection → Performance Diagnostics**, with 60-second process samples, bounded to 120 samples in RAM.
- Sample Q2 Plex process CPU time/usage, Go heap, process peak RSS and goroutine count. Read battery percentage from the Q2 sysfs interface when available; this is *not* a battery-current/power sensor.
- Write anonymized `.plex/diagnostics.csv` **only on explicit Export**, avoiding ongoing SD writes. Do not log filenames, listening history, Plex tokens or server addresses.
- Physical device benchmarking still required before claiming better battery life. Run matched 30–60-minute trials for Q2 native local music, q2-plex Wi-Fi streaming and Plex downloads, with the screen brightness/audio volume/output held constant. Measure initial/final charge, CPU, RSS and any stutters. Q2 Pod retains power management and audio decoding.
- Tune only bottlenecks observed from recorded measurements in Plex API, relay, caching, downloads and sync.

## v1.0.0 — Stable release

Regression test sign-in, streaming, playlists, search, favorites, downloads, queue restore, firmware compatibility and upgrades. Publish documented install package with clear compatibility matrix and known limitations.

## Deferred / limitations

- Phone-hotspot/Plexamp phone relay remains future work, outside the v0.9 performance milestone.


- Exact-second resume cannot be promised unless Q2 Pod exposes a compatible seek-on-restore API.
- Offline history backfill is not included yet.
- Starting a Streaming app with Wi-Fi disabled may remain subject to firmware launch behavior. Native Folder view can still open downloaded audio.
- No video playback or custom audio decoding.
