package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/recovery"
	"github.com/oukeidos/focst-local/internal/srt"
)

func writeSessionLog(t *testing.T, dir string, log *recovery.SessionLog) string {
	t.Helper()
	path := filepath.Join(dir, "session_log.json")
	if err := recovery.SaveSessionLog(path, log); err != nil {
		t.Fatalf("SaveSessionLog failed: %v", err)
	}
	return path
}

func buildRecoveryLog(t *testing.T, inputPath, outputPath string, noPreprocess bool) *recovery.SessionLog {
	t.Helper()

	inputHash, err := recovery.HashFileHex(inputPath)
	if err != nil {
		t.Fatalf("HashFileHex failed: %v", err)
	}
	segments, err := srt.Load(inputPath)
	if err != nil {
		t.Fatalf("failed to load subtitle file: %v", err)
	}
	if !noPreprocess {
		segments = srt.Preprocess(segments, "en")
	}
	segmentsChecksum := srt.SegmentsChecksumHex(segments)

	return &recovery.SessionLog{
		LogVersion:       recovery.CurrentLogVersion,
		InputPath:        filepath.Base(inputPath),
		OutputPath:       outputPath,
		InputHash:        inputHash,
		SegmentsChecksum: segmentsChecksum,
		Model:            localllm.DefaultModel,
		Provider:         "llama.cpp",
		BaseURL:          localllm.DefaultBaseURL,
		MaxTokens:        localllm.DefaultMaxTokens,
		ChunkSize:        10,
		ContextSize:      0,
		Concurrency:      1,
		SourceLang:       "en",
		TargetLang:       "ko",
		FailedChunks:     []int{0},
		TotalChunks:      1,
		Status:           "Failure",
		NoPreprocess:     noPreprocess,
	}
}

func TestRunRepair_ValidationOrder(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.srt")
	if err := os.WriteFile(inputPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0600); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	validLog := buildRecoveryLog(t, inputPath, "output.srt", true)

	t.Run("Runtime validation happens after log validation", func(t *testing.T) {
		logPath := writeSessionLog(t, tmpDir, validLog)
		cfg := Config{
			LogPath: logPath,
			BaseURL: "http://127.0.0.1:1/v1",
		}

		_, err := RunRepair(context.Background(), cfg)
		if err == nil || !strings.Contains(err.Error(), "local LLM is not ready") {
			t.Fatalf("expected local LLM readiness error, got: %v", err)
		}
		if strings.Contains(err.Error(), "chunkSize") {
			t.Fatalf("unexpected chunkSize error: %v", err)
		}
	})

	t.Run("Invalid log fails before runtime config validation", func(t *testing.T) {
		invalidLog := *validLog
		invalidLog.ChunkSize = 0
		logPath := writeSessionLog(t, tmpDir, &invalidLog)
		cfg := Config{
			LogPath: logPath,
		}

		_, err := RunRepair(context.Background(), cfg)
		if err == nil || !strings.Contains(err.Error(), "invalid chunk_size") {
			t.Fatalf("expected invalid chunk_size error, got: %v", err)
		}
	})
}

func TestRunRepair_InputHashMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.srt")
	if err := os.WriteFile(inputPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0600); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	log := buildRecoveryLog(t, inputPath, "output.srt", true)
	logPath := writeSessionLog(t, tmpDir, log)

	if err := os.WriteFile(inputPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nChanged\n"), 0600); err != nil {
		t.Fatalf("failed to modify input file: %v", err)
	}

	cfg := Config{
		LogPath: logPath,
		BaseURL: "http://127.0.0.1:1/v1",
	}
	_, err := RunRepair(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "input file content mismatch") {
		t.Fatalf("expected input hash mismatch error, got: %v", err)
	}
}

func TestRunRepair_SegmentChecksumMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.srt")
	if err := os.WriteFile(inputPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0600); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	log := buildRecoveryLog(t, inputPath, "output.srt", true)
	log.SegmentsChecksum = "sha256:deadbeef"
	logPath := writeSessionLog(t, tmpDir, log)

	cfg := Config{
		LogPath: logPath,
		BaseURL: "http://127.0.0.1:1/v1",
	}
	_, err := RunRepair(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "segment checksum mismatch") {
		t.Fatalf("expected segment checksum mismatch error, got: %v", err)
	}
}

func TestRunRepair_ResolvesRelativeInputPathFromLogDir(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.srt")
	if err := os.WriteFile(inputPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0600); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	log := buildRecoveryLog(t, inputPath, "output.srt", true)
	logPath := writeSessionLog(t, tmpDir, log)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	otherDir := t.TempDir()
	if err := os.Chdir(otherDir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", otherDir, err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	cfg := Config{
		LogPath: logPath,
		BaseURL: "http://127.0.0.1:1/v1",
	}
	_, err = RunRepair(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "local LLM is not ready") {
		t.Fatalf("expected local LLM readiness error after successful log-path resolution, got: %v", err)
	}
	if strings.Contains(err.Error(), "input file not found") {
		t.Fatalf("input path should resolve relative to log dir, got: %v", err)
	}
}

func TestResolveRuntimeSessionLog_DoesNotMutateOriginalPaths(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.srt")
	namesPath := filepath.Join(tmpDir, "names.json")
	if err := os.WriteFile(inputPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0600); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}
	if err := os.WriteFile(namesPath, []byte(`[{"en":"John","ko":"존"}]`), 0600); err != nil {
		t.Fatalf("failed to create names file: %v", err)
	}

	logFile := buildRecoveryLog(t, inputPath, "output.srt", true)
	logFile.NamesPath = filepath.Base(namesPath)
	logPath := filepath.Join(tmpDir, "session_log.json")

	runtimeLog, err := resolveRuntimeSessionLog(logPath, logFile)
	if err != nil {
		t.Fatalf("resolveRuntimeSessionLog failed: %v", err)
	}

	wantInput := filepath.Join(tmpDir, filepath.Base(inputPath))
	wantNames := filepath.Join(tmpDir, filepath.Base(namesPath))
	if filepath.Clean(runtimeLog.InputPath) != filepath.Clean(wantInput) {
		t.Fatalf("runtime input path = %q, want %q", runtimeLog.InputPath, wantInput)
	}
	if filepath.Clean(runtimeLog.NamesPath) != filepath.Clean(wantNames) {
		t.Fatalf("runtime names path = %q, want %q", runtimeLog.NamesPath, wantNames)
	}

	if logFile.InputPath != filepath.Base(inputPath) {
		t.Fatalf("original input path was mutated: %q", logFile.InputPath)
	}
	if logFile.NamesPath != filepath.Base(namesPath) {
		t.Fatalf("original names path was mutated: %q", logFile.NamesPath)
	}
}
