//go:build ignore

// This fixture helper runs with `go run` from the root module. E2E databases
// need initial passwords before the public reset-redemption flow exists.
package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"os"
)

func main() {
	password, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
	if err != nil {
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.MinCost)
	if err != nil {
		os.Exit(1)
	}
	if _, err := fmt.Fprintln(os.Stdout, string(hash)); err != nil {
		os.Exit(1)
	}
}
