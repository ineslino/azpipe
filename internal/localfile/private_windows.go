package localfile

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func privateDescriptor(path string) (*windows.SECURITY_DESCRIPTOR, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("ficheiro privado não pode ser um symlink")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	flags := ""
	if info.IsDir() {
		flags = "OICI"
	}
	// Windows administrators and SYSTEM have the same recovery role as Unix root.
	sddl := fmt.Sprintf("D:P(A;%s;FA;;;%s)(A;%s;FA;;;SY)(A;%s;FA;;;BA)", flags, user.User.Sid.String(), flags, flags)
	return windows.SecurityDescriptorFromString(sddl)
}

func Protect(path string) error {
	sd, err := privateDescriptor(path)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func Check(path string) error {
	expected, err := privateDescriptor(path)
	if err != nil {
		return err
	}
	actual, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	canonical := func(sd *windows.SECURITY_DESCRIPTOR) string {
		s := sd.String()
		i := strings.Index(s, "D:")
		if i < 0 {
			return ""
		}
		s = s[i:]
		if i = strings.Index(s, "("); i >= 0 {
			s = strings.ReplaceAll(strings.ReplaceAll(s[:i], "AI", ""), "AR", "") + s[i:]
		}
		return s
	}
	if canonical(actual) != canonical(expected) {
		return fmt.Errorf("ACL local inesperada")
	}
	return nil
}
