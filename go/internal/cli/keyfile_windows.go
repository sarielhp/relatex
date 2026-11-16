//go:build windows

package cli

import (
	"fmt"
	"io/fs"
	"unsafe"

	"golang.org/x/sys/windows"
)

// readAccess are the access rights that let a holder read a file's content.
const readAccess = 0x1 /* FILE_READ_DATA */ | windows.GENERIC_READ | windows.GENERIC_ALL

// keyFileExposure is why others may read the key file, or "" when only its owner can (decision
// 129, D2): Windows reports every file as mode 0666, so the rule reads the file's access list
// instead. An entry that grants read to anyone but the owner, SYSTEM or the Administrators
// exposes it; so does a file with no access list at all.
func keyFileExposure(path string, _ fs.FileInfo) string {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Sprintf("has an access list that cannot be read (%v)", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Sprintf("has no readable owner (%v)", err)
	}
	list, _, err := descriptor.DACL()
	if err != nil || list == nil {
		return "has no access list, so everyone may read it"
	}
	for index := uint16(0); index < list.AceCount; index++ {
		var entry *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(list, uint32(index), &entry) != nil {
			return "has an access list that cannot be read"
		}
		if entry.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || entry.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 ||
			entry.Mask&readAccess == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&entry.SidStart))
		if sid.Equals(owner) || isCurrentUser(sid) || sid.IsWellKnown(windows.WinLocalSystemSid) ||
			sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			continue
		}
		return "may be read by " + accountName(sid)
	}
	return ""
}

// isCurrentUser: the user running relatex may read its own key. A file an administrator creates
// is owned by the Administrators group, not by the user, so the owner alone does not name them.
func isCurrentUser(sid *windows.SID) bool {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && sid.Equals(user.User.Sid)
}

func accountName(sid *windows.SID) string {
	account, domain, _, err := sid.LookupAccount("")
	if err != nil {
		return sid.String()
	}
	if domain == "" {
		return account
	}
	return domain + `\` + account
}

// keyFileRemedy is how to make the key file the owner's alone.
func keyFileRemedy(path string) string {
	return fmt.Sprintf(`icacls "%s" /inheritance:r /grant:r "%%USERNAME%%:R"`, path)
}
