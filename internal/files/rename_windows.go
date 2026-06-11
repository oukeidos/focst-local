//go:build windows

package files

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func renameAtomic(oldPath, newPath string) error {
	oldPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return fmt.Errorf("invalid source path: %w", err)
	}
	newPtr, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return fmt.Errorf("invalid destination path: %w", err)
	}

	flags := uint32(windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH)
	if err := windows.MoveFileEx(oldPtr, newPtr, flags); err != nil {
		return err
	}
	return nil
}
