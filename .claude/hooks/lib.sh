#!/usr/bin/env bash
# Shared helpers for this project's Claude Code hooks and git hooks.
#
# Portability: macOS (bash 3.2, BSD userland) and Linux (bash 4/5, GNU userland).
# Keep to portable constructs: no associative arrays, no ${var,,}, no mapfile,
# no GNU-only flags (sed -i, readlink -f, timeout, date -d).
#
# Claude Code hook contract: event JSON arrives on stdin; exit 0 = no objection;
# exit 2 = block, and stderr is fed back to Claude. https://code.claude.com/docs/en/hooks

HOOKS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(cd "$HOOKS_DIR/../.." && pwd)}"
STATE_DIR="$PROJECT_DIR/.claude/.state"

# ---- defaults (override in hooks.conf) -----------------------------------------
BACKEND_DIR="backend"
FRONTEND_DIR="frontend"
GUARDRAILS="on"
GO_DEPENDENCY_POLICY="stdlib-only"
PROTECT_CLAUDE_CONFIG="on"
QUALITY_GATE="on"
QUALITY_GATE_MAX_RETRIES=3
FORMAT_ON_COMMIT="on"
SUBAGENT_STALE_MINUTES=120

if [ -f "$HOOKS_DIR/hooks.conf" ]; then
  # shellcheck source=hooks.conf
  . "$HOOKS_DIR/hooks.conf"
fi

is_on() {
  case "$1" in
    on | ON | On | true | TRUE | True | yes | YES | 1) return 0 ;;
    *) return 1 ;;
  esac
}

warn() {
  printf '%s\n' "$*" >&2
}

# block <line>... — print the reason for Claude and block the action (exit 2).
block() {
  {
    printf 'Blocked by project guardrail (.claude/hooks/%s):\n' "$(basename "$0")"
    printf '%s\n' "$@"
  } >&2
  exit 2
}

# ---- hook input -----------------------------------------------------------------
HOOK_INPUT=""
JSON_PARSER=""

read_hook_input() {
  HOOK_INPUT="$(cat 2>/dev/null)" || HOOK_INPUT=""
}

detect_json_parser() {
  [ -n "$JSON_PARSER" ] && return 0
  if command -v jq >/dev/null 2>&1; then
    JSON_PARSER="jq"
  elif command -v node >/dev/null 2>&1; then
    JSON_PARSER="node"
  elif command -v python3 >/dev/null 2>&1; then
    JSON_PARSER="python3"
  else
    JSON_PARSER="none"
  fi
}

# json_get <dot.separated.path>
# Prints the value at the path in $HOOK_INPUT: strings raw, other values as JSON,
# nothing when the value is missing, null or false.
json_get() {
  detect_json_parser
  case "$JSON_PARSER" in
    jq)
      printf '%s' "$HOOK_INPUT" | jq -r --arg p "$1" \
        'try (getpath($p | split(".")) | if . == null or . == false then empty elif type == "string" then . else tojson end) catch empty' \
        2>/dev/null
      ;;
    node)
      printf '%s' "$HOOK_INPUT" | node -e '
let s = "";
process.stdin.on("data", (d) => (s += d)).on("end", () => {
  try {
    let v = JSON.parse(s);
    for (const k of process.argv[1].split(".")) v = v !== null && typeof v === "object" ? v[k] : undefined;
    if (v === undefined || v === null || v === false) return;
    process.stdout.write(typeof v === "string" ? v : JSON.stringify(v));
  } catch {}
});' "$1" 2>/dev/null
      ;;
    python3)
      printf '%s' "$HOOK_INPUT" | python3 -c '
import json, sys
try:
    v = json.load(sys.stdin)
    for k in sys.argv[1].split("."):
        v = v.get(k) if isinstance(v, dict) else None
    if v is not None and v is not False:
        sys.stdout.write(v if isinstance(v, str) else json.dumps(v))
except Exception:
    pass' "$1" 2>/dev/null
      ;;
    *) return 1 ;;
  esac
  return 0
}

# ---- paths & state ----------------------------------------------------------------
ensure_state_dir() {
  mkdir -p "$STATE_DIR/agents" 2>/dev/null || true
}

