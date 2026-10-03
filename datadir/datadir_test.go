package datadir

import "testing"

func TestPath(t *testing.T) {
	t.Setenv(EnvVar, "")
	if got := Path(".env"); got != ".env" {
		t.Errorf("no data dir: Path(.env) = %q, want .env unchanged", got)
	}
	t.Setenv(EnvVar, "/data")
	for rel, want := range map[string]string{
		".env":             "/data/.env",
		"goblog.db":        "/data/goblog.db",
		"plugins/wasm":     "/data/plugins/wasm",
		"/var/lib/blog.db": "/var/lib/blog.db", // absolute paths are left alone
	} {
		if got := Path(rel); got != want {
			t.Errorf("Path(%q) = %q, want %q", rel, got, want)
		}
	}
}
