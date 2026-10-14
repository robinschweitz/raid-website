//go:build windows

package sheetsync

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsCredentialKeyUsesProtectedDACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.key")
	ciphertext, err := encryptCredential(path, "sensitive-test-credential")
	if err != nil {
		t.Fatal(err)
	}
	// Protection must survive opening the same key on the next request.
	if plaintext, err := decryptCredential(path, ciphertext); err != nil || plaintext != "sensitive-test-credential" {
		t.Fatalf("key not readable after restart: %v", err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("key file must not inherit broad directory ACLs")
	}
	desc := sd.String()
	if !strings.Contains(desc, "D:P") || strings.Contains(desc, ";;;WD") || strings.Contains(desc, ";;;BU") {
		t.Fatalf("key ACL not private: %s", desc)
	}
}
