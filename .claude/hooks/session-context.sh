#!/usr/bin/env bash
# SessionStart: prints a short project snapshot that Claude Code adds to the context
# (components, specs and plan progress, toolchain, git, hook status). Also enables the
# format-on-commit git hook when the project is a git repository. Never blocks.
# Safe to run by hand: bash .claude/hooks/session-context.sh </dev/null

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

if [ -t 0 ]; then HOOK_INPUT=""; else read_hook_input; fi
detect_json_parser
source="$(json_get source)"
ensure_state_dir
# A new Claude Code process starts without running subagents.
[ "$source" = "startup" ] && rm -f "$STATE_DIR"/agents/* 2>/dev/null

say() { printf '%s\n' "$*"; }
version_of() { # version_of <cmd> <args...>
  if command -v "$1" >/dev/null 2>&1; then
    "$@" 2>/dev/null | head -n 1 | sed -n -E 's/[^0-9]*([0-9]+(\.[0-9]+)+).*/\1/p'
  else
    printf 'missing'
  fi
}

say "## Project snapshot (.claude/hooks/session-context.sh)"

# ---- components ------------------------------------------------------------------------
bdir="$PROJECT_DIR/$BACKEND_DIR"
fdir="$PROJECT_DIR/$FRONTEND_DIR"
if [ -f "$bdir/go.mod" ]; then
  mod="$(go_module_path "$bdir/go.mod")"
  gover="$(sed -n -E 's/^go[[:space:]]+([0-9.]+).*/\1/p' "$bdir/go.mod" | head -n 1)"
  say "- Backend: $BACKEND_DIR/ (module $mod, go $gover)"
else
  say "- Backend: not scaffolded yet ($BACKEND_DIR/go.mod missing)"
fi
if [ -f "$fdir/package.json" ]; then
  if [ -d "$fdir/node_modules" ]; then
    say "- Frontend: $FRONTEND_DIR/ (dependencies installed)"
  else
    say "- Frontend: $FRONTEND_DIR/ (node_modules missing: run npm ci in $FRONTEND_DIR/)"
  fi
else
  say "- Frontend: not scaffolded yet ($FRONTEND_DIR/package.json missing)"
fi

# ---- specs -------------------------------------------------------------------------------
specs=""
for f in "$PROJECT_DIR"/specs/*.md; do
  [ -f "$f" ] || continue
  name="$(basename "$f" .md)"
  case "$name" in _* | README | *.plan) continue ;; esac
  plan="$PROJECT_DIR/specs/$name.plan.md"
  if [ -f "$plan" ]; then
    done_n="$(grep -c -E '^[[:space:]]*- \[[xX]\]' "$plan" 2>/dev/null)"
    todo_n="$(grep -c -E '^[[:space:]]*- \[ \]' "$plan" 2>/dev/null)"
    done_n="${done_n:-0}"
    todo_n="${todo_n:-0}"
    specs="$specs $name (plan $done_n/$((done_n + todo_n)) tasks done),"
  else
    specs="$specs $name (no plan yet),"
  fi
done
if [ -n "$specs" ]; then
  say "- Specs:${specs%,}"
else
  say "- Specs: none yet (create one with /spec <name> or copy specs/_template.md)"
fi

# ---- toolchain ---------------------------------------------------------------------------
gov="missing"
gobin=""
if command -v go >/dev/null 2>&1; then
  gov="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  gobin="$(go env GOPATH 2>/dev/null)/bin"
fi
extra_tool() { # extra_tool <name> → present | present (not on PATH) | missing
  if command -v "$1" >/dev/null 2>&1; then
    printf 'yes'
  elif [ -n "$gobin" ] && [ -x "$gobin/$1" ]; then
    printf 'yes (in %s, not on PATH)' "$gobin"
  else
    printf 'missing'
  fi
}
docker_state="missing"
command -v docker >/dev/null 2>&1 && docker_state="installed"
say "- Toolchain: go $gov · node $(version_of node --version) · npm $(version_of npm --version) · docker $docker_state"
say "- Go tools: golangci-lint $(extra_tool golangci-lint) · govulncheck $(extra_tool govulncheck) · goimports $(extra_tool goimports)"

# ---- git -----------------------------------------------------------------------------------
if git -C "$PROJECT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  branch="$(git -C "$PROJECT_DIR" symbolic-ref --short -q HEAD 2>/dev/null || echo detached)"
  changes="$(git -C "$PROJECT_DIR" status --porcelain 2>/dev/null | wc -l | tr -d ' ')"
  last="$(git -C "$PROJECT_DIR" log -1 --pretty=%s 2>/dev/null)"
  hooks_state="$(ensure_git_hooks)"
  say "- Git: branch $branch, $changes uncommitted path(s)${last:+, last commit \"$last\"}; format-on-commit hook: $hooks_state"
  if [ -z "$(git -C "$PROJECT_DIR" config user.email 2>/dev/null)" ]; then
    say "- WARNING: git user.email is not configured, so commits will fail. Ask the user to set it (never invent an identity)."
  fi
else
  say "- Git: not a repository yet (/implement initialises it)"
fi

# ---- hooks ---------------------------------------------------------------------------------
say "- Hooks: guardrails $GUARDRAILS (Go deps: $GO_DEPENDENCY_POLICY) · quality gate $QUALITY_GATE · format on commit $FORMAT_ON_COMMIT"
if [ -s "$STATE_DIR/dirty" ]; then
  say "- Pending: unverified changes in $(tr '\n' ' ' <"$STATE_DIR/dirty")(the quality gate checks them when you finish)"
fi
if [ "$JSON_PARSER" = "none" ]; then
  say "- WARNING: hooks need jq, node or python3 to read their input; none was found. Ask the user to install jq."
fi
say "- Workflow: /spec <name> → /implement <name> → /verify → /review → /readme (standards: .claude/CLAUDE.md, .claude/rules/)"
exit 0
