# Review: <scope> (<YYYY-MM-DD HH:MM>)

- **Scope**: <diff | all | path | spec:name> at commit `<sha>`
- **Baseline checks**: verify quick → <pass | fail> (<one-line summary>)
- **Reviewers**: code-reviewer (backend, frontend, delivery), bug-hunter, spec-analyst (compliance)

## Summary

| Severity | Confirmed | Plausible | Fixed |
|---|---:|---:|---:|
| Critical | 0 | 0 | 0 |
| High | 0 | 0 | 0 |
| Medium | 0 | 0 | 0 |
| Low | 0 | 0 | 0 |
| Nit | 0 | 0 | 0 |

## Findings

### R-1 [High] <short title>

- **Where**: `path/to/file.go:42` (and other locations)
- **Category**: correctness | edge case | contract | security | concurrency | design |
  accessibility | tests | docs | delivery
- **Status**: Confirmed | Plausible
- **What happens**: <observable behaviour>
- **Evidence**: <failing test name and output, command and output, or precise reasoning>
- **Impact**: <who is affected and how>
- **Fix**: <concrete change> and the regression test to add
- **Outcome**: open | fixed (<files>, guarded by `<test name>`)

## Spec compliance (spec scope only)

| Requirement | Implemented | Tested | Documented | Status | Notes |
|---|---|---|---|---|---|

## Tests and coverage

<gaps in the test suite, uncovered critical code, flakiness>

## Appendix: rejected findings

| Claim | Source | Why it was rejected |
|---|---|---|
