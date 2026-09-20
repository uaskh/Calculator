#!/usr/bin/env bash
# Runs the backend and the frontend dev servers together (make dev).
# Ctrl+C, or either server stopping, stops both.
set -euo pipefail
cd "$(dirname "$0")/.."

# Job control gives each server its own process group, so stopping a group also stops the
# processes it started (the binary built by go run, Vite's workers). Standard input is
# detached so neither server waits for terminal input.
set -m
make --no-print-directory run-backend </dev/null &
backend=$!
make --no-print-directory run-frontend </dev/null &
frontend=$!
ticker=""

# shellcheck disable=SC2329 # invoked by the traps below
stop() {
  trap - INT TERM EXIT
  kill -TERM -- "-$backend" "-$frontend" 2>/dev/null || true
  [ -z "$ticker" ] || kill "$ticker" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap 'stop; exit 130' INT
trap 'stop; exit 143' TERM
trap stop EXIT

# Poll with a background sleep: `wait` returns as soon as a trapped signal arrives, and the
# terminal stays with make, so Ctrl+C reaches this script.
while kill -0 "$backend" 2>/dev/null && kill -0 "$frontend" 2>/dev/null; do
  sleep 1 &
  ticker=$!
  wait "$ticker" || true
done
echo "dev: a server stopped; stopping the other one." >&2
exit 1
