#!/usr/bin/env bash
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/../common.sh"
[[ "${GOOS:-linux}/${GOARCH:-amd64}" == linux/amd64 && "${CGO_ENABLED:-0}" == 0 ]] || fail 'Module images require static Linux AMD64 Go executables (CGO_ENABLED=0)'
use_go
export GO_BINARY
safe_path .tools/images
safe_file .tools/images/build.lock
safe_path .tools/media
for file in examples/general-bot/manifest.json modules/music-bot/manifest.json modules/audio-player/manifest.json tests/integration/fixtures/sandbox/manifest.json bin/test-module bin/chatanium-musicbot bin/voice-player-module bin/sandbox-module; do
    safe_file "$file"
done
if [[ "${1:?Expected module or media}" == module ]]; then
    # Reuse the approved distro pin without introducing an independently
    # maintained image version or allowing a floating trust-store source.
    run python3 -c '
import json
from pathlib import Path
import sys
sys.path.insert(0, "tools")
from sdk import resolve

sdk = resolve(sys.argv[1])
base = json.loads((Path(sdk["Dir"]) / "tools/native/native-lock.json").read_text())["builder_base"]
source = Path("tools/build/Module.Dockerfile").read_text().splitlines()
if f"FROM {base} AS trust" not in source:
    raise SystemExit("Module trust source must match native-lock.json builder_base")
' "$GO_BINARY"
fi
run python3 tools/build/images.py "${1:?Expected module or media}"
