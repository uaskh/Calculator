#!/usr/bin/env bash
# Copies the validated project skeleton from ../templates into the repository.
#
#   scaffold.sh repo
#       Root files: Makefile, CI workflow, compose file, .gitignore, .editorconfig, .nvmrc,
#       scripts/coverage-report.sh, docs/adr. Existing files are never overwritten.
#   scaffold.sh backend <module-path>
#       backend/: Go service skeleton (standard library only), personalised with the module
#       path and the installed Go version, then vetted and tested.
#   scaffold.sh frontend "<Product name>" "<one-line description>"
#       Overlays frontend/ (created first with the official Vite react-ts template):
#       API client, app shell, design tokens, test setup, Vitest/Playwright/Prettier config,
#       nginx + Dockerfile, npm scripts. Removes the Vite starter files.
#
# The script never installs packages: dependency installs stay visible to the user.
# Works with the bash and BSD tools shipped on macOS as well as on Linux.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
templates="$(cd "$here/../templates" && pwd)"
root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

die() {
  printf 'scaffold: %s\n' "$*" >&2
  exit 1
}
say() { printf 'scaffold: %s\n' "$*"; }

# copy_tree <src> <dest> <overwrite:yes|no> — copies files, creating directories.
copy_tree() {
  _src="$1"
  _dest="$2"
  _overwrite="$3"
  (cd "$_src" && find . -type f) | while IFS= read -r rel; do
    rel="${rel#./}"
    target="$_dest/$rel"
    case "$rel" in
      gitignore) target="$_dest/.gitignore" ;;
    esac
    if [ -e "$target" ] && [ "$_overwrite" = "no" ]; then
      say "kept existing ${target#"$root"/}"
      continue
    fi
    mkdir -p "$(dirname "$target")"
    cp "$_src/$rel" "$target"
  done
}

# replace_in <file> <literal> <replacement> — portable in-place literal replacement.
replace_in() {
  _file="$1"
  _from="$(printf '%s' "$2" | sed -e 's/[]\/$*.^[]/\\&/g')"
  _to="$(printf '%s' "$3" | sed -e 's/[\/&]/\\&/g')"
  sed -e "s/$_from/$_to/g" "$_file" >"$_file.scaffold.tmp"
  cat "$_file.scaffold.tmp" >"$_file"
  rm -f "$_file.scaffold.tmp"
}

# set_arg <Dockerfile> <ARG name> <value> — sets the default value of a build argument.
set_arg() {
  _file="$1"
  _value="$(printf '%s' "$3" | sed -e 's/[\/&]/\\&/g')"
  sed -E -e "s/^ARG $2=.*/ARG $2=$_value/" "$_file" >"$_file.scaffold.tmp"
  cat "$_file.scaffold.tmp" >"$_file"
  rm -f "$_file.scaffold.tmp"
}

go_minor_version() {
  go env GOVERSION | sed -E 's/^go([0-9]+\.[0-9]+).*/\1/'
}

scaffold_repo() {
  copy_tree "$templates/repo" "$root" no
  chmod +x "$root/scripts/coverage-report.sh" "$root/scripts/dev.sh"
  if [ -f "$root/docs/adr/0001-architecture-and-dependency-policy.md" ]; then
    replace_in "$root/docs/adr/0001-architecture-and-dependency-policy.md" "__DATE__" "$(date +%Y-%m-%d)"
  fi
  if [ ! -f "$root/.nvmrc" ]; then
    command -v node >/dev/null 2>&1 || die "node is required to write .nvmrc"
    node --version | sed -E 's/^v([0-9]+\.[0-9]+).*/\1/' >"$root/.nvmrc"
    say "wrote .nvmrc ($(cat "$root/.nvmrc"))"
  fi
  if [ -f "$root/backend/Dockerfile" ] || [ -f "$root/frontend/Dockerfile" ]; then
    sync_versions
  fi
  say "repo files ready"
}

# sync_versions — align Dockerfile base images with go.mod and .nvmrc.
sync_versions() {
  if [ -f "$root/backend/Dockerfile" ] && command -v go >/dev/null 2>&1; then
    set_arg "$root/backend/Dockerfile" GO_VERSION "$(go_minor_version)"
  fi
  if [ -f "$root/frontend/Dockerfile" ] && [ -f "$root/.nvmrc" ]; then
    set_arg "$root/frontend/Dockerfile" NODE_VERSION "$(tr -d '[:space:]' <"$root/.nvmrc")"
  fi
}

