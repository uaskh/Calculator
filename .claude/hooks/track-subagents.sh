#!/usr/bin/env bash
# SubagentStart / SubagentStop: keeps a marker per running subagent so the Stop quality
# gate does not test half-written code while subagents are still working. Never blocks.
#
#   track-subagents.sh start|stop

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

read_hook_input
id="$(json_get agent_id | tr -cd 'A-Za-z0-9_-')"
[ -n "$id" ] || exit 0

ensure_state_dir
case "${1:-}" in
  start) : >"$STATE_DIR/agents/$id" ;;
  stop) rm -f "$STATE_DIR/agents/$id" ;;
esac
exit 0
