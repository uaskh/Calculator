# Prompts used to produce this repository

The assessment brief itself lives in `docs/brief.md`, which is git-ignored and never
committed (CI fails if it is tracked). This file records, verbatim, the prompts given to
Claude Code in this repository. The workflow, rules and agent definitions they invoke are
under `.claude/`.

## `/spec calculator`

The specification session ran before this repository's git history began and its prompt
was not kept. The author is reconstructing it from the session transcript and will add it
here verbatim (plan decision D-7). Until then, `specs/calculator.md` section 11 records
every answer given during that session as decisions 1–34, and the plan's decision log
records the later ones.

## `/implement`

Session of 2026-09-18. The command was invoked without arguments:

```text
/implement
```

Claude listed the available specifications and asked which one to build; the answer was
`calculator`. Every later input in that session was an answer to a question Claude asked
through its clarification prompts; those answers are recorded as decisions D-1 to D-8 in
`specs/calculator.plan.md`.
