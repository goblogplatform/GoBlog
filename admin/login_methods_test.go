package admin_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goblogplatform/goblog/admin"
	"github.com/goblogplatform/goblog/blog"
	"github.com/goblogplatform/goblog/datadir"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestSaveGithubLogin checks the Login methods section's save (#668): only
// an admin may call it, a value that could add a line to .env is refused, and
// a good one replaces any earlier client_id/client_secret in .env, keeps the
// other lines, and takes effect in the environment without a restart.
func TestSaveGithubLogin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(datadir.EnvVar, dir)
	t.Setenv("client_id", "")
	t.Setenv("client_secret", "")
	env := filepath.Join(dir, ".env")
	os.WriteFile(env, []byte("SESSION_KEY=k\nclient_id=old\n# note\nclient_secret=old\ndatabase=sqlite\n"), 0600)

	put := func(isAdmin bool, body string) *httptest.ResponseRecorder {
		db, _ := gorm.Open(sqlite.Open(":memory:"))
		a := &Auth{}
		a.On("IsAdmin", mock.Anything).Return(isAdmin)
		b := blog.New(db, a, "test")
		ad := admin.New(db, a, &b, "test")
		router := gin.New()
		router.PUT("/api/v1/login-methods/github", ad.SaveGithubLogin)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPut, "/api/v1/login-methods/github", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		return w
	}

	if w := put(false, `{"client_id":"Ov23li","client_secret":"abc"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("non-admin: %d, want 401", w.Code)
	}
	for _, body := range []string{
		`{"client_id":"Ov23li","client_secret":""}`,
		`{"client_id":"Ov23li\nadmin_login=evil","client_secret":"abc"}`,
	} {
		if w := put(true, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, w.Code)
		}
	}
	if w := put(true, `{"client_id":" Ov23li.x ","client_secret":"abc123"}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	got, _ := os.ReadFile(env)
	want := "SESSION_KEY=k\n# note\ndatabase=sqlite\nclient_id=Ov23li.x\nclient_secret=abc123\n"
	if string(got) != want {
		t.Errorf(".env =\n%s\nwant\n%s", got, want)
	}
	if os.Getenv("client_id") != "Ov23li.x" || os.Getenv("client_secret") != "abc123" {
		t.Errorf("environment not updated: %q %q", os.Getenv("client_id"), os.Getenv("client_secret"))
	}
}
