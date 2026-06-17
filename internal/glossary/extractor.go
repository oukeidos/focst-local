package glossary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type ExtractConfig struct {
	InputPath        string
	SegmentsChecksum string
	SourceLang       language.Language
	TargetLang       language.Language
	Model            string
	BaseURL          string
	Runs             int
	WindowChunks     int
	MaxTokens        int
	ArtifactDir      string
}

func Extract(ctx context.Context, completer translation.TextCompleter, segments []srt.Segment, plan chunker.ChunkPlan, cfg ExtractConfig) (Artifact, translation.UsageMetadata, error) {
	if cfg.Runs <= 0 {
		cfg.Runs = DefaultRuns
	}
	if cfg.WindowChunks <= 0 {
		cfg.WindowChunks = DefaultWindowChunks
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	allSegments := toGlossarySegments(segments)
	windows, err := BuildWindows(segments, plan, cfg.WindowChunks)
	if err != nil {
		return Artifact{}, translation.UsageMetadata{}, err
	}
	if cfg.ArtifactDir != "" {
		if err := os.MkdirAll(cfg.ArtifactDir, 0700); err != nil {
			return Artifact{}, translation.UsageMetadata{}, fmt.Errorf("failed to create glossary artifacts dir: %w", err)
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "glossary_config.json"), map[string]any{
			"prompt_version":                      PromptVersion,
			"source_language":                     cfg.SourceLang.Code,
			"target_language":                     cfg.TargetLang.Code,
			"model":                               cfg.Model,
			"base_url":                            cfg.BaseURL,
			"temperature":                         1,
			"top_p":                               0.95,
			"top_k":                               64,
			"max_tokens":                          cfg.MaxTokens,
			"glossary_runs":                       cfg.Runs,
			"glossary_window_chunks":              cfg.WindowChunks,
			"response_format":                     "text",
			"rendering_safety_filter_version":     RenderingSafetyFilterVersion,
			"rendering_safety_filter_enabled":     true,
			"rendering_safety_filter_temperature": DefaultRenderingSafetyTemperature,
			"rendering_safety_filter_top_p":       DefaultRenderingSafetyTopP,
			"rendering_safety_filter_top_k":       DefaultRenderingSafetyTopK,
		}); err != nil {
			return Artifact{}, translation.UsageMetadata{}, fmt.Errorf("failed to write glossary config: %w", err)
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "windows.json"), windows); err != nil {
			return Artifact{}, translation.UsageMetadata{}, fmt.Errorf("failed to write glossary windows: %w", err)
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "chunk_plan.json"), plan); err != nil {
			return Artifact{}, translation.UsageMetadata{}, fmt.Errorf("failed to write glossary chunk plan: %w", err)
		}
	}

	var usage translation.UsageMetadata
	var accepted []ValidatedCandidate
	var allRejected []RejectedCandidate
	expectedHeader := RenderingHeader(cfg.TargetLang)
	for _, window := range windows {
		windowDir := ""
		if cfg.ArtifactDir != "" {
			windowDir = filepath.Join(cfg.ArtifactDir, fmt.Sprintf("window_%03d", window.Index))
			if err := os.MkdirAll(windowDir, 0700); err != nil {
				return Artifact{}, usage, fmt.Errorf("failed to create glossary window dir: %w", err)
			}
		}
		var windowRecords []RunRecord
		for run := 1; run <= cfg.Runs; run++ {
			userPrompt, err := RenderUserPrompt(cfg.SourceLang, cfg.TargetLang, window.Segments)
			if err != nil {
				return Artifact{}, usage, err
			}
			promptPath := ""
			if windowDir != "" {
				promptPath = filepath.Join(windowDir, fmt.Sprintf("run_%03d_prompt.txt", run))
				if err := os.WriteFile(promptPath, []byte(userPrompt), 0600); err != nil {
					return Artifact{}, usage, fmt.Errorf("failed to write glossary prompt: %w", err)
				}
			}
			started := time.Now()
			resp, err := completer.CompleteText(ctx, SystemPrompt, userPrompt, cfg.MaxTokens)
			if err != nil {
				return Artifact{}, usage, fmt.Errorf("glossary window %d run %d failed: %w", window.Index, run, err)
			}
			usage.PromptTokenCount += resp.Usage.PromptTokenCount
			usage.CandidatesTokenCount += resp.Usage.CandidatesTokenCount
			usage.TotalTokenCount += resp.Usage.TotalTokenCount
			parseResult := ParseMarkdownTable(resp.Content, expectedHeader, window.Index, run)
			validated, rejected := ValidateCandidates(parseResult.Candidates, allSegments)
			rejected = append(rejected, parseResult.Rejected...)
			allRejected = append(allRejected, rejected...)
			accepted = append(accepted, validated...)

			record := RunRecord{
				WindowIndex: window.Index,
				RunIndex:    run,
				PromptPath:  promptPath,
				Response:    resp.Content,
				Usage:       resp.Usage,
				Parsed:      parseResult.Candidates,
				Rejected:    rejected,
				Violations:  parseResult.Violations,
			}
			windowRecords = append(windowRecords, record)
			if windowDir != "" {
				if err := os.WriteFile(filepath.Join(windowDir, fmt.Sprintf("run_%03d_response.md", run)), []byte(resp.Content), 0600); err != nil {
					return Artifact{}, usage, fmt.Errorf("failed to write glossary response: %w", err)
				}
				if err := WriteJSON(filepath.Join(windowDir, fmt.Sprintf("run_%03d_parsed.json", run)), parseResult.Candidates); err != nil {
					return Artifact{}, usage, fmt.Errorf("failed to write parsed glossary rows: %w", err)
				}
				if err := WriteJSON(filepath.Join(windowDir, fmt.Sprintf("run_%03d_rejected.json", run)), rejected); err != nil {
					return Artifact{}, usage, fmt.Errorf("failed to write rejected glossary rows: %w", err)
				}
				if err := WriteJSON(filepath.Join(windowDir, fmt.Sprintf("run_%03d_usage.json", run)), resp.Usage); err != nil {
					return Artifact{}, usage, fmt.Errorf("failed to write glossary usage: %w", err)
				}
			}
			logger.Info("Glossary run completed",
				"event", "glossary_run_completed",
				"window", window.Index,
				"run", run,
				"duration_ms", time.Since(started).Milliseconds(),
				"prompt_tokens", resp.Usage.PromptTokenCount,
				"completion_tokens", resp.Usage.CandidatesTokenCount,
				"accepted", len(validated),
				"rejected", len(rejected),
			)
		}
		if windowDir != "" {
			if err := WriteJSON(filepath.Join(windowDir, "window_summary.json"), windowRecords); err != nil {
				return Artifact{}, usage, fmt.Errorf("failed to write glossary window summary: %w", err)
			}
		}
	}

	entries := Merge(accepted, allSegments)
	createdAt := time.Now().UTC()
	if cfg.ArtifactDir != "" {
		prefilter := Artifact{
			Version:       1,
			PromptVersion: PromptVersion,
			SourceLang:    cfg.SourceLang.Code,
			TargetLang:    cfg.TargetLang.Code,
			CreatedAt:     createdAt,
			Input: InputInfo{
				Path:                     cfg.InputPath,
				PreprocessedSegmentCount: len(segments),
				SegmentsChecksum:         cfg.SegmentsChecksum,
			},
			Config: RunConfig{
				Model:                cfg.Model,
				BaseURL:              cfg.BaseURL,
				Temperature:          1,
				TopP:                 0.95,
				TopK:                 64,
				MaxTokens:            cfg.MaxTokens,
				GlossaryRuns:         cfg.Runs,
				GlossaryWindowChunks: cfg.WindowChunks,
			},
			Entries:       entries,
			RejectedCount: len(allRejected),
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "merged_glossary.prefilter.json"), prefilter); err != nil {
			return Artifact{}, usage, fmt.Errorf("failed to write prefilter glossary debug artifact: %w", err)
		}
	}
	filterResult, filterUsage, err := ApplyRenderingSafetyFilter(ctx, completer, entries, allSegments, RenderingSafetyFilterConfig{
		SourceLang:  cfg.SourceLang,
		TargetLang:  cfg.TargetLang,
		MaxTokens:   cfg.MaxTokens,
		ArtifactDir: cfg.ArtifactDir,
	})
	if err != nil {
		return Artifact{}, usage, err
	}
	usage.PromptTokenCount += filterUsage.PromptTokenCount
	usage.CandidatesTokenCount += filterUsage.CandidatesTokenCount
	usage.TotalTokenCount += filterUsage.TotalTokenCount
	entries = filterResult.Entries
	filterInfo := filterResult.Info
	artifact := Artifact{
		Version:       1,
		PromptVersion: PromptVersion,
		SourceLang:    cfg.SourceLang.Code,
		TargetLang:    cfg.TargetLang.Code,
		CreatedAt:     createdAt,
		Input: InputInfo{
			Path:                     cfg.InputPath,
			PreprocessedSegmentCount: len(segments),
			SegmentsChecksum:         cfg.SegmentsChecksum,
		},
		Config: RunConfig{
			Model:                cfg.Model,
			BaseURL:              cfg.BaseURL,
			Temperature:          1,
			TopP:                 0.95,
			TopK:                 64,
			MaxTokens:            cfg.MaxTokens,
			GlossaryRuns:         cfg.Runs,
			GlossaryWindowChunks: cfg.WindowChunks,
		},
		RenderingSafetyFilter: &filterInfo,
		Entries:               entries,
		RejectedCount:         len(allRejected),
	}
	if cfg.ArtifactDir != "" {
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "merged_glossary.json"), artifact); err != nil {
			return Artifact{}, usage, fmt.Errorf("failed to write merged glossary debug artifact: %w", err)
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "names_compatible.json"), NamesCompatible(entries, cfg.SourceLang.Code, cfg.TargetLang.Code)); err != nil {
			return Artifact{}, usage, fmt.Errorf("failed to write names-compatible glossary artifact: %w", err)
		}
	}
	return artifact, usage, nil
}

