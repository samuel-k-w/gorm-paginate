# GORM Integration for go-paginate

This package provides a robust, zero-dependency (other than GORM) integration layer for `go-paginate`, preserving all of your existing GORM configurations and connection states.

## Installation

Because this package introduces `gorm.io/gorm` as a dependency, it is maintained as a separate Go module inside the `v4` directory to prevent polluting the core `go-paginate` module for users who use raw SQL.

```bash
go get github.com/booscaaa/go-paginate/v4/gormpaginate
```

## Quick Start

The integration acts as an adapter. It parses your HTTP query into `PaginationParams` using the core library, then safely applies those parameters as GORM clauses without overwriting your query context or transaction state.

```go
import (
	"encoding/json"
	"net/http"

	"github.com/booscaaa/go-paginate/v4/gormpaginate"
	"github.com/booscaaa/go-paginate/v4/paginate"
	"gorm.io/gorm"
)

func ListUsers(w http.ResponseWriter, r *http.Request) {
	// 1. Bind query parameters
	params, _ := paginate.BindQueryParamsToStruct(r.URL.Query())

	// 2. Build your GORM query exactly as usual
	query := db.WithContext(r.Context()).Preload("Company").Where("status = ?", "active")

	// 3. Paginate!
	page, err := gormpaginate.Paginate[User](query, params, r.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(page)
}
```

## Core Architectural Guarantees

This integration was designed with strict correctness invariants:

1. **Transaction & Context Safety**: Calling `Paginate` inside a `db.Transaction(...)` block guarantees the `.Count()` and `.Find()` queries are executed on the same locked `*sql.Tx` connection. `db.Statement.Context` is natively propagated for cancellation.
2. **Caller State Preservation**: The library **never** drops your `Where`, `Preload`, `Joins`, `Unscoped`, or `Select` clauses.
3. **Cursor Type Correctness**: `go-paginate` uses JSON for cursor tokens (converting ints to `float64`). This package automatically inspects your GORM schema and coerces token values back into `int`, `time.Time`, or `uuid.UUID` before passing them to the database, preventing driver panics.
4. **Deterministic Keyset Cursors**: If your sort parameters do not form a unique constraint, the library automatically injects your Primary Key as a tie-breaker.

## Joins & Count Behavior

GORM's `Count()` generates `SELECT count(*)`. If your query contains expanding (1-to-many) joins, a standard count will artificially inflate your total items due to duplicate rows.

**We do not guess your intent.** If you are joining tables that expand the row count, you *must* apply `.Distinct()` to your primary key before passing the query to the paginator.

```go
// CORRECT: Counts distinct users, not total duplicated rows
query := db.Joins("Orders").Distinct("users.id")
page, _ := gormpaginate.Paginate[User](query, params, r.URL)
```

## Security & Allowlisting

By default, the library permits filtering and sorting on **any field explicitly mapped in your GORM struct**. Non-existent struct fields are silently ignored to prevent SQL injection.

For stricter API control, use the configuration options:

```go
page, err := gormpaginate.Paginate[User](query, params, r.URL,
	gormpaginate.AllowFilters("status", "age"),
	gormpaginate.AllowSorts("created_at"),
)
```

## Soft Deletes

GORM automatically filters out soft-deleted records (`DeletedAt IS NULL`). The paginator natively inherits this behavior. To paginate over soft-deleted records, just pass an unscoped query:

```go
query := db.Unscoped()
page, _ := gormpaginate.Paginate[User](query, params, r.URL)
```
