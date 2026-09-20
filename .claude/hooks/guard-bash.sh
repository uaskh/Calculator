#!/usr/bin/env bash
# PreToolUse(Bash) guardrail: blocks destructive or policy-violating shell commands.
# Exit 2 blocks the command and shows the reason to Claude.
#
# This is a best-effort safety net on top of Claude Code's permission rules, not a
# shell parser: commands are split on control operators and inspected one by one.

# shellcheck source=lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

is_on "$GUARDRAILS" || exit 0
read_hook_input
cmd="$(json_get tool_input.command)"
[ -n "$cmd" ] || exit 0

# The shell's working directory when the command starts; `cd` segments move it.
cwd="$(json_get cwd)"
[ -n "$cwd" ] || cwd="$PROJECT_DIR"
PROJECT_REAL="$(cd "$PROJECT_DIR" 2>/dev/null && pwd -P)" || PROJECT_REAL="$PROJECT_DIR"

# macOS file systems are case-insensitive: "rm -rf Frontend" deletes frontend/.
shopt -s nocasematch

violation=""

# plain_args <word>... — prints one word per line. A multi-word quoted string (such as a
# commit message) becomes the single placeholder __quoted__, so words inside it are never
# mistaken for flags; single quoted words are unquoted.
plain_args() {
  _q=""
  for _a in "$@"; do
    if [ -n "$_q" ]; then
      case "$_a" in *"$_q") _q="" ;; esac
      continue
    fi
    case "$_a" in
      \"*\" | \'*\')
        _a="${_a#?}"
        _a="${_a%?}"
        ;;
      \"*)
        _q='"'
        _a="__quoted__"
        ;;
      \'*)
        _q="'"
        _a="__quoted__"
        ;;
    esac
    printf '%s\n' "$_a"
  done
}

# lexical_path <absolute path> — collapses "//", "." and ".." without touching the disk.
lexical_path() {
  _lp=""
  _lp_ifs="$IFS"
  IFS='/'
  set -f
  # shellcheck disable=SC2086 # split on "/" on purpose; globbing is disabled
  set -- $1
  set +f
  IFS="$_lp_ifs"
  for _seg in "$@"; do
    case "$_seg" in
      '' | .) ;;
      ..) _lp="${_lp%/*}" ;;
      *) _lp="$_lp/$_seg" ;;
    esac
  done
  printf '%s' "${_lp:-/}"
}