scaffold_backend() {
  module="${1:-}"
  [ -n "$module" ] || die "usage: scaffold.sh backend <module-path>"
  printf '%s' "$module" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._~/-]*$' || die "invalid module path: $module"
  command -v go >/dev/null 2>&1 || die "go is not installed"
  dest="$root/backend"
  [ ! -e "$dest/go.mod" ] || die "backend/go.mod already exists; refusing to overwrite"

  copy_tree "$templates/backend" "$dest" no
  find "$dest" -type f \( -name '*.go' -o -name 'go.mod' -o -name '.golangci.yml' \) | while IFS= read -r f; do
    replace_in "$f" "example.com/service" "$module"
  done
  (cd "$dest" && go mod edit -go="$(go_minor_version)")
  sync_versions

  say "verifying backend skeleton"
  (cd "$dest" && test -z "$(gofmt -l .)") || die "gofmt reports unformatted files"
  (cd "$dest" && go vet ./... && go test -count=1 ./...) || die "backend skeleton does not pass go vet/test"
  if command -v golangci-lint >/dev/null 2>&1; then
    (cd "$dest" && golangci-lint run ./...) || die "golangci-lint reports issues"
  else
    say "golangci-lint not installed; lint step skipped"
  fi
  say "backend ready (module $module)"
}

scaffold_frontend() {
  name="${1:-}"
  description="${2:-}"
  [ -n "$name" ] && [ -n "$description" ] || die 'usage: scaffold.sh frontend "<Product name>" "<description>"'
  case "$name$description" in
    *[\<\>\"\'\\\`\$]*) die "name and description must not contain < > \" ' \\ \` or \$" ;;
  esac
  dest="$root/frontend"
  [ -f "$dest/package.json" ] || die "create frontend/ with the Vite react-ts template first"
  [ ! -e "$dest/src/api/client.ts" ] || die "frontend/src/api/client.ts already exists; refusing to overwrite"
  command -v npm >/dev/null 2>&1 || die "npm is not installed"

  keep_vite_config=no
  if ! grep -q '"@vitejs/plugin-react"' "$dest/package.json"; then
    keep_vite_config=yes
    say "template does not use @vitejs/plugin-react: vite.config.ts kept, add the proxy settings by hand"
    cp "$dest/vite.config.ts" "$dest/vite.config.ts.keep" 2>/dev/null || true
  fi

  copy_tree "$templates/frontend" "$dest" yes
  if [ "$keep_vite_config" = "yes" ] && [ -f "$dest/vite.config.ts.keep" ]; then
    mv "$dest/vite.config.ts.keep" "$dest/vite.config.ts"
  fi

  for starter in src/App.tsx src/App.css src/index.css src/assets/react.svg public/vite.svg src/vite-env.d.ts; do
    if [ -f "$dest/$starter" ]; then
      rm -f "$dest/$starter"
      say "removed Vite starter file $starter"
    fi
  done
  rmdir "$dest/src/assets" "$dest/public" 2>/dev/null || true

  html_name="$(printf '%s' "$name" | sed -e 's/&/\&amp;/g')"
  html_description="$(printf '%s' "$description" | sed -e 's/&/\&amp;/g')"
  replace_in "$dest/index.html" "__APP_NAME__" "$html_name"
  replace_in "$dest/index.html" "__APP_DESCRIPTION__" "$html_description"
  for f in .env.example src/config.ts; do
    replace_in "$dest/$f" "__APP_NAME__" "$name"
  done

  (
    cd "$dest"
    npm pkg set \
      scripts.typecheck="tsc -b" \
      scripts.lint="eslint . --max-warnings=0" \
      scripts.format="prettier --write ." \
      scripts.format:check="prettier --check ." \
      scripts.test="vitest run" \
      scripts.test:watch="vitest" \
      scripts.test:coverage="vitest run --coverage" \
      scripts.test:e2e="playwright test"
    if [ -f "$root/.nvmrc" ]; then
      npm pkg set engines.node=">=$(tr -d '[:space:]' <"$root/.nvmrc")"
    fi
  )
  sync_versions
  say "frontend overlay applied; next steps are in references/scaffold.md (dev dependencies, tsconfig and ESLint merge)"
}

case "${1:-}" in
  repo) scaffold_repo ;;
  backend)
    shift
    scaffold_backend "$@"
    ;;
  frontend)
    shift
    scaffold_frontend "$@"
    ;;
  *) die "usage: scaffold.sh repo | backend <module-path> | frontend \"<name>\" \"<description>\"" ;;
esac
