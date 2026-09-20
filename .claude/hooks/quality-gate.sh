#!/usr/bin/env bash
# Stop hook — quality gate. When backend or frontend code changed, Claude cannot end its
# turn until the fast checks pass:
#   backend : go build, go vet, go test -short (+ standard-library-only go.mod)
#   frontend: npm run typecheck, npm run lint, npm test
# Exit 2 keeps Claude working and feeds the failures back to it. Formatting is not
# checked here: it is applied automatically on commit. After QUALITY_GATE_MAX_RETRIES
# failed attempts in a row the turn may end, with a warning for the user.
#
# To pause the gate for one turn yourself: touch .claude/.state/skip-quality-gate
# (or set QUALITY_GATE="off" in hooks.conf).

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

is_on "$QUALITY_GATE" || exit 0
read_hook_input
detect_json_parser
ensure_state_dir

attempts_file="$STATE_DIR/gate-attempts"
skip_file="$STATE_DIR/skip-quality-gate"
dirty_file="$STATE_DIR/dirty"

# notice <text> — shown to the user; the turn still ends normally.
notice() {
  printf '{"systemMessage": "Quality gate: %s"}\n' "$1"
}

if [ -f "$skip_file" ]; then
  rm -f "$skip_file" "$attempts_file"
  exit 0
fi

if [ ! -s "$dirty_file" ]; then
  rm -f "$attempts_file"
  exit 0
fi

# Subagents may still be editing files; the gate runs when the last one has finished.
running="$(running_subagents)"
if [ "$running" -gt 0 ]; then
  notice "checks deferred while $running subagent(s) are still running."
  exit 0
fi

# Count consecutive blocks within one stop sequence; a fresh stop starts at zero.
attempts=0
if [ "$JSON_PARSER" = "none" ] || [ "$(json_get stop_hook_active)" = "true" ]; then
  attempts="$(cat "$attempts_file" 2>/dev/null)"
  case "$attempts" in '' | *[!0-9]*) attempts=0 ;; esac
fi
if [ "$attempts" -ge "$QUALITY_GATE_MAX_RETRIES" ]; then
  rm -f "$attempts_file"
  notice "checks still fail after $attempts attempts. Run /verify to see the details."
  exit 0
fi

report="$(mktemp "${TMPDIR:-/tmp}/quality-gate.XXXXXX")" || exit 0
failed=0
pending="" # parts that could not be checked yet; they stay marked as changed
skipped="" # why a part was not checked

# run_check <label> <dir> <command...>
run_check() {
  _label="$1"
  _dir="$2"
  shift 2
  _out="$(cd "$_dir" && "$@" 2>&1)"
  _status=$?
  if [ "$_status" -ne 0 ]; then
    failed=1
    {
      printf '\n### %s (exit %s, in %s/)\n' "$_label" "$_status" "$(rel_path "$_dir")"
      printf '%s\n' "$_out" | tail -n 40
    } >>"$report"
  fi
  return "$_status"
}

if grep -qx backend "$dirty_file"; then
  bdir="$PROJECT_DIR/$BACKEND_DIR"
  if [ ! -f "$bdir/go.mod" ]; then
    :
  elif ! command -v go >/dev/null 2>&1; then
    skipped="$skipped backend checks skipped because go is not installed."
  else
    if [ "$GO_DEPENDENCY_POLICY" = "stdlib-only" ] && grep -Eq "$GO_MOD_DEPS_RE" "$bdir/go.mod"; then
      failed=1
      printf '\n### %s/go.mod declares module dependencies, but the backend policy is standard library only\n' "$BACKEND_DIR" >>"$report"
    fi
    run_check "go build ./..." "$bdir" go build ./... &&
      run_check "go vet ./..." "$bdir" go vet ./... &&
      run_check "go test -short ./..." "$bdir" go test -count=1 -short -timeout=5m ./...
  fi
fi

if grep -qx frontend "$dirty_file"; then
  fdir="$PROJECT_DIR/$FRONTEND_DIR"
  if [ ! -f "$fdir/package.json" ]; then
    :
  elif [ ! -d "$fdir/node_modules" ]; then
    pending="frontend"
    skipped="$skipped frontend checks skipped until dependencies are installed (npm ci in $FRONTEND_DIR/)."
  elif ! command -v npm >/dev/null 2>&1; then
    skipped="$skipped frontend checks skipped because npm is not installed."
  else
    run_check "npm run typecheck" "$fdir" npm run --silent --if-present typecheck &&
      run_check "npm run lint" "$fdir" npm run --silent --if-present lint &&
      run_check "npm test" "$fdir" npm run --silent --if-present test
  fi
fi

if [ "$failed" -eq 0 ]; then
  rm -f "$attempts_file" "$report"
  if [ -n "$pending" ]; then
    printf '%s\n' "$pending" >"$dirty_file"
  else
    rm -f "$dirty_file"
  fi
  [ -z "$skipped" ] || notice "${skipped# }"
  exit 0
fi

attempts=$((attempts + 1))
printf '%s\n' "$attempts" >"$attempts_file"
{
  printf 'Quality gate failed (attempt %s of %s): the code you changed does not pass its checks.\n' "$attempts" "$QUALITY_GATE_MAX_RETRIES"
  cat "$report"
  [ -z "$skipped" ] || printf '\nNot checked:%s\n' "$skipped"
  printf '\nFix the root cause (never skip, delete or weaken tests or lint rules), re-run the failing command, then finish.\n'
  printf 'If you need a decision from the user first, ask with AskUserQuestion: waiting for the answer does not end the turn.\n'
} >&2
rm -f "$report"
exit 2
