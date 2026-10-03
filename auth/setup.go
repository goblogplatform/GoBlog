package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// The setup code (#654, #658). Until a site has an admin, its install
// wizard has to work for somebody who cannot log in, and without a check it
// works for anybody who finds the site first. So goblog prints a random
// code to its log at startup and the wizard asks for it: being able to read
// the server's log is the proof of owning the server.
//
// The code lives in memory only. A restart makes a new one, which also
// invalidates every browser that was unlocked with the old one. It is
// package state rather than a field of Auth because Auth values are copied
// (the wizard builds its own) and they must all agree.

const (
	setupSessionKey = "setup_code"
	// No 0/O, 1/I/L: the code is read off a terminal and typed.
	setupAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	setupLength   = 12
)

var setup struct {
	mu   sync.RWMutex
	code string // "" means locked: nothing unlocks the wizard
}

// NewSetupCode makes and remembers a new setup code and returns it
// formatted for printing (XXXX-XXXX-XXXX).
func NewSetupCode() (string, error) {
	raw := make([]byte, setupLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := make([]byte, setupLength)
	for i, b := range raw {
		// 31 symbols: the modulo bias is far below what a 12-symbol code
		// with a rate limit in front of it could ever care about.
		code[i] = setupAlphabet[int(b)%len(setupAlphabet)]
	}
	setup.mu.Lock()
	setup.code = string(code)
	setup.mu.Unlock()
	return string(code[0:4]) + "-" + string(code[4:8]) + "-" + string(code[8:12]), nil
}

// ClearSetupCode forgets the setup code, locking the wizard again. Called
// once the site has an admin.
func ClearSetupCode() {
	setup.mu.Lock()
	setup.code = ""
	setup.mu.Unlock()
}

func currentSetupCode() string {
	setup.mu.RLock()
	defer setup.mu.RUnlock()
	return setup.code
}

// normalizeSetupCode makes what a person typed comparable: case, dashes and
// spaces do not matter.
func normalizeSetupCode(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
}

func setupCodeMatches(given string) bool {
	want := currentSetupCode()
	return want != "" && subtle.ConstantTimeCompare([]byte(normalizeSetupCode(given)), []byte(want)) == 1
}

// ErrSetupCodeWrong and ErrSetupRateLimited are UnlockSetup's refusals.
var (
	ErrSetupCodeWrong   = errors.New("that is not the setup code")
	ErrSetupRateLimited = errors.New("too many attempts; wait a few minutes and try again")
)

// setupLimiter throttles guesses at the setup code per client IP.
var setupLimiter = newLimiter(10, 10*time.Minute)

// UnlockSetup checks code and, when it is right, remembers that in the
// browser's session.
func UnlockSetup(c *gin.Context, code string) error {
	if !setupLimiter.allow(c.ClientIP(), time.Now()) {
		return ErrSetupRateLimited
	}
	if !setupCodeMatches(code) {
		return ErrSetupCodeWrong
	}
	session := sessions.Default(c)
	session.Set(setupSessionKey, currentSetupCode())
	return session.Save()
}

// SetupUnlocked reports whether this browser has entered the current setup
// code. The session holds the code itself, not a flag, so a code from before
// a restart no longer counts.
func SetupUnlocked(c *gin.Context) bool {
	given, _ := sessions.Default(c).Get(setupSessionKey).(string)
	return given != "" && setupCodeMatches(given)
}
