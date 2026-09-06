#!/bin/sh
# Run from the repository root. Requires Go, Python 3, VHS, ttyd and ffmpeg.
set -eu
DEMO_DIR=$(mktemp -d "${TMPDIR:-/tmp}/oku-recording.XXXXXX")
trap 'rm -rf "$DEMO_DIR"' EXIT
export XDG_CONFIG_HOME="$DEMO_DIR/config"
export XDG_DATA_HOME="$DEMO_DIR/data"
export HARDCOVER_TOKEN=demo-placeholder
export OKU_TUI_DEMO_DATA=1
export PATH="$DEMO_DIR/bin:$PATH"
mkdir -p "$DEMO_DIR/bin"
go build -o "$DEMO_DIR/bin/oku" ./cmd/oku
# Initialize only the disposable database; this command needs no API.
oku timer status >/dev/null
python3 docs/demo/seed.py
vhs oku-demo.tape
