//go:build windows

package sheetsync

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Go exposes only Windows' read-only attribute via FileMode.Perm(), reporting
// 0666 for writable files even if OpenFile was called with 0600. Restrict the
// NTFS DACL to the server account, LOCAL SYSTEM, and Administrators instead.
// This is also applied when reading old keys created before this fix.
func secureCredentialKey(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("identify service account: %w", err)
	}
	// The SID is from the Windows token, not external input.
	sddl := "D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;SY)(A;;GA;;;BA)"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("build private key ACL: %w", err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read private key ACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		return fmt.Errorf("protect private key ACL: %w", err)
	}
	return nil
}
