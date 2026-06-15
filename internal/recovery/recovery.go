package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/localllm"
)

// SessionLog stores the state of a translation session for later repair.
type SessionLog struct {
	LogVersion            int                `json:"log_version"`
	InputPath             string             `json:"input_path"`
	OutputPath            string             `json:"output_path"`
	InputHash             string             `json:"input_hash"`
	SegmentsChecksum      string             `json:"segments_checksum"`
	Model                 string             `json:"model"`
	Provider              string             `json:"provider"`
	BaseURL               string             `json:"base_url"`
	LlamaServerMode       string             `json:"llama_server_mode,omitempty"`
	LlamaServerBin        string             `json:"llama_server_bin,omitempty"`
	LlamaModelPath        string             `json:"llama_model_path,omitempty"`
	LlamaCtxSize          int                `json:"llama_ctx_size,omitempty"`
	LlamaParallel         int                `json:"llama_parallel,omitempty"`
	LlamaExtraArgs        []string           `json:"llama_extra_args,omitempty"`
	LlamaLogFile          string             `json:"llama_log_file,omitempty"`
	MaxTokens             int                `json:"max_tokens"`
	NamesPath             string             `json:"names_path,omitempty"`
	GlossaryPath          string             `json:"glossary_path,omitempty"`
	GlossaryChecksum      string             `json:"glossary_checksum,omitempty"`
	GlossaryPromptVersion string             `json:"glossary_prompt_version,omitempty"`
	GlossaryOverrideCount int                `json:"glossary_override_count,omitempty"`
	ChunkSize             int                `json:"chunk_size"`
	ContextSize           int                `json:"context_size"`
	SentenceAwareChunks   bool               `json:"sentence_aware_chunks,omitempty"`
	MinChunkSize          int                `json:"min_chunk_size,omitempty"`
	MaxChunkSize          int                `json:"max_chunk_size,omitempty"`
	ChunkBoundaryPlanner  string             `json:"chunk_boundary_planner,omitempty"`
	ChunkPlan             *chunker.ChunkPlan `json:"chunk_plan,omitempty"`
	Concurrency           int                `json:"concurrency"`
	NoPreprocess          bool               `json:"no_preprocess"`
	NoPostprocess         bool               `json:"no_postprocess"`
	NoLangPreprocess      bool               `json:"no_lang_preprocess"`
	NoLangPostprocess     bool               `json:"no_lang_postprocess"`
	SourceLang            string             `json:"source_lang"`
	TargetLang            string             `json:"target_lang"`
	FailedChunks          []int              `json:"failed_chunks"`
	TotalChunks           int                `json:"total_chunks"`
	Status                string             `json:"status"` // "Success", "Partial Success", "Failure"
	StatusReason          string             `json:"status_reason,omitempty"`
}

const CurrentLogVersion = 5

