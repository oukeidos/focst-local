package srt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSave_Atomic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.srt")

	// 1. Initial save
	segments := []Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"Hello"}},
	}
	if err := Save(path, segments); err != nil {
		t.Fatalf("Initial Save failed: %v", err)
	}

	// Verify file exists and has content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if !strings.Contains(string(content), "Hello") {
		t.Errorf("Saved content missing expected text: %s", string(content))
	}

	// 2. Save with different content (Atomic replacement)
	newSegments := []Segment{
		{ID: 1, StartTime: "00:00:03,000", EndTime: "00:00:04,000", Lines: []string{"World"}},
	}
	if err := Save(path, newSegments); err != nil {
		t.Fatalf("Atomic Save failed: %v", err)
	}

	// Verify file is updated
	newContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}
	if !strings.Contains(string(newContent), "World") || strings.Contains(string(newContent), "Hello") {
		t.Errorf("Updated content incorrect: %s", string(newContent))
	}

	// 3. Verify no temp files left behind
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read tmp dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "focst-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("Leaked temp file found: %s", entry.Name())
		}
	}
}

func TestSave_DirectoryError(t *testing.T) {
	// Use a path in a non-existent directory
	invalidPath := "/non/existent/path/test.srt"
	segments := []Segment{{ID: 1, Lines: []string{"Test"}}}

	err := Save(invalidPath, segments)
	if err == nil {
		t.Errorf("Expected error for invalid directory path, got nil")
	}
}