# absolute_target <word> — the normalised absolute path a command-line path refers to;
# nothing when it depends on a variable or command substitution we cannot resolve.
absolute_target() {
  _p="$1"
  # shellcheck disable=SC2016,SC2088 # literal command text, not expansions
  case "$_p" in
    '~') _p="${HOME:-}" ;;
    '~/'*) _p="${HOME:-}/${_p#\~/}" ;;
    '$HOME' | '${HOME}') _p="${HOME:-}" ;;
    '$HOME/'*) _p="${HOME:-}/${_p#\$HOME/}" ;;
    '${HOME}/'*) _p="${HOME:-}/${_p#\$\{HOME\}/}" ;;
    '$PWD' | '${PWD}') _p="$cwd" ;;
    '$PWD/'*) _p="$cwd/${_p#\$PWD/}" ;;
    '${PWD}/'*) _p="$cwd/${_p#\$\{PWD\}/}" ;;
    '$CLAUDE_PROJECT_DIR' | '${CLAUDE_PROJECT_DIR}') _p="$PROJECT_DIR" ;;
    '$CLAUDE_PROJECT_DIR/'*) _p="$PROJECT_DIR/${_p#\$CLAUDE_PROJECT_DIR/}" ;;
    '${CLAUDE_PROJECT_DIR}/'*) _p="$PROJECT_DIR/${_p#\$\{CLAUDE_PROJECT_DIR\}/}" ;;
    *'$'* | *'`'*) return 0 ;;
    /*) ;;
    *) _p="$cwd/$_p" ;;
  esac
  [ -n "$_p" ] || return 0
  lexical_path "$_p"
}

# expand_braces <word> — prints the word with its {a,b} alternatives expanded, one per line.
expand_braces() {
  _eb_list="$1"
  _eb_round=0
  while [ "$_eb_round" -lt 3 ]; do
    case "$_eb_list" in *'{'*','*'}'*) ;; *) break ;; esac
    _eb_next=""
    while IFS= read -r _eb_w; do
      _eb_body=""
      case "$_eb_w" in
        *'{'*','*'}'*)
          _eb_pre="${_eb_w%%\{*}"
          _eb_rest="${_eb_w#*\{}"
          _eb_body="${_eb_rest%%\}*}"
          _eb_post="${_eb_rest#*\}}"
          ;;
      esac
      case "$_eb_body" in
        *,*)
          _eb_ifs="$IFS"
          IFS=','
          set -f
          for _eb_alt in $_eb_body; do
            _eb_next="$_eb_next$_eb_pre$_eb_alt$_eb_post
"
          done
          set +f
          IFS="$_eb_ifs"
          # "a{x,}" also expands to "a"; field splitting drops that trailing empty field.
          case "$_eb_body" in *,) _eb_next="$_eb_next$_eb_pre$_eb_post
" ;; esac
          ;;
        *)
          _eb_next="$_eb_next$_eb_w
"
          ;;
      esac
    done <<EOF
$_eb_list
EOF
    _eb_list="${_eb_next%?}"
    _eb_round=$((_eb_round + 1))
  done
  printf '%s\n' "$_eb_list"
}

# is_dangerous_target <word> — true when deleting the path would destroy the project, one
# of its top-level parts, the user's home directory or anything above them.
is_dangerous_target() {
  _t="$1"
  # Quotes left over from splitting a quoted command string ("rm -rf /").
  _t="${_t#[\"\']}"
  _t="${_t%[\"\']}"
  # "dir/*", "dir/.", "dir/./" and "dir/" empty or delete the directory as surely as "dir".
  while :; do
    case "$_t" in
      */\* | */.) _t="${_t%/*}" ;;
      ?*/) _t="${_t%/}" ;;
      *) break ;;
    esac
  done
  # shellcheck disable=SC2016 # literal command text, not expansions
  case "$_t" in
    '' | '/' | '~' | '$HOME' | '${HOME}' | '.' | '..' | '*' | '.*') return 0 ;;
    .git | */.git) return 0 ;; # repository history, wherever it is
  esac
  _abs="$(absolute_target "$_t")"
  [ -n "$_abs" ] || return 1
  [ "$_abs" != "/" ] || return 0
  # $_abs is used as a pattern on purpose: "rm -rf fr*" matches frontend.
  for _root in "$PROJECT_DIR" "$PROJECT_REAL"; do
    # shellcheck disable=SC2254
    case "$_root/" in
      $_abs/*) return 0 ;;
    esac
    for _sub in .git .claude specs docs "$BACKEND_DIR" "$FRONTEND_DIR"; do
      # shellcheck disable=SC2254
      case "$_root/$_sub" in
        $_abs) return 0 ;;
      esac
    done
  done
  if [ -n "${HOME:-}" ]; then
    # shellcheck disable=SC2254
    case "$HOME/" in
      $_abs/*) return 0 ;;
    esac
  fi
  return 1
}

check_rm() {
  _recursive=0
  _danger=""
  while IFS= read -r _w; do
    case "$_w" in
      --recursive) _recursive=1 ;;
      --*) ;;
      -*) case "$_w" in *[rR]*) _recursive=1 ;; esac ;;
      '') ;;
      *)
        while IFS= read -r _alt; do
          if is_dangerous_target "$_alt"; then _danger="$_alt"; fi
        done <<EOF_ALT
$(expand_braces "$_w")
EOF_ALT
        ;;
    esac
  done <<EOF
$(plain_args "$@")
EOF
  if [ "$_recursive" -eq 1 ] && [ -n "$_danger" ]; then
    violation="Recursive delete of '$_danger' is not allowed: it would destroy the project, its history or the user's files. Delete specific build artefacts instead (for example rm -rf $FRONTEND_DIR/dist), or ask the user."
  fi
}

check_git() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -C | -c | --git-dir | --work-tree | --namespace)
        shift
        [ "$#" -gt 0 ] && shift
        ;;
      -*) shift ;;
      *) break ;;
    esac
  done
  [ "$#" -gt 0 ] || return 0
  _sub="$1"
  shift
  _args="$(plain_args "$@")"

  case "$_sub" in
    push)
      while IFS= read -r _a; do
        case "$_a" in
          --no-verify)
            violation="git push --no-verify bypasses repository hooks and is not allowed."
            return
            ;;
          --force | --force-with-lease | --force-with-lease=* | --force-if-includes | --mirror | --delete | --prune)
            violation="Destructive pushes (git push $_a) are not allowed. History is never rewritten and pushing needs the user's explicit request."
            return
            ;;
          --*) ;;
          -*[fd]* | +*)
            violation="Destructive pushes (git push $_a) are not allowed. History is never rewritten and pushing needs the user's explicit request."
            return
            ;;
        esac
      done <<EOF
$_args
EOF
      ;;
    reset)
      if printf '%s\n' "$_args" | grep -Eqx -- '--hard'; then
        violation="git reset --hard discards uncommitted work and is not allowed. Use git stash, revert with a new commit, or ask the user."
      fi
      ;;
    clean)
      if ! printf '%s\n' "$_args" | grep -Eqx -- '-n|--dry-run'; then
        violation="git clean permanently deletes untracked files and is not allowed (only --dry-run is). Remove specific files instead, or ask the user."
      fi
      ;;
    checkout)
      if printf '%s\n' "$_args" | grep -Eqx -- '\.|\./|:/|\*|-f|--force'; then
        violation="This git checkout would discard local changes across the working tree and is not allowed. Restore individual files you changed, or ask the user."
      fi
      ;;
    restore)
      if printf '%s\n' "$_args" | grep -Eqx -- '\.|\./|:/|\*'; then
        if ! printf '%s\n' "$_args" | grep -Eqx -- '--staged|-S' || printf '%s\n' "$_args" | grep -Eqx -- '--worktree|-W'; then
          violation="git restore of the whole working tree discards local changes and is not allowed. Restore individual files you changed, or ask the user."
        fi
      fi
      ;;
    branch)
      if printf '%s\n' "$_args" | grep -Eq -- '^-[a-zA-Z]*D'; then
        violation="Force-deleting branches (git branch -D) is not allowed."
      fi
      ;;
    stash)
      case "$(printf '%s\n' "$_args" | head -n 1)" in
        drop | clear) violation="git stash drop/clear permanently deletes stashed work and is not allowed." ;;
      esac
      ;;
    commit)
      while IFS= read -r _a; do
        case "$_a" in
          --no-verify)
            violation="git commit --no-verify skips the pre-commit hook (format on commit) and is not allowed."
            return
            ;;
          --amend)
            violation="Amending commits rewrites history and is not allowed; create a new commit instead."
            return
            ;;
          --*) ;;
          -*n*)
            violation="git commit -n (--no-verify) skips the pre-commit hook (format on commit) and is not allowed."
            return
            ;;
        esac
      done <<EOF
$_args
EOF
      ;;
    rebase | filter-branch | filter-repo | replace | reflog | update-ref | prune)
      violation="git $_sub rewrites or discards history and is not allowed. Create new commits instead, or ask the user."
      ;;
    config)
      if printf '%s\n' "$_args" | grep -Eqx -- '--global|--system'; then
        if ! printf '%s\n' "$_args" | grep -Eqx -- '--get|--get-all|--get-regexp|--list|-l|--show-origin|--show-scope'; then
          violation="Changing global/system git configuration is not allowed from this project."
        fi
      else
        # Reading user.name/user.email is fine; setting or unsetting them is not.
        _key=0
        _write=0
        while IFS= read -r _a; do
          case "$_a" in
            '') ;;
            --unset | --unset-all | --replace-all | --add) _write=1 ;;
            -*) ;;
            *)
              if [ "$_key" -eq 1 ]; then
                _write=1
              elif printf '%s\n' "$_a" | grep -Eqix 'user\.(name|email|signingkey)'; then
                _key=1
              fi
              ;;
          esac
        done <<EOF
$_args
EOF
        if [ "$_key" -eq 1 ] && [ "$_write" -eq 1 ]; then
          violation="Commit identity must come from the user's own git configuration. Ask the user to set user.name/user.email; never invent one."
        fi
      fi
      ;;
  esac
}

check_go() {
  [ "$GO_DEPENDENCY_POLICY" = "off" ] && return 0
  [ "$#" -gt 0 ] || return 0
  _sub="$1"
  shift
  case "$_sub" in
    get)
      while IFS= read -r _a; do
        case "$_a" in
          '' | go@* | toolchain@*) ;;
          -tool)
            if [ "$GO_DEPENDENCY_POLICY" = "stdlib-only" ]; then
              violation="go get -tool records a tool dependency in go.mod, but the backend policy is standard library only. Run tools with 'go run pkg@version' or an installed binary instead."
              return
            fi
            ;;
          -*) ;;
          *)
            if [ "$GO_DEPENDENCY_POLICY" = "stdlib-only" ]; then
              violation="The backend uses the Go standard library only: 'go get $_a' would add a module dependency. Implement it with the standard library, or ask the user to approve an exception (they can change GO_DEPENDENCY_POLICY in .claude/hooks/hooks.conf)."
              return
            fi
            if is_go_framework "$_a"; then
              violation="Go web frameworks and third-party routers are not allowed ($_a). Use net/http and http.ServeMux."
              return
            fi
            ;;
        esac
      done <<EOF
$(plain_args "$@")
EOF
      ;;
    mod)
      if [ "${1:-}" = "edit" ] && [ "$GO_DEPENDENCY_POLICY" = "stdlib-only" ]; then
        shift
        if plain_args "$@" | grep -Eq -- '^-(require|replace|tool)(=|$)'; then
          violation="go mod edit may not add requirements, replacements or tools: the backend uses the Go standard library only."
        fi
      fi
      ;;
  esac
}

check_node_pm() {
  _pm="$1"
  shift
  case "$_pm" in
    pnpm | yarn | bun)
      violation="This repository uses npm (package-lock.json). Use npm/npx instead of $_pm."
      return
      ;;
  esac
  _args="$(plain_args "$@")"
  if printf '%s\n' "$_args" | grep -Eqx -- '-g|--global|--location=global'; then
    violation="Global package installs change the user's machine. Add a devDependency to the project or use npx instead."
    return
  fi
  if [ "$_pm" = "npm" ]; then
    case "$(printf '%s\n' "$_args" | head -n 1)" in
      publish | unpublish | deprecate | owner | access | adduser | login | logout | token)
        violation="npm registry/account commands are not allowed from this project."
        ;;
    esac
  fi
}

check_chmod() {
  if plain_args "$@" | grep -Eqx -- '0?777|a\+rwx|ugo\+rwx|\+rwx|a\+w|o\+w|o\+rwx'; then
    violation="World-writable permissions are not allowed."
  fi
}

check_docker() {
  _joined=" $(plain_args "$@" | tr '\n' ' ')"
  case "$_joined" in
    *" system prune"* | *" volume prune"* | *" volume rm "* | *" image prune"* | *" builder prune"* | *" container prune"* | *" network prune"*)
      violation="Docker prune/remove commands affect other projects on this machine and are not allowed. Use 'docker compose down' for this project."
      ;;
  esac
}

check_segment() {
  set -f
  # shellcheck disable=SC2086 # intentional word splitting; globbing is disabled
  set -- $1
  set +f
  while [ "$#" -gt 0 ]; do
    case "$1" in
      [A-Za-z_]*=*) shift ;;
      command | builtin | exec | nohup | time | nice | noglob | env | stdbuf | xargs) shift ;;
      # Shell grammar around the actual command ("if x; then rm …; fi", "{ rm …; }").
      '{' | '}' | '!' | if | then | else | elif | do | while | until) shift ;;
      *) break ;;
    esac
  done
  [ "$#" -gt 0 ] || return 0
  _prog="${1##*/}"
  shift
  case "$_prog" in
    cd)
      _to="${1:-}"
      _to="${_to#[\"\']}"
      _to="${_to%[\"\']}"
      case "$_to" in
        '') [ -z "${HOME:-}" ] || cwd="$HOME" ;;
        -*) ;;
        *)
          _new="$(absolute_target "$_to")"
          [ -z "$_new" ] || cwd="$_new"
          ;;
      esac
      ;;
    sudo | doas | su)
      violation="Privilege escalation ($_prog) is not allowed. Ask the user to run the command themselves if it is really needed."
      ;;
    bash | sh | zsh | dash | ksh)
      # Inspect the command string of `bash -c "..."` as well.
      case "${1:-}" in
        --*) ;;
        -*c*)
          shift
          _inner="$*"
          _inner="${_inner#[\"\']}"
          _inner="${_inner%[\"\']}"
          check_segment "$_inner"
          ;;
      esac
      ;;
    rm) check_rm "$@" ;;
    git) check_git "$@" ;;
    go) check_go "$@" ;;
    npm | npx | pnpm | yarn | bun) check_node_pm "$_prog" "$@" ;;
    chmod) check_chmod "$@" ;;
    docker) check_docker "$@" ;;
    killall | pkill)
      violation="Killing processes by name can terminate the user's other programs. Stop only the background tasks you started."
      ;;
    shutdown | reboot | halt | mkfs | mkfs.* | diskutil | dd | launchctl | security | csrutil | systemsetup | defaults | crontab)
      violation="'$_prog' changes the operating system or user environment and is not allowed from this project."
      ;;
  esac
}