// Validate checks if the session log is consistent and safe to resume.
func (log *SessionLog) Validate() error {
	if log.LogVersion == 0 {
		log.LogVersion = CurrentLogVersion
	}
	if log.LogVersion != CurrentLogVersion {
		return fmt.Errorf("unsupported log_version: %d", log.LogVersion)
	}
	if log.InputPath == "" {
		return fmt.Errorf("input_path is empty")
	}
	if filepath.IsAbs(log.InputPath) {
		return fmt.Errorf("input_path must be relative, not absolute: %s", log.InputPath)
	}
	if log.OutputPath == "" {
		return fmt.Errorf("output_path is empty")
	}
	// Security: Reject absolute paths (must be relative to log file)
	if filepath.IsAbs(log.OutputPath) {
		return fmt.Errorf("output_path must be relative, not absolute: %s", log.OutputPath)
	}
	// Security: Reject path traversal attempts
	clean := filepath.Clean(log.OutputPath)
	if strings.HasPrefix(clean, "..") {
		return fmt.Errorf("output_path cannot traverse parent directories: %s", log.OutputPath)
	}
	if log.NamesPath != "" {
		if filepath.IsAbs(log.NamesPath) {
			return fmt.Errorf("names_path must be relative, not absolute: %s", log.NamesPath)
		}
	}
	if log.GlossaryPath != "" {
		if filepath.IsAbs(log.GlossaryPath) {
			return fmt.Errorf("glossary_path must be relative, not absolute: %s", log.GlossaryPath)
		}
		if log.GlossaryChecksum == "" {
			return fmt.Errorf("glossary_checksum is required when glossary_path is set")
		}
		if !strings.HasPrefix(log.GlossaryChecksum, "sha256:") {
			return fmt.Errorf("invalid glossary_checksum: %s", log.GlossaryChecksum)
		}
	}
	if log.InputHash == "" {
		return fmt.Errorf("input_hash is empty")
	}
	if !strings.HasPrefix(log.InputHash, "sha256:") {
		return fmt.Errorf("invalid input_hash: %s", log.InputHash)
	}
	if log.SegmentsChecksum == "" {
		return fmt.Errorf("segments_checksum is empty")
	}
	if !strings.HasPrefix(log.SegmentsChecksum, "sha256:") {
		return fmt.Errorf("invalid segments_checksum: %s", log.SegmentsChecksum)
	}
	if log.ChunkSize <= 0 {
		return fmt.Errorf("invalid chunk_size: %d", log.ChunkSize)
	}
	if log.Concurrency <= 0 {
		return fmt.Errorf("invalid concurrency: %d", log.Concurrency)
	}
	if log.ContextSize < 0 {
		return fmt.Errorf("invalid context_size: %d", log.ContextSize)
	}
	if log.SentenceAwareChunks {
		if log.MinChunkSize <= 0 {
			return fmt.Errorf("invalid min_chunk_size: %d", log.MinChunkSize)
		}
		if log.MaxChunkSize <= 0 {
			return fmt.Errorf("invalid max_chunk_size: %d", log.MaxChunkSize)
		}
		if log.MinChunkSize > log.MaxChunkSize {
			return fmt.Errorf("invalid chunk range: min_chunk_size %d > max_chunk_size %d", log.MinChunkSize, log.MaxChunkSize)
		}
		if log.ChunkBoundaryPlanner == "" {
			log.ChunkBoundaryPlanner = "local-llm"
		}
		switch log.ChunkBoundaryPlanner {
		case "off", "deterministic", "local-llm":
		default:
			return fmt.Errorf("invalid chunk_boundary_planner: %s", log.ChunkBoundaryPlanner)
		}
		if log.ChunkPlan == nil {
			return fmt.Errorf("chunk_plan is required for sentence-aware repair")
		}
	}
	if log.TotalChunks <= 0 {
		return fmt.Errorf("invalid total_chunks: %d", log.TotalChunks)
	}
	if log.ChunkPlan != nil && len(log.ChunkPlan.Chunks) != log.TotalChunks {
		return fmt.Errorf("chunk_plan length mismatch: expected %d, got %d", log.TotalChunks, len(log.ChunkPlan.Chunks))
	}
	if log.ChunkPlan != nil {
		expectedStart := 0
		for i, chunk := range log.ChunkPlan.Chunks {
			if chunk.Index != i {
				return fmt.Errorf("chunk_plan index mismatch at %d: got %d", i, chunk.Index)
			}
			if chunk.StartIndex != expectedStart {
				return fmt.Errorf("chunk_plan start mismatch at %d: got %d, want %d", i, chunk.StartIndex, expectedStart)
			}
			if chunk.EndIndex <= chunk.StartIndex {
				return fmt.Errorf("chunk_plan empty or invalid range at %d: %d..%d", i, chunk.StartIndex, chunk.EndIndex)
			}
			expectedStart = chunk.EndIndex
		}
	}
	if len(log.FailedChunks) == 0 {
		return fmt.Errorf("failed_chunks list is empty")
	}
	for _, idx := range log.FailedChunks {
		if idx < 0 || idx >= log.TotalChunks {
			return fmt.Errorf("failed chunk index out of range: %d", idx)
		}
	}
	if _, ok := language.GetLanguage(log.SourceLang); !ok {
		return fmt.Errorf("unsupported source language: %s", log.SourceLang)
	}
	if _, ok := language.GetLanguage(log.TargetLang); !ok {
		return fmt.Errorf("unsupported target language: %s", log.TargetLang)
	}
	if log.Model == "" {
		return fmt.Errorf("model name is empty")
	}
	if log.Provider == "" {
		log.Provider = "llama.cpp"
	}
	if log.BaseURL == "" {
		log.BaseURL = localllm.DefaultBaseURL
	}
	if log.MaxTokens <= 0 {
		log.MaxTokens = localllm.DefaultMaxTokens
	}
	if log.Status == "" {
		return fmt.Errorf("session status is empty")
	}
	if log.StatusReason != "" && log.StatusReason != "canceled" {
		return fmt.Errorf("invalid status_reason: %s", log.StatusReason)
	}
	return nil
}

