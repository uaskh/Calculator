---
name: review
description: Deep code review and bug hunt for this repository - correctness, edge cases, API contract, security, concurrency, SOLID design, accessibility, test quality and documentation drift - with evidence for every finding and optional fixes backed by regression tests. Use when asked to review code, find bugs, audit changes or a spec's implementation, or before finishing a feature.
argument-hint: "[diff | all | <path> | spec:<name>] [--fix]"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Agent
  - AskUserQuestion
  - Skill
  - Bash(git *)
  - Bash(go *)
  - Bash(golangci-lint *)
  - Bash(npm *)
  - Bash(npx *)
  - Bash(make *)
  - Bash(curl *)
  - Bash(mkdir *)
---

# /review

Scope and flags: `$ARGUMENTS`

Find real defects and risks, prove them, and with `--fix` remove them without weakening
anything. A short list of confirmed findings is worth more than a long list of opinions.

## 1. Scope

| Argument | What is reviewed |
|---|---|
| none or `diff` | uncommitted changes (`git diff HEAD` plus untracked files). If the tree is clean, ask which range to review (last commit, last phase, whole repository). |
| `all` | the whole codebase: backend, frontend, containers, CI, docs |
| `<path>` | that file or directory |
| `spec:<name>` | the whole codebase plus compliance with `specs/<name>.md` and its plan |

`--fix` fixes every confirmed critical, high and medium finding after the report. Without
it, ask with `AskUserQuestion` (multi-select) which findings to fix.

## 2. Baseline evidence

Run the `verify` skill with `quick` (or the equivalent commands if the Makefile doesn't
exist yet). Every failing check is a finding. Note coverage numbers and uncovered critical code.

## 3. Parallel review

Build a review packet: the file list, the diff (if any), relevant spec and plan excerpts,
the API contract, decisions `D-n` and assumptions `A-n`. Then launch, in one message,
the reviewers that apply to the scope:

- `code-reviewer`, focus **backend** (Go)
- `code-reviewer`, focus **frontend** (React, TypeScript, CSS, accessibility)
- `code-reviewer`, focus **delivery** (Makefile, Dockerfiles, compose, nginx, CI, README and OpenAPI accuracy)
- `bug-hunter` for the scope (adversarial, with executable probes)
- `spec-analyst` in compliance mode, when a spec is in scope

Each gets [references/checklist.md](references/checklist.md) and returns findings in the
format of [references/report-template.md](references/report-template.md).

## 4. Triage (your job, not the subagents')

For every finding:

1. Open the cited code and confirm it says what the finding claims.
2. Reproduce behavioural claims with a failing test (preferred), a probe from
   `bug-hunter`, or `curl` against a locally started backend.
3. Classify it:
   - **Confirmed**: you reproduced it.
   - **Plausible**: strong reasoning, but you couldn't reproduce it now; say why.
   - **Rejected**: a false positive; say why and move it to the appendix.
4. Assign a severity:
   - **Critical**: a security hole, data loss, a crash or panic, wrong results on a main path, or a broken build.
   - **High**: wrong behaviour on realistic edge cases, a contract violation, failing or flaky tests, or an accessibility blocker.
   - **Medium**: missing validation or error handling, missing tests for important paths, or design problems with concrete impact (including SOLID violations).
   - **Low**: minor inconsistencies, small UX or documentation gaps.
   - **Nit**: style only, and only where the tooling doesn't enforce it.
5. Merge duplicates that share a root cause.

## 5. Report

Write `.reviews/<YYYY-MM-DD-HHMM>-<scope>.md` from the template (add `/.reviews/` to
`.gitignore` if it is missing; never write reports under `.claude/`, which is a protected
path). In chat, show the counts per severity and the confirmed findings with file:line and
one-line summaries.

## 6. Fix (with `--fix` or the user's approval)

For each confirmed finding, most severe first:

1. Write a regression test and watch it fail for the right reason.
2. Fix the root cause. Delegate larger fixes to `go-backend-engineer` or
   `react-frontend-engineer` with a complete brief.
3. Run the affected tests, then `verify quick`.
4. Record the outcome in the report: the files changed and the test that guards the fix.

Commit with `fix(<scope>): …` per the git policy and summarise what changed.

Never "fix" a finding by any of these:

- deleting or weakening tests, or lowering thresholds;
- adding `//nolint` or `eslint-disable` without a documented false positive;
- swallowing errors.
