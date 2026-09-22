package gormpaginate

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/booscaaa/go-paginate/v4/paginate"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func applyCursor(db *gorm.DB, params *paginate.PaginationParams, resolver *schemaResolver) *gorm.DB {
	if params.Cursor == "" {
		return db
	}

	ct, err := decodeCursorToken(params.Cursor)
	if err != nil {
		return db
	}

	// Single-column fallback (legacy)
	if len(ct.Columns) == 0 && ct.Column != "" {
		ct.Columns = []string{ct.Column}
		ct.Values = []any{ct.Value}
	}

	if len(ct.Columns) == 0 || len(ct.Values) == 0 {
		return db
	}

	return applyMultiColumnCursor(db, ct, resolver)
}

func applyMultiColumnCursor(db *gorm.DB, ct *cursorToken, resolver *schemaResolver) *gorm.DB {
	orDB := db.Session(&gorm.Session{NewDB: true})
	first := true

	// Tie-breaker PK should be included in keyset if it was appended during sorting
	// But `ct.Columns` comes from the client token. We rely on what's in the token.
	for i := range ct.Columns {
		colName := ct.Columns[i]
		f := resolver.ResolveColumn(colName)
		if f == nil {
			continue // skip invalid columns
		}

		sortDir := "ASC"
		if i < len(ct.SortDirs) {
			sortDir = strings.ToUpper(ct.SortDirs[i])
		}
		op := cursorCompareOp(ct.Direction, sortDir)

		var parts []string
		var args []any

		// Equality prefix for preceding columns
		valid := true
		for j := 0; j < i; j++ {
			prevField := resolver.ResolveColumn(ct.Columns[j])
			if prevField == nil {
				valid = false
				break
			}
			parts = append(parts, prevField.DBName+" = ?")
			val := coerceType(ct.Values[j], prevField)
			args = append(args, val)
		}
		if !valid {
			continue
		}

		// Comparison for the current column
		parts = append(parts, f.DBName+" "+op+" ?")
		val := coerceType(ct.Values[i], f)
		args = append(args, val)

		clause := strings.Join(parts, " AND ")
		if first {
			orDB = orDB.Where(clause, args...)
			first = false
		} else {
			orDB = orDB.Or(clause, args...)
		}
	}

	if !first {
		db = db.Where(orDB)
	}
	return db
}

func cursorCompareOp(paginDir, sortDir string) string {
	if paginDir == "before" {
		if sortDir == "DESC" {
			return ">"
		}
		return "<"
	}
	if sortDir == "DESC" {
		return "<"
	}
	return ">"
}

// coerceType attempts to restore the concrete type of a value decoded from JSON,
// based on the destination GORM schema field type. JSON unmarshals numbers as float64
// and dates as strings, which can cause SQL parameter binding errors.
func coerceType(val any, field *schema.Field) any {
	if val == nil {
		return nil
	}

	// Float64 (from JSON number) -> integer types
	if f, ok := val.(float64); ok {
		switch field.FieldType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(f)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return uint64(f)
		}
	}

	// String (from JSON string) -> time.Time
	if s, ok := val.(string); ok {
		if field.FieldType == reflect.TypeOf(time.Time{}) {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return t
			}
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t
			}
		}
		// String -> UUID (handled natively by database/sql if passed as string in most drivers,
		// or explicitly cast if needed, but standard gorm handles string UUIDs).
	}

	return val
}

type cursorToken struct {
	Columns   []string `json:"cols,omitempty"`
	Values    []any    `json:"vals,omitempty"`
	SortDirs  []string `json:"dirs,omitempty"`
	Column    string   `json:"col,omitempty"`
	Value     any      `json:"val,omitempty"`
	Direction string   `json:"dir"`
}

func decodeCursorToken(token string) (*cursorToken, error) {
	b, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor token: %w", err)
	}
	var ct cursorToken
	if err := json.Unmarshal(b, &ct); err != nil {
		return nil, fmt.Errorf("invalid cursor token: %w", err)
	}
	return &ct, nil
}
