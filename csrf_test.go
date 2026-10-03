package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestSessionOptions(t *testing.T) {
	o := sessionOptions(true)
	if !o.HttpOnly || !o.Secure || o.SameSite != http.SameSiteLaxMode || o.Path != "/" || o.MaxAge <= 0 {
		t.Errorf("options: %+v", o)
	}
	if sessionOptions(false).Secure {
		t.Error("sessionOptions(false) is Secure")
	}
}

// TestCookieSecure: the session cookie is Secure unless the operator said
// otherwise or the request is plain http for a host on a local network,
// where a Secure cookie would be dropped and nobody could log in (#665).
func TestCookieSecure(t *testing.T) {
	type req struct {
		env, host, proto string
		tls              bool
	}
	for name, tc := range map[string]struct {
		req
		want bool
	}{
		"LAN address":                    {req{host: "192.168.1.20:7000"}, false},
		"loopback address":               {req{host: "127.0.0.1:7000"}, false},
		"IPv6 address":                   {req{host: "[fd00::1]:7000"}, false},
		"bare host name":                 {req{host: "nas:7000"}, false},
		"mDNS name":                      {req{host: "blog.local"}, false},
		"localhost":                      {req{host: "localhost:7000"}, false},
		"public name, plain http":        {req{host: "blog.example.com"}, true}, // or a proxy that forwards no headers
		"public name behind https proxy": {req{host: "blog.example.com", proto: "https"}, true},
		"LAN address behind https proxy": {req{host: "192.168.1.20", proto: "HTTPS, http"}, true},
		"LAN address over TLS":           {req{host: "192.168.1.20:7000", tls: true}, true},
		"no host":                        {req{host: ""}, true},
		"forced on, LAN address":         {req{env: "true", host: "192.168.1.20:7000"}, true},
		"forced off, public name":        {req{env: "false", host: "blog.example.com", proto: "https"}, false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Host = tc.host
		if tc.proto != "" {
			r.Header.Set("X-Forwarded-Proto", tc.proto)
		}
		if tc.tls {
			r.TLS = &tls.ConnectionState{}
		}
		if got := cookieSecure(tc.env, r); got != tc.want {
			t.Errorf("%s: Secure = %v, want %v", name, got, tc.want)
		}
	}
}

// TestSessionCookieSecurity: the middleware's decision is what ends up on
// the Set-Cookie header.
func TestSessionCookieSecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := cookie.NewStore([]byte("test"))
	store.Options(sessionOptions(true))
	r.Use(sessions.Sessions("session", store))
	r.Use(sessionCookieSecurity(""))
	r.GET("/", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("k", "v")
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
	})
	for host, wantSecure := range map[string]bool{"192.168.1.20:7000": false, "blog.example.com": true} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		header := w.Header().Get("Set-Cookie")
		if header == "" {
			t.Fatalf("%s: no Set-Cookie", host)
		}
		if got := strings.Contains(header, "Secure"); got != wantSecure {
			t.Errorf("%s: Secure = %v, want %v (%s)", host, got, wantSecure, header)
		}
		if !strings.Contains(header, "HttpOnly") || !strings.Contains(header, "SameSite=Lax") {
			t.Errorf("%s: cookie lost HttpOnly or SameSite: %s", host, header)
		}
	}
}

func TestRequireJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requireJSON())
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.POST("/api/v1/posts", ok)
	r.DELETE("/api/v1/comments", ok)
	r.DELETE("/api/v1/plugins/:name", ok)
	r.POST("/api/v1/themes/activate", ok)
	r.POST("/api/v1/upload", ok)
	r.POST("/api/v1/directory/repos/:id/approve", ok)
	r.GET("/api/v1/posts", ok)
	r.POST("/wizard_db", ok)

	cases := []struct {
		name, method, path, contentType, body string
		want                                  int
	}{
		{"json accepted", "POST", "/api/v1/posts", "application/json", `{}`, 200},
		{"json with charset accepted", "POST", "/api/v1/posts", "application/json; charset=UTF-8", `{}`, 200},
		{"text/plain rejected", "DELETE", "/api/v1/comments", "text/plain", `{"id":1}`, 415},
		{"form rejected", "POST", "/api/v1/posts", "application/x-www-form-urlencoded", "a=b", 415},
		{"missing type with body rejected", "POST", "/api/v1/posts", "", `{}`, 415},
		{"no body allowed", "DELETE", "/api/v1/plugins/hello", "", "", 200},
		// A repo submitted publicly must not be approvable by a cross-site
		// form post against a logged-in admin: forms cannot send JSON.
		{"cross-site approve rejected", "POST", "/api/v1/directory/repos/1/approve", "application/x-www-form-urlencoded", "", 415},
		{"admin approve allowed", "POST", "/api/v1/directory/repos/1/approve", "application/json", "", 200},
		{"cross-site theme activate rejected", "POST", "/api/v1/themes/activate", "application/x-www-form-urlencoded", "name=x", 415},
		{"theme activate allowed", "POST", "/api/v1/themes/activate", "application/json", `{"name":"x"}`, 200},
		{"multipart upload allowed", "POST", "/api/v1/upload", "multipart/form-data; boundary=x", "--x--", 200},
		{"multipart elsewhere rejected", "POST", "/api/v1/posts", "multipart/form-data; boundary=x", "--x--", 415},
		{"GET untouched", "GET", "/api/v1/posts", "", "", 200},
		{"wizard untouched", "POST", "/wizard_db", "application/x-www-form-urlencoded", "a=b", 200},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.contentType != "" {
			req.Header.Set("Content-Type", tc.contentType)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, w.Code, tc.want)
		}
	}
}

// TestNoRawOAuthCodeEndpoint: /api/login took an OAuth code as a form post and
// set the session from it, and requireJSON deliberately let it through, so a
// cross-site form could plant a session on a visitor. SameSite=Lax does not
// help — it governs sending a cookie cross-site, not setting one. GitHub
// returns to /login now and the server does the exchange itself, against a
// state it minted (#637), so nothing accepts a bare code any more.
func TestNoRawOAuthCodeEndpoint(t *testing.T) {
	source, err := os.ReadFile("goblog.go")
	if err != nil {
		t.Fatalf("read goblog.go: %v", err)
	}
	// The closing quote matters: "/api/login/email" does not contain
	// `"/api/login"`, so the OTP routes below are not caught by this.
	if strings.Contains(string(source), `"/api/login"`) {
		t.Error(`/api/login is routed again: it accepts an OAuth code from anywhere, which is the login CSRF #637 closed`)
	}
	// Asserted positively so the check above is demonstrably about the OAuth
	// route alone, and so removing it does not quietly take the OTP endpoints
	// with it: those are a different flow and still wanted.
	for _, keep := range []string{`"/api/login/email"`, `"/api/login/email/verify"`} {
		if !strings.Contains(string(source), keep) {
			t.Errorf("%s is no longer routed; email login needs it", keep)
		}
	}
}
