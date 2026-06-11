//go:build !windows

package files

import "os"

func renameAtomic(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
