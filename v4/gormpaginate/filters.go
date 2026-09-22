package gormpaginate

import (
	"github.com/booscaaa/go-paginate/v4/paginate"
	"gorm.io/gorm"
)

// applyFilters applies the Where/Or clauses from PaginationParams to the GORM DB.
func applyFilters(db *gorm.DB, params *paginate.PaginationParams, resolver *schemaResolver, cfg *config) *gorm.DB {
	// Search (ILIKE / LIKE across multiple fields)
	if params.Search != "" && len(params.SearchFields) > 0 {
		db = applySearch(db, params.Search, params.SearchFields, resolver, cfg)
	}

	// Like filters (AND group)
	for field, values := range params.Like {
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsFilterAllowed(field, cfg) || len(values) == 0 {
			continue
		}
		sub := db.Session(&gorm.Session{NewDB: true})
		for i, v := range values {
			if i == 0 {
				sub = sub.Where(likeExpr(db, f.DBName), "%"+v+"%")
			} else {
				sub = sub.Or(likeExpr(db, f.DBName), "%"+v+"%")
			}
		}
		db = db.Where(sub)
	}

	// LikeAnd filters
	for field, values := range params.LikeAnd {
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsFilterAllowed(field, cfg) || len(values) == 0 {
			continue
		}
		for _, v := range values {
			db = db.Where(likeExpr(db, f.DBName), "%"+v+"%")
		}
	}

	// Eq filters (multiple values OR'd within same field, AND'd with rest)
	for field, values := range params.Eq {
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsFilterAllowed(field, cfg) || len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			db = db.Where(f.DBName+" = ?", values[0])
		} else {
			db = db.Where(f.DBName+" IN ?", values)
		}
	}

	// EqAnd filters
	for field, values := range params.EqAnd {
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsFilterAllowed(field, cfg) || len(values) == 0 {
			continue
		}
		for _, v := range values {
			db = db.Where(f.DBName+" = ?", v)
		}
	}

	// Gte, Gt, Lte, Lt
	db = applyComparisonMap(db, params.Gte, resolver, cfg, ">=")
	db = applyComparisonMap(db, params.Gt, resolver, cfg, ">")
	db = applyComparisonMap(db, params.Lte, resolver, cfg, "<=")
	db = applyComparisonMap(db, params.Lt, resolver, cfg, "<")

	// In / NotIn
	for field, values := range params.In {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) && len(values) > 0 {
			db = db.Where(f.DBName+" IN ?", values)
		}
	}
	for field, values := range params.NotIn {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) && len(values) > 0 {
			db = db.Where(f.DBName+" NOT IN ?", values)
		}
	}

	// Between
	for field, vals := range params.Between {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) {
			db = db.Where(f.DBName+" BETWEEN ? AND ?", vals[0], vals[1])
		}
	}

	// IsNull / IsNotNull
	for _, field := range params.IsNull {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) {
			db = db.Where(f.DBName + " IS NULL")
		}
	}
	for _, field := range params.IsNotNull {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) {
			db = db.Where(f.DBName + " IS NOT NULL")
		}
	}

	// OR groups
	db = applyOrGroup(db, params, resolver, cfg)

	return db
}

func applyComparisonMap(db *gorm.DB, m map[string]any, resolver *schemaResolver, cfg *config, op string) *gorm.DB {
	for field, v := range m {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) {
			db = db.Where(f.DBName+" "+op+" ?", v)
		}
	}
	return db
}

func applyOrGroup(db *gorm.DB, params *paginate.PaginationParams, resolver *schemaResolver, cfg *config) *gorm.DB {
	hasOr := len(params.LikeOr) > 0 || len(params.EqOr) > 0 ||
		len(params.GteOr) > 0 || len(params.GtOr) > 0 ||
		len(params.LteOr) > 0 || len(params.LtOr) > 0 ||
		len(params.InOr) > 0 || len(params.NotInOr) > 0 ||
		len(params.IsNullOr) > 0 || len(params.IsNotNullOr) > 0

	if !hasOr {
		return db
	}

	orDB := db.Session(&gorm.Session{NewDB: true})
	first := true

	addCond := func(expr string, args ...any) {
		if first {
			orDB = orDB.Where(expr, args...)
			first = false
		} else {
			orDB = orDB.Or(expr, args...)
		}
	}

	for field, values := range params.LikeOr {
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsFilterAllowed(field, cfg) {
			continue
		}
		for _, v := range values {
			addCond(likeExpr(db, f.DBName), "%"+v+"%")
		}
	}

	for field, values := range params.EqOr {
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsFilterAllowed(field, cfg) {
			continue
		}
		for _, v := range values {
			addCond(f.DBName+" = ?", v)
		}
	}

	applyComparisonMapOr(orDB, params.GteOr, resolver, cfg, ">=", &first)
	applyComparisonMapOr(orDB, params.GtOr, resolver, cfg, ">", &first)
	applyComparisonMapOr(orDB, params.LteOr, resolver, cfg, "<=", &first)
	applyComparisonMapOr(orDB, params.LtOr, resolver, cfg, "<", &first)

	for field, values := range params.InOr {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) && len(values) > 0 {
			addCond(f.DBName+" IN ?", values)
		}
	}
	for field, values := range params.NotInOr {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) && len(values) > 0 {
			addCond(f.DBName+" NOT IN ?", values)
		}
	}
	for _, field := range params.IsNullOr {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) {
			addCond(f.DBName + " IS NULL")
		}
	}
	for _, field := range params.IsNotNullOr {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) {
			addCond(f.DBName + " IS NOT NULL")
		}
	}

	if !first {
		db = db.Where(orDB)
	}
	return db
}

func applyComparisonMapOr(orDB *gorm.DB, m map[string]any, resolver *schemaResolver, cfg *config, op string, first *bool) {
	for field, v := range m {
		f := resolver.ResolveColumn(field)
		if f != nil && resolver.IsFilterAllowed(field, cfg) {
			if *first {
				orDB.Where(f.DBName+" "+op+" ?", v)
				*first = false
			} else {
				orDB.Or(f.DBName+" "+op+" ?", v)
			}
		}
	}
}

func applySearch(db *gorm.DB, term string, fields []string, resolver *schemaResolver, cfg *config) *gorm.DB {
	searchDB := db.Session(&gorm.Session{NewDB: true})
	first := true
	for _, field := range fields {
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsFilterAllowed(field, cfg) {
			continue
		}
		if first {
			searchDB = searchDB.Where(likeExpr(db, f.DBName), "%"+term+"%")
			first = false
		} else {
			searchDB = searchDB.Or(likeExpr(db, f.DBName), "%"+term+"%")
		}
	}
	if !first {
		db = db.Where(searchDB)
	}
	return db
}

// likeExpr returns the LIKE expression appropriate for the dialect.
func likeExpr(db *gorm.DB, col string) string {
	if db.Dialector != nil && db.Dialector.Name() == "postgres" {
		return col + " ILIKE ?"
	}
	return col + " LIKE ?"
}
