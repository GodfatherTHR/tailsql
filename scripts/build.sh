#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bash "$ROOT/scripts/fetch-h2.sh"
bash "$ROOT/scripts/build-bridge.sh"

pushd "$ROOT/frontend" >/dev/null
if [[ ! -d node_modules ]]; then
  npm install --include=dev
fi
npm run build
popd >/dev/null

if ! command -v wails >/dev/null 2>&1; then
  echo "Wails CLI not found. Install it with:" >&2
  echo "  go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1" >&2
  exit 1
fi

pushd "$ROOT" >/dev/null
wails build
mkdir -p build/bin/assets
cp assets/h2.jar build/bin/assets/
if [[ -f assets/h2.jar.sha256 ]]; then
  cp assets/h2.jar.sha256 build/bin/assets/
fi
popd >/dev/null

echo "Build complete: $ROOT/build/bin/HySQL"
