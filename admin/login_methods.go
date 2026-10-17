package admin

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/goblogplatform/goblog/datadir"
)

// githubCredential matches a GitHub OAuth app's client id ("Ov23li…",
// "Iv1.…") or secret (hex). Anything else is refused rather than quoted, so a
// value can never add a line to .env.
var githubCredential = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// SaveGithubLogin (PUT /api/v1/login-methods/github) stores a GitHub OAuth
// app's client id and secret in .env and in the environment, which the login
// page and /login/github read on each request, so GitHub login works without
// a restart. Before this only the install wizard could set them, so a site
// installed with a password admin needed .env edited by hand (#668).
func (a *Admin) SaveGithubLogin(c *gin.Context) {
	if !a.auth.IsAdmin(c) {
		c.JSON(http.StatusUnauthorized, "Not Authorized")
		return
	}
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, "Couldn't read the request: "+err.Error())
		return
	}
	id, secret := strings.TrimSpace(req.ClientID), strings.TrimSpace(req.ClientSecret)
	if !githubCredential.MatchString(id) || !githubCredential.MatchString(secret) {
		c.JSON(http.StatusBadRequest, "Enter the Client ID and a Client Secret exactly as GitHub shows them")
		return
	}
	if err := setGithubEnv(datadir.Path(".env"), id, secret); err != nil {
		c.JSON(http.StatusInternalServerError, "Couldn't write the .env file: "+err.Error())
		return
	}
	os.Setenv("client_id", id)
	os.Setenv("client_secret", secret)
	c.JSON(http.StatusOK, "GitHub login saved")
}

// setGithubEnv rewrites the .env file at path with client_id and
// client_secret set to id and secret, keeping every other line.
func setGithubEnv(path, id, secret string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var out strings.Builder
	if kept := strings.TrimRight(string(existing), "\n"); kept != "" {
		for _, line := range strings.Split(kept, "\n") {
			key, _, _ := strings.Cut(strings.TrimSpace(line), "=")
			if key = strings.TrimSpace(key); key != "client_id" && key != "client_secret" {
				out.WriteString(line + "\n")
			}
		}
	}
	out.WriteString("client_id=" + id + "\nclient_secret=" + secret + "\n")
	return os.WriteFile(path, []byte(out.String()), 0600)
}
