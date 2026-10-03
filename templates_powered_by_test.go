package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"goblog/blog"
	"goblog/theme"
)

// TestPoweredBy: the shared _powered_by partial shows the credit unless the
// show_powered_by setting is "false" — including before the setting exists
// (an upgraded site, the wizard pages).
func TestPoweredBy(t *testing.T) {
	tmpl, _, err := theme.Load(theme.DefaultName, theme.FuncMap())
	if err != nil {
		t.Fatalf("load default theme: %v", err)
	}
	setting := func(v string) map[string]blog.Setting {
		return map[string]blog.Setting{"show_powered_by": {Key: "show_powered_by", Type: "checkbox", Value: v}}
	}
	for name, tc := range map[string]struct {
		data gin.H
		want bool
	}{
		"no settings (wizard)": {gin.H{"version": "v1"}, true},
		"setting not seeded":   {gin.H{"version": "v1", "settings": map[string]blog.Setting{}}, true},
		"on":                   {gin.H{"version": "v1", "settings": setting("true")}, true},
		"off":                  {gin.H{"version": "v1", "settings": setting("false")}, false},
	} {
		var out bytes.Buffer
		if err := tmpl.ExecuteTemplate(&out, "_powered_by", tc.data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := strings.Contains(out.String(), `Powered by <a href="https://www.goblog.live"`)
		if got != tc.want {
			t.Errorf("%s: credit shown = %v, want %v (output %q)", name, got, tc.want, out.String())
		}
		if !tc.want && strings.TrimSpace(out.String()) != "" {
			t.Errorf("%s: expected no output, got %q", name, out.String())
		}
	}
	// footer.html must go through the partial, or the setting does nothing.
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "footer.html", gin.H{"version": "v1", "settings": setting("false"), "recent": blog.Post{}}); err != nil {
		t.Fatalf("footer.html: %v", err)
	}
	if strings.Contains(out.String(), "Powered by") {
		t.Error("footer.html shows the credit with show_powered_by off")
	}
}
