package postpolish

import (
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/translation"
)

const (
	PromptVersion          = "post-polish-v1"
	DefaultBroadChunkSize  = 30
	DefaultRepairChunkSize = 100
	DefaultMaxTokens       = 2048
	DefaultTemperature     = 0.0

	PassBroad  = "pass_broad"
	PassRepair = "pass_repair"
)

type Config struct {
	SourceLanguage      language.Language
	TargetLanguage      language.Language
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
	Version             int                       `json:"version"`
	PromptVersion       string                    `json:"prompt_version"`
	SourceLanguage      string                    `json:"source_language"`
	TargetLanguage      string                    `json:"target_language"`
	BroadChunkSize      int                       `json:"broad_chunk_size"`
	RepairChunkSize     int                       `json:"repair_chunk_size"`
	MaxTokens           int                       `json:"max_tokens"`
	Accepted            []Correction              `json:"accepted"`
	Rejected            []RejectedCorrection      `json:"rejected"`
	GuardRejected       int                       `json:"guard_rejected"`
	FailedRequests      int                       `json:"failed_requests"`
	Usage               translation.UsageMetadata `json:"usage"`
	ProtectedRenderings int                       `json:"protected_renderings"`
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