// SaveSessionLog saves the session state to a JSON file.
func SaveSessionLog(path string, log *SessionLog) error {
	if log.LogVersion == 0 {
		log.LogVersion = CurrentLogVersion
	}
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}
	return files.AtomicWriteExclusive(path, data, 0600)
}

// GenerateRecoveryPath creates a unique filename for the recovery session log.
// Logic:
// 1. [basename]_recovery.json
// 2. [basename]_recovery_0.json ~ _9.json
// 3. [basename]_recovery_[UUIDv7].json (with collision check)
func GenerateRecoveryPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))

	// Stage 1: Primary
	primary := filepath.Join(dir, fmt.Sprintf("%s_recovery.json", base))
	if _, err := os.Stat(primary); os.IsNotExist(err) {
		return primary
	}

	// Stage 2: Short Loop (0-9)
	for i := 0; i <= 9; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_recovery_%d.json", base, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}

	// Stage 3: Fallback (UUID v7)
	for i := 0; i < 100; i++ {
		u, err := uuid.NewV7()
		var suffix string
		if err != nil {
			suffix = uuid.NewString()[:8]
		} else {
			suffix = u.String()
		}
		candidate := filepath.Join(dir, fmt.Sprintf("%s_recovery_%s.json", base, suffix))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}

	// Final fallback
	return filepath.Join(dir, fmt.Sprintf("%s_recovery_final_%d.json", base, os.Getpid()))
}

// LoadSessionLog loads the session state from a JSON file.
func LoadSessionLog(path string) (*SessionLog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var log SessionLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	if log.LogVersion == 0 {
		log.LogVersion = CurrentLogVersion
	}
	return &log, nil
}

// LoadSessionLogWithHash loads the session log and returns a content hash.
func LoadSessionLogWithHash(path string) (*SessionLog, [32]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, [32]byte{}, err
	}
	var log SessionLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, [32]byte{}, err
	}
	if log.LogVersion == 0 {
		log.LogVersion = CurrentLogVersion
	}
	return &log, sha256.Sum256(data), nil
}

// HashFile returns a SHA-256 hash of the given file contents.
func HashFile(path string) ([32]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// HashFileHex returns a sha256-prefixed hex string of the file contents.
func HashFileHex(path string) (string, error) {
	sum, err := HashFile(path)
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// CalculateStatus determines the session status based on failed and total chunks.
func CalculateStatus(failedCount, totalCount int) string {
	if failedCount == 0 {
		return "Success"
	}
	if failedCount < totalCount {
		return "Partial Success"
	}
	return "Failure"
}

// ResolveOutputPath resolves the relative output_path based on the log file location.
func ResolveOutputPath(logPath, outputPath string) string {
	if filepath.IsAbs(outputPath) {
		return outputPath
	}
	logDir := filepath.Dir(logPath)
	return filepath.Join(logDir, outputPath)
}

// ResolveInputPath resolves the relative input_path based on the log file location.
func ResolveInputPath(logPath, inputPath string) string {
	if filepath.IsAbs(inputPath) {
		return inputPath
	}
	logDir := filepath.Dir(logPath)
	return filepath.Join(logDir, inputPath)
}

// ToRelativeOutputPath converts an absolute output path to relative based on log location.
func ToRelativeOutputPath(logPath, outputPath string) (string, error) {
	rel, err := toRelativePath(logPath, outputPath)
	if err != nil {
		return "", err
	}
	// Security check: output path should not traverse parent directories.
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("output path is not within log directory")
	}
	return rel, nil
}

// ToRelativeInputPath converts an absolute input path to relative based on log location.
func ToRelativeInputPath(logPath, inputPath string) (string, error) {
	return toRelativePath(logPath, inputPath)
}

func toRelativePath(logPath, targetPath string) (string, error) {
	logDir := filepath.Dir(logPath)
	absLogDir, err := filepath.Abs(logDir)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absLogDir, absTarget)
	if err != nil {
		return "", err
	}
	return rel, nil
}
