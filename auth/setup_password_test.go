package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"goblog/auth"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// browser is a cookie-keeping client for a gin router: enough of a browser
// to carry a session from one request to the next.
type browser struct {
	t       *testing.T
	router  *gin.Engine
	cookies map[string]string
}

func (b *browser) do(method, path string, form url.Values) *httptest.ResponseRecorder {
	b.t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	for name, value := range b.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	w := httptest.NewRecorder()
	b.router.ServeHTTP(w, req)
	for _, c := range w.Result().Cookies() {
		b.cookies[c.Name] = c.Value
	}
	return w
}

// authApp is a router exposing just what these tests poke at.
func authApp(t *testing.T, a *auth.Auth) func() *browser {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("test"))))
	yes := func(ok bool) int {
		if ok {
			return http.StatusOK
		}
		return http.StatusForbidden
	}
	r.POST("/unlock", func(c *gin.Context) {
		err := auth.UnlockSetup(c, c.PostForm("setup_code"))
		switch {
		case err == nil:
			c.Status(http.StatusOK)
		case errors.Is(err, auth.ErrSetupRateLimited):
			c.Status(http.StatusTooManyRequests)
		default:
			c.Status(http.StatusForbidden)
		}
	})
	r.GET("/wizard-mode", func(c *gin.Context) { c.Status(yes(a.IsWizardMode(c))) })
	r.GET("/is-admin", func(c *gin.Context) { c.Status(yes(a.IsAdmin(c))) })
	r.POST("/login/password", func(c *gin.Context) { a.PasswordLogin(c, "/done") })
	return func() *browser { return &browser{t: t, router: r, cookies: map[string]string{}} }
}

func setupCode(t *testing.T) string {
	t.Helper()
	code, err := auth.NewSetupCode()
	if err != nil {
		t.Fatal(err)
	}
	auth.ResetSetupLimiter()
	t.Cleanup(auth.ClearSetupCode)
	return code
}

// TestSetupCode: wizard mode is for the browser that entered the code the
// server printed, and for nobody else.
func TestSetupCode(t *testing.T) {
	a, _ := newAuth(t)
	newBrowser := authApp(t, a)
	code := setupCode(t)

	owner, stranger := newBrowser(), newBrowser()
	if got := owner.do("GET", "/wizard-mode", nil).Code; got != http.StatusForbidden {
		t.Fatalf("wizard mode before the code was entered: %d, want 403", got)
	}
	if got := stranger.do("POST", "/unlock", url.Values{"setup_code": {"AAAA-AAAA-AAAA"}}).Code; got != http.StatusForbidden {
		t.Fatalf("unlock with a wrong code: %d, want 403", got)
	}
	// As typed by a person: lower case, spaces instead of dashes.
	typed := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	if got := owner.do("POST", "/unlock", url.Values{"setup_code": {typed}}).Code; got != http.StatusOK {
		t.Fatalf("unlock with the right code: %d, want 200", got)
	}
	if got := owner.do("GET", "/wizard-mode", nil).Code; got != http.StatusOK {
		t.Errorf("wizard mode after unlocking: %d, want 200", got)
	}
	if got := stranger.do("GET", "/wizard-mode", nil).Code; got != http.StatusForbidden {
		t.Errorf("wizard mode in another browser: %d, want 403", got)
	}

	// A restart prints a new code; a browser unlocked with the old one is
	// locked again.
	setupCode(t)
	if got := owner.do("GET", "/wizard-mode", nil).Code; got != http.StatusForbidden {
		t.Errorf("wizard mode after the code changed: %d, want 403", got)
	}

	// No code at all (an installed site) unlocks nothing, including "".
	auth.ClearSetupCode()
	if got := newBrowser().do("POST", "/unlock", url.Values{"setup_code": {""}}).Code; got != http.StatusForbidden {
		t.Errorf("unlock with no code set: %d, want 403", got)
	}
}

func TestSetupCodeGuessesAreRateLimited(t *testing.T) {
	a, _ := newAuth(t)
	b := authApp(t, a)()
	setupCode(t)
	limited := false
	for i := 0; i < 40 && !limited; i++ {
		limited = b.do("POST", "/unlock", url.Values{"setup_code": {"AAAA-AAAA-AAAA"}}).Code == http.StatusTooManyRequests
	}
	if !limited {
		t.Fatal("40 wrong setup codes in a row were all answered")
	}
}

