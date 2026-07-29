//go:build windows

package runtimedelegate

import (
	"io/fs"

	"golang.org/x/sys/windows"
)

func ownedByCurrentUser(file string, _ fs.FileInfo) bool {
	descriptor, err := windows.GetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return false
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && user != nil && user.User.Sid != nil && owner.Equals(user.User.Sid)
}
