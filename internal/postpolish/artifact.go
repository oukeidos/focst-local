package postpolish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/oukeidos/focst-local/internal/files"
)

func SaveArtifact(path string, artifact Artifact) error {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode post-polish artifact: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create post-polish artifact directory: %w", err)
	}
	return files.AtomicWrite(path, data, 0600)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return files.AtomicWrite(path, data, 0600)
}

func writeText(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return files.AtomicWrite(path, []byte(value), 0600)
}