func TestCreatePasswordAdmin(t *testing.T) {
	a, db := newAuth(t)
	for name, tc := range map[string]struct{ email, password string }{
		"short password":   {"me@example.com", "too short"},
		"over 72 bytes":    {"me@example.com", strings.Repeat("x", 73)},
		"not an email":     {"me", "a long enough password"},
		"two addresses":    {"a@example.com,b@example.com", "a long enough password"},
		"empty everything": {"", ""},
	} {
		if _, err := a.CreatePasswordAdmin(tc.email, tc.password); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if a.AdminExists() {
		t.Fatal("a refused request created an admin")
	}

	user, err := a.CreatePasswordAdmin(" Me@Example.com ", "a long enough password")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if user.Provider != auth.ProviderPassword || user.ProviderID != "me@example.com" || user.AccessToken == "" {
		t.Errorf("unexpected user: %+v", user)
	}
	var stored auth.BlogUser
	db.First(&stored, user.ID)
	if stored.PasswordHash == "" || strings.Contains(stored.PasswordHash, "a long enough password") {
		t.Errorf("password not stored as a hash: %q", stored.PasswordHash)
	}
	if !a.AdminExists() {
		t.Fatal("no admin after CreatePasswordAdmin")
	}
	// The wizard creates the first admin and nothing more.
	if _, err := a.CreatePasswordAdmin("other@example.com", "another long password"); !errors.Is(err, auth.ErrAdminExists) {
		t.Errorf("second admin: err = %v, want ErrAdminExists", err)
	}
}

func TestPasswordLogin(t *testing.T) {
	a, _ := newAuth(t)
	newBrowser := authApp(t, a)
	const email, password = "me@example.com", "a long enough password"
	if _, err := a.CreatePasswordAdmin(email, password); err != nil {
		t.Fatal(err)
	}

	for name, form := range map[string]url.Values{
		"wrong password": {"email": {email}, "password": {"not the password"}},
		"unknown email":  {"email": {"you@example.com"}, "password": {password}},
		"no password":    {"email": {email}},
		"over 72 bytes":  {"email": {email}, "password": {password + strings.Repeat("x", 80)}},
	} {
		b := newBrowser()
		w := b.do("POST", "/login/password", form)
		if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, "/login?password=1&login_error=invalid") {
			t.Errorf("%s: redirected to %q, want the login page with login_error=invalid", name, loc)
		}
		if b.do("GET", "/is-admin", nil).Code == http.StatusOK {
			t.Errorf("%s: logged in", name)
		}
	}

	b := newBrowser()
	w := b.do("POST", "/login/password", url.Values{"email": {"ME@example.com"}, "password": {password}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/done" {
		t.Fatalf("right password: %d to %q, want 303 to /done", w.Code, w.Header().Get("Location"))
	}
	if got := b.do("GET", "/is-admin", nil).Code; got != http.StatusOK {
		t.Fatalf("not an admin after logging in: %d", got)
	}
}

func TestPasswordLoginIsRateLimited(t *testing.T) {
	a, _ := newAuth(t)
	b := authApp(t, a)()
	const email, password = "me@example.com", "a long enough password"
	if _, err := a.CreatePasswordAdmin(email, password); err != nil {
		t.Fatal(err)
	}
	limited := false
	for i := 0; i < 40 && !limited; i++ {
		w := b.do("POST", "/login/password", url.Values{"email": {email}, "password": {"guess"}})
		limited = strings.Contains(w.Header().Get("Location"), "login_error=rate_limit")
	}
	if !limited {
		t.Fatal("40 wrong passwords in a row were all checked")
	}
	// The right password does not get through a limited client either.
	w := b.do("POST", "/login/password", url.Values{"email": {email}, "password": {password}})
	if !strings.Contains(w.Header().Get("Location"), "login_error=rate_limit") {
		t.Errorf("limited client logged in: %q", w.Header().Get("Location"))
	}
}

func TestResetPassword(t *testing.T) {
	a, _ := newAuth(t)
	if _, _, err := a.ResetPassword(""); !errors.Is(err, auth.ErrNoPasswordUser) {
		t.Fatalf("reset with no password user: %v, want ErrNoPasswordUser", err)
	}
	const email, old = "me@example.com", "a long enough password"
	if _, err := a.CreatePasswordAdmin(email, old); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.ResetPassword("you@example.com"); !errors.Is(err, auth.ErrNoPasswordUser) {
		t.Fatalf("reset for an unknown email: %v, want ErrNoPasswordUser", err)
	}
	who, fresh, err := a.ResetPassword("")
	if err != nil || who != email || len(fresh) < auth.MinPasswordLength {
		t.Fatalf("reset: who=%q password=%q err=%v", who, fresh, err)
	}
	newBrowser := authApp(t, a)
	if loc := newBrowser().do("POST", "/login/password", url.Values{"email": {email}, "password": {old}}).Header().Get("Location"); loc == "/done" {
		t.Error("the old password still works")
	}
	if loc := newBrowser().do("POST", "/login/password", url.Values{"email": {email}, "password": {fresh}}).Header().Get("Location"); loc != "/done" {
		t.Errorf("the new password does not work: redirected to %q", loc)
	}
}
