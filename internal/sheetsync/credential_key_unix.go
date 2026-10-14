//go:build !windows

package sheetsync

import (
	"errors"
	"os"
)

// On Unix the encryption key must be private to the server account.
func secureCredentialKey(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.New("service-account key file must be regular and mode 0600")
	}
	return nil
}
