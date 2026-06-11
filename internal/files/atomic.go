package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oukeidos/focst-local/internal/logger"
)

// AtomicWrite writes data to a temp file and renames it into place.
func AtomicWrite(path string, data []byte, perms os.FileMode) error {
	if err := RejectSymlinkPath(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "focst-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanup := true
	defer func() {
		if cleanup {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	if err := tmpFile.Chmod(perms); err != nil {
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := renameAtomic(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file to destination: %w", err)
	}
	if err := syncDir(dir); err != nil {
		logger.Warn("Directory fsync failed (safe to ignore on some platforms)", "path", dir, "error", err)
	}

	cleanup = false
	return nil
}

// AtomicWriteExclusive writes data to a temp file and renames it into place,
// retrying with a numbered suffix to avoid collisions.
func AtomicWriteExclusive(path string, data []byte, perms os.FileMode) error {
	if err := RejectSymlinkPath(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	ext := filepath.Ext(path)

	var lastErr error
	for i := 0; i < 10; i++ {
		candidate := path
		if i > 0 {
			candidate = filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		}
		tmpPath := candidate + ".tmp"

		tmp, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perms)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				lastErr = err
				continue
			}
			return err
		}

		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
		if err := tmp.Sync(); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpPath)
			return err
		}

		if err := renameAtomic(tmpPath, candidate); err != nil {
			os.Remove(tmpPath)
			return err
		}
		if err := syncDir(dir); err != nil {
			logger.Warn("Directory fsync failed (safe to ignore on some platforms)", "path", dir, "error", err)
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("failed to create log file")
}

func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		logger.Info("Directory fsync not supported on Windows; skipping", "path", dir)
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
