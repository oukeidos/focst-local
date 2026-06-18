package postpolish

import (
	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/translation"
)

const (
	PromptVersionLegacy       = "post-polish-legacy-v1"
	PromptVersionSegmentLocal = "post-polish-segment-local-v2"
	PromptVersionChunkFlow    = "post-polish-chunk-flow-v2"
	PromptVersion             = PromptVersionLegacy

	DefaultBroadChunkSize    = 30
	DefaultRepairChunkSize   = 100
	DefaultLegacyMaxTokens   = 2048
	DefaultV2MaxTokens       = 8192
	DefaultMaxTokens         = DefaultV2MaxTokens
	DefaultV2ChunkSize       = 8
	DefaultV2MinChunkSize    = 5
	DefaultV2MaxChunkSize    = 9
	DefaultV2SentenceAware   = true
	DefaultV2BoundaryPlanner = "local-llm"
	DefaultTemperature       = 0.0

	PassBroad  = "pass_broad"
	PassRepair = "pass_repair"
)

type Profile string

const (
	ProfileSegmentLocal Profile = "segment-local"
	ProfileChunkFlow    Profile = "chunk-flow"
	ProfileLegacy       Profile = "legacy"
)

type Config struct {
	SourceLanguage      language.Language
	TargetLanguage      language.Language
	Model               string
	BaseURL             string
	Profile             Profile
	ChunkPlan           chunker.ChunkPlan
	ChunkSize           int
	BroadChunkSize      int
	RepairChunkSize     int
	MaxTokens           int
	ArtifactDir         string
	ProtectedRenderings map[string]string
}

type Correction struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Pass       string `json:"pass"`
	ChunkIndex int    `json:"chunk_index"`
}

type RejectedCorrection struct {
	ID         int      `json:"id,omitempty"`
	SourceText string   `json:"source_text,omitempty"`
	Before     string   `json:"before,omitempty"`
	After      string   `json:"after,omitempty"`
	Pass       string   `json:"pass"`
	ChunkIndex int      `json:"chunk_index"`
	Reason     string   `json:"reason"`
	GuardHits  []string `json:"guard_hits,omitempty"`
}

type Artifact struct {
	Version                  int                       `json:"version"`
	PromptVersion            string                    `json:"prompt_version"`
	InstructionPromptVersion string                    `json:"instruction_prompt_version,omitempty"`
	ApplicationPromptVersion string                    `json:"application_prompt_version,omitempty"`
	Profile                  string                    `json:"profile,omitempty"`
	Model                    string                    `json:"model,omitempty"`
	BaseURL                  string                    `json:"base_url,omitempty"`
	SourceLanguage           string                    `json:"source_language"`
	TargetLanguage           string                    `json:"target_language"`
	ChunkSize                int                       `json:"chunk_size,omitempty"`
	ChunkPlan                chunker.ChunkPlan         `json:"chunk_plan,omitempty"`
	BroadChunkSize           int                       `json:"broad_chunk_size"`
	RepairChunkSize          int                       `json:"repair_chunk_size"`
	MaxTokens                int                       `json:"max_tokens"`
	Accepted                 []Correction              `json:"accepted"`
	Rejected                 []RejectedCorrection      `json:"rejected"`
	GuardRejected            int                       `json:"guard_rejected"`
	FailedRequests           int                       `json:"failed_requests"`
	Usage                    translation.UsageMetadata `json:"usage"`
	ProtectedRenderings      int                       `json:"protected_renderings"`
	Instructions             []InstructionRecord       `json:"instructions,omitempty"`
	AppliedRows              []AppliedRowRecord        `json:"applied_rows,omitempty"`
	Stats                    ArtifactStats             `json:"stats,omitempty"`
}

type Result struct {
	Accepted       []Correction
	Rejected       []RejectedCorrection
	GuardRejected  int
	FailedRequests int
	Usage          translation.UsageMetadata
	Artifact       Artifact
}

type Request struct {
	Segments []RequestSegment `json:"segments"`
}

type RequestSegment struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
	Text       string `json:"text"`
}

type Response struct {
	Corrections []ResponseCorrection `json:"corrections"`
}

type ResponseCorrection struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
	Text       string `json:"text"`
}

type InstructionRecord struct {
	ChunkIndex          int                  `json:"chunk_index"`
	StartID             int                  `json:"start_id"`
	EndID               int                  `json:"end_id"`
	Instruction         string               `json:"instruction,omitempty"`
	SegmentInstructions []SegmentInstruction `json:"segment_instructions,omitempty"`
}

type SegmentInstruction struct {
	ID              int    `json:"id"`
	SourceText      string `json:"source_text"`
	EditInstruction string `json:"edit_instruction"`
}

type AppliedRowRecord struct {
	ChunkIndex     int    `json:"chunk_index"`
	ID             int    `json:"id"`
	SourceText     string `json:"source_text"`
	Before         string `json:"before"`
	After          string `json:"after"`
	ForcedNoChange bool   `json:"forced_no_change,omitempty"`
}

type ArtifactStats struct {
	RequestOK       int `json:"request_ok,omitempty"`
	RequestError    int `json:"request_error,omitempty"`
	ValidationError int `json:"validation_error,omitempty"`
	ChangedSegments int `json:"changed_segments,omitempty"`
	ForcedNoChange  int `json:"forced_no_change,omitempty"`
	EmptyRejected   int `json:"empty_rejected,omitempty"`
	MetaRejected    int `json:"meta_rejected,omitempty"`
	GuardRejected   int `json:"guard_rejected,omitempty"`
}

func NormalizeProfile(value string) (Profile, bool) {
	switch Profile(value) {
	case "", ProfileSegmentLocal:
		return ProfileSegmentLocal, true
	case ProfileChunkFlow:
		return ProfileChunkFlow, true
	case ProfileLegacy:
		return ProfileLegacy, true
	default:
		return "", false
	}
}

func ValidProfile(value Profile) bool {
	_, ok := NormalizeProfile(string(value))
	return ok
}

func NeedsChunkPlan(profile Profile) bool {
	profile, _ = NormalizeProfile(string(profile))
	return profile != ProfileLegacy
}