# rel_path <path> — the path relative to the project root (unchanged when outside it).
rel_path() {
  _rp="$1"
  case "$_rp" in
    "$PROJECT_DIR"/*) _rp="${_rp#"$PROJECT_DIR"/}" ;;
    ./*) _rp="${_rp#./}" ;;
  esac
  printf '%s' "$_rp"
}

# go_module_path <go.mod> — the module path declared in a go.mod file.
go_module_path() {
  [ -f "$1" ] || return 0
  sed -n -E 's/^module[[:space:]]+"?([^"[:space:]]+)"?.*/\1/p' "$1" | head -n 1
}

# Extended regex matching a go.mod directive that pulls in other modules. The "\n"
# alternative also catches directives inside JSON-escaped text (legacy MultiEdit input).
# shellcheck disable=SC2034 # used by the scripts that source this file
GO_MOD_DEPS_RE='(^|\\n)[[:space:]]*(require|replace|tool)([[:space:]]|\(|$)'

# is_go_framework <import-or-module-path> — web frameworks/routers/toolkits we never use.
is_go_framework() {
  case "$1" in
    github.com/gin-gonic/* | github.com/labstack/echo* | github.com/gofiber/* | \
      github.com/go-chi/* | github.com/gorilla/mux* | github.com/julienschmidt/httprouter* | \
      github.com/beego/* | github.com/astaxie/beego* | github.com/revel/* | \
      github.com/kataras/iris* | github.com/gobuffalo/* | github.com/valyala/fasthttp* | \
      github.com/go-martini/* | github.com/zenazn/goji* | goji.io* | \
      github.com/emicklei/go-restful* | github.com/uptrace/bunrouter* | \
      github.com/zeromicro/go-zero* | github.com/cloudwego/hertz* | github.com/go-kratos/* | \
      github.com/gogf/gf* | github.com/danielgtaylor/huma* | github.com/go-fuego/* | \
      github.com/go-kit/kit* | github.com/bmizerany/pat* | github.com/dimfeld/httptreemux*)
      return 0
      ;;
  esac
  return 1
}

# running_subagents — number of subagents that started and have not stopped yet.
running_subagents() {
  if [ ! -d "$STATE_DIR/agents" ]; then
    echo 0
    return 0
  fi
  find "$STATE_DIR/agents" -type f -mmin +"$SUBAGENT_STALE_MINUTES" -exec rm -f {} + 2>/dev/null
  find "$STATE_DIR/agents" -type f 2>/dev/null | wc -l | tr -d ' '
}

# ---- git hooks (format on commit) --------------------------------------------------
# has_own_git_hooks <dir> — true when the repository already uses hooks of its own (any
# executable hook other than git's *.sample files), which core.hooksPath would disable.
has_own_git_hooks() {
  for _h in "$1"/*; do
    case "$_h" in *.sample) continue ;; esac
    if [ -f "$_h" ] && [ -x "$_h" ]; then
      return 0
    fi
  done
  return 1
}

# ensure_git_hooks — point core.hooksPath at .claude/githooks unless the repository
# already has hooks of its own. Prints: active | enabled | disabled | no-repo |
# custom-hook | custom-path | error.
ensure_git_hooks() {
  if ! is_on "$FORMAT_ON_COMMIT"; then
    echo "disabled"
    return 0
  fi
  if ! git -C "$PROJECT_DIR" rev-parse --git-dir >/dev/null 2>&1; then
    echo "no-repo"
    return 0
  fi
  chmod +x "$PROJECT_DIR/.claude/githooks/pre-commit" "$HOOKS_DIR"/*.sh 2>/dev/null
  _hp="$(git -C "$PROJECT_DIR" config --get core.hooksPath 2>/dev/null)"
  case "$_hp" in
    .claude/githooks | .claude/githooks/ | "$PROJECT_DIR/.claude/githooks" | "$PROJECT_DIR/.claude/githooks/")
      echo "active"
      ;;
    "")
      _hooks="$(git -C "$PROJECT_DIR" rev-parse --git-path hooks 2>/dev/null)"
      case "$_hooks" in
        /*) ;;
        *) _hooks="$PROJECT_DIR/$_hooks" ;;
      esac
      if has_own_git_hooks "$_hooks"; then
        echo "custom-hook"
      elif git -C "$PROJECT_DIR" config core.hooksPath .claude/githooks 2>/dev/null; then
        echo "enabled"
      else
        echo "error"
      fi
      ;;
    *)
      echo "custom-path"
      ;;
  esac
}
