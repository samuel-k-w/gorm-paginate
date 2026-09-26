package gormpaginate

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/samuel-k-w/gorm-paginate/v4/paginate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Age       int       `json:"age"`
	Status    string    `json:"status"`
	CompanyID uint      `json:"companyId"`
	Company   Company   `json:"company,omitempty" gorm:"foreignKey:CompanyID"`
	CreatedAt time.Time `json:"createdAt"`
}

type Company struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `json:"name"`
}

func setupDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	err = db.AutoMigrate(&Company{}, &User{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func seedData(db *gorm.DB) {
	c1 := Company{Name: "Acme Corp"}
	c2 := Company{Name: "Globex"}
	db.Create(&c1)
	db.Create(&c2)

	now := time.Now().Round(time.Second)

	users := []User{
		{Name: "Alice", Email: "alice@test.com", Age: 30, Status: "active", CompanyID: c1.ID, CreatedAt: now.Add(-time.Hour)},
		{Name: "Bob", Email: "bob@test.com", Age: 25, Status: "inactive", CompanyID: c1.ID, CreatedAt: now.Add(-2 * time.Hour)},
		{Name: "Charlie", Email: "charlie@test.com", Age: 35, Status: "active", CompanyID: c2.ID, CreatedAt: now.Add(-3 * time.Hour)},
		{Name: "Dave", Email: "dave@test.com", Age: 40, Status: "active", CompanyID: c2.ID, CreatedAt: now.Add(-4 * time.Hour)},
		{Name: "Eve", Email: "eve@test.com", Age: 28, Status: "inactive", CompanyID: c1.ID, CreatedAt: now.Add(-5 * time.Hour)},
	}
	db.Create(&users)
}

func TestPaginate_Basic(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{
		Page:  1,
		Limit: 2,
		Sort:  []string{"-age"},
	}
	u, _ := url.Parse("http://localhost/users")

	page, err := Paginate[User](db, params, u)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if page.Meta.TotalItems != 5 {
		t.Errorf("expected 5 total items, got %d", page.Meta.TotalItems)
	}
	if len(page.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(page.Data))
	}
	// Dave (40), Charlie (35)
	if page.Data[0].Name != "Dave" {
		t.Errorf("expected Dave, got %s", page.Data[0].Name)
	}
}

func TestCursorPaginate_Basic(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{
		Limit: 2,
		Sort:  []string{"-age"},
	}
	u, _ := url.Parse("http://localhost/users")

	page1, err := CursorPaginate[User](db, params, u)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(page1.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(page1.Data))
	}
	if !page1.Meta.HasNext {
		t.Errorf("expected HasNext true")
	}

	// Fetch next page using the cursor
	nextURL, _ := url.Parse(*page1.Links.Next)
	nextParams, _ := paginate.BindQueryStringToStruct(nextURL.RawQuery)
	nextParams.Limit = 2

	page2, err := CursorPaginate[User](db, nextParams, u)
	if err != nil {
		t.Fatalf("expected no error on page 2, got %v", err)
	}
	if len(page2.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(page2.Data))
	}
}

func TestPaginate_Transaction(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{
		Page:  1,
		Limit: 10,
	}
	u, _ := url.Parse("http://localhost/users")

	// Create a new user inside transaction, then paginate inside same transaction
	err := db.Transaction(func(tx *gorm.DB) error {
		tx.Create(&User{Name: "TxUser", Age: 99})

		page, err := Paginate[User](tx, params, u)
		if err != nil {
			return err
		}

		found := false
		for _, v := range page.Data {
			if v.Name == "TxUser" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find TxUser within transaction")
		}
		// Rollback explicitly to test
		return fmt.Errorf("rollback")
	})

	if err == nil || err.Error() != "rollback" {
		t.Fatalf("expected rollback error")
	}

	// Verify outside transaction
	page, _ := Paginate[User](db, params, u)
	for _, v := range page.Data {
		if v.Name == "TxUser" {
			t.Errorf("did not expect to find TxUser outside transaction")
		}
	}
}

func TestPaginate_ContextCancellation(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{
		Page:  1,
		Limit: 10,
	}
	u, _ := url.Parse("http://localhost/users")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := Paginate[User](db.WithContext(ctx), params, u)
	if err == nil {
		t.Fatalf("expected context canceled error, got nil")
	}
}

func TestPaginate_Preload(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{
		Page:  1,
		Limit: 2,
	}
	u, _ := url.Parse("http://localhost/users")

	// Chaining Preload before Paginate
	page, err := Paginate[User](db.Preload("Company"), params, u)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(page.Data) == 0 {
		t.Fatalf("expected data")
	}
	if page.Data[0].Company.Name == "" {
		fmt.Printf("Data: %+v\n", page.Data[0])
		t.Errorf("expected Company to be preloaded")
	}
}

func TestPaginate_Allowlist(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{
		Page:  1,
		Limit: 10,
		Eq: map[string][]any{
			"status": {"active"},
		},
		Sort: []string{"-age"},
	}
	u, _ := url.Parse("http://localhost/users")

	// Only allow name filtering, NOT status or age
	page, err := Paginate[User](db, params, u, AllowFilters("name"), AllowSorts("name"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Because status filtering is blocked, all 5 items should be returned, not just active ones
	if page.Meta.TotalItems != 5 {
		t.Errorf("expected 5 items (filter blocked), got %d", page.Meta.TotalItems)
	}
}
