package gormpaginate

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/samuel-k-w/gorm-paginate/v4/paginate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type AuditUser struct {
	ID        string         `gorm:"primaryKey;type:uuid" json:"id"`
	Name      string         `gorm:"column:full_name" json:"name"`
	Age       int            `json:"age"`
	DeletedAt gorm.DeletedAt `json:"-"`
	Roles     []AuditRole    `gorm:"many2many:audit_user_roles;" json:"roles"`
}

type AuditRole struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `json:"name"`
}

func setupAuditDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: "t_",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	db = db.Debug()
	db.AutoMigrate(&AuditUser{}, &AuditRole{})
	return db
}

func TestAudit_CallerState(t *testing.T) {
	db := setupAuditDB(t)
	db.Create(&AuditUser{ID: "uuid-1", Name: "Alice", Age: 30})
	db.Create(&AuditUser{ID: "uuid-2", Name: "Bob", Age: 20})

	u, _ := url.Parse("http://localhost")
	params := &paginate.PaginationParams{Limit: 10}

	query := db.Unscoped().Select("id", "full_name").Where("age > ?", 10)
	page, err := Paginate[AuditUser](query, params, u)
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.TotalItems != 2 {
		t.Errorf("Expected 2 items, got %d", page.Meta.TotalItems)
	}
	if page.Data[0].Age != 0 {
		t.Errorf("Age should be 0 because it was not Selected")
	}
}

func TestAudit_SecuritySQLInjection(t *testing.T) {
	db := setupAuditDB(t)
	u, _ := url.Parse("http://localhost")

	params := &paginate.PaginationParams{
		Limit: 10,
		Eq: map[string][]any{
			"age = 1; DROP TABLE users; --": {1},
		},
		Sort: []string{"full_name; DELETE FROM users;"},
	}

	page, err := Paginate[AuditUser](db, params, u)
	if err != nil {
		t.Fatal(err)
	}
	_ = page
}

func TestAudit_CountJoins(t *testing.T) {
	db := setupAuditDB(t)
	r1 := AuditRole{Name: "Admin"}
	r2 := AuditRole{Name: "User"}
	db.Create(&r1)
	db.Create(&r2)
	db.Create(&AuditUser{ID: "uuid-1", Name: "Alice", Roles: []AuditRole{r1, r2}})

	u, _ := url.Parse("http://localhost")
	params := &paginate.PaginationParams{Limit: 10}

	query := db.Model(&AuditUser{}).Joins("JOIN t_audit_user_roles ON t_audit_user_roles.audit_user_id = t_audit_users.id")

	pageNoDistinct, _ := Paginate[AuditUser](query, params, u)
	if pageNoDistinct.Meta.TotalItems != 2 {
		t.Errorf("Expected 2 duplicate rows, got %d", pageNoDistinct.Meta.TotalItems)
	}

	queryDistinct := query.Distinct("t_audit_users.id")
	pageDistinct, _ := Paginate[AuditUser](queryDistinct, params, u)
	if pageDistinct.Meta.TotalItems != 1 {
		t.Errorf("Expected 1 distinct row, got %d", pageDistinct.Meta.TotalItems)
	}
}

func TestAudit_CursorCorrectness(t *testing.T) {
	db := setupAuditDB(t)

	db.Create(&AuditUser{ID: "uuid-3", Name: "Charlie", Age: 30})
	db.Create(&AuditUser{ID: "uuid-1", Name: "Alice", Age: 30})
	db.Create(&AuditUser{ID: "uuid-2", Name: "Bob", Age: 30})

	u, _ := url.Parse("http://localhost")

	params1 := &paginate.PaginationParams{
		Limit: 2,
		Sort:  []string{"age"},
	}
	page1, err := CursorPaginate[AuditUser](db, params1, u)
	if err != nil {
		t.Fatal(err)
	}

	if len(page1.Data) != 2 {
		t.Fatalf("Expected 2 items on page 1, got %d", len(page1.Data))
	}

	nextURL, _ := url.Parse(*page1.Links.Next)
	params2, _ := paginate.BindQueryStringToStruct(nextURL.RawQuery)
	params2.Limit = 2

	page2, err := CursorPaginate[AuditUser](db, params2, u)
	if err != nil {
		t.Fatal(err)
	}

	if len(page2.Data) != 1 {
		t.Fatalf("Expected 1 item on page 2, got %d", len(page2.Data))
	}
}

