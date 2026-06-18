package residue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type JSONCompleter interface {
	CompleteJSONWithOptions(ctx context.Context, systemPrompt, userPrompt string, schema map[string]any, opts translation.TextCompletionOptions) (*translation.TextCompletion, error)
}

type repairResponse struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
	Text       string `json:"text"`
}

type repairPrompt struct {
	TargetLanguage  string                `json:"target_language"`
	CurrentID       int                   `json:"current_id"`
	DetectedResidue []string              `json:"detected_residue"`
	Segments        []repairPromptSegment `json:"segments"`
}

type repairPromptSegment struct {
	ID         int    `json:"id"`
	Role       string `json:"role"`
	SourceText string `json:"source_text"`
	Text       string `json:"text"`
}

func Repair(ctx context.Context, client JSONCompleter, sourceSegments, translatedSegments []srt.Segment, artifact Artifact, cfg RepairOptions) (RepairResult, error) {
	if err := validateAligned(sourceSegments, translatedSegments); err != nil {
		return RepairResult{}, err
	}
	scripts, err := scriptsFromArtifact(artifact)
	if err != nil {
		return RepairResult{}, err
	}
	cfg = normalizeRepairOptions(cfg)
	out := make([]srt.Segment, len(translatedSegments))
	copy(out, translatedSegments)
	sourceByID := segmentIndex(sourceSegments)
	targetByID := segmentIndex(out)
	targetPosByID := segmentPositionIndex(out)
	result := RepairResult{Segments: out}
	temperature := DefaultTemperature
	opts := translation.TextCompletionOptions{
		MaxTokens:   cfg.MaxTokens,
		Temperature: &temperature,
	}
	for _, candidate := range artifact.Candidates {
		source, ok := sourceByID[candidate.ID]
		if !ok {
			result.Records = append(result.Records, RepairRecord{ID: candidate.ID, SourceText: candidate.SourceText, Before: candidate.CurrentText, Residues: candidate.Residues, Status: "rejected", Reason: "unknown_source_id"})
			continue
		}
		target, ok := targetByID[candidate.ID]
		if !ok {
			result.Records = append(result.Records, RepairRecord{ID: candidate.ID, SourceText: candidate.SourceText, Before: candidate.CurrentText, Residues: candidate.Residues, Status: "rejected", Reason: "unknown_target_id"})
			continue
		}
		sourceText := translation.SourceTextFromLines(source.Lines)
		before := translation.SourceTextFromLines(target.Lines)
		prompt, err := buildRepairUserPrompt(sourceSegments, out, candidate.ID, candidate.Residues, cfg.TargetLanguage.Name)
		if err != nil {
			result.Records = append(result.Records, RepairRecord{ID: candidate.ID, SourceText: sourceText, Before: before, Residues: candidate.Residues, Status: "error", Reason: err.Error()})
			continue
		}
		started := time.Now()
		completion, err := client.CompleteJSONWithOptions(ctx, repairSystemPrompt(), prompt, repairSchema(candidate.ID, sourceText), opts)
		if err != nil {
			result.Records = append(result.Records, RepairRecord{ID: candidate.ID, SourceText: sourceText, Before: before, Residues: candidate.Residues, Status: "error", Reason: err.Error(), LatencyMS: time.Since(started).Milliseconds()})
			continue
		}
		result.Usage = addUsage(result.Usage, completion.Usage)
		record := RepairRecord{
			ID:         candidate.ID,
			SourceText: sourceText,
			Before:     before,
			Residues:   candidate.Residues,
			LatencyMS:  time.Since(started).Milliseconds(),
			Usage:      completion.Usage,
		}
		proposed, status, reason, hits := validateRepairResponse(completion.Content, candidate, sourceText, before, scripts, cfg.ProtectedRenderings)
		record.Proposed = proposed
		record.Status = status
		record.Reason = reason
		record.GuardHits = hits
		if status == "accepted" {
			pos := targetPosByID[candidate.ID]
			out[pos].Lines = []string{proposed}
			targetByID[candidate.ID] = out[pos]
		}
		result.Records = append(result.Records, record)
	}
	result.Segments = out
	return result, nil
}

func Apply(translated []srt.Segment, records []RepairRecord) []srt.Segment {
	if len(records) == 0 {
		return translated
	}
	accepted := map[int]string{}
	for _, record := range records {
		if record.Status == "accepted" {
			accepted[record.ID] = record.Proposed
		}
	}
	out := make([]srt.Segment, len(translated))
	copy(out, translated)
	for i := range out {
		if text, ok := accepted[out[i].ID]; ok {
			out[i].Lines = []string{text}
		}
	}
	return out
}

func normalizeRepairOptions(cfg RepairOptions) RepairOptions {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultRepairMaxTokens
	}
	if cfg.ProtectedRenderings == nil {
		cfg.ProtectedRenderings = map[string]string{}
	}
	return cfg
}

func scriptsFromArtifact(artifact Artifact) ([]Script, error) {
	if len(artifact.Config.SelectedScripts) == 0 {
		return nil, nil
	}
	return scriptsFromNames(artifact.Config.SelectedScripts)
}

