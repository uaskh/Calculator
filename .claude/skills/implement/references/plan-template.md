# <Spec title>: implementation plan

- Spec: `specs/<name>.md` (read on YYYY-MM-DD)
- Mode: greenfield | brownfield | resumed
- Status: draft | approved | in progress | done

## 1. Requirements matrix

| ID | Type | Requirement (spec §) | Acceptance criteria | Priority | Tests |
|---|---|---|---|---|---|
| FR-1 | Functional | … (§4.1) | Given … when … then … | Must | `TestX/…`, `Widget.test.tsx › …`, `e2e/x.spec.ts › …` |
| NFR-1 | Non-functional | … | … | Must | … |

Priority: Must, Should, Could, Optional (Optional is built only when the user opts in).

## 2. Decisions and assumptions

| ID | Question | Decision (who, when) | Consequence |
|---|---|---|---|
| D-1 | … | … (user, YYYY-MM-DD) | … |

| ID | Assumption | Why it is safe to assume |
|---|---|---|
| A-1 | … | … |

## 3. Architecture

- Components and responsibilities (Mermaid diagram).
- Backend packages and the interfaces between them.
- Frontend structure: features, state, data flow.
- Extension points: how a new variant (rule, type, field or format) is added without
  editing unrelated code.

## 4. API contract

| Method | Path | Request | Success | Errors (status/code) |
|---|---|---|---|---|

Schemas with validation rules and limits, error codes, example request/response pairs.

## 5. UI design

Regions and components; states (idle, loading, success, empty, error); copy; keyboard
behaviour; responsive behaviour; accessibility notes.

## 6. Configuration

| Variable | Component | Default | Purpose |
|---|---|---|---|

## 7. Test strategy

What each layer covers for this spec, the edge-case catalogue, e2e journeys, coverage targets.

## 8. Tasks

Vertical slices, each including its tests. Keep the checkboxes current.

### Phase 3: Scaffold
- [ ] T-1 …

### Phase 4: Backend
- [ ] T-… (FR-…) …

### Phase 5: Frontend
- [ ] T-… (FR-…) …

### Phase 6: Integration and end-to-end
- [ ] T-… …

### Phase 7: Verify, review, document
- [ ] Full verification
- [ ] Review and fixes
- [ ] ADRs
- [ ] README

## 9. Dependencies to add

| Package | Side | Why | Approved by user |
|---|---|---|---|

## 10. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|

## 11. Progress log

- YYYY-MM-DD: Phase n complete, commit `abc1234` (`feat(api): …`).
