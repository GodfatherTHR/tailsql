#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${H2_VERSION:-2.3.232}"
DESTINATION="${1:-$ROOT/assets}"
TARGET="$DESTINATION/h2.jar"

for dependency in curl unzip; do
  if ! command -v "$dependency" >/dev/null 2>&1; then
    echo "$dependency is required to fetch the H2 engine." >&2
    exit 1
  fi
done

mkdir -p "$DESTINATION"

valid_jar() {
  [[ -f "$1" ]] || return 1
  unzip -l "$1" 2>/dev/null | grep -q "org/h2/tools/Server.class" &&
    unzip -l "$1" 2>/dev/null | grep -q "org/h2/Driver.class" &&
    unzip -l "$1" 2>/dev/null | grep -q "org/h2/tools/RunScript.class"
}

if valid_jar "$TARGET"; then
  echo "H2 $VERSION already present and valid at $TARGET"
  exit 0
fi

URL="https://repo1.maven.org/maven2/com/h2database/h2/$VERSION/h2-$VERSION.jar"
TEMP="$TARGET.download"
rm -f "$TEMP"

echo "Downloading H2 $VERSION from Maven Central..."
curl --fail --location --silent --show-error --output "$TEMP" "$URL"
valid_jar "$TEMP" || { rm -f "$TEMP"; echo "Downloaded file is not a valid H2 jar" >&2; exit 1; }
mv "$TEMP" "$TARGET"

if command -v sha256sum >/dev/null 2>&1; then
  SHA="$(sha256sum "$TARGET" | awk '{print $1}')"
  printf '%s  h2.jar\n' "$SHA" > "$TARGET.sha256"
  echo "SHA-256: $SHA"
fi
echo "Installed $TARGET"
