#!/usr/bin/env bash
#
# start_container.sh — start the Aurify stack with docker compose.
#
# The PostgreSQL service stores its data on the host at ./.data/postgres via a
# bind-backed named volume (see docker-compose.yml). Because that bind uses
# `type: none, o: bind`, Docker will NOT create the source directory for you, so
# this script guards it: if the directory is missing and --make-mount was not
# passed, it fails fast instead of letting `docker compose` error out cryptically
# (or create a root-owned directory).

set -euo pipefail

# Operate from the repo root (this script's directory) so ${PWD} in the compose
# file resolves correctly no matter where the script is invoked from.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

PG_DATA_DIR="$ROOT_DIR/.data/postgres"

detached=false
make_mount=false
build=false
services=()

usage() {
  cat <<'EOF'
Usage: ./start_container.sh [options] [service...]

Start the Aurify stack (docker compose up). With no service names it starts
everything; naming services starts only those, e.g. running the API natively
against the containerised dependencies:

  ./start_container.sh -dm postgres mailpit

Options:
  -d, --detached     run in the background (docker compose up -d)
  -m, --make-mount   create the PostgreSQL host data directory if it is missing
  -b, --build        rebuild images before starting (docker compose up --build)
  -h, --help         show this help and exit

Flags can be combined, e.g.:  ./start_container.sh -dmb

PostgreSQL data is persisted on the host at ./.data/postgres. If that directory
does not exist and --make-mount/-m is not given, the script exits with an error.
EOF
}

# Set a single short flag by its letter.
set_flag() {
  case "$1" in
    d) detached=true ;;
    m) make_mount=true ;;
    b) build=true ;;
    h) usage; exit 0 ;;
    *) echo "start_container.sh: unknown flag '-$1'" >&2; exit 2 ;;
  esac
}

# Parse flags: long forms (--build), short forms (-b), and stacked short
# clusters (-dmb).
while [[ $# -gt 0 ]]; do
  case "$1" in
    --detached)   detached=true ;;
    --make-mount) make_mount=true ;;
    --build)      build=true ;;
    --help)       usage; exit 0 ;;
    --)           shift; break ;;
    --*)
      echo "start_container.sh: unknown option '$1'" >&2
      echo "try: ./start_container.sh --help" >&2
      exit 2
      ;;
    -?*)
      cluster="${1#-}"
      for (( i = 0; i < ${#cluster}; i++ )); do
        set_flag "${cluster:i:1}"
      done
      ;;
    # Anything that is not a flag is a compose service name, passed straight
    # through. Compose itself validates the names.
    *) services+=("$1") ;;
  esac
  shift
done

# Whatever followed `--`.
services+=("$@")

command -v docker >/dev/null 2>&1 || {
  echo "start_container.sh: docker is not installed or not on PATH" >&2
  exit 1
}

# Guard the PostgreSQL bind-mount source directory.
if $make_mount; then
  mkdir -p "$PG_DATA_DIR"
  echo "✓ ensured PostgreSQL data directory: $PG_DATA_DIR"
elif [[ ! -d "$PG_DATA_DIR" ]]; then
  echo "✗ PostgreSQL data directory is missing: $PG_DATA_DIR" >&2
  echo "  Re-run with --make-mount/-m to create it (or run: mkdir -p .data/postgres)." >&2
  exit 1
fi

# Assemble and run the compose command.
compose_args=(compose up)
if $build; then compose_args+=(--build); fi
if $detached; then compose_args+=(--detach); fi
# `${arr[@]+...}` so an empty array is not an unbound variable under `set -u`
# (bash 3.2, which is what macOS ships).
compose_args+=(${services[@]+"${services[@]}"})

echo "→ docker ${compose_args[*]}"
exec docker "${compose_args[@]}"
