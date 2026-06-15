package glossary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/oukeidos/focst-local/internal/files"
)

func SaveArtifact(path string, artifact Artifact) error {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode glossary artifact: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create glossary artifact directory: %w", err)
	}
	return files.AtomicWrite(path, data, 0600)
}

func LoadArtifact(path string) (Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("failed to read glossary artifact %s: %w", path, err)
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("failed to parse glossary artifact %s: %w", path, err)
	}
	if artifact.Version != 1 {
		return Artifact{}, fmt.Errorf("unsupported glossary artifact version: %d", artifact.Version)
	}
	if artifact.PromptVersion != PromptVersion {
		return Artifact{}, fmt.Errorf("unsupported glossary prompt version: %s", artifact.PromptVersion)
	}
	return artifact, nil
}

func ChecksumFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func Mapping(entries []Entry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		out[entry.Source] = entry.Rendering
	}
	return out
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return files.AtomicWrite(path, data, 0600)
}

func NamesCompatible(entries []Entry, sourceCode, targetCode string) []map[string]string {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Source < sorted[j].Source
	})
	out := make([]map[string]string, 0, len(sorted))
	for _, entry := range sorted {
		out = append(out, map[string]string{
			sourceCode: entry.Source,
			targetCode: entry.Rendering,
		})
	}
	return out
}
