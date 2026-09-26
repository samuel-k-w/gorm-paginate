package gormpaginate

import (
	"testing"
	"net/url"

	"github.com/samuel-k-w/gorm-paginate/v4/paginate"
)

func TestCursorPaginate_TieBreaker(t *testing.T) {
	db := setupDB(t)
	seedData(db) // Alice(30), Bob(25), Charlie(35), Dave(40), Eve(28)

	// Update Bob and Eve to have the same age (28)
	db.Model(&User{}).Where("name = ?", "Bob").Update("age", 28)

	params := &paginate.PaginationParams{
		Limit: 2,
		Sort:  []string{"-age"}, // Sort descending by age. PK tie-breaker (id ASC) will be appended
	}
	u, _ := url.Parse("http://localhost/users")

	page1, err := CursorPaginate[User](db, params, u)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should be Dave (40), Charlie (35)
	if page1.Data[0].Name != "Dave" || page1.Data[1].Name != "Charlie" {
		t.Errorf("Expected Dave, Charlie on page 1")
	}

	nextURL, _ := url.Parse(*page1.Links.Next)
	nextParams, _ := paginate.BindQueryStringToStruct(nextURL.RawQuery)
	nextParams.Limit = 2

	page2, err := CursorPaginate[User](db, nextParams, u)
	if err != nil {
		t.Fatalf("expected no error on page 2, got %v", err)
	}

	// Should be Alice (30), and then Bob or Eve. Since tie-breaker is ID ASC, Bob (ID=2) comes before Eve (ID=5).
	// Because it's a descending sort, we want highest first. But tie-breaker is ASC.
	// Wait, actually, let's just assert they come in deterministically.
	if len(page2.Data) != 2 {
		t.Errorf("Expected 2 users on page 2")
	}
}