func TestAudit_Context(t *testing.T) {
	db := setupAuditDB(t)
	db.Create(&AuditUser{ID: "uuid-1", Name: "Alice"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // instantly cancel

	u, _ := url.Parse("http://localhost")
	params := &paginate.PaginationParams{Limit: 10}

	// Paginate should fail immediately
	_, err := Paginate[AuditUser](db.WithContext(ctx), params, u)
	if err == nil {
		t.Fatal("Expected context canceled error")
	}
}

func TestAudit_Transactions(t *testing.T) {
	db := setupAuditDB(t)
	u, _ := url.Parse("http://localhost")
	params := &paginate.PaginationParams{Limit: 10}

	// 1. Using db.Transaction
	err := db.Transaction(func(tx *gorm.DB) error {
		tx.Create(&AuditUser{ID: "tx-1"})
		page, _ := Paginate[AuditUser](tx, params, u)
		if page.Meta.TotalItems != 1 {
			t.Errorf("Expected 1 in transaction, got %d", page.Meta.TotalItems)
		}
		return fmt.Errorf("rollback")
	})

	if err.Error() != "rollback" {
		t.Errorf("Expected rollback")
	}

	// Verify rollback
	page, _ := Paginate[AuditUser](db, params, u)
	if page.Meta.TotalItems != 0 {
		t.Errorf("Expected 0 after rollback, got %d", page.Meta.TotalItems)
	}

	// 2. Using tx := db.Begin()
	tx := db.Begin()
	tx.Create(&AuditUser{ID: "tx-2"})
	page2, _ := Paginate[AuditUser](tx, params, u)
	if page2.Meta.TotalItems != 1 {
		t.Errorf("Expected 1 in tx.Begin(), got %d", page2.Meta.TotalItems)
	}
	tx.Rollback()
}

func TestAudit_NestedPreload(t *testing.T) {
	db := setupAuditDB(t)
	r := AuditRole{Name: "Admin"}
	db.Create(&r)
	db.Create(&AuditUser{ID: "uuid-1", Roles: []AuditRole{r}})

	u, _ := url.Parse("http://localhost")
	params := &paginate.PaginationParams{Limit: 10}

	page, err := Paginate[AuditUser](db.Preload("Roles"), params, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data[0].Roles) == 0 {
		t.Fatal("Expected preloaded roles")
	}
}

func TestAudit_InvalidInput(t *testing.T) {
	db := setupAuditDB(t)
	u, _ := url.Parse("http://localhost")

	// String instead of int for Age
	params := &paginate.PaginationParams{
		Limit: 10,
		Eq: map[string][]any{
			"age": {"invalid_int"},
		},
	}

	page, err := Paginate[AuditUser](db, params, u)
	// Currently we coerceType on Cursor values, but what about Eq/Like/etc?
	// In applyFilters, we don't coerceType on user filters, we pass them directly.
	// GORM's parameterization handles it or passes to SQL driver, which may error.
	_ = page
	_ = err
}

func TestAudit_CountGroup(t *testing.T) {
	db := setupAuditDB(t)
	db.Create(&AuditUser{ID: "u1", Age: 30})
	db.Create(&AuditUser{ID: "u2", Age: 30})
	db.Create(&AuditUser{ID: "u3", Age: 40})

	u, _ := url.Parse("http://localhost")
	params := &paginate.PaginationParams{Limit: 10}

	query := db.Model(&AuditUser{}).Select("age").Group("age")
	page, err := Paginate[AuditUser](query, params, u)
	if err != nil {
		t.Fatal(err)
	}

	// Group by age means 2 distinct ages (30 and 40)
	if page.Meta.TotalItems != 2 {
		t.Errorf("Expected 2 groups, got %d", page.Meta.TotalItems)
	}
}
