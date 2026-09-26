package paginate

import (
	"net/url"
	"testing"
	"time"
)

func TestNewPage_NilBaseURL(t *testing.T) {
	params := &PaginationParams{Page: 1, Limit: 10}
	page := NewPage([]string{"a", "b"}, 2, params, nil)
	if page.Meta.TotalItems != 2 {
		t.Fatalf("total: got %d", page.Meta.TotalItems)
	}
	if page.Links.Self != "" || page.Links.Next != nil {
		t.Fatalf("expected empty links, got %+v", page.Links)
	}
}

func TestNewCursorPage_NilBaseURL(t *testing.T) {
	params := &PaginationParams{Limit: 2, Sort: []string{"-created_at", "id"}}
	items := []cursorTestModel{
		{ID: 1, CreatedAt: time.Now()},
		{ID: 2, CreatedAt: time.Now()},
		{ID: 3, CreatedAt: time.Now()},
	}
	page := NewCursorPage(items, params, nil)
	if !page.Meta.HasNext {
		t.Fatal("expected has next")
	}
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page.Data))
	}
	if page.Links.Self != "" || page.Links.Next != nil {
		t.Fatalf("expected empty links with nil baseURL, got %+v", page.Links)
	}
}

func TestExtractFieldsByJSONTag_EmbeddedStruct(t *testing.T) {
	type Base struct {
		ID        int       `json:"id"`
		CreatedAt time.Time `json:"created_at"`
	}
	type Row struct {
		Base
		Name string `json:"name"`
	}
	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	row := Row{Base: Base{ID: 42, CreatedAt: ts}, Name: "x"}
	vals := extractFieldsByJSONTag(row, []string{"created_at", "id"})
	if len(vals) != 2 {
		t.Fatalf("len=%d", len(vals))
	}
	if !vals[0].(time.Time).Equal(ts) {
		t.Fatalf("created_at: got %#v", vals[0])
	}
	if vals[1].(int) != 42 {
		t.Fatalf("id: got %#v", vals[1])
	}

	// With baseURL, links should encode non-null vals
	u, _ := url.Parse("http://localhost/r")
	params := &PaginationParams{Limit: 1, Sort: []string{"-created_at", "id"}}
	page := NewCursorPage([]Row{row, row}, params, u)
	if page.Links.Next == nil {
		t.Fatal("expected next")
	}
	if !contains(*page.Links.Next, "cursor=") {
		t.Fatalf("expected cursor in next link: %s", *page.Links.Next)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
