package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RejectSymlinkPath returns an error if the path or its parent directory is a symlink.
func RejectSymlinkPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is empty")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	return rejectSymlinkComponents(abs)
}

func rejectSymlinkComponents(path string) error {
	volume := filepath.VolumeName(path)
	rest := path[len(volume):]
	rest = strings.TrimLeft(rest, string(os.PathSeparator))

	var current string
	if volume != "" {
		current = volume + string(os.PathSeparator)
	} else if filepath.IsAbs(path) {
		current = string(os.PathSeparator)
	}

	if rest == "" {
		return nil
	}

	parts := strings.Split(rest, string(os.PathSeparator))
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("failed to access path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write to symlink path: %s (symlink detected at %s)", path, current)
		}
		if isReparse, err := isReparsePoint(current); err != nil {
			return fmt.Errorf("failed to check reparse point: %w", err)
		} else if isReparse {
			return fmt.Errorf("refusing to write to symlink path: %s (reparse point detected at %s)", path, current)
		}
	}
	return nil
}
