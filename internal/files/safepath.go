package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// SafePath returns a non-existing path by appending _1.._9, then a UUID suffix.
// If the original path does not exist, it is returned unchanged.
func SafePath(path string) (string, bool, error) {
	if path == "" {
		return "", false, fmt.Errorf("path is empty")
	}
	if _, err := os.Stat(path); err == nil {
		ext := filepath.Ext(path)
		base := strings.TrimSuffix(path, ext)

		for i := 1; i <= 9; i++ {
			candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				return candidate, true, nil
			} else if err != nil {
				return "", false, err
			}
		}

		u, err := uuid.NewV7()
		uuidSuffix := ""
		if err != nil {
			uuidSuffix = uuid.NewString()[:8]
		} else {
			uuidSuffix = u.String()
		}
		return fmt.Sprintf("%s_%s%s", base, uuidSuffix, ext), true, nil
	} else if os.IsNotExist(err) {
		return path, false, nil
	} else {
		return "", false, err
	}
}
