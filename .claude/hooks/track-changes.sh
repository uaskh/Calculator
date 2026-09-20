#!/usr/bin/env bash
# PostToolUse(Edit|Write|NotebookEdit): remembers which component (backend/frontend)
# changed, so the Stop quality gate only runs the checks that matter. Never blocks.

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

is_on "$QUALITY_GATE" || exit 0
read_hook_input
file="$(json_get tool_input.file_path)"
[ -n "$file" ] || file="$(json_get tool_input.notebook_path)"
[ -n "$file" ] || exit 0

rel="$(rel_path "$file")"
case "$rel" in
  "$BACKEND_DIR"/*) component="backend" ;;
  "$FRONTEND_DIR"/*) component="frontend" ;;
  *) exit 0 ;;
esac

ensure_state_dir
if ! grep -qx "$component" "$STATE_DIR/dirty" 2>/dev/null; then
  printf '%s\n' "$component" >>"$STATE_DIR/dirty"
fi
exit 0
