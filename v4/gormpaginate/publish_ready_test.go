package gormpaginate

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/samuel-k-w/gorm-paginate/v4/paginate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDefaultSort_CursorEncodesSortColumn(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{Limit: 2}
	u, _ := url.Parse("http://localhost/users")

	page, err := CursorPaginate[User](db, params, u, DefaultSort("createdAt", "DESC"))
	if err != nil {
		t.Fatalf("CursorPaginate: %v", err)
	}
	if !page.Meta.HasNext || page.Links.Next == nil {
		t.Fatal("expected next link")
	}

	token := u.Query().Get("cursor")
	nextURL, err := url.Parse(*page.Links.Next)
	if err != nil {
		t.Fatal(err)
	}
	token = nextURL.Query().Get("cursor")
	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		// try with padding
		raw, err = base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("decode cursor: %v", err)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	cols, _ := payload["cols"].([]any)
	if len(cols) < 1 || cols[0] != "createdAt" {
		t.Fatalf("expected createdAt in cursor cols, got %#v", payload["cols"])
	}
	vals, _ := payload["vals"].([]any)
	if len(vals) < 1 || vals[0] == nil {
		t.Fatalf("expected non-nil createdAt value in cursor, got %#v", payload["vals"])
	}

	// Follow next page — must return remaining rows without error
	params2 := &paginate.PaginationParams{Limit: 2, Cursor: token}
	page2, err := CursorPaginate[User](db, params2, u, DefaultSort("createdAt", "DESC"))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(page2.Data) == 0 {
		t.Fatal("expected data on second cursor page")
	}
}

func TestSoftDelete_DefaultExcludesDeleted(t *testing.T) {
	type SoftUser struct {
		ID        uint           `gorm:"primaryKey" json:"id"`
		Name      string         `json:"name"`
		DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&SoftUser{}); err != nil {
		t.Fatal(err)
	}
	u1 := SoftUser{Name: "alive"}
	u2 := SoftUser{Name: "gone"}
	db.Create(&u1)
	db.Create(&u2)
	db.Delete(&u2)

	params := &paginate.PaginationParams{Page: 1, Limit: 10}
	page, err := Paginate[SoftUser](db, params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.TotalItems != 1 {
		t.Fatalf("expected 1 non-deleted, got %d", page.Meta.TotalItems)
	}

	pageAll, err := Paginate[SoftUser](db.Unscoped(), params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pageAll.Meta.TotalItems != 2 {
		t.Fatalf("expected 2 with Unscoped, got %d", pageAll.Meta.TotalItems)
	}
}

func TestNilBaseURL_NoPanic(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{Page: 1, Limit: 2, Sort: []string{"-age"}}
	page, err := Paginate[User](db, params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.TotalItems != 5 {
		t.Fatalf("expected 5, got %d", page.Meta.TotalItems)
	}
	if page.Links.Self != "" {
		t.Fatalf("expected empty self link with nil baseURL, got %q", page.Links.Self)
	}

	cpage, err := CursorPaginate[User](db, &paginate.PaginationParams{Limit: 2}, nil, DefaultSort("id", "ASC"))
	if err != nil {
		t.Fatal(err)
	}
	if cpage.Links.Self != "" || cpage.Links.Next != nil {
		t.Fatalf("expected empty cursor links with nil baseURL, got %+v", cpage.Links)
	}
}

func TestGteOr_AppliesFilter(t *testing.T) {
	db := setupDB(t)
	seedData(db)

	params := &paginate.PaginationParams{
		Page:  1,
		Limit: 20,
		GteOr: map[string]any{"age": 35},
	}
	page, err := Paginate[User](db, params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.TotalItems < 2 {
		t.Fatalf("expected at least Charlie(35) and Dave(40), got total=%d data=%v", page.Meta.TotalItems, page.Data)
	}
	for _, u := range page.Data {
		if u.Age < 35 {
			t.Fatalf("GteOr age>=35 leaked row age=%d name=%s", u.Age, u.Name)
		}
	}
}

func TestEmbeddedBase_CursorExtractsID(t *testing.T) {
	type Base struct {
		ID        uint      `gorm:"primaryKey" json:"id"`
		CreatedAt time.Time `json:"created_at"`
	}
	type EmbedUser struct {
		Base
		Name string `json:"name"`
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&EmbedUser{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Round(time.Second)
	for i, name := range []string{"a", "b", "c"} {
		db.Create(&EmbedUser{Base: Base{CreatedAt: now.Add(-time.Duration(i) * time.Hour)}, Name: name})
	}

	u, _ := url.Parse("http://localhost/embed")
	params := &paginate.PaginationParams{Limit: 2}
	page, err := CursorPaginate[EmbedUser](db, params, u, DefaultSort("created_at", "DESC"))
	if err != nil {
		t.Fatal(err)
	}
	if page.Links.Next == nil {
		t.Fatal("expected next link")
	}
	nextURL, _ := url.Parse(*page.Links.Next)
	token := nextURL.Query().Get("cursor")
	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatal(err)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	vals, _ := payload["vals"].([]any)
	if len(vals) < 2 || vals[1] == nil {
		t.Fatalf("expected embedded id in cursor vals, got %#v", payload)
	}
}
