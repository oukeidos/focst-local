package glossary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/translation"
)

type RenderingSafetyFilterConfig struct {
	SourceLang   language.Language
	TargetLang   language.Language
	MaxTokens    int
	ArtifactDir  string
	SnippetLimit int
	BatchSize    int
}

type RenderingSafetyFilterResult struct {
	Entries   []Entry
	Info      RenderingSafetyFilterInfo
	Judgments []RenderingSafetyJudgment
}

func ApplyRenderingSafetyFilter(
	ctx context.Context,
	completer translation.TextCompleter,
	entries []Entry,
	allSegments []Segment,
	cfg RenderingSafetyFilterConfig,
) (RenderingSafetyFilterResult, translation.UsageMetadata, error) {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.SnippetLimit <= 0 {
		cfg.SnippetLimit = DefaultRenderingSafetySnippetLimit
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultRenderingSafetyBatchSize
	}
	info := RenderingSafetyFilterInfo{
		Version:            RenderingSafetyFilterVersion,
		Applied:            true,
		SourceEntryCount:   len(entries),
		FilteredEntryCount: len(entries),
	}
	if len(entries) == 0 {
		return RenderingSafetyFilterResult{Entries: entries, Info: info}, translation.UsageMetadata{}, nil
	}
	filterDir := ""
	if cfg.ArtifactDir != "" {
		filterDir = filepath.Join(cfg.ArtifactDir, "rendering_safety_filter")
		if err := os.MkdirAll(filterDir, 0700); err != nil {
			return RenderingSafetyFilterResult{}, translation.UsageMetadata{}, fmt.Errorf("failed to create glossary rendering safety filter artifact dir: %w", err)
		}
	}
	rows := buildRenderingSafetyRows(entries, allSegments, cfg.SnippetLimit)
	logger.Info("Glossary rendering safety filter started",
		"event", "glossary_rendering_safety_filter_started",
		"entries", len(entries),
	)
	started := time.Now()
	var usage translation.UsageMetadata
	judgments := make([]RenderingSafetyJudgment, 0, len(entries))
	for start := 0; start < len(rows); start += cfg.BatchSize {
		end := start + cfg.BatchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]
		batchIndex := start/cfg.BatchSize + 1
		prompt := RenderRenderingSafetyPrompt(cfg.SourceLang, cfg.TargetLang, batch)
		if filterDir != "" {
			if err := os.WriteFile(filepath.Join(filterDir, fmt.Sprintf("batch_%03d_prompt.txt", batchIndex)), []byte(prompt), 0600); err != nil {
				return RenderingSafetyFilterResult{}, usage, fmt.Errorf("failed to write glossary rendering safety prompt: %w", err)
			}
		}
		resp, err := completeRenderingSafetyText(ctx, completer, RenderingSafetyFilterSystemPrompt, prompt, cfg.MaxTokens)
		if err != nil {
			return RenderingSafetyFilterResult{}, usage, fmt.Errorf("glossary rendering safety filter batch %d failed: %w", batchIndex, err)
		}
		usage.PromptTokenCount += resp.Usage.PromptTokenCount
		usage.CandidatesTokenCount += resp.Usage.CandidatesTokenCount
		usage.TotalTokenCount += resp.Usage.TotalTokenCount
		if filterDir != "" {
			if err := os.WriteFile(filepath.Join(filterDir, fmt.Sprintf("batch_%03d_response.md", batchIndex)), []byte(resp.Content), 0600); err != nil {
				return RenderingSafetyFilterResult{}, usage, fmt.Errorf("failed to write glossary rendering safety response: %w", err)
			}
			if err := WriteJSON(filepath.Join(filterDir, fmt.Sprintf("batch_%03d_usage.json", batchIndex)), resp.Usage); err != nil {
				return RenderingSafetyFilterResult{}, usage, fmt.Errorf("failed to write glossary rendering safety usage: %w", err)
			}
		}
		parsed, violations := ParseRenderingSafetyTable(resp.Content, batch)
		if filterDir != "" {
			if err := WriteJSON(filepath.Join(filterDir, fmt.Sprintf("batch_%03d_judgments.json", batchIndex)), parsed); err != nil {
				return RenderingSafetyFilterResult{}, usage, fmt.Errorf("failed to write glossary rendering safety judgments: %w", err)
			}
			if len(violations) > 0 {
				if err := WriteJSON(filepath.Join(filterDir, fmt.Sprintf("batch_%03d_violations.json", batchIndex)), violations); err != nil {
					return RenderingSafetyFilterResult{}, usage, fmt.Errorf("failed to write glossary rendering safety violations: %w", err)
				}
			}
		}
		if len(violations) > 0 {
			return RenderingSafetyFilterResult{}, usage, fmt.Errorf("glossary rendering safety filter batch %d parse violations: %v", batchIndex, violations)
		}
		judgments = append(judgments, parsed...)
	}
	judgmentByRow := make(map[int]RenderingSafetyJudgment, len(judgments))
	for _, judgment := range judgments {
		judgmentByRow[judgment.Row] = judgment
	}
	kept := make([]Entry, 0, len(entries))
	for i, entry := range entries {
		row := i + 1
		judgment, ok := judgmentByRow[row]
		if !ok {
			return RenderingSafetyFilterResult{}, usage, fmt.Errorf("missing glossary rendering safety judgment for row %d", row)
		}
		if judgment.SourcePolicyResult == SourcePolicyKeep {
			kept = append(kept, entry)
			continue
		}
		logger.Info("Glossary rendering safety filter dropped entry",
			"event", "glossary_rendering_safety_filter_dropped",
			"source", entry.Source,
			"rendering", entry.Rendering,
			"expected_strategy", judgment.ExpectedStrategy,
			"fit", judgment.Fit,
			"decision", judgment.Decision,
		)
	}
	info.FilteredEntryCount = len(kept)
	info.DroppedCount = len(entries) - len(kept)
	if filterDir != "" {
		if err := WriteJSON(filepath.Join(filterDir, "judgments.json"), judgments); err != nil {
			return RenderingSafetyFilterResult{}, usage, fmt.Errorf("failed to write glossary rendering safety combined judgments: %w", err)
		}
		if err := WriteJSON(filepath.Join(filterDir, "filtered_glossary.json"), kept); err != nil {
			return RenderingSafetyFilterResult{}, usage, fmt.Errorf("failed to write glossary rendering safety filtered entries: %w", err)
		}
	}
	logger.Info("Glossary rendering safety filter completed",
		"event", "glossary_rendering_safety_filter_completed",
		"kept", len(kept),
		"dropped", info.DroppedCount,
		"duration_ms", time.Since(started).Milliseconds(),
		"prompt_tokens", usage.PromptTokenCount,
		"completion_tokens", usage.CandidatesTokenCount,
	)
	return RenderingSafetyFilterResult{Entries: kept, Info: info, Judgments: judgments}, usage, nil
}

