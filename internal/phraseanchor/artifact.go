package phraseanchor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

func SaveArtifact(path string, artifact Artifact) error {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode phrase anchors artifact: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create phrase anchors artifact directory: %w", err)
	}
	return files.AtomicWrite(path, data, 0600)
}

func LoadArtifact(path string) (Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("failed to read phrase anchors artifact %s: %w", path, err)
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("failed to parse phrase anchors artifact %s: %w", path, err)
	}
	if artifact.Version != 1 {
		return Artifact{}, fmt.Errorf("unsupported phrase anchors artifact version: %d", artifact.Version)
	}
	if artifact.PromptVersion != PromptVersion {
		return Artifact{}, fmt.Errorf("unsupported phrase anchors prompt version: %s", artifact.PromptVersion)
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

func ValidateArtifactForSegments(artifact Artifact, segments []srt.Segment, sourceLang, targetLang, checksum string) error {
	if artifact.SourceLang != sourceLang {
		return fmt.Errorf("phrase anchors source language mismatch: artifact=%s current=%s", artifact.SourceLang, sourceLang)
	}
	if artifact.TargetLang != targetLang {
		return fmt.Errorf("phrase anchors target language mismatch: artifact=%s current=%s", artifact.TargetLang, targetLang)
	}
	if artifact.Input.SegmentsChecksum != checksum {
		return fmt.Errorf("phrase anchors segments checksum mismatch: artifact=%s current=%s", artifact.Input.SegmentsChecksum, checksum)
	}
	if len(artifact.ChunkPlan.Chunks) == 0 {
		return fmt.Errorf("phrase anchors artifact has empty chunk plan")
	}
	if err := validateChunkPlan(artifact.ChunkPlan, segments); err != nil {
		return err
	}
	sourceByID := map[int]string{}
	for _, segment := range segments {
		sourceByID[segment.ID] = translation.SourceTextFromLines(segment.Lines)
	}
	for _, entry := range artifact.Entries {
		sourceText, ok := sourceByID[entry.SegmentID]
		if !ok {
			return fmt.Errorf("phrase anchors entry references unknown segment ID %d", entry.SegmentID)
		}
		if strings.TrimSpace(entry.SourceText) != sourceText {
			return fmt.Errorf("phrase anchors source text mismatch for segment ID %d", entry.SegmentID)
		}
		if strings.TrimSpace(entry.SourceQuote) == "" || !strings.Contains(sourceText, entry.SourceQuote) {
			return fmt.Errorf("phrase anchors source quote %q not found in segment ID %d", entry.SourceQuote, entry.SegmentID)
		}
		if strings.TrimSpace(entry.Rendering) == "" {
			return fmt.Errorf("phrase anchors entry has empty rendering for segment ID %d", entry.SegmentID)
		}
	}
	return nil
}

func validateChunkPlan(plan chunker.ChunkPlan, segments []srt.Segment) error {
	expectedStart := 0
	for i, chunk := range plan.Chunks {
		if chunk.Index != i {
			return fmt.Errorf("phrase anchors chunk plan index mismatch at %d: got %d", i, chunk.Index)
		}
		if chunk.StartIndex != expectedStart {
			return fmt.Errorf("phrase anchors chunk plan is not continuous at %d: got start %d want %d", i, chunk.StartIndex, expectedStart)
		}
		if chunk.EndIndex <= chunk.StartIndex || chunk.EndIndex > len(segments) {
			return fmt.Errorf("phrase anchors invalid chunk range at %d: %d..%d", i, chunk.StartIndex, chunk.EndIndex)
		}
		if segments[chunk.StartIndex].ID != chunk.StartID {
			return fmt.Errorf("phrase anchors chunk start ID mismatch at %d: got %d want %d", i, chunk.StartID, segments[chunk.StartIndex].ID)
		}
		if segments[chunk.EndIndex-1].ID != chunk.EndID {
			return fmt.Errorf("phrase anchors chunk end ID mismatch at %d: got %d want %d", i, chunk.EndID, segments[chunk.EndIndex-1].ID)
		}
		expectedStart = chunk.EndIndex
	}
	if expectedStart != len(segments) {
		return fmt.Errorf("phrase anchors chunk plan does not cover all segments: covered %d of %d", expectedStart, len(segments))
	}
	return nil
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

func WriteText(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return files.AtomicWrite(path, []byte(value), 0600)
}
