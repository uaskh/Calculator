#!/usr/bin/env bash
# PreToolUse(Bash): before Claude runs `git commit`, make sure formatting will happen.
# Normally that means enabling the repository hook (core.hooksPath=.claude/githooks),
# which formats staged files inside the commit itself. If the repository already uses
# its own git hooks, the working-tree changes are formatted right here instead.

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

is_on "$FORMAT_ON_COMMIT" || exit 0
read_hook_input
case "$HOOK_INPUT" in
  *commit*) ;;
  *) exit 0 ;;
esac
cmd="$(json_get tool_input.command)"
printf '%s\n' "$cmd" | grep -Eq '(^|[;&|(`[:space:]])git[[:space:]]([^;&|]*[[:space:]])?commit([[:space:]]|$)' || exit 0

state="$(ensure_git_hooks)"
case "$state" in
  custom-hook | custom-path)
    if ! bash "$HOOKS_DIR/format-staged.sh" --worktree; then
      block "Formatting failed (see the errors above). Fix them before committing."
    fi
    ;;
esac
exit 0