func scriptsFromNames(names []string) ([]Script, error) {
	scripts := make([]Script, 0, len(names))
	for _, name := range names {
		resolved, table, ok := lookupScript(name)
		if !ok {
			return nil, fmt.Errorf("unsupported residue script in artifact: %s", name)
		}
		scripts = append(scripts, Script{Name: resolved, Table: table})
	}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].Name < scripts[j].Name })
	return scripts, nil
}

func buildRepairUserPrompt(sourceSegments, translatedSegments []srt.Segment, currentID int, residues []string, targetLanguageName string) (string, error) {
	sourceByID := segmentIndex(sourceSegments)
	targetByID := segmentIndex(translatedSegments)
	current, ok := sourceByID[currentID]
	if !ok {
		return "", fmt.Errorf("unknown current source id: %d", currentID)
	}
	if _, ok := targetByID[currentID]; !ok {
		return "", fmt.Errorf("unknown current target id: %d", currentID)
	}
	ids := []int{currentID - 1, currentID, currentID + 1}
	roles := map[int]string{
		currentID - 1: "previous",
		currentID:     "current",
		currentID + 1: "next",
	}
	payload := repairPrompt{
		TargetLanguage:  targetLanguageName,
		CurrentID:       current.ID,
		DetectedResidue: append([]string(nil), residues...),
	}
	for _, id := range ids {
		source, sourceOK := sourceByID[id]
		target, targetOK := targetByID[id]
		if !sourceOK || !targetOK {
			continue
		}
		payload.Segments = append(payload.Segments, repairPromptSegment{
			ID:         id,
			Role:       roles[id],
			SourceText: translation.SourceTextFromLines(source.Lines),
			Text:       translation.SourceTextFromLines(target.Lines),
		})
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode residue repair prompt: %w", err)
	}
	return string(data), nil
}

func repairSystemPrompt() string {
	return strings.Join([]string{
		"Fix only untranslated source-language residue in the current target subtitle.",
		"Use the neighboring rows only to understand context.",
		"Do not polish style.",
		"Do not rewrite unrelated wording.",
		"Return only the corrected current row.",
	}, "\n")
}

func repairSchema(id int, sourceText string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "source_text", "text"},
		"properties": map[string]any{
			"id": map[string]any{
				"type":  "integer",
				"const": id,
			},
			"source_text": map[string]any{
				"type":  "string",
				"const": sourceText,
			},
			"text": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}
}

func validateRepairResponse(content string, candidate Candidate, sourceText, before string, scripts []Script, protected map[string]string) (string, string, string, []string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return "", "rejected", "parse_failed: " + err.Error(), nil
	}
	for key := range raw {
		switch key {
		case "id", "source_text", "text":
		default:
			return "", "rejected", "unexpected_field: " + key, nil
		}
	}
	var response repairResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return "", "rejected", "parse_failed: " + err.Error(), nil
	}
	proposed := strings.TrimSpace(response.Text)
	switch {
	case response.ID != candidate.ID:
		return proposed, "rejected", "id_mismatch", nil
	case response.SourceText != sourceText:
		return proposed, "rejected", "source_text_mismatch", nil
	case proposed == "":
		return proposed, "rejected", "empty_text", nil
	case proposed == before:
		return proposed, "unchanged", "unchanged_text", nil
	}
	if FilterSelectedScripts(proposed, scripts) != "" {
		return proposed, "rejected", "residue_remaining", nil
	}
	if hits := protectedRenderingLosses(before, proposed, protected); len(hits) > 0 {
		return proposed, "rejected", "protected_rendering_removed", hits
	}
	return proposed, "accepted", "", nil
}

func protectedRenderingLosses(before, after string, protected map[string]string) []string {
	if len(protected) == 0 {
		return nil
	}
	var hits []string
	for source, target := range protected {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if strings.Contains(before, target) && !strings.Contains(after, target) {
			hits = append(hits, fmt.Sprintf("%s -> %s", source, target))
		}
	}
	sort.Strings(hits)
	return hits
}

func segmentIndex(segments []srt.Segment) map[int]srt.Segment {
	out := make(map[int]srt.Segment, len(segments))
	for _, segment := range segments {
		out[segment.ID] = segment
	}
	return out
}

func segmentPositionIndex(segments []srt.Segment) map[int]int {
	out := make(map[int]int, len(segments))
	for i, segment := range segments {
		out[segment.ID] = i
	}
	return out
}

func addUsage(a, b translation.UsageMetadata) translation.UsageMetadata {
	return translation.UsageMetadata{
		PromptTokenCount:     a.PromptTokenCount + b.PromptTokenCount,
		CandidatesTokenCount: a.CandidatesTokenCount + b.CandidatesTokenCount,
		TotalTokenCount:      a.TotalTokenCount + b.TotalTokenCount,
		WebSearchCount:       a.WebSearchCount + b.WebSearchCount,
	}
}
