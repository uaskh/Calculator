---
name: implement
description: Implement a product spec from specs/ end to end on this repository (Go standard-library REST backend and React + TypeScript frontend). Clarifies open questions with the user, plans, scaffolds an empty repo or extends existing code, builds test-first, adds end-to-end tests, verifies, reviews, documents and commits each phase. Resumable from its plan file.
argument-hint: "<spec-name | path/to/spec.md> [--from <phase>] [--only backend|frontend]"
disable-model-invocation: true
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Agent
  - AskUserQuestion
  - Skill
  - TaskCreate
  - TaskUpdate
  - TaskList
  - Bash(go *)
  - Bash(gofmt *)
  - Bash(golangci-lint *)
  - Bash(npm *)
  - Bash(npx *)
  - Bash(make *)
  - Bash(git *)
  - Bash(curl *)
  - Bash(mkdir *)
  - Bash(docker compose *)
  - Bash(bash .claude/hooks/session-context.sh *)
  - Bash(bash .claude/skills/implement/scripts/scaffold.sh *)
---

# /implement

Spec: `$ARGUMENTS`

You are the tech lead for this run: you own requirements, decisions, sequencing,
integration and quality. Delegate well-specified work to subagents; never delegate the
conversation with the user (subagents cannot ask questions; you ask, with `AskUserQuestion`).
These instructions apply for the whole run, across turns.

## Ground rules

1. `.claude/CLAUDE.md` (already in your context) and `.claude/rules/` define how to build.
   Read the rules at the start: path-scoped rules do not load until matching files exist.
2. **Clarify before building.** Ask about anything the spec leaves open that changes
   behaviour, the API contract, UX, data, security, dependencies or delivery
   ([references/clarification.md](references/clarification.md)). Decide small, conventional,
   reversible details yourself and record them as assumptions. When unsure which bucket a
   question belongs to, ask.
3. Build exactly the spec. Optional items are built only when the user opts in.
4. Test first in every slice. Every phase ends green.
5. Evidence over claims: every "done" is backed by a command you ran and its output.
6. `specs/<name>.plan.md` is the single record of progress (checkboxes), decisions and
   assumptions. Keep it current so a later `/implement <name>` resumes cleanly.
7. Commit automatically at the end of each phase once its checks pass (Conventional
   Commits, see CLAUDE.md). Never push.
8. Track the phases with the task tools (one task per phase).

## Phase 0: Resolve the spec and inspect the repository

1. Arguments: the first token names the spec; `--from <phase>` resumes at a phase (0–8);
   `--only backend|frontend` limits the scope.
2. Resolve the spec, first match wins: the token as a path, `specs/<token>.md`,
   `specs/<token>/spec.md`, `specs/<token>/README.md`. If nothing matches or no argument was
   given, list `specs/*.md` (without `_template.md`, `README.md`, `*.plan.md`) and ask which
   one to use, offering `/spec <name>` to write a new one. Stop until answered.
3. Read the whole spec, then the rules, `README.md` and `docs/adr/` if present.
4. Snapshot: `bash .claude/hooks/session-context.sh </dev/null`, `git status`,
   `git log --oneline -10`.
5. Toolchain: `go version` (1.24 or newer), `node --version` (what the current Vite needs),
   `npm --version`, `git --version`; note whether `docker`, `golangci-lint` and
   `govulncheck` exist. A missing hard requirement → give the user the exact install command
   and stop. If `git config user.email` is empty, ask the user to set it before the first
   commit.
6. Mode:
   - **Resume**: `specs/<name>.plan.md` exists → read it and ask whether to continue from
     the first unchecked task or to re-plan (for example because the spec changed).
   - **Greenfield**: neither `backend/go.mod` nor `frontend/package.json` exists → Phase 3
     scaffolds the repository.
   - **Brownfield**: code exists → map it first (use the `Explore` agent unless it is tiny):
     architecture, conventions, commands, tests, API surface, and gaps against this spec.
     Extend the existing design; raise conflicts with the non-negotiables and ask.

## Phase 1: Analyse and clarify (gate)

1. Launch `spec-analyst` with the spec path and the brownfield summary. It returns a
   requirements matrix, ambiguities with proposed options, conflicts, edge cases and
   low-risk assumptions.
2. Check its output against the spec yourself and add what it missed, using
   [references/clarification.md](references/clarification.md).
3. Ask every open question in the "always ask" categories: at most 4 questions per
   `AskUserQuestion` call, related questions together, the recommended option first with
   its trade-off. Repeat until nothing blocking remains. Never ask what the spec answers.
4. Record answers as decisions `D-n` and your own low-risk choices as assumptions `A-n`.

## Phase 2: Plan (gate)

1. Write `specs/<name>.plan.md` from [references/plan-template.md](references/plan-template.md):
   requirements matrix, architecture, API contract, UI design, configuration, test
   strategy, vertical-slice tasks with checkboxes, dependencies, risks, decisions,
   assumptions.
2. Design the API contract first (endpoints, schemas, status and error codes, limits). It is
   the handshake that lets backend and frontend work proceed in parallel.
3. Show the user a summary of at most 25 lines (architecture, endpoints, screens, test
   layers, dependencies to add, notable decisions) and ask for approval with
   `AskUserQuestion` (Approve / Change something). Apply changes; re-confirm big ones.
4. Commit the approved plan: `docs(plan): add <name> implementation plan`.

