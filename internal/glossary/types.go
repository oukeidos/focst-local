package glossary

import (
	"time"

	"github.com/oukeidos/focst-local/internal/translation"
)

const (
	PromptVersion       = "local-markdown-v08-v3c-frozen-2026-06-15"
	SystemPrompt        = "You extract glossary entries for subtitle translation."
	DefaultRuns         = 3
	DefaultWindowChunks = 3
	DefaultMaxTokens    = 8192

	RenderingSafetyFilterVersion       = "local-glossary-rendering-safety-v1"
	RenderingSafetyFilterSystemPrompt  = "You review subtitle glossary entries."
	DefaultRenderingSafetyBatchSize    = 20
	DefaultRenderingSafetySnippetLimit = 4
	DefaultRenderingSafetyTemperature  = 0.0

	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Segment is the normalized source representation used by the glossary pass.
type Segment struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
}

// Candidate is one row parsed from a model-produced glossary table.
type Candidate struct {
	Source      string `json:"source"`
	Rendering   string `json:"rendering"`
	WindowIndex int    `json:"window_index"`
	RunIndex    int    `json:"run_index"`
}

// RejectedCandidate records a parsed model row that is not safe to inject.
type RejectedCandidate struct {
	Source      string `json:"source"`
	Rendering   string `json:"rendering,omitempty"`
	Reason      string `json:"reason"`
	WindowIndex int    `json:"window_index"`
	RunIndex    int    `json:"run_index"`
}

// ValidatedCandidate is a locally verified glossary candidate.
type ValidatedCandidate struct {
	Candidate
	CanonicalSource string `json:"canonical_source"`
	OccurrenceIDs   []int  `json:"occurrence_ids"`
}

// Entry is one merged glossary entry.
type Entry struct {
	Source        string         `json:"source"`
	Rendering     string         `json:"rendering"`
	Confidence    string         `json:"confidence"`
	Votes         map[string]int `json:"votes"`
	OccurrenceIDs []int          `json:"occurrence_ids"`
	WindowsSeen   []int          `json:"windows_seen"`
}

// Artifact is the reusable generated glossary file consumed by translation.
type Artifact struct {
	Version               int                        `json:"version"`
	PromptVersion         string                     `json:"prompt_version"`
	SourceLang            string                     `json:"source_language"`
	TargetLang            string                     `json:"target_language"`
	CreatedAt             time.Time                  `json:"created_at"`
	Input                 InputInfo                  `json:"input"`
	Config                RunConfig                  `json:"config"`
	RenderingSafetyFilter *RenderingSafetyFilterInfo `json:"rendering_safety_filter,omitempty"`
	Entries               []Entry                    `json:"entries"`
	RejectedCount         int                        `json:"rejected_count"`
}

type InputInfo struct {
	Path                     string `json:"path"`
	PreprocessedSegmentCount int    `json:"preprocessed_segment_count"`
	SegmentsChecksum         string `json:"segments_checksum"`
}

type RunConfig struct {
	Model                string `json:"model"`
	BaseURL              string `json:"base_url"`
	MaxTokens            int    `json:"max_tokens"`
	GlossaryRuns         int    `json:"glossary_runs"`
	GlossaryWindowChunks int    `json:"glossary_window_chunks"`
}

type Window struct {
	Index      int       `json:"index"`
	ChunkStart int       `json:"chunk_start"`
	ChunkEnd   int       `json:"chunk_end"`
	StartID    int       `json:"start_id"`
	EndID      int       `json:"end_id"`
	Segments   []Segment `json:"segments"`
}

type RunRecord struct {
	WindowIndex int                       `json:"window_index"`
	RunIndex    int                       `json:"run_index"`
	PromptPath  string                    `json:"prompt_path,omitempty"`
	Response    string                    `json:"response,omitempty"`
	Usage       translation.UsageMetadata `json:"usage"`
	Parsed      []Candidate               `json:"parsed"`
	Rejected    []RejectedCandidate       `json:"rejected"`
	Violations  []string                  `json:"violations,omitempty"`
}

type RenderingSafetyFilterInfo struct {
	Version            string `json:"version"`
	Applied            bool   `json:"applied"`
	SourceEntryCount   int    `json:"source_entry_count"`
	FilteredEntryCount int    `json:"filtered_entry_count"`
	DroppedCount       int    `json:"dropped_count"`
}

type RenderingSafetyJudgment struct {
	Row                int    `json:"row"`
	Source             string `json:"source"`
	Rendering          string `json:"rendering"`
	ExpectedStrategy   string `json:"expected_strategy"`
	Fit                string `json:"fit"`
	Decision           string `json:"decision"`
	SourcePolicyResult string `json:"source_policy_result"`
	SourcePolicyReason string `json:"source_policy_reason"`
}
