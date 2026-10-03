package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"goblog/auth"
	"goblog/tools"
	"goblog/wizard"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestInstallOnly: the database wizard's endpoints are for installing. On a
// site that has a database and an admin, a request to them must change
// nothing; before that (no database yet, or a database with no admin) the
// wizard still has to work.
func TestInstallOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	post := func(g *goblog, path string) *httptest.ResponseRecorder {
		router := gin.New()
		router.SetHTMLTemplate(templateWithErrors(t))
		router.POST("/wizard_db", g.installOnly, updateDB)
		router.POST("/test_db", g.installOnly, testDB)
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("dbtype=sqlite&sqlite_file=other.db"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	app := func(db *gorm.DB) *goblog {
		a := auth.New(db, "test")
		wz := wizard.New(db, "test")
		return &goblog{_auth: &a, _wizard: &wz}
	}
	openDB := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := tools.Migrate(db); err != nil {
			t.Fatal(err)
		}
		return db
	}

	t.Run("installed site", func(t *testing.T) {
		t.Chdir(t.TempDir())
		const env = "database=sqlite\nsqlite_db=live.db\nclient_id=x\nclient_secret=y\n"
		if err := os.WriteFile(".env", []byte(env), 0600); err != nil {
			t.Fatal(err)
		}
		db := openDB()
		user := auth.BlogUser{Provider: auth.ProviderGitHub, ProviderID: "1", Login: "admin"}
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&auth.AdminUser{BlogUserID: user.ID}).Error; err != nil {
			t.Fatal(err)
		}
		g := app(db)
		for _, path := range []string{"/wizard_db", "/test_db"} {
			if w := post(g, path); w.Code != http.StatusForbidden {
				t.Errorf("POST %s on an installed site: status %d, want 403", path, w.Code)
			}
		}
		if got, _ := os.ReadFile(".env"); string(got) != env {
			t.Errorf(".env was rewritten on an installed site:\n%s", got)
		}
		if _, err := os.Stat("other.db"); err == nil {
			t.Error("/test_db created the file it was asked to open")
		}
	})

	t.Run("no database yet", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if w := post(app(nil), "/wizard_db"); w.Code != http.StatusSeeOther {
			t.Errorf("POST /wizard_db before install: status %d, want 303", w.Code)
		}
		if got, _ := os.ReadFile(".env"); !strings.Contains(string(got), "sqlite_db=other.db") {
			t.Errorf(".env not written by the wizard: %q", got)
		}
	})

	t.Run("database but no admin yet", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if w := post(app(openDB()), "/wizard_db"); w.Code != http.StatusSeeOther {
			t.Errorf("POST /wizard_db mid-install: status %d, want 303", w.Code)
		}
	})
}
