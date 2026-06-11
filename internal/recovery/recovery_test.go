package recovery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/chunker"
)

func TestSaveSessionLog_Permissions(t *testing.T) {
	// Skip on Windows as permission bits work differently
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir, err := os.MkdirTemp("", "focst-recovery-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "test_recovery.json")
	log := &SessionLog{
		LogVersion:   CurrentLogVersion,
		InputPath:    "test.srt",
		TotalChunks:  10,
		FailedChunks: []int{1, 2},
		StatusReason: "canceled",
	}

	err = SaveSessionLog(path, log)
	if err != nil {
		t.Fatalf("SaveSessionLog failed: %v", err)
	}

	loaded, err := LoadSessionLog(path)
	if err != nil {
		t.Fatalf("LoadSessionLog failed: %v", err)
	}
	if loaded.StatusReason != "canceled" {
		t.Fatalf("expected StatusReason to persist, got %q", loaded.StatusReason)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("expected permission 0600, got %o", mode)
	}
}

func TestSaveSessionLog_Exclusive(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "focst-recovery-exclusive-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "test_recovery.json")
	log := &SessionLog{
		LogVersion:   CurrentLogVersion,
		InputPath:    "test.srt",
		TotalChunks:  10,
		FailedChunks: []int{1, 2},
	}

	if err := SaveSessionLog(path, log); err != nil {
		t.Fatalf("SaveSessionLog failed: %v", err)
	}
	if err := SaveSessionLog(path, log); err != nil {
		t.Fatalf("SaveSessionLog should retry with a new name: %v", err)
	}
}

func TestGenerateRecoveryPath(t *testing.T) {
	tests := []struct {
		name      string
		inputPath string
		wantDir   string
	}{
		{
			name:      "current directory",
			inputPath: "input.srt",
			wantDir:   ".",
		},
		{
			name:      "absolute path",
			inputPath: "/tmp/data/file.srt",
			wantDir:   "/tmp/data",
		},
		{
			name:      "relative path",
			inputPath: "sub/dir/test.srt",
			wantDir:   "sub/dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRecoveryPath(tt.inputPath)
			gotDir := filepath.Dir(got)

			// Clean paths for comparison
			if filepath.Clean(gotDir) != filepath.Clean(tt.wantDir) {
				t.Errorf("GenerateRecoveryPath(%q) = %q; want directory %q", tt.inputPath, got, tt.wantDir)
			}

			if !strings.Contains(filepath.Base(got), "input_recovery") &&
				!strings.Contains(filepath.Base(got), "file_recovery") &&
				!strings.Contains(filepath.Base(got), "test_recovery") {
				t.Errorf("GenerateRecoveryPath(%q) = %q; missing expected base name", tt.inputPath, got)
			}
		})
	}
}

func TestSessionLog_Validate_Security(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "focst-security-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.srt")
	if err := os.WriteFile(inputPath, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	validLog := &SessionLog{
		LogVersion:       CurrentLogVersion,
		InputPath:        "input.srt",
		OutputPath:       "output.srt", // Relative path (valid)
		InputHash:        "sha256:dummy",
		SegmentsChecksum: "sha256:dummy",
		Model:            "gemma-4-26b-a4b-qat-q4_0",
		Provider:         "llama.cpp",
		BaseURL:          "http://127.0.0.1:8080/v1",
		MaxTokens:        4096,
		ChunkSize:        100,
		ContextSize:      0,
		Concurrency:      1,
		TotalChunks:      1,
		FailedChunks:     []int{0},
		SourceLang:       "en",
		TargetLang:       "ko",
		Status:           "Failure",
	}

	t.Run("Valid Relative Path", func(t *testing.T) {
		if err := validLog.Validate(); err != nil {
			t.Errorf("expected valid log to pass, got error: %v", err)
		}
	})

	t.Run("Empty OutputPath", func(t *testing.T) {
		log := *validLog
		log.OutputPath = ""
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "output_path is empty") {
			t.Errorf("expected error for empty output_path, got: %v", err)
		}
	})

	t.Run("Absolute OutputPath is rejected", func(t *testing.T) {
		log := *validLog
		log.OutputPath = "/etc/passwd"
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "output_path must be relative") {
			t.Errorf("expected error for absolute output_path, got: %v", err)
		}
	})

	t.Run("Path traversal is rejected", func(t *testing.T) {
		log := *validLog
		log.OutputPath = "../../../etc/passwd"
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "cannot traverse parent directories") {
			t.Errorf("expected error for path traversal, got: %v", err)
		}
	})

	t.Run("Absolute InputPath is rejected", func(t *testing.T) {
		log := *validLog
		log.InputPath = inputPath
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "input_path must be relative") {
			t.Errorf("expected error for absolute input_path, got: %v", err)
		}
	})

	t.Run("Old log version is rejected", func(t *testing.T) {
		log := *validLog
		log.LogVersion = CurrentLogVersion - 1
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported log_version") {
			t.Errorf("expected unsupported log_version error, got: %v", err)
		}
	})

	t.Run("Invalid Concurrency", func(t *testing.T) {
		log := *validLog
		log.Concurrency = 0
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "invalid concurrency") {
			t.Errorf("expected error for invalid concurrency, got: %v", err)
		}
	})

	t.Run("Invalid ContextSize", func(t *testing.T) {
		log := *validLog
		log.ContextSize = -1
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "invalid context_size") {
			t.Errorf("expected error for invalid context_size, got: %v", err)
		}
	})
}

