package gormpaginate

import (
	"strings"

	"github.com/samuel-k-w/gorm-paginate/v4/paginate"
	"gorm.io/gorm"
)

func applySorting(db *gorm.DB, params *paginate.PaginationParams, resolver *schemaResolver, cfg *config) *gorm.DB {
	var sortedColumns []string
	sortApplied := false

	// Modern sort pattern: sort=[-]field
	for _, s := range params.Sort {
		dir := "ASC"
		field := s
		if strings.HasPrefix(s, "-") {
			dir = "DESC"
			field = strings.TrimPrefix(s, "-")
		}
		f := resolver.ResolveColumn(field)
		if f == nil || !resolver.IsSortAllowed(field, cfg) {
			continue
		}
		db = applyOrder(db, f.DBName, dir)
		sortedColumns = append(sortedColumns, f.DBName)
		sortApplied = true
	}

	// Legacy sort pattern
	if !sortApplied {
		for i, col := range params.SortColumns {
			f := resolver.ResolveColumn(col)
			if f == nil || !resolver.IsSortAllowed(col, cfg) {
				continue
			}
			dir := "ASC"
			if i < len(params.SortDirections) && strings.ToUpper(params.SortDirections[i]) == "DESC" {
				dir = "DESC"
			}
			db = applyOrder(db, f.DBName, dir)
			sortedColumns = append(sortedColumns, f.DBName)
			sortApplied = true
		}
	}

	// Default sort — mutate params so NewCursorPage encodes the same columns as ORDER BY.
	if !sortApplied && cfg.defaultSort != nil {
		f := resolver.ResolveColumn(cfg.defaultSort.column)
		if f != nil {
			dir := strings.ToUpper(cfg.defaultSort.direction)
			db = applyOrder(db, f.DBName, dir)
			sortedColumns = append(sortedColumns, f.DBName)
			sortApplied = true

			jsonKey := cfg.defaultSort.column
			for _, tag := range strings.Split(f.Tag.Get("json"), ",") {
				if tag != "" && tag != "omitempty" {
					jsonKey = tag
					break
				}
			}
			if dir == "DESC" {
				params.Sort = append(params.Sort, "-"+jsonKey)
			} else {
				params.Sort = append(params.Sort, jsonKey)
			}
		}
	}

	// For deterministic keyset pagination, we MUST append the primary key as a tie-breaker
	// if it is not already present in the sort list.
	pkField := resolver.PrimaryKey()
	if pkField != nil {
		hasPK := false
		for _, col := range sortedColumns {
			if col == pkField.DBName {
				hasPK = true
				break
			}
		}
		if !hasPK {
			// Find the JSON key for the PK field to append to params
			pkJSONKey := pkField.Name
			for _, tag := range strings.Split(pkField.Tag.Get("json"), ",") {
				if tag != "" && tag != "omitempty" {
					pkJSONKey = tag
					break
				}
			}
			
			db = db.Order(pkField.DBName + " ASC")
			
			// MUST mutate params so NewCursorPage encodes the tie-breaker in the next/prev links!
			if len(params.Sort) > 0 {
				params.Sort = append(params.Sort, pkJSONKey)
			} else {
				params.SortColumns = append(params.SortColumns, pkJSONKey)
				params.SortDirections = append(params.SortDirections, "ASC")
			}
		}
	}

	return db
}

func applyOrder(db *gorm.DB, col string, dir string) *gorm.DB {
	// Standard handling for most SQL dialects
	dialect := ""
	if db.Dialector != nil {
		dialect = db.Dialector.Name()
	}

	// Dialect-specific NULL sorting
	// For Postgres/SQLite, NULLS LAST is native syntax. For MySQL, it requires IS NULL prefix.
	if dir == "DESC" {
		if dialect == "mysql" {
			// MySQL DESC defaults to NULLS LAST implicitly, but if we want strictly NULLS FIRST, we'd do `col IS NOT NULL, col DESC`
			// We will just let MySQL do its default.
		}
	}

	return db.Order(col + " " + dir)
}
