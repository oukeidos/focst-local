package srt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// GenerateOutputPath creates an output path with a language-specific suffix and handles collisions.
func GenerateOutputPath(inputPath string, targetLang string) string {
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(inputPath, ext)

	// Normalize targetLang (e.g., zh-Hans -> zh) for suffix if desired,
	// but using the full code is often safer/more explicit.
	// User requested _ko for Korean specifically.
	suffixBase := "_" + targetLang

	// Stage 1: Primary
	primary := fmt.Sprintf("%s%s%s", base, suffixBase, ext)
	if _, err := os.Stat(primary); os.IsNotExist(err) {
		return primary
	}

	// Stage 2: Sequential fallback _0 to _9
	for i := 0; i <= 9; i++ {
		candidate := fmt.Sprintf("%s%s_%d%s", base, suffixBase, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}

	// Stage 3: UUID fallback
	u, err := uuid.NewV7()
	var uuidSuffix string
	if err != nil {
		uuidSuffix = uuid.NewString()[:8]
	} else {
		uuidSuffix = u.String()
	}
	return fmt.Sprintf("%s%s_%s%s", base, suffixBase, uuidSuffix, ext)
}
