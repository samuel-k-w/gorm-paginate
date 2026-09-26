# Production-Readiness Notes
**Module**: `github.com/samuel-k-w/gorm-paginate/v4/gormpaginate`  
**Updated**: 2026-09-26

Fork of `booscaaa/go-paginate` focused on publishing **v4 + GORM** from `samuel-k-w/gorm-paginate`.

## Correctness fixes (2026-09-26)

| Issue | Fix |
| :--- | :--- |
| `DefaultSort` did not update `params` → cursor tokens omitted sort columns | `applySorting` mutates `params.Sort` when applying DefaultSort |
| `extractFieldsByJSONTag` skipped anonymous embeds → `null` cursor vals | Walks embedded structs |
| Nil `baseURL` panicked in `NewPage` / `NewCursorPage` | Empty links, no panic |
| `GteOr` / `GtOr` / `LteOr` / `LtOr` discarded GORM chain | `applyComparisonMapOr` reassigns `orDB` |

## Architectural invariants (still hold)

| Requirement | Coverage |
| :--- | :--- |
| Caller GORM state preserved | `TestAudit_CallerState` |
| Transactions | `TestAudit_Transactions` |
| Context | `TestPaginate_ContextCancellation` |
| Count before sort | `TestAudit_CountGroup` / `TestAudit_CountJoins` |
| Cursor PK tie-breaker | `TestCursorPaginate_TieBreaker` |
| DefaultSort ↔ cursor params | `TestDefaultSort_CursorEncodesSortColumn` |
| Embedded JSON fields | `TestEmbeddedBase_CursorExtractsID` |
| Soft-delete default / Unscoped | `TestSoftDelete_DefaultExcludesDeleted` |
| Nil baseURL | `TestNilBaseURL_NoPanic` |
| OR comparison filters | `TestGteOr_AppliesFilter` |
| Security allowlists | `TestAudit_SecuritySQLInjection` |
| Preloads | `TestPaginate_Preload` / `TestAudit_NestedPreload` |

## How to test

```bash
cd v4 && go test ./paginate/... -count=1
cd v4/gormpaginate && go test ./... -count=1
```

## Supported range

- Go 1.22+
- GORM ≥ 1.25 (tested with 1.30)
- SQLite in unit tests; Postgres/MySQL should work via GORM dialectors (not CI-matrixed yet)
