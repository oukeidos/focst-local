package residue

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
		return fmt.Errorf("failed to encode residue artifact: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create residue artifact directory: %w", err)
	}
	return files.AtomicWrite(path, data, 0600)
}

func LoadArtifact(path string) (Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("failed to read residue artifact %s: %w", path, err)
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("failed to parse residue artifact %s: %w", path, err)
	}
	if artifact.Version != ArtifactVersion {
		return Artifact{}, fmt.Errorf("unsupported residue artifact version: %d", artifact.Version)
	}
	return artifact, nil
}
