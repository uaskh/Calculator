---
name: spec
description: Create or refine a product specification in specs/<name>.md from rough notes, a pasted brief or a file, using the project's spec template and asking the user to resolve gaps before implementation. Use when the user wants to write, import or tighten a spec.
argument-hint: "<spec-name> [notes | path/to/notes]"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - AskUserQuestion
  - Agent
---

# /spec

Arguments: `$ARGUMENTS`

1. **Name**: the first argument, in kebab-case (ask if it's missing). The target is
   `specs/<name>.md`. If the file exists you are refining it: read it and keep its content
   unless the user changes it.
2. **Source material**: the rest of the arguments, as inline notes or a path to a file. If
   there is none, ask the user to paste the brief.
3. Read [references/spec-template.md](references/spec-template.md) and `specs/README.md`.
4. **Map the source into the template**:
   - Keep the user's wording where it is precise; don't embellish.
   - Give every requirement an ID (`FR-n`, `NFR-n`, `C-n`) and at least one testable
     acceptance criterion (Given/When/Then). Mark optional items as Optional.
   - Put deliverables, constraints (stack, dependency rules) and non-functional
     requirements in their own sections.
   - Never invent behaviour. Anything unclear goes into "Open questions".
5. **Resolve the open questions with the user**:
   - For long briefs, run `spec-analyst` for a gap analysis first.
   - Ask with `AskUserQuestion`: at most 4 questions per call, recommended option first.
   - Write each answer into the requirement text and the "Decisions" table.
   - Use one to three rounds. The only questions left afterwards are ones the user
     explicitly deferred; `/implement` confirms them before building.
6. Write the file, show a short summary (number of requirements, optional items,
   remaining questions) and suggest `/implement <name>`.
