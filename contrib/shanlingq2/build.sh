#!/bin/sh
# Build an installable .plex card app for Q2 Pod. Requires Go 1.22+, python3 and curl.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
OUT="$ROOT/dist"
STAGE="$OUT/.plex"
mkdir -p "$OUT" "$STAGE"
cd "$ROOT"
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -trimpath -ldflags="-s -w -buildid=" -o "$STAGE/q2-plex" ./cmd/q2-plex
cp contrib/shanlingq2/q2app "$STAGE/q2app"
cp contrib/shanlingq2/launcher.sh "$STAGE/run"
chmod 755 "$STAGE/run" "$STAGE/q2-plex"
# Q2 has no system root CA bundle: ship Mozilla roots for VERIFIED TLS.
# Never silently skip cert verification.
if [ -n "${Q2_PLEX_CA_FILE:-}" ] && [ -r "$Q2_PLEX_CA_FILE" ]; then
  cp "$Q2_PLEX_CA_FILE" "$STAGE/cacert.pem"
else
  curl -fsSL --max-time 30 https://curl.se/ca/cacert.pem -o "$STAGE/cacert.pem"
fi
python3 contrib/shanlingq2/package.py "$STAGE" "$OUT/q2-plex-v0.2.0.zip"
