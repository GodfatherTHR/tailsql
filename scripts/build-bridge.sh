#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
JAR="$ROOT/assets/h2.jar"

if [[ ! -f "$JAR" ]]; then
  echo "H2 jar missing; fetching it first."
  bash "$ROOT/scripts/fetch-h2.sh"
fi

if ! command -v javac >/dev/null 2>&1; then
  echo "javac was not found. Install a JDK 11+ to rebuild the embedded JDBC bridge." >&2
  exit 1
fi

echo "Compiling JdbcBridge with --release 11..."
javac --release 11 -cp "$JAR" -d "$ROOT/internal/jdbc" "$ROOT/internal/jdbc/JdbcBridge.java"
echo "Bridge classes written to $ROOT/internal/jdbc"
