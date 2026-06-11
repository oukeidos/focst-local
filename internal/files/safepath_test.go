package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafePath_NoChange(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "output.srt")
	got, changed, err := SafePath(path)
	if err != nil {
		t.Fatalf("SafePath failed: %v", err)
	}
	if changed {
		t.Fatalf("expected unchanged path")
	}
	if got != path {
		t.Fatalf("expected %q, got %q", path, got)
	}
}

func TestSafePath_WithCollision(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "output.srt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	got, changed, err := SafePath(path)
	if err != nil {
		t.Fatalf("SafePath failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed path")
	}
	if got == path {
		t.Fatalf("expected different path")
	}
}
