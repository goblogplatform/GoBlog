package admin_test

import (
	"html/template"
	"net/http"
	"testing"

	"github.com/goblogplatform/goblog/admin"
	"github.com/goblogplatform/goblog/auth"
	"github.com/goblogplatform/goblog/blog"
	"github.com/goblogplatform/goblog/theme"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// crudAPI serves the page and post-type endpoints over a db holding post
// types "posts" (1, with a post) and "notes" (2, empty), and pages "about"
// (1) and "contact" (2).
func crudAPI(t *testing.T, isAdmin bool) *gin.Engine {
	t.Helper()
	db, _ := gorm.Open(sqlite.Open(":memory:"))
	db.AutoMigrate(&auth.BlogUser{}, &blog.PostType{}, &blog.Post{}, &blog.Tag{}, &blog.Setting{}, &blog.Page{})
	db.Create(&blog.PostType{Name: "Post", Slug: "posts"})
	db.Create(&blog.PostType{Name: "Notes", Slug: "notes"})
	db.Create(&blog.Post{Title: "p", Slug: "p", PostTypeID: 1})
	db.Create(&blog.Page{Title: "About", Slug: "about", PageType: blog.PageTypeCustom})
	db.Create(&blog.Page{Title: "Contact", Slug: "contact", PageType: blog.PageTypeCustom})
	a := &Auth{}
	a.On("IsAdmin", mock.Anything).Return(isAdmin)
	a.On("IsLoggedIn", mock.Anything).Return(isAdmin)
	b := blog.New(db, a, "test")
	ad := admin.New(db, a, &b, "test")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	tmpl := template.Must(template.New("").Funcs(theme.FuncMap()).ParseGlob("../templates/shared/*.html"))
	template.Must(tmpl.ParseGlob("../themes/default/templates/*.html"))
	r.SetHTMLTemplate(tmpl)
	r.POST("/api/v1/pages", ad.CreatePage)
	r.PATCH("/api/v1/pages", ad.UpdatePage)
	r.DELETE("/api/v1/pages", ad.DeletePage)
	r.GET("/api/v1/post-types", ad.ListPostTypes)
	r.POST("/api/v1/post-types", ad.CreatePostType)
	r.PATCH("/api/v1/post-types", ad.UpdatePostType)
	r.DELETE("/api/v1/post-types", ad.DeletePostType)
	r.GET("/admin/pages/:id", ad.AdminEditPage)
	r.GET("/admin/post-types/:id", ad.AdminEditPostType)
	return r
}

func TestPageAndPostTypeAPI_NonAdmin(t *testing.T) {
	r := crudAPI(t, false)
	for _, m := range []string{"POST", "PATCH", "DELETE"} {
		for _, p := range []string{"/api/v1/pages", "/api/v1/post-types"} {
			if w := call(r, m, p, "application/json", `{}`); w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401, got %d", m, p, w.Code)
			}
		}
	}
	if w := call(r, "GET", "/api/v1/post-types", "", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("GET post-types: expected 401, got %d", w.Code)
	}
}

