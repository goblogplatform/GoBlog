// Package datadir says where goblog keeps the files it writes: .env, a
// SQLite database, uploads, and installed plugins and themes.
//
// By default those live in the working directory, next to the templates
// and static files goblog ships with, which is how every install before
// this package existed is laid out. Setting GOBLOG_DATA_DIR moves all of
// them under one directory, so a container needs a single volume to keep
// its site (#653).
package datadir

import (
	"os"
	"path/filepath"
)

// EnvVar names the environment variable that sets the data directory.
const EnvVar = "GOBLOG_DATA_DIR"

// Dir returns the data directory, or "" when none is set.
func Dir() string { return os.Getenv(EnvVar) }

// Path returns where the file or directory rel lives: under the data
// directory when one is set, otherwise rel unchanged (relative to the
// working directory). An absolute rel is returned as it is, so a path the
// operator spelled out in full is never moved.
func Path(rel string) string {
	dir := Dir()
	if dir == "" || filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(dir, rel)
}