func completeRenderingSafetyText(ctx context.Context, completer translation.TextCompleter, systemPrompt, userPrompt string, maxTokens int) (*translation.TextCompletion, error) {
	if withOptions, ok := completer.(translation.TextCompleterWithOptions); ok {
		return withOptions.CompleteTextWithOptions(ctx, systemPrompt, userPrompt, translation.TextCompletionOptions{
			MaxTokens:   maxTokens,
			Temperature: DefaultRenderingSafetyTemperature,
			TopP:        DefaultRenderingSafetyTopP,
			TopK:        DefaultRenderingSafetyTopK,
		})
	}
	return completer.CompleteText(ctx, systemPrompt, userPrompt, maxTokens)
}

func buildRenderingSafetyRows(entries []Entry, allSegments []Segment, snippetLimit int) []renderingSafetyRow {
	segmentByID := make(map[int]Segment, len(allSegments))
	for _, segment := range allSegments {
		segmentByID[segment.ID] = segment
	}
	rows := make([]renderingSafetyRow, 0, len(entries))
	for i, entry := range entries {
		rows = append(rows, renderingSafetyRow{
			Row:         i + 1,
			Source:      entry.Source,
			Rendering:   entry.Rendering,
			Occurrences: pickRenderingSafetyOccurrences(entry, allSegments, segmentByID, snippetLimit),
		})
	}
	return rows
}

func pickRenderingSafetyOccurrences(entry Entry, allSegments []Segment, segmentByID map[int]Segment, limit int) []Segment {
	out := make([]Segment, 0, limit)
	for _, id := range entry.OccurrenceIDs {
		segment, ok := segmentByID[id]
		if !ok || segment.SourceText == "" {
			continue
		}
		out = append(out, segment)
		if len(out) >= limit {
			return out
		}
	}
	for _, segment := range allSegments {
		if entry.Source != "" && strings.Contains(segment.SourceText, entry.Source) {
			out = append(out, segment)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}
