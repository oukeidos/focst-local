package residue

import (
	"time"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

const (
	ArtifactVersion        = 1
	PromptVersion          = "source-residue-repair-v1"
	AutoScripts            = "auto"
	DefaultRepairMaxTokens = 1024
	DefaultTemperature     = 0.0
)

type DetectOptions struct {
	SourceLanguage   language.Language
	TargetLanguage   language.Language
	SourcePath       string
	TranslatedPath   string
	ScriptSpec       string
	NoPreprocess     bool
	NoLangPreprocess bool
}

type RepairOptions struct {
	SourceLanguage      language.Language
	TargetLanguage      language.Language
	Model               string
	BaseURL             string
	MaxTokens           int
	ProtectedRenderings map[string]string
}

type Artifact struct {
	Version        int          `json:"version"`
	PromptVersion  string       `json:"prompt_version,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	SourceLanguage string       `json:"source_language"`
	TargetLanguage string       `json:"target_language"`
	Input          InputInfo    `json:"input"`
	Config         RunConfig    `json:"config"`
	ScriptStats    []ScriptStat `json:"script_stats"`
	Candidates     []Candidate  `json:"candidates"`
}

type InputInfo struct {
	SourcePath                     string `json:"source_path"`
	TranslatedPath                 string `json:"translated_path"`
	PreprocessedSourceSegmentCount int    `json:"preprocessed_source_segment_count"`
	TranslatedSegmentCount         int    `json:"translated_segment_count"`
	SourceSegmentsChecksum         string `json:"source_segments_checksum"`
	TranslatedSegmentsChecksum     string `json:"translated_segments_checksum"`
}

type RunConfig struct {
	ScriptSpec      string   `json:"script_spec"`
	SelectedScripts []string `json:"selected_scripts"`
}

type ScriptStat struct {
	Name        string  `json:"name"`
	SourceCount int     `json:"source_count"`
	TargetCount int     `json:"target_count"`
	SourceShare float64 `json:"source_share"`
	TargetShare float64 `json:"target_share"`
	Selected    bool    `json:"selected"`
}

type Candidate struct {
	ID                 int      `json:"id"`
	StartTime          string   `json:"start_time"`
	EndTime            string   `json:"end_time"`
	SourceText         string   `json:"source_text"`
	CurrentText        string   `json:"current_text"`
	Scripts            []string `json:"scripts"`
	FilteredSourceText string   `json:"filtered_source_text"`
	FilteredTargetText string   `json:"filtered_target_text"`
	Residues           []string `json:"residues"`
}

type RepairResult struct {
	Segments []srt.Segment
	Records  []RepairRecord
	Usage    translation.UsageMetadata
}

type RepairRecord struct {
	ID         int                       `json:"id"`
	SourceText string                    `json:"source_text"`
	Before     string                    `json:"before"`
	Residues   []string                  `json:"residues"`
	Proposed   string                    `json:"proposed,omitempty"`
	Status     string                    `json:"status"`
	Reason     string                    `json:"reason,omitempty"`
	GuardHits  []string                  `json:"guard_hits,omitempty"`
	LatencyMS  int64                     `json:"latency_ms,omitempty"`
	Usage      translation.UsageMetadata `json:"usage,omitempty"`
}

type Report struct {
	Artifact Artifact       `json:"artifact"`
	Repairs  []RepairRecord `json:"repairs,omitempty"`
}
