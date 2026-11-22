#!/bin/sh
# Copy executable to /tmp so USB storage can unmount the card.
pid=/tmp/q2-plex
if [ "$1" != "--loop" ]; then
  if [ -f "$pid" ] && kill -0 "$(cat "$pid")" 2>/dev/null; then exit 0; fi
  dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
  cp -f "$0" /tmp/q2-plex.run || exit 1
  /bin/sh /tmp/q2-plex.run --loop "$dir" </dev/null >/dev/null 2>&1 &
  echo $! > "$pid"
  exit 0
fi
dir=$2
cp -f "$dir/q2-plex" /tmp/q2-plex.bin || exit 1
chmod 755 /tmp/q2-plex.bin
child=
trap '[ -n "$child" ] && kill -TERM "$child" 2>/dev/null; rm -f /tmp/q2-plex /tmp/q2-plex.bin /tmp/q2-plex.run; exit 0' TERM INT
/tmp/q2-plex.bin -dir "$dir" >/tmp/q2-plex.log 2>&1 &
child=$!
wait "$child"
rm -f /tmp/q2-plex /tmp/q2-plex.bin /tmp/q2-plex.run
