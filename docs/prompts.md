# Prompts used to produce this repository

The assessment brief itself lives in `docs/brief.md`, which is git-ignored and never
committed (CI fails if it is tracked). The prompts for each step are kept in the
[`prompts/`](../prompts/) folder; the workflow, rules and agent definitions they invoke are
under `.claude/`.

| Step | Prompt                                                                        | Produces                                                                                              |
| ---- | ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| 1    | [`prompts/01-claude-project-setup.md`](../prompts/01-claude-project-setup.md) | `.claude/` (engineering guide, rules, agents, skills, hooks, settings) and `specs/` with its template |
| 2    | [`prompts/02-calculator-spec.md`](../prompts/02-calculator-spec.md)           | `specs/calculator.md`, including its decisions table                                                  |
| 3    | `/implement` (see below)                                                      | everything else: plan, code, tests, docs, containers, CI                                              |

## `/implement`

Session of 2026-09-18. The command was invoked without arguments:

```text
/implement
```

Claude listed the available specifications and asked which one to build; the answer was
`calculator`. Every later input in that session and the sessions after it was an answer
to a question Claude asked through its clarification prompts, or a request for a change;
the answers are recorded as decisions D-1 to D-15 in `specs/calculator.plan.md` and, where
they changed the product, as decisions 35 to 40 in `specs/calculator.md`.
