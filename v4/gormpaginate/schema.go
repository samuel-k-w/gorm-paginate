package gormpaginate

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// schemaResolver helps map JSON fields/tags to GORM schema fields.
type schemaResolver struct {
	db     *gorm.DB
	schema *schema.Schema
}

// newSchemaResolver creates a new resolver by parsing the model metadata using GORM.
func newSchemaResolver(db *gorm.DB, model any) (*schemaResolver, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return nil, err
	}
	return &schemaResolver{
		db:     db,
		schema: stmt.Schema,
	}, nil
}

// ResolveColumn maps a JSON/query parameter key to a GORM schema field.
// It checks the `json` tag, then falls back to direct field name lookup.
func (r *schemaResolver) ResolveColumn(jsonKey string) *schema.Field {
	if r.schema == nil {
		return nil
	}

	// 1. Try to find the field by checking JSON tags directly (like v4/paginate does)
	for _, field := range r.schema.Fields {
		tag := field.Tag.Get("json")
		tagValue := strings.Split(tag, ",")[0]
		if tagValue == jsonKey {
			return field
		}
	}

	// 2. Fallback: try by looking up field name directly in GORM schema
	if field := r.schema.LookUpField(jsonKey); field != nil {
		return field
	}

	// 3. Fallback for explicit `paginate:"column_name"` legacy support
	for _, field := range r.schema.Fields {
		paginateTag := field.Tag.Get("paginate")
		if paginateTag == jsonKey {
			return field
		}
	}

	return nil
}

// IsFilterAllowed checks if the column is allowed by the config or schema.
func (r *schemaResolver) IsFilterAllowed(jsonKey string, cfg *config) bool {
	if len(cfg.allowFilters) > 0 {
		allowed := false
		for _, c := range cfg.allowFilters {
			if c == jsonKey {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	// Verify it exists in schema
	return r.ResolveColumn(jsonKey) != nil
}

// IsSortAllowed checks if the column is allowed by the config or schema for sorting.
func (r *schemaResolver) IsSortAllowed(jsonKey string, cfg *config) bool {
	if len(cfg.allowSorts) > 0 {
		allowed := false
		for _, c := range cfg.allowSorts {
			if c == jsonKey {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return r.ResolveColumn(jsonKey) != nil
}

// PrimaryKey returns the first primary key field of the schema, if any.
func (r *schemaResolver) PrimaryKey() *schema.Field {
	if r.schema == nil || len(r.schema.PrimaryFields) == 0 {
		return nil
	}
	return r.schema.PrimaryFields[0]
}
