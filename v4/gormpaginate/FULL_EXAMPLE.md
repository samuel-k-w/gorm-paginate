# GORM Paginate: Full Production Example

This guide demonstrates a production-grade implementation of `gormpaginate` inside a layered **Gin** application (Handler → Service → Repository). 

It showcases **all** features of the library, including context propagation, transactions, preloads, security allowlists, offset pagination, cursor pagination, soft-deletes, and complex filtering.

---

## 1. The Models (Domain)

We define models with relationships (Company, Roles) and soft-deletes (`gorm.DeletedAt`).

```go
package domain

import (
	"time"
	"gorm.io/gorm"
)

type Company struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CompanyID    uint           `json:"company_id"`
	Company      Company        `json:"company"` // Belongs-To
	Name         string         `json:"name"`
	Email        string         `json:"email"`
	Age          int            `json:"age"`
	Status       string         `json:"status"`
	PasswordHash string         `json:"-"` // Never exposed, but needs query protection!
	CreatedAt    time.Time      `json:"created_at"`
	DeletedAt    gorm.DeletedAt `json:"-"` // Soft deletes
}
```

---

## 2. The Repository (Data Layer)

This layer handles the database connection. It applies our base business rules (Preloads, Scopes) and utilizes `gormpaginate` to securely apply the dynamic client parameters.

```go
package repository

import (
	"context"
	"net/url"

	"github.com/booscaaa/go-paginate/v4/gormpaginate"
	"github.com/booscaaa/go-paginate/v4/paginate"
	"gorm.io/gorm"
	"myapp/domain"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// 1. Offset Pagination with Preloads and Security Allowlists
func (r *UserRepository) FindOffset(ctx context.Context, params *paginate.PaginationParams, baseURL *url.URL) (paginate.Page[domain.User], error) {
	// Base Query: Propagate context, preload relationships, and filter active only
	query := r.db.WithContext(ctx).
		Preload("Company").
		Where("status = ?", "active")

	// Paginator: Apply client params on top of the base query
	return gormpaginate.Paginate[domain.User](query, params, baseURL,
		// SECURITY: Prevent users from brute-forcing passwords via ?eq[PasswordHash]=...
		gormpaginate.AllowFilters("name", "email", "age", "company_id"),
		gormpaginate.AllowSorts("name", "created_at", "age"),
	)
}

// 2. Cursor Pagination inside a Transaction with Soft-Deletes included
func (r *UserRepository) FindCursorInTx(ctx context.Context, params *paginate.PaginationParams, baseURL *url.URL) (paginate.CursorPage[domain.User], error) {
	var page paginate.CursorPage[domain.User]

	// Execute inside a safe transaction block.
	// The paginator natively uses this transaction for both Count() and Find() queries.
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		
		// Base Query: Include soft-deleted records (Unscoped)
		query := tx.Unscoped().Preload("Company")

		var txErr error
		page, txErr = gormpaginate.CursorPaginate[domain.User](query, params, baseURL,
			gormpaginate.DefaultSort("created_at", "DESC"),
		)
		return txErr
	})

	return page, err
}
```

---

## 3. The Service (Business Logic Layer)

The service acts as the orchestrator. It knows nothing about HTTP or Gin, and nothing about GORM internals. 

```go
package service

import (
	"context"
	"net/url"

	"github.com/booscaaa/go-paginate/v4/paginate"
	"myapp/domain"
	"myapp/repository"
)

type UserService struct {
	repo *repository.UserRepository
}

func NewUserService(repo *repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) GetUsers(ctx context.Context, params *paginate.PaginationParams, baseURL *url.URL) (paginate.Page[domain.User], error) {
	// Example Business Rule: Max limit is 50
	if params.Limit > 50 {
		params.Limit = 50
	}
	return s.repo.FindOffset(ctx, params, baseURL)
}

func (s *UserService) GetUsersCursor(ctx context.Context, params *paginate.PaginationParams, baseURL *url.URL) (paginate.CursorPage[domain.User], error) {
	return s.repo.FindCursorInTx(ctx, params, baseURL)
}
```

---

## 4. The Handler (Gin / HTTP Layer)

The handler binds the raw HTTP query string into the `PaginationParams` DTO, passing it down to the service.

```go
package handler

import (
	"net/http"

	"github.com/booscaaa/go-paginate/v4/paginate"
	"github.com/gin-gonic/gin"
	"myapp/service"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) ListOffset(c *gin.Context) {
	// 1. Bind the URL query string
	params, err := paginate.BindQueryParamsToStruct(c.Request.URL.Query())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pagination parameters"})
		return
	}

	// 2. Fetch data (Passing Request Context for timeout/cancellation safety)
	page, err := h.svc.GetUsers(c.Request.Context(), params, c.Request.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 3. Return payload
	c.JSON(http.StatusOK, page)
}

func (h *UserHandler) ListCursor(c *gin.Context) {
	params, _ := paginate.BindQueryParamsToStruct(c.Request.URL.Query())
	
	page, err := h.svc.GetUsersCursor(c.Request.Context(), params, c.Request.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}
```

---

## 5. Client Usage: The Magic

Because the backend passes `PaginationParams` dynamically to the `gormpaginate` adapter, the frontend gets massive query flexibility **without requiring any extra backend code**.

### Basic Pagination
```http
GET /users?page=2&limit=15
```

### Multi-Column Searching
Automatically generates `(name ILIKE '%john%' OR email ILIKE '%john%')`
```http
GET /users?search=john&search_fields=name,email
```

### Complex Filtering
Find users matching specific criteria (Age between 20-30, Name matches "Ali", Company is 1 or 2).
```http
GET /users?between[age]=20,30&like[name]=Ali&in[company_id]=1,2
```

### Multi-Column Sorting
Sort descending by `created_at`, then ascending by `name`.
```http
GET /users?sort=-created_at,name
```

### Cursor Pagination (Next Page)
The initial request:
```http
GET /users/cursor?limit=25
```
Yields a response containing a `next` URL. The library automatically built a mathematical tie-breaker using the Primary Key under the hood, guaranteeing records don't skip or duplicate. The frontend simply calls the provided `next` link:
```http
GET /users/cursor?limit=25&cursor=eyJjb2xzIjpbImFnZSIsImlkIl0sInZhbHMiOlszMCwidXVpZC0yIl0sImRpcnMiOlsiREVTQyIsIkFTQyJdLCJkaXIiOiJuZXh0In0=
```
