---
paths:
  - "Makefile"
  - "**/Dockerfile"
  - "**/.dockerignore"
  - "docker-compose.yml"
  - "compose.yaml"
  - "**/nginx.conf"
  - ".github/**"
  - "scripts/**"
  - ".nvmrc"
  - ".editorconfig"
  - ".gitignore"
---

# Build, container and CI rules

Reference files (verified): `.claude/skills/implement/templates/repo/` and the container files in
`.claude/skills/implement/templates/{backend,frontend}/`.

## Makefile

- The single entry point for humans, CI and Claude: `setup dev run-backend run-frontend
  fmt fmt-check lint typecheck test test-e2e coverage build vuln verify docker-build
  docker-up docker-down clean`, each with a `## description` for `make help`.
- Compatible with GNU Make 3.81 (macOS): no `.ONESHELL`, `.SHELLFLAGS`, `$(file …)` or `!=`.
  Recipes are short and delegate to `go`/`npm` scripts; every target is `.PHONY`.
- `verify` runs every CI check except the container smoke test (`make docker-up`).
  README commands use make targets. `dev` delegates to `scripts/dev.sh`, which runs both
  servers and stops them together.

## Containers

- Multi-stage builds; final images are minimal and run as a non-root numeric user:
  backend on `gcr.io/distroless/static-debian12` (static `CGO_ENABLED=0` binary,
  `-trimpath -ldflags="-s -w"`), frontend on `nginxinc/nginx-unprivileged` serving
  `dist/` and proxying `/api/` to the backend service.
- Base image tags pin at least the minor version and match `go.mod` / `.nvmrc`
  (`ARG GO_VERSION`, `ARG NODE_VERSION`); never `latest`.
- `HEALTHCHECK` on both images (the Go binary's `-healthcheck` flag, `wget` in nginx);
  compose waits on health (`depends_on: condition: service_healthy`).
- `.dockerignore` keeps contexts small (no `node_modules`, tests, coverage, `.env*`).
- nginx: SPA fallback to `index.html`, long-lived caching only for fingerprinted
  `/assets/`, `no-cache` for the shell, security headers at server level (don't add
  `add_header` inside locations, it drops the inherited ones), gzip, `server_tokens off`.
- `docker compose up --build --wait` must bring up a working stack; document ports.
- If a single image serving both tiers is requested, build the frontend in a Node stage
  and let the Go service serve it from an optional `STATIC_DIR` with SPA fallback,
  keeping `/api` routing and security headers unchanged; confirm the topology with the user.
- Validate with `docker compose config --quiet` and, when available, `hadolint`.

## CI (GitHub Actions)

- `.github/workflows/ci.yml` mirrors `make verify`: backend (gofmt check, golangci-lint,
  `go test -race` with the coverage thresholds of `scripts/coverage-report.sh`, govulncheck,
  build), frontend (format check, lint,
  typecheck, coverage, build, npm audit), e2e (Playwright with the real backend), docker
  (compose build, up `--wait`, smoke request).
- Least privilege (`permissions: contents: read`), concurrency cancellation, dependency
  caching, `node-version-file`/`go-version-file` instead of hard-coded versions.
- Use each action's current major version; check the action's releases page before
  writing the workflow. Validate with `actionlint` when available.

## Repository hygiene

- `.gitignore` covers dependencies, build output, coverage/test reports, local review
  reports (`.reviews/`), `.env*` (except `.env.example`) and OS/editor files. Nothing generated is committed except
  `docs/coverage.md`.
- `.editorconfig` defines indentation (tabs for Go and Makefiles, 2 spaces elsewhere),
  LF line endings and final newlines.
- Shell scripts start with `set -euo pipefail`, pass `shellcheck`, and work with the bash
  shipped on macOS (3.2) and Linux.
