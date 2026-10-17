package tools_test

import (
	"testing"
	"time"

	"github.com/goblogplatform/goblog/blog"
	"github.com/goblogplatform/goblog/tools"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestMigrationSeedsBacklinks: posts that predate the backlinks table get
// their internal links indexed once, skipping self links, duplicates and
// links to posts that don't exist.
func TestMigrationSeedsBacklinks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	day := time.Date(2024, 3, 5, 12, 0, 0, 0, time.UTC)
	target := blog.Post{Title: "Target", Slug: "target", CreatedAt: day}
	db.Create(&target)
	source := blog.Post{Title: "Source", Slug: "source", CreatedAt: day, Content: "[a](/posts/2024/03/05/target) " +
		"[again](/2024/3/5/target) [self](/posts/2024/03/05/source) [gone](/posts/2024/03/05/missing)"}
	db.Create(&source)

	if err := tools.Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var links []blog.Backlink
	db.Find(&links)
	if len(links) != 1 || links[0].SourcePostID != source.ID || links[0].TargetPostID != target.ID {
		t.Fatalf("expected one source->target backlink, got %+v", links)
	}
}
