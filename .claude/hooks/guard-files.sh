#!/usr/bin/env bash
# PreToolUse(Edit|Write|NotebookEdit) guardrail: protects secrets, lockfiles, generated
# output, git internals and the guardrail configuration itself, and enforces the Go
# dependency policy on go.mod and Go imports. Exit 2 blocks the edit.

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

is_on "$GUARDRAILS" || exit 0
read_hook_input
file="$(json_get tool_input.file_path)"
[ -n "$file" ] || file="$(json_get tool_input.notebook_path)"
[ -n "$file" ] || exit 0

rel="$(rel_path "$file")"
base="${rel##*/}"

# Text being written: full content (Write) or replacement text (Edit / legacy MultiEdit).
new_text() {
  _nt="$(json_get tool_input.content)"
  [ -n "$_nt" ] || _nt="$(json_get tool_input.new_string)"
  [ -n "$_nt" ] || _nt="$(json_get tool_input.edits)"
  printf '%s\n' "$_nt"
}

# ---- 1. secrets & credentials ---------------------------------------------------------
case "$base" in
  .env.example | .env.sample | .env.template) ;;
  .env | .env.* | *.env | *.pem | *.key | *.p12 | *.pfx | *.jks | *.keystore | \
    id_rsa* | id_ed25519* | id_ecdsa* | *.kubeconfig | .netrc | credentials.json)
    block "$rel is a secrets/credentials file and must not be created or edited by Claude." \
      "Put documented placeholder values in .env.example and let the user provide real values."
    ;;
esac

# ---- 2. lockfiles, generated output, vendored code, git internals -----------------------
case "$base" in
  package-lock.json | npm-shrinkwrap.json | yarn.lock | pnpm-lock.yaml | bun.lock | bun.lockb | go.sum | go.work.sum)
    block "$rel is maintained by the package manager." \
      "Change dependencies with npm/go commands instead of editing the lockfile."
    ;;
esac
# Output directories are matched where the build tools create them, so a feature may still
# be called "coverage", "vendor" or "dist" (e.g. src/features/coverage/).
case "/$rel/" in
  */.git/*)
    block "Files inside .git/ must never be edited directly."
    ;;
  */node_modules/* | "/$BACKEND_DIR/vendor/"*)
    block "$rel is third-party code. Never patch installed dependencies."
    ;;
  "/$FRONTEND_DIR/dist/"* | "/$FRONTEND_DIR/coverage/"* | "/$FRONTEND_DIR/playwright-report/"* | \
    "/$FRONTEND_DIR/test-results/"* | /coverage/* | /bin/* | /.claude/.state/*)
    block "$rel is generated output. Change the source or configuration that produces it instead."
    ;;
esac

# ---- 3. guardrail configuration -------------------------------------------------------
if is_on "$PROTECT_CLAUDE_CONFIG"; then
  case "$rel" in
    .claude/settings.json | .claude/settings.local.json | .claude/hooks/* | .claude/githooks/*)
      block "$rel is part of this project's guardrail configuration and can't be changed by Claude." \
        "If the user asked for this change, show them the exact edit to make instead."
      ;;
  esac
fi

# ---- 4. Go dependency policy ----------------------------------------------------------
[ "$GO_DEPENDENCY_POLICY" = "off" ] && exit 0

case "$base" in
  go.mod)
    txt="$(new_text)"
    if [ "$GO_DEPENDENCY_POLICY" = "stdlib-only" ]; then
      if printf '%s\n' "$txt" | grep -Eq "$GO_MOD_DEPS_RE"; then
        block "go.mod must not declare require/replace/tool directives: the backend uses the Go standard library only." \
          "Implement the capability with the standard library, or ask the user to approve an exception."
      fi
    else
      for mod in $(printf '%s\n' "$txt" | grep -Eo '(github\.com|gopkg\.in|goji\.io)/[A-Za-z0-9._/-]+'); do
        if is_go_framework "$mod"; then
          block "Go web frameworks and third-party routers are not allowed ($mod). Use net/http and http.ServeMux."
        fi
      done
    fi
    ;;
  *.go)
    txt="$(new_text)"
    # Nearest go.mod above the file decides which imports are "local".
    abs="$file"
    case "$abs" in /*) ;; *) abs="$PROJECT_DIR/$abs" ;; esac
    dir="$(dirname "$abs")"
    gomod=""
    while :; do
      if [ -f "$dir/go.mod" ]; then
        gomod="$dir/go.mod"
        break
      fi
      case "$dir" in "$PROJECT_DIR" | / | .) break ;; esac
      dir="$(dirname "$dir")"
    done
    [ -n "$gomod" ] || gomod="$PROJECT_DIR/$BACKEND_DIR/go.mod"
    module="$(go_module_path "$gomod")"

    # Import specs: optional "import", optional alias, a quoted path alone on its line.
    paths="$(printf '%s\n' "$txt" | sed -n -E 's/^[[:space:]]*(import[[:space:]]+)?(([A-Za-z_][A-Za-z0-9_]*|\.)[[:space:]]+)?"([^"[:space:]]+)"[[:space:]]*(\/\/.*)?$/\4/p')"
    bad=""
    for p in $paths; do
      first="${p%%/*}"
      case "$first" in
        *.*) ;;
        *) continue ;; # standard library (or a module path without dots)
      esac
      if [ -n "$module" ]; then
        case "$p" in "$module" | "$module"/*) continue ;; esac
      fi
      if [ "$GO_DEPENDENCY_POLICY" = "stdlib-only" ] || is_go_framework "$p"; then
        bad="$bad $p"
      fi
    done
    if [ -n "$bad" ]; then
      if [ "$GO_DEPENDENCY_POLICY" = "stdlib-only" ]; then
        block "Non-standard-library import(s) in $rel:$bad" \
          "The backend uses the Go standard library only. Use net/http, encoding/json, log/slog, testing, etc." \
          "(If this is the module's own package, create $BACKEND_DIR/go.mod with the right module path first.)"
      else
        block "Go web frameworks and third-party routers are not allowed:$bad. Use net/http and http.ServeMux."
      fi
    fi
    ;;
esac

exit 0