// TestPageAndPostTypeAPI_Validation walks each handler's guards in order.
// Cases run against one db, so the successful deletes come last.
func TestPageAndPostTypeAPI_Validation(t *testing.T) {
	r := crudAPI(t, true)
	const pages, types = "/api/v1/pages", "/api/v1/post-types"
	for _, c := range []struct {
		name, method, path, contentType, body string
		want                                  int
	}{
		{"page create: not JSON", "POST", pages, "text/plain", `{}`, 415},
		{"page create: malformed", "POST", pages, "application/json", `{`, 400},
		{"page create: no title", "POST", pages, "application/json", `{"slug":"x"}`, 400},
		{"page create: no slug", "POST", pages, "application/json", `{"title":"T"}`, 400},
		{"page create: bad slug", "POST", pages, "application/json", `{"title":"T","slug":"a_b"}`, 400},
		{"page create: reserved", "POST", pages, "application/json", `{"title":"T","slug":"admin"}`, 400},
		{"page create: taken by page", "POST", pages, "application/json", `{"title":"T","slug":"about"}`, 409},
		{"page create: taken by type", "POST", pages, "application/json", `{"title":"T","slug":"notes"}`, 409},

		{"page update: not JSON", "PATCH", pages, "text/plain", `{}`, 415},
		{"page update: malformed", "PATCH", pages, "application/json", `{`, 400},
		{"page update: no id", "PATCH", pages, "application/json", `{"slug":"x"}`, 400},
		{"page update: unknown id", "PATCH", pages, "application/json", `{"id":99,"slug":"x"}`, 404},
		{"page update: no slug", "PATCH", pages, "application/json", `{"id":1}`, 400},
		{"page update: bad slug", "PATCH", pages, "application/json", `{"id":1,"slug":"a_b"}`, 400},
		{"page update: reserved", "PATCH", pages, "application/json", `{"id":1,"slug":"admin"}`, 400},
		{"page update: taken by page", "PATCH", pages, "application/json", `{"id":1,"slug":"contact"}`, 409},
		{"page update: taken by type", "PATCH", pages, "application/json", `{"id":1,"slug":"notes"}`, 409},

		{"type create: not JSON", "POST", types, "text/plain", `{}`, 415},
		{"type create: malformed", "POST", types, "application/json", `{`, 400},
		{"type create: no name", "POST", types, "application/json", `{"slug":"x"}`, 400},
		{"type create: no slug", "POST", types, "application/json", `{"name":"N"}`, 400},
		{"type create: bad slug", "POST", types, "application/json", `{"name":"N","slug":"a_b"}`, 400},
		{"type create: reserved", "POST", types, "application/json", `{"name":"N","slug":"admin"}`, 400},
		{"type create: taken by type", "POST", types, "application/json", `{"name":"N","slug":"notes"}`, 409},
		{"type create: taken by page", "POST", types, "application/json", `{"name":"N","slug":"about"}`, 409},

		{"type update: not JSON", "PATCH", types, "text/plain", `{}`, 415},
		{"type update: malformed", "PATCH", types, "application/json", `{`, 400},
		{"type update: no id", "PATCH", types, "application/json", `{"name":"N"}`, 400},
		{"type update: unknown id", "PATCH", types, "application/json", `{"id":99,"name":"N"}`, 404},
		{"type update: no name", "PATCH", types, "application/json", `{"id":2,"slug":"x"}`, 400},
		{"type update: no slug", "PATCH", types, "application/json", `{"id":2,"name":"N"}`, 400},
		{"type update: bad slug", "PATCH", types, "application/json", `{"id":2,"name":"N","slug":"a_b"}`, 400},
		{"type update: reserved", "PATCH", types, "application/json", `{"id":2,"name":"N","slug":"admin"}`, 400},
		{"type update: taken by type", "PATCH", types, "application/json", `{"id":2,"name":"N","slug":"posts"}`, 409},
		{"type update: taken by page", "PATCH", types, "application/json", `{"id":2,"name":"N","slug":"about"}`, 409},
		{"type update: ok", "PATCH", types, "application/json", `{"id":2,"name":"Notes","slug":"notes","description":"short"}`, 202},
		{"type list", "GET", types, "", "", 200},

		{"type delete: not JSON", "DELETE", types, "text/plain", `{}`, 415},
		{"type delete: malformed", "DELETE", types, "application/json", `{`, 400},
		{"type delete: no id", "DELETE", types, "application/json", `{}`, 400},
		{"type delete: unknown id", "DELETE", types, "application/json", `{"id":99}`, 404},
		{"type delete: in use", "DELETE", types, "application/json", `{"id":1}`, 400},
		{"type delete: ok", "DELETE", types, "application/json", `{"id":2}`, 200},

		{"page delete: not JSON", "DELETE", pages, "text/plain", `{}`, 415},
		{"page delete: malformed", "DELETE", pages, "application/json", `{`, 400},
		{"page delete: ok", "DELETE", pages, "application/json", `{"id":2}`, 200},
	} {
		if w := call(r, c.method, c.path, c.contentType, c.body); w.Code != c.want {
			t.Errorf("%s: expected %d, got %d %s", c.name, c.want, w.Code, w.Body.String())
		}
	}
}

func TestAdminEditPages_BadAndUnknownID(t *testing.T) {
	r := crudAPI(t, true)
	for path, want := range map[string]int{
		"/admin/pages/abc": 400, "/admin/pages/99": 404, "/admin/pages/1": 200,
		"/admin/post-types/abc": 400, "/admin/post-types/99": 404, "/admin/post-types/1": 200,
	} {
		if w := call(r, "GET", path, "", ""); w.Code != want {
			t.Errorf("%s: expected %d, got %d", path, want, w.Code)
		}
	}
}
