package pipeline

import (
	"fmt"
	"time"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/llamaserver"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/phraseanchor"
	"github.com/oukeidos/focst-local/internal/postpolish"
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

	// Local generated glossary. AutoGlossary generates a new glossary before
	// translation; GlossaryPath reuses an existing glossary artifact.
	AutoGlossary         bool
	GlossaryPath         string
	SaveGlossaryPath     string
	GlossaryArtifactsDir string
	GlossaryRuns         int
	GlossaryWindowChunks int

	// Local generated phrase anchors. AutoPhraseAnchors generates a new
	// chunk-local artifact before translation; PhraseAnchorsPath reuses an
	// existing phrase anchor artifact and its saved chunk plan.
	AutoPhraseAnchors                    bool
	PhraseAnchorsPath                    string
	SavePhraseAnchorsPath                string
	PhraseAnchorsArtifactsDir            string
	PhraseAnchorThesisRounds             int
	PhraseAnchorVotes                    int
	PhraseAnchorQuoteFilterBatchSize     int
	PhraseAnchorProperFilterRuns         int
	PhraseAnchorProperFilterWindowChunks int

	// Optional post-translation polish. This is independent from glossary,
	// names, and phrase anchors; mappings are used only as a protection guard
	// when available.
	PostPolish                 bool
	PostPolishProfile          string
	SavePolishCorrectionsPath  string
	PolishArtifactsDir         string
	PolishBroadChunkSize       int
	PolishRepairChunkSize      int
	PolishMaxTokens            int
	PolishChunkSize            int
	PolishMinChunkSize         int
	PolishMaxChunkSize         int
	PolishSentenceAwareChunks  bool
	PolishChunkBoundaryPlanner string

	// Optional source residue repair. This is a narrow cleanup pass that
	// detects selected source-script residue in the translated output and asks
	// the local model to repair only those rows.
	RepairResidue             bool
	ResidueScripts            string
	SaveResidueCandidatesPath string
	ResidueReportPath         string

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
	if c.GlossaryRuns <= 0 {
		c.GlossaryRuns = 3
	}
	if c.GlossaryWindowChunks <= 0 {
		c.GlossaryWindowChunks = 3
	}
	if c.PhraseAnchorThesisRounds <= 0 {
		c.PhraseAnchorThesisRounds = phraseanchor.DefaultThesisRounds
	}
	if c.PhraseAnchorVotes <= 0 {
		c.PhraseAnchorVotes = phraseanchor.DefaultSynthesisVotes
	}
	if c.PhraseAnchorQuoteFilterBatchSize <= 0 {
		c.PhraseAnchorQuoteFilterBatchSize = phraseanchor.DefaultQuoteFilterBatchSize
	}
	if c.PhraseAnchorProperFilterRuns <= 0 {
		c.PhraseAnchorProperFilterRuns = phraseanchor.DefaultProperFilterRuns
	}
	if c.PhraseAnchorProperFilterWindowChunks <= 0 {
		c.PhraseAnchorProperFilterWindowChunks = phraseanchor.DefaultProperFilterWindowChunks
	}
	if profile, ok := postpolish.NormalizeProfile(c.PostPolishProfile); ok {
		c.PostPolishProfile = string(profile)
	}
	if c.PolishBroadChunkSize <= 0 {
		c.PolishBroadChunkSize = postpolish.DefaultBroadChunkSize
	}
	if c.PolishRepairChunkSize <= 0 {
		c.PolishRepairChunkSize = postpolish.DefaultRepairChunkSize
	}
	if c.PolishMaxTokens <= 0 {
		if c.PostPolishProfile == string(postpolish.ProfileLegacy) {
			c.PolishMaxTokens = postpolish.DefaultLegacyMaxTokens
		} else {
			c.PolishMaxTokens = postpolish.DefaultV2MaxTokens
		}
	}
	if c.PolishChunkSize <= 0 {
		c.PolishChunkSize = postpolish.DefaultV2ChunkSize
	}
	if c.PolishMinChunkSize <= 0 {
		c.PolishMinChunkSize = postpolish.DefaultV2MinChunkSize
	}
	if c.PolishMaxChunkSize <= 0 {
		c.PolishMaxChunkSize = postpolish.DefaultV2MaxChunkSize
	}
	if c.PolishChunkBoundaryPlanner == "" {
		c.PolishChunkBoundaryPlanner = postpolish.DefaultV2BoundaryPlanner
		c.PolishSentenceAwareChunks = postpolish.DefaultV2SentenceAware
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
	if c.AutoGlossary && c.GlossaryPath != "" {
		return fmt.Errorf("--auto-glossary and --glossary-file cannot be used together")
	}
	if c.SaveGlossaryPath != "" && !c.AutoGlossary {
		return fmt.Errorf("--save-glossary requires --auto-glossary")
	}
	if c.GlossaryRuns <= 0 {
		return fmt.Errorf("glossaryRuns must be greater than 0, got %d", c.GlossaryRuns)
	}
	if c.GlossaryWindowChunks <= 0 {
		return fmt.Errorf("glossaryWindowChunks must be greater than 0, got %d", c.GlossaryWindowChunks)
	}
	if c.AutoPhraseAnchors && c.PhraseAnchorsPath != "" {
		return fmt.Errorf("--auto-phrase-anchors and --phrase-anchors-file cannot be used together")
	}
	if c.SavePhraseAnchorsPath != "" && !c.AutoPhraseAnchors {
		return fmt.Errorf("--save-phrase-anchors requires --auto-phrase-anchors")
	}
	if (c.AutoPhraseAnchors || c.PhraseAnchorsPath != "") && c.Concurrency != 1 {
		return fmt.Errorf("phrase anchors require --concurrency 1")
	}
	if c.PhraseAnchorThesisRounds <= 0 {
		return fmt.Errorf("phraseAnchorThesisRounds must be greater than 0, got %d", c.PhraseAnchorThesisRounds)
	}
	if c.PhraseAnchorVotes <= 0 {
		return fmt.Errorf("phraseAnchorVotes must be greater than 0, got %d", c.PhraseAnchorVotes)
	}
	if c.PhraseAnchorQuoteFilterBatchSize <= 0 {
		return fmt.Errorf("phraseAnchorQuoteFilterBatchSize must be greater than 0, got %d", c.PhraseAnchorQuoteFilterBatchSize)
	}
	if c.PhraseAnchorProperFilterRuns <= 0 {
		return fmt.Errorf("phraseAnchorProperFilterRuns must be greater than 0, got %d", c.PhraseAnchorProperFilterRuns)
	}
	if c.PhraseAnchorProperFilterWindowChunks <= 0 {
		return fmt.Errorf("phraseAnchorProperFilterWindowChunks must be greater than 0, got %d", c.PhraseAnchorProperFilterWindowChunks)
	}
	if c.SavePolishCorrectionsPath != "" && !c.PostPolish {
		return fmt.Errorf("--save-polish-corrections requires --post-polish")
	}
	if c.RepairResidue && c.ResidueScripts == "" {
		return fmt.Errorf("--repair-residue requires --residue-scripts")
	}
	if c.SaveResidueCandidatesPath != "" && !c.RepairResidue {
		return fmt.Errorf("--save-residue-candidates requires --repair-residue")
	}
	if c.ResidueReportPath != "" && !c.RepairResidue {
		return fmt.Errorf("--residue-report requires --repair-residue")
	}
	if _, ok := postpolish.NormalizeProfile(c.PostPolishProfile); !ok {
		return fmt.Errorf("invalid post-polish profile: %s", c.PostPolishProfile)
	}
	if c.PolishBroadChunkSize <= 0 {
		return fmt.Errorf("polishBroadChunkSize must be greater than 0, got %d", c.PolishBroadChunkSize)
	}
	if c.PolishRepairChunkSize <= 0 {
		return fmt.Errorf("polishRepairChunkSize must be greater than 0, got %d", c.PolishRepairChunkSize)
	}
	if c.PolishMaxTokens <= 0 {
		return fmt.Errorf("polishMaxTokens must be greater than 0, got %d", c.PolishMaxTokens)
	}
	if c.PolishChunkSize <= 0 {
		return fmt.Errorf("polishChunkSize must be greater than 0, got %d", c.PolishChunkSize)
	}
	if c.PolishMinChunkSize <= 0 {
		return fmt.Errorf("polishMinChunkSize must be greater than 0, got %d", c.PolishMinChunkSize)
	}
	if c.PolishMaxChunkSize <= 0 {
		return fmt.Errorf("polishMaxChunkSize must be greater than 0, got %d", c.PolishMaxChunkSize)
	}
	if c.PolishMinChunkSize > c.PolishMaxChunkSize {
		return fmt.Errorf("polishMinChunkSize must be <= polishMaxChunkSize, got %d > %d", c.PolishMinChunkSize, c.PolishMaxChunkSize)
	}
	if c.PolishChunkSize < c.PolishMinChunkSize || c.PolishChunkSize > c.PolishMaxChunkSize {
		return fmt.Errorf("polishChunkSize must be inside polish sentence-aware range, got polishChunkSize=%d range=%d..%d", c.PolishChunkSize, c.PolishMinChunkSize, c.PolishMaxChunkSize)
	}
	switch c.PolishChunkBoundaryPlanner {
	case ChunkBoundaryPlannerOff, ChunkBoundaryPlannerDeterministic, ChunkBoundaryPlannerLocalLLM:
	default:
		return fmt.Errorf("invalid polish chunk boundary planner: %s", c.PolishChunkBoundaryPlanner)
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

func (c Config) PolishChunkPlanOptions() chunker.PlanOptions {
	return chunker.PlanOptions{
		NominalSize:         c.PolishChunkSize,
		MinSize:             c.PolishMinChunkSize,
		MaxSize:             c.PolishMaxChunkSize,
		ContextSize:         0,
		EnableSentenceAware: c.PolishSentenceAwareChunks && c.PolishChunkBoundaryPlanner != ChunkBoundaryPlannerOff,
	}
}