func BuildWindows(segments []srt.Segment, plan chunker.ChunkPlan, windowChunks int) ([]Window, error) {
	if windowChunks <= 0 {
		windowChunks = DefaultWindowChunks
	}
	if len(plan.Chunks) == 0 {
		return nil, fmt.Errorf("glossary extraction requires a non-empty chunk plan")
	}
	var windows []Window
	for i := 0; i < len(plan.Chunks); i += windowChunks {
		endChunk := i + windowChunks
		if endChunk > len(plan.Chunks) {
			endChunk = len(plan.Chunks)
		}
		startIndex := plan.Chunks[i].StartIndex
		endIndex := plan.Chunks[endChunk-1].EndIndex
		if startIndex < 0 || endIndex > len(segments) || startIndex >= endIndex {
			return nil, fmt.Errorf("invalid glossary window range: %d..%d", startIndex, endIndex)
		}
		windowSegments := toGlossarySegments(segments[startIndex:endIndex])
		windows = append(windows, Window{
			Index:      len(windows),
			ChunkStart: i,
			ChunkEnd:   endChunk - 1,
			StartID:    windowSegments[0].ID,
			EndID:      windowSegments[len(windowSegments)-1].ID,
			Segments:   windowSegments,
		})
	}
	return windows, nil
}

func toGlossarySegments(segments []srt.Segment) []Segment {
	out := make([]Segment, len(segments))
	for i, segment := range segments {
		out[i] = Segment{
			ID:         segment.ID,
			SourceText: translation.SourceTextFromLines(segment.Lines),
		}
	}
	return out
}
