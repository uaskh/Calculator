#!/usr/bin/env bash
# Formats the files that go into the next commit and re-stages them.
#   Go files        → golangci-lint fmt (module has .golangci.yml), else goimports, else gofmt
#   frontend files  → the project's Prettier (frontend/node_modules/.bin/prettier)
#
# Usage:
#   format-staged.sh             staged files only (git pre-commit hook)
#   format-staged.sh --worktree  every changed/untracked file; re-stages the ones that were
#                                fully staged (used when the repo has its own git hooks)
#
# Files with both staged and unstaged changes are left alone in pre-commit mode, so
# unrelated work is never swept into a commit. Exits 1 if a formatter reports errors.

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

is_on "$FORMAT_ON_COMMIT" || exit 0
mode="staged"
[ "${1:-}" = "--worktree" ] && mode="worktree"

root="$(git rev-parse --show-toplevel 2>/dev/null)" || exit 0
cd "$root" || exit 0

staged="$(git -c core.quotePath=off diff --cached --name-only --diff-filter=ACMR)"
unstaged="$(git -c core.quotePath=off diff --name-only --diff-filter=ACMR)"
if [ "$mode" = "worktree" ]; then
  untracked="$(git -c core.quotePath=off ls-files --others --exclude-standard)"
  changed_files="$(printf '%s\n%s\n%s\n' "$staged" "$unstaged" "$untracked" | sort -u)"
else
  changed_files="$staged"
fi
[ -n "$(printf '%s' "$changed_files" | tr -d '[:space:]')" ] || exit 0

in_list() { # in_list <item> <newline-separated list>
  printf '%s\n' "$2" | grep -Fqx -- "$1"
}

go_files=""
web_files=""
restage=""
skipped=""
while IFS= read -r f; do
  [ -n "$f" ] && [ -f "$f" ] || continue
  case "/$f" in
    */node_modules/* | "/$BACKEND_DIR/vendor/"* | "/$FRONTEND_DIR/dist/"* | "/$FRONTEND_DIR/coverage/"* | /coverage/*) continue ;;
  esac
  kind=""
  case "$f" in
    *.go) kind="go" ;;
    "$FRONTEND_DIR"/package-lock.json) ;;
    "$FRONTEND_DIR"/*.js | "$FRONTEND_DIR"/*.jsx | "$FRONTEND_DIR"/*.mjs | "$FRONTEND_DIR"/*.cjs | \
      "$FRONTEND_DIR"/*.ts | "$FRONTEND_DIR"/*.tsx | "$FRONTEND_DIR"/*.mts | "$FRONTEND_DIR"/*.cts | \
      "$FRONTEND_DIR"/*.json | "$FRONTEND_DIR"/*.css | "$FRONTEND_DIR"/*.scss | "$FRONTEND_DIR"/*.html | \
      "$FRONTEND_DIR"/*.md | "$FRONTEND_DIR"/*.mdx | "$FRONTEND_DIR"/*.yml | "$FRONTEND_DIR"/*.yaml)
      kind="web"
      ;;
  esac
  [ -n "$kind" ] || continue

  partial=0
  in_list "$f" "$unstaged" && partial=1
  if [ "$mode" = "staged" ] && [ "$partial" -eq 1 ]; then
    skipped="$skipped $f"
    continue
  fi
  if in_list "$f" "$staged" && [ "$partial" -eq 0 ]; then
    restage="$restage
$f"
  fi
  if [ "$kind" = "go" ]; then
    go_files="$go_files
$f"
  else
    web_files="$web_files
${f#"$FRONTEND_DIR"/}"
  fi
done <<EOF
$changed_files
EOF

status=0
count=0

# ---- Go ------------------------------------------------------------------------------
# Preferred: `golangci-lint fmt` with the module's .golangci.yml (same formatters and
# import grouping as CI). Fallback: goimports -local <module>, then plain gofmt.
find_tool() { # find_tool <name> — PATH first, then $(go env GOPATH)/bin
  if command -v "$1" >/dev/null 2>&1; then
    command -v "$1"
  elif command -v go >/dev/null 2>&1 && [ -x "$(go env GOPATH 2>/dev/null)/bin/$1" ]; then
    printf '%s/bin/%s\n' "$(go env GOPATH 2>/dev/null)" "$1"
  fi
}

format_go_file() { # format_go_file <path relative to repo root>
  _f="$1"
  _mod="$(dirname "$_f")"
  while [ "$_mod" != "." ] && [ "$_mod" != "/" ] && [ ! -f "$_mod/go.mod" ]; do _mod="$(dirname "$_mod")"; done
  if [ -n "$golangci" ] && [ -f "$_mod/go.mod" ] &&
    { [ -f "$_mod/.golangci.yml" ] || [ -f "$_mod/.golangci.yaml" ]; }; then
    (cd "$_mod" && "$golangci" fmt "${_f#"$_mod"/}" 2>&1)
    return
  fi
  if [ -n "$goimports" ]; then
    _module=""
    [ -f "$_mod/go.mod" ] && _module="$(go_module_path "$_mod/go.mod")"
    if [ -n "$_module" ]; then
      "$goimports" -local "$_module" -w "$_f" 2>&1
    else
      "$goimports" -w "$_f" 2>&1
    fi
    return
  fi
  gofmt -w "$_f" 2>&1
}

if [ -n "$go_files" ]; then
  golangci="$(find_tool golangci-lint)"
  goimports="$(find_tool goimports)"
  if [ -z "$golangci" ] && [ -z "$goimports" ] && ! command -v gofmt >/dev/null 2>&1; then
    warn "format-on-commit: no Go formatter found (gofmt); Go files were not formatted."
  else
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      if out="$(format_go_file "$f")"; then
        count=$((count + 1))
      else
        status=1
        printf '%s\n' "$out" >&2
      fi
    done <<EOF
$go_files
EOF
  fi
fi

# ---- frontend (Prettier) ----------------------------------------------------------------
if [ -n "$web_files" ]; then
  if [ -x "$FRONTEND_DIR/node_modules/.bin/prettier" ]; then
    set --
    while IFS= read -r f; do
      [ -n "$f" ] && set -- "$@" "$f"
    done <<EOF
$web_files
EOF
    if [ "$#" -gt 0 ]; then
      if (cd "$FRONTEND_DIR" && ./node_modules/.bin/prettier --write --ignore-unknown --log-level=warn -- "$@" >/dev/null); then
        count=$((count + $#))
      else
        status=1
      fi
    fi
  else
    warn "format-on-commit: Prettier is not installed in $FRONTEND_DIR/ (run npm ci); frontend files were not formatted."
  fi
fi

# ---- re-stage what was fully staged -------------------------------------------------------
while IFS= read -r f; do
  [ -n "$f" ] && [ -f "$f" ] && git add -- "$f"
done <<EOF
$restage
EOF

[ -z "$skipped" ] || warn "format-on-commit: not formatted (staged and unstaged changes in the same file):$skipped"
if [ "$status" -ne 0 ]; then
  warn "format-on-commit: a formatter reported errors (see above); fix them and commit again."
  exit 1
fi
[ "$count" -eq 0 ] || warn "format-on-commit: formatters ran on $count file(s)."
exit 0
