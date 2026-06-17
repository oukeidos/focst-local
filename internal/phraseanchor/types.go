package phraseanchor

import (
	"time"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/translation"
)

const (
	PromptVersion = "phrase-anchor-local-review-v2"

	CandidateDiscoveryName = "candidate-discovery-v1"
	CandidateAddRoundName  = "candidate-add-round-v1"
	QuoteKindFilterName    = "quote-kind-filter-v1"
	SourceNameFilterName   = "source-name-filter-v1"
	AlternativeName        = "alternative-rendering-v1"
	RenderingVoteName      = "rendering-vote-v1"

	ExperimentCandidateDiscovery = "hybrid-core3-strict-noun-quote-minimal"
	ExperimentQuoteKindFilter    = "local_expression_other"
	ExperimentSourceNameFilter   = "source-only-proper-noun-v01-from-glossary-local-markdown-v08-v3c"
	ExperimentAlternative        = "type-guided-table-only"

	DefaultThesisRounds             = 3
	DefaultSynthesisVotes           = 5
	DefaultQuoteFilterBatchSize     = 80
	DefaultProperFilterRuns         = 3
	DefaultProperFilterWindowChunks = 3
	QuoteKindFilterMaxTokens        = 2048
	QuoteKindFilterTemperature      = 0.0
	SourceNameFilterSourceKind      = "proper nouns"
	SourceNameFilterSystemPrompt    = "You extract glossary entries for subtitle translation."
	TypeAmbiguity                   = "ambiguity"
	TypeIdiom                       = "idiom"
	TypeWordplay                    = "wordplay"
	CategoryProperNoun              = "proper_noun"
	CategoryCommonNoun              = "common_noun"
	CategoryOther                   = "other"
	CategoryUnclassified            = "unclassified"
)

type ExtractConfig struct {
	InputPath                string
	SegmentsChecksum         string
	SourceLanguageCode       string
	SourceLanguageName       string
	TargetLanguageCode       string
	TargetLanguageName       string
	Model                    string
	BaseURL                  string
	MaxTokens                int
	ContextSize              int
	ChunkSize                int
	SentenceAwareChunks      bool
	MinChunkSize             int
	MaxChunkSize             int
	ChunkBoundaryPlanner     string
	ThesisRounds             int
	SynthesisVotes           int
	QuoteFilterBatchSize     int
	ProperFilterRuns         int
	ProperFilterWindowChunks int
	ArtifactDir              string
}

type Artifact struct {
	Version       int               `json:"version"`
	PromptVersion string            `json:"prompt_version"`
	SourceLang    string            `json:"source_language"`
	TargetLang    string            `json:"target_language"`
	CreatedAt     time.Time         `json:"created_at"`
	Input         InputInfo         `json:"input"`
	Config        RunConfig         `json:"config"`
	StageMapping  StageMap          `json:"stage_mapping"`
	ChunkPlan     chunker.ChunkPlan `json:"chunk_plan"`
	Entries       []Entry           `json:"entries"`
	RejectedCount int               `json:"rejected_count"`
}

type InputInfo struct {
	Path                     string `json:"path"`
	PreprocessedSegmentCount int    `json:"preprocessed_segment_count"`
	SegmentsChecksum         string `json:"segments_checksum"`
}

type RunConfig struct {
	Model                    string  `json:"model"`
	BaseURL                  string  `json:"base_url"`
	MaxTokens                int     `json:"max_tokens"`
	TranslationChunkSize     int     `json:"translation_chunk_size"`
	ContextSize              int     `json:"context_size"`
	SentenceAwareChunks      bool    `json:"sentence_aware_chunks"`
	MinChunkSize             int     `json:"min_chunk_size"`
	MaxChunkSize             int     `json:"max_chunk_size"`
	ChunkBoundaryPlanner     string  `json:"chunk_boundary_planner"`
	ThesisRounds             int     `json:"thesis_rounds"`
	SynthesisVotes           int     `json:"synthesis_votes"`
	QuoteFilterBatchSize     int     `json:"quote_filter_batch_size"`
	QuoteFilterTemperature   float64 `json:"quote_filter_temperature"`
	QuoteFilterMaxTokens     int     `json:"quote_filter_max_tokens"`
	ProperFilterRuns         int     `json:"proper_filter_runs"`
	ProperFilterWindowChunks int     `json:"proper_filter_window_chunks"`
}

type StageMap struct {
	CandidateDiscovery   string `json:"candidate_discovery"`
	QuoteKindFilter      string `json:"quote_kind_filter"`
	SourceNameFilter     string `json:"source_name_filter"`
	AlternativeRendering string `json:"alternative_rendering"`
}

type Entry struct {
	SegmentID             int            `json:"segment_id"`
	SourceText            string         `json:"source_text"`
	Type                  string         `json:"type"`
	SourceQuote           string         `json:"source_quote"`
	Rendering             string         `json:"rendering"`
	TranslationChunkIndex int            `json:"translation_chunk_index"`
	PhraseWindowIndex     int            `json:"phrase_window_index"`
	Origin                string         `json:"origin"`
	Votes                 map[string]int `json:"votes,omitempty"`
}

type Window struct {
	Index                 int                       `json:"index"`
	TranslationChunkIndex int                       `json:"translation_chunk_index"`
	StartIndex            int                       `json:"start_index"`
	EndIndex              int                       `json:"end_index"`
	StartID               int                       `json:"start_id"`
	EndID                 int                       `json:"end_id"`
	ContextBefore         []translation.SegmentData `json:"context_before"`
	Target                []translation.SegmentData `json:"target"`
	ContextAfter          []translation.SegmentData `json:"context_after"`
}

type Candidate struct {
	SegmentID             int    `json:"segment_id"`
	SourceText            string `json:"source_text"`
	Type                  string `json:"type"`
	SourceQuote           string `json:"source_quote"`
	Rendering             string `json:"rendering"`
	TranslationChunkIndex int    `json:"translation_chunk_index"`
	PhraseWindowIndex     int    `json:"phrase_window_index"`
	Stage                 string `json:"stage,omitempty"`
}

type RejectedCandidate struct {
	SegmentID         int    `json:"segment_id,omitempty"`
	SourceText        string `json:"source_text,omitempty"`
	Type              string `json:"type,omitempty"`
	SourceQuote       string `json:"source_quote,omitempty"`
	Rendering         string `json:"rendering,omitempty"`
	Reason            string `json:"reason"`
	PhraseWindowIndex int    `json:"phrase_window_index,omitempty"`
	Stage             string `json:"stage,omitempty"`
}

type Alternative struct {
	Candidate
	AlternativeRendering string `json:"alternative_rendering"`
}

type VoteRow struct {
	Row       int
	Candidate Candidate
	A         string
	B         string
}

type VoteTally struct {
	CountA       int
	CountB       int
	InvalidVotes int
	Choice       string
}
