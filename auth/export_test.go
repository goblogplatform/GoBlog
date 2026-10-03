package auth

import "time"

// ResetSetupLimiter gives tests a fresh setup-code rate limiter: it is
// package state, and every test request comes from the same address.
func ResetSetupLimiter() { setupLimiter = newLimiter(10, 10*time.Minute) }
