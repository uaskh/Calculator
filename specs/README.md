# Specifications

Product requirements live here, one file per product or feature.

| File | Purpose |
|---|---|
| `<name>.md` | the specification: what to build and why |
| `<name>.plan.md` | the implementation plan: requirements matrix, decisions, tasks, progress |
| `_template.md` | starting point for a new specification |

## Writing a specification

- Copy `_template.md` to `<name>.md` (kebab-case), or run `/spec <name>` with your notes to
  have it drafted and to be asked about the gaps.
- Be precise about behaviour, edge cases, constraints and deliverables, and mark optional
  items as Optional. Anything left open is raised as a question before implementation starts.

## Implementing

- `/implement <name>` asks the open questions, writes `<name>.plan.md` for approval, then
  builds, tests, reviews, documents and commits the work phase by phase.
- Running `/implement <name>` again resumes from the plan.
