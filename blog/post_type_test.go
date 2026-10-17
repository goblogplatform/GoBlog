package blog_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goblogplatform/goblog/auth"
	"github.com/goblogplatform/goblog/blog"
	"github.com/goblogplatform/goblog/theme"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// postTypeFixture seeds a "notes" post type with one published and one draft
// note, and serves NoRoute with the default theme as the given visitor.
func postTypeFixture(t *testing.T, user *auth.BlogUser, isAdmin bool) (*gin.Engine, *blog.Blog, *gorm.DB, blog.Post, blog.Post) {
	t.Helper()
	db, _ := gorm.Open(sqlite.Open(":memory:"))
	db.AutoMigrate(&auth.BlogUser{}, &blog.PostType{}, &blog.Post{}, &blog.Tag{}, &blog.Comment{}, &blog.Setting{}, &blog.Page{}, &blog.Backlink{}, &blog.ExternalBacklink{})
	notes := blog.PostType{Name: "Notes", Slug: "notes"}
	db.Create(&notes)
	published := blog.Post{Title: "Published note", Slug: "published-note", Content: "hi", PostTypeID: notes.ID}
	draft := blog.Post{Title: "Draft note", Slug: "draft-note", Content: "wip", PostTypeID: notes.ID, Draft: true}
	db.Create(&published)
	db.Create(&draft)
	published.PostType = notes

	a := &Auth{user: user}
	a.On("IsAdmin", mock.Anything).Return(isAdmin)
	a.On("IsLoggedIn", mock.Anything).Return(user != nil)
	b := blog.New(db, a, "test")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	tmpl := template.Must(template.New("").Funcs(theme.FuncMap()).ParseGlob("../templates/shared/*.html"))
	template.Must(tmpl.ParseGlob("../themes/default/templates/*.html"))
	router.SetHTMLTemplate(tmpl)
	router.NoRoute(b.NoRoute)
	return router, &b, db, published, draft
}

func serve(r http.Handler, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestPostTypeListing: /{type-slug} lists that type's published posts only.
func TestPostTypeListing(t *testing.T) {
	router, b, _, _, _ := postTypeFixture(t, nil, false)
	w := serve(router, "/notes")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Published note") || strings.Contains(body, "Draft note") {
		t.Fatalf("listing should show published notes only:\n%s", body)
	}
	if got := len(b.GetPostsByType(1, true)); got != 2 {
		t.Errorf("GetPostsByType with drafts: expected 2, got %d", got)
	}
}

// TestAdminTypedPostURL covers /admin/{type}/{yyyy}/{mm}/{dd}/{slug} for each
// kind of visitor RequireAdminPage distinguishes.
func TestAdminTypedPostURL(t *testing.T) {
	t.Run("admin gets the edit view", func(t *testing.T) {
		router, _, _, post, _ := postTypeFixture(t, &auth.BlogUser{Name: "Admin"}, true)
		w := serve(router, post.Adminlink())
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Published note") {
			t.Fatalf("expected the admin post view, got %d", w.Code)
		}
	})
	t.Run("anonymous is sent to login and back", func(t *testing.T) {
		router, _, _, post, _ := postTypeFixture(t, nil, false)
		w := serve(router, post.Adminlink())
		if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/login?next=%2Fadmin%2Fnotes%2F") {
			t.Fatalf("expected a redirect to login, got %d %q", w.Code, w.Header().Get("Location"))
		}
	})
	t.Run("signed-in non-admin gets 403 naming them", func(t *testing.T) {
		router, _, _, post, _ := postTypeFixture(t, &auth.BlogUser{Name: "<Mallory>"}, false)
		w := serve(router, post.Adminlink())
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "&lt;Mallory&gt;") {
			t.Fatalf("expected 403 with the escaped name, got %d:\n%s", w.Code, w.Body.String())
		}
	})
}

func TestRecentCommentsAndPostsByIDs(t *testing.T) {
	_, b, db, post, draft := postTypeFixture(t, nil, false)
	db.Create(&blog.Comment{PostID: post.ID, Content: "first"})
	db.Create(&blog.Comment{PostID: draft.ID, Content: "second"})

	if got := b.GetRecentComments(1); len(got) != 1 {
		t.Fatalf("expected the limit to apply, got %d comments", len(got))
	}
	if got := b.GetPostsByIDs(nil); len(got) != 0 {
		t.Errorf("no ids should give an empty map, got %v", got)
	}
	got := b.GetPostsByIDs([]uint{post.ID, draft.ID, 999})
	if len(got) != 2 || got[post.ID].PostType.Slug != "notes" {
		t.Fatalf("expected both posts with their type, got %+v", got)
	}
}
