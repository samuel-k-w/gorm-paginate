package gormpaginate

import (
	"fmt"
	"net/url"

	"github.com/booscaaa/go-paginate/v4/paginate"
	"gorm.io/gorm"
)

// Paginate executes an offset-paginated query returning a Page[T].
// It preserves all existing caller scopes, transactions, and context on the provided db.
func Paginate[T any](db *gorm.DB, params *paginate.PaginationParams, baseURL *url.URL, opts ...Option) (paginate.Page[T], error) {
	cfg := newConfig(opts...)

	model := new(T)
	if db.Statement.Model == nil {
		db = db.Model(model)
	}
	resolver, err := newSchemaResolver(db, model)
	if err != nil {
		return paginate.Page[T]{}, fmt.Errorf("failed to parse schema: %w", err)
	}

	// Apply dynamic filters
	query := applyFilters(db, params, resolver, cfg)

	// Safe count fork: use Session to clone statement before applying sorting.
	// We also temporarily remove preloads so they don't break the Count query.
	preloads := query.Statement.Preloads
	query.Statement.Preloads = nil

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return paginate.Page[T]{}, fmt.Errorf("count query failed: %w", err)
	}

	// Restore preloads
	query.Statement.Preloads = preloads

	// Apply sorting for the data query
	query = applySorting(query, params, resolver, cfg)

	// Resolve limits
	page, limit := resolvePageLimit(params)
	offset := (page - 1) * limit

	var data []T
	if err := query.Limit(limit).Offset(offset).Find(&data).Error; err != nil {
		return paginate.Page[T]{}, fmt.Errorf("data query failed: %w", err)
	}

	// Handle empty dataset slice nicely instead of nil
	if data == nil {
		data = make([]T, 0)
	}

	return paginate.NewPage(data, int(total), params, baseURL), nil
}

// CursorPaginate executes a keyset-paginated query returning a CursorPage[T].
// It preserves all existing caller scopes, transactions, and context on the provided db.
func CursorPaginate[T any](db *gorm.DB, params *paginate.PaginationParams, baseURL *url.URL, opts ...Option) (paginate.CursorPage[T], error) {
	cfg := newConfig(opts...)

	model := new(T)
	if db.Statement.Model == nil {
		db = db.Model(model)
	}
	resolver, err := newSchemaResolver(db, model)
	if err != nil {
		return paginate.CursorPage[T]{}, fmt.Errorf("failed to parse schema: %w", err)
	}

	// Apply dynamic filters, cursor seek logic, and sorting
	query := applyFilters(db, params, resolver, cfg)
	query = applyCursor(query, params, resolver)
	query = applySorting(query, params, resolver, cfg)

	_, limit := resolvePageLimit(params)

	var data []T
	// Fetch limit + 1 to detect if there is a next page
	if err := query.Limit(limit + 1).Find(&data).Error; err != nil {
		return paginate.CursorPage[T]{}, fmt.Errorf("data query failed: %w", err)
	}

	if data == nil {
		data = make([]T, 0)
	}

	return paginate.NewCursorPage(data, params, baseURL), nil
}

func resolvePageLimit(params *paginate.PaginationParams) (int, int) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 {
		limit = paginate.GetDefaultLimit()
	}
	maxLimit := paginate.GetMaxLimit()
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}
