# Definition of done and final report

## Checklist (every item true, with evidence)

**Functionality**
- [ ] Every Must and Should requirement, and every optional one the user opted into, is implemented.
- [ ] The traceability matrix names at least one passing test per acceptance criterion.
- [ ] The edge cases listed in the plan are handled and tested.

**Quality**
- [ ] `make verify` passes: formatting, lint, typecheck, tests with the race detector,
      coverage thresholds, build, end-to-end. Quote the summary lines.
- [ ] `backend/go.mod` requires only `github.com/shopspring/decimal` (the single approved
      module) with `go.sum` committed; frontend runtime dependencies are `react`,
      `react-dom` and anything the user approved.
- [ ] No TODO/FIXME, commented-out code, debug output, skipped or focused tests.
- [ ] Review: no open critical, high or medium findings; low ones are listed.
- [ ] Containers (when in scope): `docker compose up --build --wait` works and the smoke test passed.

**Documentation**
- [ ] README complete per `.claude/rules/documentation.md`, every command and example executed.
- [ ] `backend/api/openapi.yaml` matches the implementation.
- [ ] ADRs for the significant decisions; `docs/coverage.md` is current.

**Delivery**
- [ ] Plan checkboxes and progress log are current.
- [ ] Work is committed in phase commits and the working tree is clean.

## Report template

Use this structure for the final message:

    ## <Spec title>: implemented

    **Summary**: two or three sentences.

    **What was built**
    - Backend: endpoints and behaviour
    - Frontend: screens and behaviour
    - Delivery: Makefile, CI, containers

    **Traceability**
    | Requirement | Implementation | Tests |
    |---|---|---|

    **How to run**: `make setup`, `make dev` (web http://localhost:5173, API
    http://localhost:8080), `make verify`, `make docker-up` (web http://localhost:3000)

    **Quality evidence**
    - Tests: N Go, M Vitest, K Playwright, all passing
    - Coverage: backend X% (domain Y%), frontend Z% lines
    - Lint and typecheck: clean

    **Decisions and assumptions**: one line each (full list in the plan and ADRs)

    **Known limitations and next steps**

    **Commits**: `git log --oneline` for this run
