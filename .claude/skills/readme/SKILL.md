---
name: readme
description: Write or refresh README.md and the docs/adr index from the actual code - product overview, architecture, setup, running backend and frontend, configuration, API reference with executed curl examples, testing and coverage, design decisions, assumptions and limitations. Use after features change or when asked to document the project; with --check it reports drift without editing.
argument-hint: "[--check]"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Agent
  - Bash(make *)
  - Bash(go *)
  - Bash(npm *)
  - Bash(npx *)
  - Bash(curl *)
  - Bash(docker compose *)
  - Bash(git log *)
  - Bash(git ls-files *)
  - Bash(git status *)
  - Bash(lsof -i *)
---

# /readme

Arguments: `$ARGUMENTS`

The README is written from facts gathered in this run, never from memory. Standards:
`.claude/rules/documentation.md`. Structure: [references/readme-template.md](references/readme-template.md).

## 1. Gather facts

- Product and decisions: `specs/*.md`, the plans' decision logs, `docs/adr/`.
- Commands: `make help`; `cd frontend && npm pkg get scripts`.
- Versions: the `go` directive in `backend/go.mod`, `.nvmrc`, `frontend/package.json`
  (engines and main packages), Docker base images.
- Configuration: every variable read in `backend/internal/config`, every `VITE_*` variable
  and `API_PROXY_TARGET` in the frontend, with defaults.
- API: routes in `backend/internal/httpapi/router.go`, schemas in `backend/api/openapi.yaml`,
  error codes in `problem.go` and the domain error mappers.
- UI: features, states and keyboard behaviour in `frontend/src/features/`.
- Tests: how many per layer (`go test ./... -v | grep -c '^=== RUN'`, the Vitest summary,
  `npx playwright test --list`) and coverage from `docs/coverage.md` (run `make coverage`
  if it is missing or older than the code).
- Layout: `git ls-files`, condensed to an annotated tree two or three levels deep.

## 2. Run every example

1. Make sure port 8080 is free (`lsof -i :8080`). Start `make run-backend` in the
   background and wait until `curl -s http://localhost:8080/healthz` answers.
2. For every endpoint, run one success example and the representative errors (validation,
   rule violation, malformed JSON, unsupported media type, unknown route or method) with
   `curl -s -i`. Capture status lines and bodies exactly as returned; pretty-printing JSON
   and shortening request IDs is fine.
3. Stop the backend. If Docker is running, check `docker compose up --build --detach --wait`
   and a request through `http://localhost:3000`, then `docker compose down`.

## 3. Write

- For a new or heavily changed README, give `docs-writer` the facts and the captured
  outputs; make small updates yourself.
- Keep every section of the template and fill every placeholder; remove the guidance comments.
- Update `docs/adr/README.md` when ADRs changed.

## 4. Check before finishing

- Every command in the README was executed above or exists in `make help` or the npm
  scripts; list any exception and the reason.
- Relative links resolve; the Mermaid diagram uses a valid diagram type and balanced syntax.
- No placeholders, TODOs, marketing language or remarks about how the repository was produced.
- `--check`: don't edit anything; report drift as `section → problem → evidence`.
