module gorm-example

go 1.25.5

replace github.com/booscaaa/go-paginate/v4 => ../../v4

replace github.com/booscaaa/go-paginate/v4/gormpaginate => ../../v4/gormpaginate

require (
	github.com/booscaaa/go-paginate/v4 v4.0.1 // indirect
	github.com/booscaaa/go-paginate/v4/gormpaginate v0.0.0-00010101000000-000000000000 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.15 // indirect
	gorm.io/driver/sqlite v1.5.0 // indirect
	gorm.io/gorm v1.25.0 // indirect
)
