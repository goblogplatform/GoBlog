package main

import (
	"errors"
	"fmt"
	"io"

	"goblog/auth"
)

// runResetAdminPassword implements `goblog reset-admin-password [email]`:
// it gives the password admin a new random password and prints it. Without
// SMTP there is no "forgot my password" email, so the way back in is being
// able to run a command on the server. Run it from goblog's working
// directory (or with GOBLOG_DATA_DIR set) so it finds .env. Returns the
// process exit code.
func runResetAdminPassword(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: goblog reset-admin-password [email]")
		return 2
	}
	email := ""
	if len(args) == 1 {
		email = args[0]
	}
	db := attemptConnectDb()
	if db == nil {
		fmt.Fprintln(stderr, "could not connect to the database configured in .env")
		return 1
	}
	a := auth.New(db, Version)
	who, password, err := a.ResetPassword(email)
	if errors.Is(err, auth.ErrNoPasswordUser) {
		fmt.Fprintln(stderr, "there is no password login for that email on this site")
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "could not reset the password: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "New password for %s: %s\n", who, password)
	return 0
}
