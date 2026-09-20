# Architecture decision records

Each record captures one significant decision: its context, the decision and its
consequences. Records are immutable once accepted; a later record supersedes an earlier one.

| #                                                  | Title                                                            | Status   |
| -------------------------------------------------- | ---------------------------------------------------------------- | -------- |
| [0001](0001-architecture-and-dependency-policy.md) | Build on the standard library plus one decimal module            | Accepted |
| [0002](0002-api-shape-and-error-model.md)          | Expose one evaluation endpoint with RFC 9457 problem details     | Accepted |
| [0003](0003-number-representation.md)              | Compute with exact decimals and bounded precision                | Accepted |
| [0004](0004-lenient-normalization.md)              | Normalize incomplete expressions before parsing                  | Accepted |
| [0005](0005-percent-semantics.md)                  | Give `%` calculator semantics as a table-driven postfix operator | Accepted |
| [0006](0006-live-preview-strategy.md)              | Debounce live evaluation and cancel superseded requests          | Accepted |
| [0007](0007-container-topology.md)                 | Ship two containers behind a static web server                   | Accepted |