# strip_data — removes text that is data rather than commands, so words inside it are not
# inspected as commands: bodies of heredocs read by `cat`, and commit messages passed with
# -m/--message (unless they contain a command substitution other than `$(cat <<EOF`).
strip_data() {
  awk -v q="'" '
    function neutral(s,    out, m, p, chk) {
      out = ""
      while (match(s, re_msg)) {
        m = substr(s, RSTART, RLENGTH)
        p = m
        sub(re_flag, "", p)
        chk = p
        gsub(re_cat, "", chk)
        if (index(chk, "`") == 0 && index(chk, "$(") == 0) {
          out = out substr(s, 1, RSTART - 1) "-m __msg__"
        } else {
          out = out substr(s, 1, RSTART + RLENGTH - 1)
        }
        s = substr(s, RSTART + RLENGTH)
      }
      return out s
    }
    BEGIN {
      flags = "(-[aeiopqsuvzS]*m|--message)[= \t]*"
      re_flag = "^" flags
      re_msg = flags "(\"[^\"]*\"|" q "[^" q "]*" q ")"
      re_here = "cat[ \t]+<<-?[ \t]*[" q "\"]?[A-Za-z_][A-Za-z0-9_]*[" q "\"]?"
      re_cat = "\\$\\(" re_here
      delim = ""
      text = ""
    }
    {
      line = $0
      if (delim != "") {
        t = line
        sub(/^\t+/, "", t)
        if (t == delim) {
          delim = ""
          text = text line "\n"
        } else {
          text = text "\n"
        }
        next
      }
      if (match(line, re_here)) {
        d = substr(line, RSTART, RLENGTH)
        sub(/^cat[ \t]+<<-?[ \t]*/, "", d)
        gsub(/[^A-Za-z0-9_]/, "", d)
        delim = d
      }
      text = text line "\n"
    }
    END { printf "%s", neutral(text) }
  '
}

# ---- whole-command checks -----------------------------------------------------------
if printf '%s\n' "$cmd" | grep -Eq '(curl|wget)[^|]*\|[[:space:]]*(sudo[[:space:]]+)?(sh|bash|zsh|dash|ksh)([[:space:]]|$)'; then
  block "Piping a downloaded script into a shell is not allowed." \
    "Download it, review it, and ask the user before running it."
fi

# ---- per-command checks ------------------------------------------------------------
inspected="$cmd"
if command -v awk >/dev/null 2>&1; then
  inspected="$(printf '%s\n' "$cmd" | strip_data)" || inspected="$cmd"
fi
# shellcheck disable=SC2020 # every operator character becomes a line break
segments="$(printf '%s\n' "$inspected" | tr ';|&()`' '\n\n\n\n\n\n')"
while IFS= read -r seg; do
  [ -n "$seg" ] || continue
  check_segment "$seg"
  [ -z "$violation" ] || break
done <<EOF
$segments
EOF

[ -z "$violation" ] || block "$violation"
exit 0
