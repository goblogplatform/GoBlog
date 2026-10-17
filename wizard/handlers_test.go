package wizard

import (
	"github.com/goblogplatform/goblog/datadir"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// newRouter wires the wizard's public handlers with stub templates that echo
// which page rendered and any error, so tests can assert on the branch taken.
func newRouter(t *testing.T, w *Wizard) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("test"))))
	tmpl := template.Must(template.New("wizard_auth.html").Parse(`auth:{{.errors}}`))
	template.Must(tmpl.New("wizard_settings.html").Parse(`settings`))
	r.SetHTMLTemplate(tmpl)
	r.GET("/wizard", w.Landing)
	r.GET("/token", w.SaveToken)
	r.GET("/code", func(c *gin.Context) {
		if err := w.LoginCode(c); err != nil {
			c.String(http.StatusBadRequest, err.Error())
		}
	})
	return r
}

func get(r http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestLanding(t *testing.T) {
	w, _ := newWizard(t)
	r := newRouter(t, w)
	if body := get(r, "/wizard?page=auth").Body.String(); !strings.HasPrefix(body, "auth:") {
		t.Fatalf("page=auth should render the auth page, got %q", body)
	}
	if body := get(r, "/wizard").Body.String(); body != "settings" {
		t.Fatalf("default should render the settings page, got %q", body)
	}
}

func TestSaveToken(t *testing.T) {
	w, _ := newWizard(t)
	r := newRouter(t, w)

	if body := get(r, "/token").Body.String(); body != "auth:" {
		t.Fatalf("missing client_id should re-render auth without error, got %q", body)
	}
	if body := get(r, "/token?client_id=abc").Body.String(); !strings.Contains(body, "Client Secret must not be empty") {
		t.Fatalf("missing client_secret should report an error, got %q", body)
	}
	rec := get(r, "/token?client_id=abc&client_secret=shh")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://github.com/login/oauth/authorize?client_id=abc" {
		t.Fatalf("expected redirect to GitHub authorize, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

// TestLoginCode_EarlyErrors covers the guards before the GitHub exchange,
// which is the part that can run without network access.
func TestLoginCode_EarlyErrors(t *testing.T) {
	nilDb := New(nil, "test")
	if body := get(newRouter(t, &nilDb), "/code").Body.String(); body != "db is nil" {
		t.Fatalf("expected db is nil, got %q", body)
	}

	w, _ := newWizard(t)
	r := newRouter(t, w)
	if body := get(r, "/code").Body.String(); body != "can't retrieve client_id" {
		t.Fatalf("expected missing client_id error, got %q", body)
	}

	// A session holding only client_id fails on the secret.
	r.GET("/half", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("client_id", "abc")
		_ = s.Save()
	})
	cookies := get(r, "/half").Result().Cookies()
	if body := get(r, "/code", cookies...).Body.String(); body != "can't retrieve client_secret" {
		t.Fatalf("expected missing client_secret error, got %q", body)
	}
}

func TestUpdateDb(t *testing.T) {
	w := New(nil, "test")
	if !w.IsDbNil() {
		t.Fatal("expected nil db")
	}
	_, db := newWizard(t)
	w.UpdateDb(db)
	if w.IsDbNil() {
		t.Fatal("expected db after UpdateDb")
	}
	w2 := New(nil, "test")
	if err := w2.updateAdminUser("x"); err == nil || err.Error() != "db is nil" {
		t.Fatalf("expected db is nil, got %v", err)
	}
}

// fakeGitHub answers the token exchange and /user calls LoginCode makes, so the
// whole flow runs without network access.
type fakeGitHub struct{ tokenStatus int }

func (f fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	status, body := http.StatusOK, `{"id":42,"login":"operator"}`
	if req.URL.Path == "/login/oauth/access_token" {
		status, body = f.tokenStatus, `{"access_token":"tok"}`
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: req}, nil
}

func TestLoginCode_CompletesInstall(t *testing.T) {
	t.Setenv("admin_login", "")
	t.Setenv("admin_github_id", "")
	dir := t.TempDir()
	t.Setenv(datadir.EnvVar, dir)
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })
	for _, tc := range []struct {
		status int
		want   string
	}{
		{http.StatusInternalServerError, "status was not 200: but was, 500"},
		{http.StatusOK, ""},
	} {
		http.DefaultTransport = fakeGitHub{tc.status}

		w, db := newWizard(t)
		r := newRouter(t, w)
		cookies := get(r, "/token?client_id=abc&client_secret=shh").Result().Cookies()
		if body := get(r, "/code?code=xyz", cookies...).Body.String(); body != tc.want {
			t.Fatalf("status %d: expected %q, got %q", tc.status, tc.want, body)
		}
		if tc.want != "" {
			continue
		}
		env, err := os.ReadFile(filepath.Join(dir, ".env"))
		if err != nil || string(env) != "client_id=abc\nclient_secret=shh\n" {
			t.Fatalf("expected credentials in .env, got %q (%v)", env, err)
		}
		if got := adminCount(t, db); got != 1 {
			t.Fatalf("expected the GitHub user to become admin, got %d admins", got)
		}
	}
}
