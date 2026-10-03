package auth

import (
	"strings"
	"time"
)

// Values for BlogUser.Provider.
const (
	ProviderGitHub   = "github"
	ProviderEmail    = "email"
	ProviderPassword = "password"
)

// BlogUser is a user of the blog from any login provider. The (Provider,
// ProviderID) pair identifies the user in the external system: the GitHub
// numeric id as a string, or the normalised email address (for both email
// and password users). ID is an internal
// key assigned by the database. The only role that matters is admin (see
// AdminUser); otherwise users exist for comments.
type BlogUser struct {
	ID         int    `gorm:"primaryKey" json:"id"`
	Provider   string `gorm:"uniqueIndex:idx_blog_users_provider;size:32" json:"provider"`
	ProviderID string `gorm:"uniqueIndex:idx_blog_users_provider;size:255" json:"provider_id"`
	Login      string `json:"login"` // github handle, or the email address for email users
	AvatarURL  string `json:"avatar_url"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	// AccessToken is the session credential: the GitHub OAuth token for
	// GitHub users, a random token for email and password users. Never sent
	// to clients.
	AccessToken string `json:"-"`
	// PasswordHash is the bcrypt hash of a password user's password; empty
	// for every other provider. Never sent to clients.
	PasswordHash string `json:"-"`
}

// DisplayName is the name to show for the user where one is needed, e.g. as
// the default author of a comment: the profile name, else the login, with an
// email-address login reduced to its local part so addresses aren't shown.
func (u BlogUser) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	if at := strings.Index(u.Login, "@"); at > 0 {
		return u.Login[:at]
	}
	return u.Login
}

type AdminUser struct {
	BlogUserID int
	BlogUser   BlogUser
}

// LoginCode is an outstanding one-time email login code. One row per
// address; requesting a new code replaces it. The code itself is never
// stored, only its hex-encoded SHA-256.
type LoginCode struct {
	Email     string `gorm:"primaryKey;size:254"`
	CodeHash  string `gorm:"size:64"`
	ExpiresAt time.Time
	Attempts  int
	CreatedAt time.Time
}
