package plugin_test

import (
	"testing"
	"time"

	"github.com/goblogplatform/goblog/blog"
	"github.com/goblogplatform/goblog/plugin"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegistryPosts(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"))
	if err := db.AutoMigrate(&blog.Post{}, &blog.Tag{}, &blog.PostType{}, &blog.Setting{}); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	db.Create(&blog.Setting{Key: "site_url", Value: "https://example.com/"})
	db.Create(&blog.Post{Title: "Old", Slug: "old", Content: "# old", CreatedAt: day})
	db.Create(&blog.Post{Title: "New", Slug: "new", Content: "# new", CreatedAt: day.AddDate(0, 0, 1), Tags: []blog.Tag{{Name: "go"}}})
	db.Create(&blog.Post{Title: "Draft", Slug: "draft", Draft: true, CreatedAt: day.AddDate(0, 0, 2)})

	reg := plugin.NewRegistry(db)
	got, err := reg.Posts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "New" || got[1].Title != "Old" {
		t.Fatalf("want New, Old (no draft), got %+v", got)
	}
	if got[0].URL != "https://example.com/posts/2026/10/02/new" || got[0].Content != "# new" || len(got[0].Tags) != 1 || got[0].Tags[0] != "go" {
		t.Errorf("post = %+v", got[0])
	}
	if got, _ := reg.Posts(1); len(got) != 1 || got[0].Title != "New" {
		t.Errorf("limit 1 = %+v", got)
	}
	if _, err := plugin.NewRegistry(nil).Posts(1); err == nil {
		t.Error("no database should be an error")
	}
}