## Phase 3: Scaffold (greenfield only)

Follow [references/scaffold.md](references/scaffold.md). The skeleton in `templates/` is
already verified; `scripts/scaffold.sh` copies and personalises it, so don't retype it.

1. `git init -b main` if needed, `git config core.hooksPath .claude/githooks`, then
   `bash .claude/skills/implement/scripts/scaffold.sh repo` (Makefile, CI, compose,
   `.gitignore`, `.editorconfig`, `.nvmrc`, coverage script, ADR 0001).
2. `bash .claude/skills/implement/scripts/scaffold.sh backend <module-path>`: Go skeleton
   (config, server, router, strict decoding, problem details, middleware, health, e2e
   tests, golangci-lint config, Dockerfile, OpenAPI), vetted and tested by the script.
3. Frontend: `npm create vite@latest frontend -- --template react-ts`, then
   `bash .claude/skills/implement/scripts/scaffold.sh frontend "<Product name>" "<description>"`,
   then install the dev dependencies with the single `npm install -D …` command from the
   guide (with `npm create`, the only installs the user has to approve) and merge the
   TypeScript and ESLint settings as the guide describes.
4. Read the files the guide lists as conventions; skim the rest with Glob.
5. Exit when `make fmt lint typecheck test build` passes (including the API-client canary
   test) and `make dev`, started as a background task, serves both apps (check the 404
   problem document through the Vite proxy, then stop the task). Commit
   `chore: scaffold backend and frontend`.

## Phase 4: Backend, test-first

- Delegate slices to `go-backend-engineer` with the brief template below, pointing it to
  [references/backend-feature-pattern.md](references/backend-feature-pattern.md): domain →
  HTTP handler → wiring → OpenAPI → handler and black-box tests. Slices that touch different
  files may run in parallel (several `Agent` calls in one message); otherwise in sequence.
- After each slice read the diff (`git diff`), run `cd backend && go test -race ./...` and
  tick the plan.
- Exit: every backend requirement implemented and tested; `go vet`, `golangci-lint run` and
  `go test -race` green; coverage at or above the thresholds; `openapi.yaml` matches the
  handlers. Commit `feat(api): …`.

## Phase 5: Frontend, test-first

- Can run in parallel with Phase 4 once the contract is fixed: launch both engineers in the
  same message (they own different directories).
- Delegate to `react-frontend-engineer`, pointing it to
  [references/frontend-feature-pattern.md](references/frontend-feature-pattern.md): API
  types, parsers and endpoint functions (with MSW handlers) → pure model or reducer → hooks
  → components → page composition → responsive and accessibility polish.
- Exit: every UI requirement implemented and tested; typecheck, lint and Vitest coverage
  green; `npm run build` succeeds. Commit `feat(web): …`.

## Phase 6: Integration and end-to-end

- Delegate to `e2e-test-engineer`: black-box API tests in `backend/test/e2e` and Playwright
  journeys (happy paths, validation errors, server and network failures, keyboard-only
  use, mobile viewport).
- Check the running product yourself: start `make dev` as a background task; call every
  endpoint with `curl` (valid and invalid input); take screenshots with
  `cd frontend && npx playwright screenshot --viewport-size=375,812 http://localhost:5173 test-results/mobile.png`
  and again at `1280,800` (`test-results/desktop.png`), and look at them with Read. Fix what
  looks wrong, then stop the task.
- Containers in scope: `docker compose up --build --wait`, smoke-test through
  `http://localhost:3000`, then `docker compose down`.
- Exit: `make test-e2e` green. Commit `test(e2e): …`.

## Phase 7: Verify, review, document

1. Run the `verify` skill with `full`. Fix every failure at its root cause.
2. Run the `review` skill with `spec:<name> --fix`. Every confirmed critical, high and
   medium finding gets a regression test and a fix; verify again.
3. Write ADRs for the significant decisions (`docs/adr/`, indexed in `docs/adr/README.md`).
4. Run the `readme` skill: it rewrites README.md from the code and executes every example.
5. Final `make verify`. Commit `docs: …` (the review skill has already committed its fixes).

## Phase 8: Report

Follow [references/definition-of-done.md](references/definition-of-done.md): check every
item, then report what was built, traceability (requirement → code → tests), how to run
it, real test and coverage numbers, decisions and assumptions, limitations and the commits
created. List anything unmet explicitly; never claim done without evidence.

## Subagent brief template

```text
Task: <slice name> (plan task T-n) for spec specs/<name>.md
Requirements: FR-x, FR-y. Acceptance criteria:
  - Given … when … then …
Contract: <endpoint, schema and error-code excerpt, or UI states and copy>
Applicable decisions and assumptions: D-n, A-n
Files you own: <paths>. Do not modify: <paths owned by others>.
Read before coding: .claude/rules/<…>.md, <existing files to follow>
Done when: tests written first and passing; <commands> green; no new dependencies;
no TODOs. Do not commit.
Return: summary, files changed, tests added, commands run with results, deviations,
open questions.
```

## When things go wrong

- The same check fails three times → stop, explain the root cause and the options, ask.
- The spec turns out contradictory mid-way → pause, ask, update the plan and decision log.
- A guardrail hook blocks an action → follow its reason, or ask the user.
- Tooling or network unavailable → give the user the exact command to run; no workarounds.
- You need a decision while checks fail → ask with `AskUserQuestion` (the quality gate
  only runs when a turn ends).
