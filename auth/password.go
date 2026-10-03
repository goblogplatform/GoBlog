package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Password login (#654): an admin account that needs nothing outside the
// server, so a fresh install can be finished without a GitHub OAuth app or
// an SMTP relay. The install wizard creates the one password user there is;
// there is no sign-up.
const (
	// MinPasswordLength is in characters. bcrypt reads at most 72 bytes and
	// silently ignores the rest, so longer passwords are refused rather than
	// truncated.
	MinPasswordLength = 10
	maxPasswordBytes  = 72

	passwordAttemptsPerIP  = 10
	passwordAttemptsWindow = 10 * time.Minute
)

// ErrAdminExists is returned by CreatePasswordAdmin once the site has an
// admin: the wizard creates the first one and nothing more.
var ErrAdminExists = errors.New("this site already has an admin")

// ErrNoPasswordUser is returned by ResetPassword when there is no password
// user to reset (or none with the given email).
var ErrNoPasswordUser = errors.New("no matching password user")

// dummyHash is compared against when the email matches no user, so a login
// for an unknown address takes as long as one for a known address.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not a real password"), bcrypt.DefaultCost)

// CheckPassword says whether password is acceptable as a new password.
func CheckPassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("the password must be at least %d characters", MinPasswordLength)
	}
	if len(password) > maxPasswordBytes {
		return fmt.Errorf("the password must be at most %d bytes", maxPasswordBytes)
	}
	return nil
}

// CreatePasswordAdmin creates the site's first admin as a password user and
// returns it with a fresh session token. It refuses with ErrAdminExists when
// there already is an admin; the check and both inserts share a transaction
// so two concurrent requests cannot both succeed.
func (a *Auth) CreatePasswordAdmin(email, password string) (*BlogUser, error) {
	email, ok := normalizeEmail(email)
	if !ok {
		return nil, errors.New("that does not look like an email address")
	}
	if err := CheckPassword(password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	token, err := newSessionToken()
	if err != nil {
		return nil, err
	}
	user := &BlogUser{
		Provider:     ProviderPassword,
		ProviderID:   email,
		Login:        email,
		Email:        email,
		AccessToken:  token,
		PasswordHash: string(hash),
	}
	err = (*a.db).Transaction(func(tx *gorm.DB) error {
		var admin AdminUser
		err := tx.First(&admin).Error
		if err == nil {
			return ErrAdminExists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// A row left by an install that got this far and no further is
		// taken over rather than tripping the unique index.
		var existing BlogUser
		err = tx.Where("provider = ? AND provider_id = ?", ProviderPassword, email).First(&existing).Error
		switch {
		case err == nil:
			user.ID = existing.ID
			if err := tx.Save(user).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(user).Error; err != nil {
				return err
			}
		default:
			return err
		}
		return tx.Create(&AdminUser{BlogUserID: user.ID}).Error
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

// PasswordLoginEnabled reports whether the login page should offer a
// password form: whether any password user exists.
func (a *Auth) PasswordLoginEnabled() bool {
	var user BlogUser
	return (*a.db).Where("provider = ?", ProviderPassword).First(&user).Error == nil
}

// StartSession logs user in on this browser, exactly as the other login
// flows do: the user's AccessToken goes into the session.
func (a *Auth) StartSession(c *gin.Context, user *BlogUser) error {
	session := sessions.Default(c)
	session.Set("token", user.AccessToken)
	return session.Save()
}

// PasswordLogin handles the login page's password form (fields: email,
// password) and redirects: to next on success, back to the password login
// page with login_error set otherwise. next must already be a safe same-site path.
// A wrong password and an unknown email are indistinguishable, in the
// answer and in how long it takes.
func (a *Auth) PasswordLogin(c *gin.Context, next string) {
	back := func(reason string) {
		c.Redirect(http.StatusSeeOther, "/login?password=1&login_error="+reason+"&next="+url.QueryEscape(next))
	}
	if !a.passwordLimiter.allow(c.ClientIP(), time.Now()) {
		back("rate_limit")
		return
	}
	email, _ := normalizeEmail(c.PostForm("email"))
	password := c.PostForm("password")

	var user BlogUser
	found := email != "" &&
		(*a.db).Where("provider = ? AND provider_id = ?", ProviderPassword, email).First(&user).Error == nil
	hash := dummyHash
	if found && user.PasswordHash != "" {
		hash = []byte(user.PasswordHash)
	}
	matches := len(password) <= maxPasswordBytes && bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
	if !found || user.PasswordHash == "" || !matches {
		back("invalid")
		return
	}

	token, err := newSessionToken()
	if err != nil {
		log.Printf("generating session token: %v", err)
		back("server")
		return
	}
	user.AccessToken = token
	if err := (*a.db).Model(&user).UpdateColumn("access_token", token).Error; err != nil {
		log.Printf("storing session token for %s: %v", email, err)
		back("server")
		return
	}
	if err := a.StartSession(c, &user); err != nil {
		log.Printf("saving session: %v", err)
		back("server")
		return
	}
	c.Redirect(http.StatusSeeOther, next)
}

// ResetPassword gives a password user a new random password and returns the
// user's email with it. With email "" it resets the only password user and
// fails if there is more than one. The user's session token is replaced too,
// so anyone logged in as them is logged out. For the reset-admin-password
// command: without SMTP there is no other way back in.
func (a *Auth) ResetPassword(email string) (string, string, error) {
	var users []BlogUser
	q := (*a.db).Where("provider = ?", ProviderPassword)
	if email != "" {
		normalized, ok := normalizeEmail(email)
		if !ok {
			return "", "", ErrNoPasswordUser
		}
		q = q.Where("provider_id = ?", normalized)
	}
	if err := q.Find(&users).Error; err != nil {
		return "", "", err
	}
	if len(users) == 0 {
		return "", "", ErrNoPasswordUser
	}
	if len(users) > 1 {
		return "", "", errors.New("more than one password user; name one by email")
	}
	password, err := randomPassword()
	if err != nil {
		return "", "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	token, err := newSessionToken()
	if err != nil {
		return "", "", err
	}
	err = (*a.db).Model(&users[0]).Updates(map[string]any{"password_hash": string(hash), "access_token": token}).Error
	if err != nil {
		return "", "", err
	}
	return users[0].ProviderID, password, nil
}

// randomPassword returns 20 characters from the setup-code alphabet (no
// look-alikes), about 99 bits.
func randomPassword() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i, b := range raw {
		raw[i] = setupAlphabet[int(b)%len(setupAlphabet)]
	}
	return string(raw), nil
}
