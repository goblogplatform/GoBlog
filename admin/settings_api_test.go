package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goblogplatform/goblog/admin"
	"github.com/goblogplatform/goblog/auth"
	"github.com/goblogplatform/goblog/blog"
	"github.com/goblogplatform/goblog/plugin"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// settingsAPI wires the settings, plugin-settings and publish/draft endpoints
// against an in-memory db; isAdmin is what the mocked auth answers.
func settingsAPI(t *testing.T, isAdmin bool) (*gin.Engine, *gorm.DB, *admin.Admin) {
	t.Helper()
	db, _ := gorm.Open(sqlite.Open(":memory:"))
	db.AutoMigrate(&auth.BlogUser{}, &blog.PostType{}, &blog.Post{}, &blog.Tag{}, &blog.Setting{}, &plugin.PluginSetting{})
	a := &Auth{}
	a.On("IsAdmin", mock.Anything).Return(isAdmin)
	a.On("IsWizardMode", mock.Anything).Return(false)
	b := blog.New(db, a, "test")
	ad := admin.New(db, a, &b, "test")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PATCH("/api/v1/publish/:id", ad.PublishPost)
	r.PATCH("/api/v1/draft/:id", ad.DraftPost)
	r.GET("/api/v1/setting/:slug", ad.GetSetting)
	r.POST("/api/v1/setting", ad.AddSetting)
	r.PATCH("/api/v1/settings", ad.UpdateSettings)
	r.PATCH("/api/v1/plugin-settings", ad.UpdatePluginSettings)
	reg := plugin.NewRegistry(db)
	r.PATCH("/with-registry/plugin-settings", plugin.Middleware(reg), ad.UpdatePluginSettings)
	return r, db, &ad
}

func call(r http.Handler, method, path, contentType, body string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestSettingsAPI_NonAdmin: every endpoint here is admin-only, including
// publish/draft, which used to let anyone flip a post's draft flag.
func TestSettingsAPI_NonAdmin(t *testing.T) {
	r, db, _ := settingsAPI(t, false)
	pt := blog.PostType{Name: "Post", Slug: "posts"}
	db.Create(&pt)
	post := blog.Post{Title: "secret", Slug: "secret", Draft: true, PostTypeID: pt.ID}
	db.Create(&post)

	for _, c := range []struct{ m, p, body string }{
		{"PATCH", "/api/v1/publish/1", ""}, {"PATCH", "/api/v1/draft/1", ""},
		{"GET", "/api/v1/setting/title", ""}, {"POST", "/api/v1/setting", `{"key":"x"}`},
		{"PATCH", "/api/v1/settings", `[]`}, {"PATCH", "/api/v1/plugin-settings", `[]`},
	} {
		if w := call(r, c.m, c.p, "application/json", c.body); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", c.m, c.p, w.Code)
		}
	}
	db.First(&post, post.ID)
	if !post.Draft {
		t.Fatal("a non-admin must not be able to publish a draft")
	}
}

func TestPublishAndDraftPost(t *testing.T) {
	r, db, _ := settingsAPI(t, true)
	pt := blog.PostType{Name: "Post", Slug: "posts"}
	db.Create(&pt)
	post := blog.Post{Title: "hello", Slug: "hello", Draft: true, PostTypeID: pt.ID}
	db.Create(&post)

	w := call(r, "PATCH", "/api/v1/publish/1", "", "")
	db.First(&post, post.ID)
	if w.Code != http.StatusAccepted || post.Draft {
		t.Fatalf("publish: got %d, draft=%v", w.Code, post.Draft)
	}
	// The admin script redirects using the returned post's type slug.
	if !strings.Contains(w.Body.String(), `"slug":"posts"`) {
		t.Errorf("publish response should carry the post type: %s", w.Body.String())
	}
	w = call(r, "PATCH", "/api/v1/draft/1", "", "")
	db.First(&post, post.ID)
	if w.Code != http.StatusAccepted || !post.Draft {
		t.Fatalf("draft: got %d, draft=%v", w.Code, post.Draft)
	}
}

func TestAddAndGetSetting(t *testing.T) {
	r, _, _ := settingsAPI(t, true)

	if w := call(r, "POST", "/api/v1/setting", "text/plain", `{"key":"a"}`); w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("wrong content type: expected 415, got %d", w.Code)
	}
	if w := call(r, "POST", "/api/v1/setting", "application/json", `{`); w.Code != http.StatusBadRequest {
		t.Errorf("malformed: expected 400, got %d", w.Code)
	}
	if w := call(r, "POST", "/api/v1/setting", "application/json", `{"value":"v"}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing key: expected 400, got %d", w.Code)
	}
	if w := call(r, "GET", "/api/v1/setting/greeting", "", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown setting: expected 404, got %d", w.Code)
	}
	if w := call(r, "POST", "/api/v1/setting", "application/json", `{"key":"greeting","value":"hi"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}
	w := call(r, "GET", "/api/v1/setting/greeting", "", "")
	var got blog.Setting
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != http.StatusOK || got.Key != "greeting" || got.Value != "hi" {
		t.Fatalf("get: expected the stored setting, got %d %s", w.Code, w.Body.String())
	}
}

func TestUpdateSettings(t *testing.T) {
	r, db, ad := settingsAPI(t, true)
	var reloaded string
	ad.OnThemeChange = func(theme string) { reloaded = theme }

	if w := call(r, "PATCH", "/api/v1/settings", "text/plain", `[]`); w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("wrong content type: expected 415, got %d", w.Code)
	}
	if w := call(r, "PATCH", "/api/v1/settings", "application/json", `{`); w.Code != http.StatusBadRequest {
		t.Errorf("malformed: expected 400, got %d", w.Code)
	}
	if w := call(r, "PATCH", "/api/v1/settings", "application/json", `[{"value":"v"}]`); w.Code != http.StatusBadRequest {
		t.Errorf("missing key: expected 400, got %d", w.Code)
	}
	w := call(r, "PATCH", "/api/v1/settings", "application/json", `[{"key":"title","value":"Mine"},{"key":"theme","value":"forest"}]`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("update: expected 202, got %d", w.Code)
	}
	var title blog.Setting
	db.First(&title, "key = ?", "title")
	if title.Value != "Mine" {
		t.Errorf("expected title saved, got %q", title.Value)
	}
	if reloaded != "forest" {
		t.Errorf("expected a theme change to reload templates, got %q", reloaded)
	}
}

func TestUpdatePluginSettings(t *testing.T) {
	r, db, _ := settingsAPI(t, true)

	if w := call(r, "PATCH", "/with-registry/plugin-settings", "application/json", `{`); w.Code != http.StatusBadRequest {
		t.Errorf("malformed: expected 400, got %d", w.Code)
	}
	if w := call(r, "PATCH", "/api/v1/plugin-settings", "application/json", `[]`); w.Code != http.StatusInternalServerError {
		t.Errorf("no registry: expected 500, got %d", w.Code)
	}
	w := call(r, "PATCH", "/with-registry/plugin-settings", "application/json",
		`[{"key":"hello.greeting","value":"hi"},{"key":"nodot","value":"ignored"}]`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("update: expected 202, got %d", w.Code)
	}
	var rows []plugin.PluginSetting
	db.Find(&rows)
	if len(rows) != 1 || rows[0].PluginName != "hello" || rows[0].Key != "greeting" || rows[0].Value != "hi" {
		t.Fatalf("expected one hello.greeting row, got %+v", rows)
	}
}
