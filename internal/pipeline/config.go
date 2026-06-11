package pipeline

import (
	"fmt"
	"time"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/llamaserver"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/translator"
)

// Config holds all configuration required for running a translation or repair session.
type Config struct {
	// IO Paths
	InputPath  string
	OutputPath string
	LogPath    string // Optional: for JSONL logs in CLI or specific log file in GUI

	// Local LLM configuration
	BaseURL   string
	Model     string
	MaxTokens int
	// TranslationTimeout is the per-request timeout for translation calls.
	// A zero value disables the HTTP client timeout for slow local models.
	TranslationTimeout time.Duration

	// llama.cpp server management
	LlamaServer  llamaserver.LaunchConfig
	LlamaManager llamaserver.Manager

	// Processing Parameters
	ChunkSize            int
	ContextSize          int
	Concurrency          int
	SentenceAwareChunks  bool
	MinChunkSize         int
	MaxChunkSize         int
	ChunkBoundaryPlanner string

	// Flags
	NoPreprocess      bool
	NoPostprocess     bool
	Overwrite         bool // If true, overwrite output file without asking (CLI mostly)
	ForceRepair       bool // If true, ignore unusable existing output during repair
	NoLangPreprocess  bool
	NoLangPostprocess bool

	// Languages
	SourceLang string
	TargetLang string

	// names Mapping (Source Name -> Target Name)
	NamesMapping map[string]string
	NamesPath    string

	// Callbacks
	// OnProgress is called with translation progress updates.
	OnProgress func(translator.TranslationProgress)

	// OnConfirmOverwrite is called when the output file exists.
	// It should return true if the file should be overwritten.
	// If nil, it assumes Overwrite flag accounts for it or it's already checked.
	OnConfirmOverwrite func(path string) bool
}

const (
	MinConcurrency = 1
	MaxConcurrency = 20
	MaxChunkSize   = 200
	MaxContextSize = 20

	ChunkBoundaryPlannerOff           = "off"
	ChunkBoundaryPlannerDeterministic = "deterministic"
	ChunkBoundaryPlannerLocalLLM      = "local-llm"
)

func ClampConcurrency(value int) (int, bool) {
	if value < MinConcurrency {
		return MinConcurrency, true
	}
	if value > MaxConcurrency {
		return MaxConcurrency, true
	}
	return value, false
}

// Normalize applies safe bounds to config values and returns any adjustments.
func (c Config) Normalize() (Config, []string) {
	var notes []string
	if c.BaseURL == "" {
		c.BaseURL = localllm.DefaultBaseURL
	}
	if c.Model == "" {
		c.Model = localllm.DefaultModel
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = localllm.DefaultMaxTokens
	}
	if c.LlamaServer.ModelAlias == "" {
		c.LlamaServer.ModelAlias = c.Model
	}
	if c.LlamaServer.BaseURL == "" {
		c.LlamaServer.BaseURL = c.BaseURL
	}
	if c.LlamaServer.Mode == "" {
		c.LlamaServer.Mode = llamaserver.ModeExternal
	}
	c.LlamaServer = llamaserver.Normalize(c.LlamaServer)
	c.BaseURL = c.LlamaServer.BaseURL
	c.Model = c.LlamaServer.ModelAlias
	if c.ChunkBoundaryPlanner == "" {
		c.ChunkBoundaryPlanner = ChunkBoundaryPlannerLocalLLM
	}
	if clamped, changed := ClampConcurrency(c.Concurrency); changed {
		notes = append(notes, fmt.Sprintf("concurrency clamped from %d to %d (max %d)", c.Concurrency, clamped, MaxConcurrency))
		c.Concurrency = clamped
	}
	if c.ChunkSize > MaxChunkSize {
		notes = append(notes, fmt.Sprintf("chunk-size clamped from %d to %d (max %d)", c.ChunkSize, MaxChunkSize, MaxChunkSize))
		c.ChunkSize = MaxChunkSize
	}
	if c.ContextSize > MaxContextSize {
		notes = append(notes, fmt.Sprintf("context-size clamped from %d to %d (max %d)", c.ContextSize, MaxContextSize, MaxContextSize))
		c.ContextSize = MaxContextSize
	}
	if c.SentenceAwareChunks {
		if c.MinChunkSize <= 0 {
			c.MinChunkSize = c.ChunkSize - 10
			if c.MinChunkSize < 1 {
				c.MinChunkSize = 1
			}
		}
		if c.MaxChunkSize <= 0 {
			c.MaxChunkSize = c.ChunkSize + 10
		}
	}
	if c.MaxChunkSize > MaxChunkSize {
		notes = append(notes, fmt.Sprintf("max-chunk-size clamped from %d to %d (max %d)", c.MaxChunkSize, MaxChunkSize, MaxChunkSize))
		c.MaxChunkSize = MaxChunkSize
	}
	return c, notes
}

// Validate checks if the configuration is valid.
func (c Config) Validate() error {
	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunkSize must be greater than 0, got %d", c.ChunkSize)
	}
	if c.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than 0, got %d", c.Concurrency)
	}
	if c.ContextSize < 0 {
		return fmt.Errorf("contextSize must be 0 or greater, got %d", c.ContextSize)
	}
	if c.TranslationTimeout < 0 {
		return fmt.Errorf("translationTimeout must be 0 or greater, got %s", c.TranslationTimeout)
	}
	if c.BaseURL == "" {
		return fmt.Errorf("llama base URL is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model name is required")
	}
	if c.SentenceAwareChunks {
		if c.MinChunkSize <= 0 {
			return fmt.Errorf("minChunkSize must be greater than 0, got %d", c.MinChunkSize)
		}
		if c.MaxChunkSize <= 0 {
			return fmt.Errorf("maxChunkSize must be greater than 0, got %d", c.MaxChunkSize)
		}
		if c.MinChunkSize > c.MaxChunkSize {
			return fmt.Errorf("minChunkSize must be <= maxChunkSize, got %d > %d", c.MinChunkSize, c.MaxChunkSize)
		}
		if c.ChunkSize < c.MinChunkSize || c.ChunkSize > c.MaxChunkSize {
			return fmt.Errorf("chunkSize must be inside sentence-aware range, got chunkSize=%d range=%d..%d", c.ChunkSize, c.MinChunkSize, c.MaxChunkSize)
		}
	}
	switch c.ChunkBoundaryPlanner {
	case ChunkBoundaryPlannerOff, ChunkBoundaryPlannerDeterministic, ChunkBoundaryPlannerLocalLLM:
	default:
		return fmt.Errorf("invalid chunk boundary planner: %s", c.ChunkBoundaryPlanner)
	}
	return nil
}

// ValidateRepairRuntime checks only runtime config required for repair.
// Log-derived settings (chunk/concurrency/context/model/lang) are validated on the session log.
func (c Config) ValidateRepairRuntime() error {
	if c.BaseURL == "" {
		return fmt.Errorf("llama base URL is required")
	}
	return nil
}

func (c Config) ChunkPlanOptions() chunker.PlanOptions {
	return chunker.PlanOptions{
		NominalSize:         c.ChunkSize,
		MinSize:             c.MinChunkSize,
		MaxSize:             c.MaxChunkSize,
		ContextSize:         c.ContextSize,
		EnableSentenceAware: c.SentenceAwareChunks && c.ChunkBoundaryPlanner != ChunkBoundaryPlannerOff,
	}
}
