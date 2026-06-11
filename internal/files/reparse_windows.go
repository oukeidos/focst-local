//go:build windows

package files

import "golang.org/x/sys/windows"

func isReparsePoint(path string) (bool, error) {
	attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path))
	if err != nil {
		return false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}