func TestPathResolutionForSessionLog(t *testing.T) {
	baseDir := t.TempDir()
	logPath := filepath.Join(baseDir, "out_recovery.json")
	inputAbs := filepath.Join(baseDir, "sub", "input.srt")
	if err := os.MkdirAll(filepath.Dir(inputAbs), 0o700); err != nil {
		t.Fatalf("failed to create input directory: %v", err)
	}
	if err := os.WriteFile(inputAbs, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	relInput, err := ToRelativeInputPath(logPath, inputAbs)
	if err != nil {
		t.Fatalf("ToRelativeInputPath failed: %v", err)
	}
	if filepath.IsAbs(relInput) {
		t.Fatalf("expected relative input path, got %q", relInput)
	}

	resolvedInput := ResolveInputPath(logPath, relInput)
	if filepath.Clean(resolvedInput) != filepath.Clean(inputAbs) {
		t.Fatalf("resolved input mismatch: got %q want %q", resolvedInput, inputAbs)
	}

	outsideOutput := filepath.Join(baseDir, "..", "elsewhere", "out.srt")
	if _, err := ToRelativeOutputPath(logPath, outsideOutput); err == nil {
		t.Fatalf("expected ToRelativeOutputPath to reject paths outside log directory")
	}
}

func TestSessionLog_ValidateSentenceAwareChunkPlan(t *testing.T) {
	base := &SessionLog{
		LogVersion:           CurrentLogVersion,
		InputPath:            "input.srt",
		OutputPath:           "output.srt",
		InputHash:            "sha256:dummy",
		SegmentsChecksum:     "sha256:dummy",
		Model:                "gemma-4-26b-a4b-qat-q4_0",
		Provider:             "llama.cpp",
		BaseURL:              "http://127.0.0.1:8080/v1",
		MaxTokens:            4096,
		ChunkSize:            100,
		ContextSize:          5,
		SentenceAwareChunks:  true,
		MinChunkSize:         90,
		MaxChunkSize:         110,
		ChunkBoundaryPlanner: "local-llm",
		Concurrency:          1,
		TotalChunks:          1,
		FailedChunks:         []int{0},
		SourceLang:           "en",
		TargetLang:           "ko",
		Status:               "Failure",
	}

	t.Run("missing chunk plan rejected", func(t *testing.T) {
		log := *base
		if err := log.Validate(); err == nil || !strings.Contains(err.Error(), "chunk_plan is required") {
			t.Fatalf("expected chunk_plan required error, got: %v", err)
		}
	})

	t.Run("valid saved chunk plan accepted", func(t *testing.T) {
		log := *base
		log.ChunkPlan = &chunker.ChunkPlan{Chunks: []chunker.PlannedChunk{
			{Index: 0, StartIndex: 0, EndIndex: 42, StartID: 1, EndID: 42},
		}}
		if err := log.Validate(); err != nil {
			t.Fatalf("expected valid sentence-aware log, got: %v", err)
		}
	})
}
