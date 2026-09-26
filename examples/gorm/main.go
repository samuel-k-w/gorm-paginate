package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/samuel-k-w/gorm-paginate/v4/gormpaginate"
	"github.com/samuel-k-w/gorm-paginate/v4/paginate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

var db *gorm.DB

func main() {
	var err error
	db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.AutoMigrate(&User{})

	// Seed data
	for i := 1; i <= 25; i++ {
		role := "user"
		if i%5 == 0 {
			role = "admin"
		}
		db.Create(&User{
			Name:      fmt.Sprintf("User %d", i),
			Role:      role,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	http.HandleFunc("/users", listUsers)
	http.HandleFunc("/users/cursor", listUsersCursor)

	fmt.Println("Server running on http://localhost:8080")
	fmt.Println("Try: http://localhost:8080/users?page=1&limit=5&sort=-createdAt")
	fmt.Println("Try: http://localhost:8080/users?eq[role]=admin")
	
	http.ListenAndServe(":8080", nil)
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	params, err := paginate.BindQueryParamsToStruct(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// GORM integration automatically handles the query configuration
	page, err := gormpaginate.Paginate[User](db, params, r.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

func listUsersCursor(w http.ResponseWriter, r *http.Request) {
	params, err := paginate.BindQueryParamsToStruct(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// We can pass options like DefaultSort to ensure the cursor works well
	page, err := gormpaginate.CursorPaginate[User](db, params, r.URL,
		gormpaginate.DefaultSort("createdAt", "DESC"),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
