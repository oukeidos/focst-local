package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/phraseanchor"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
	"github.com/oukeidos/focst-local/internal/translator"
)

type PhraseAnchorExtractionResult struct {
	OutputPath string
	Artifact   phraseanchor.Artifact
	Usage      translation.UsageMetadata
}

func RunPhraseAnchorExtraction(ctx context.Context, cfg Config) (PhraseAnchorExtractionResult, error) {
	var notes []string
	cfg, notes = cfg.Normalize()
	for _, note := range notes {
		logger.Warn("Config normalized", "detail", note)
	}
	if err := cfg.Validate(); err != nil {
		return PhraseAnchorExtractionResult{}, fmt.Errorf("invalid configuration: %w", err)
	}
	if cfg.OutputPath == "" {
		return PhraseAnchorExtractionResult{}, fmt.Errorf("phrase anchors output path is required")
	}
	if err := files.RejectSymlinkPath(cfg.OutputPath); err != nil {
		return PhraseAnchorExtractionResult{}, err
	}
	if cfg.PhraseAnchorsArtifactsDir != "" {
		if err := files.RejectSymlinkPath(cfg.PhraseAnchorsArtifactsDir); err != nil {
			return PhraseAnchorExtractionResult{}, err
		}
	}

	if _, err := os.Stat(cfg.OutputPath); err == nil {
		overwrite := cfg.Overwrite
		if cfg.OnConfirmOverwrite != nil {
			overwrite = cfg.OnConfirmOverwrite(cfg.OutputPath)
		}
		if !overwrite {
			return PhraseAnchorExtractionResult{}, fmt.Errorf("phrase anchors output exists: %s", cfg.OutputPath)
		}
	} else if !os.IsNotExist(err) {
		return PhraseAnchorExtractionResult{}, fmt.Errorf("failed to stat phrase anchors output path: %w", err)
	}

	segments, srcLang, tgtLang, err := loadPreprocessedSegments(cfg)
	if err != nil {
		return PhraseAnchorExtractionResult{}, err
	}

	server, cleanupServer, err := ensureLlamaServer(ctx, cfg, cfg.BaseURL, cfg.Model)
	if err != nil {
		return PhraseAnchorExtractionResult{}, err
	}
	defer cleanupLlamaServer(cleanupServer)
	cfg.BaseURL = server.BaseURL
	client := localllm.NewClient(cfg.BaseURL, cfg.Model)
	client.SetMaxTokens(cfg.MaxTokens)
	client.SetTranslationTimeout(cfg.TranslationTimeout)

	var boundaryPlanner chunker.BoundaryPlanner
	if cfg.SentenceAwareChunks && cfg.ChunkBoundaryPlanner == ChunkBoundaryPlannerLocalLLM {
		boundaryPlanner = client
	}
	_, plan, err := chunker.PlanChunks(ctx, segments, cfg.ChunkPlanOptions(), boundaryPlanner)
	if err != nil {
		return PhraseAnchorExtractionResult{}, fmt.Errorf("failed to plan phrase anchor chunks: %w", err)
	}
	artifact, usage, err := extractPhraseAnchorsWithClient(ctx, cfg, client, segments, plan, srcLang, tgtLang)
	if err != nil {
		return PhraseAnchorExtractionResult{}, err
	}
	if err := phraseanchor.ValidateArtifactForSegments(artifact, segments, srcLang.Code, tgtLang.Code, srt.SegmentsChecksumHex(segments)); err != nil {
		return PhraseAnchorExtractionResult{}, err
	}
	if err := phraseanchor.SaveArtifact(cfg.OutputPath, artifact); err != nil {
		return PhraseAnchorExtractionResult{}, err
	}
	logger.Info("Phrase anchors saved", "event", "phrase_anchors_saved", "path", cfg.OutputPath, "entries", len(artifact.Entries), "rejected", artifact.RejectedCount)
	return PhraseAnchorExtractionResult{OutputPath: cfg.OutputPath, Artifact: artifact, Usage: usage}, nil
}

func extractPhraseAnchorsWithClient(ctx context.Context, cfg Config, client phraseanchor.Client, segments []srt.Segment, plan chunker.ChunkPlan, srcLang, tgtLang language.Language) (phraseanchor.Artifact, translation.UsageMetadata, error) {
	return phraseanchor.Extract(ctx, client, segments, plan, phraseanchor.ExtractConfig{
		InputPath:                cfg.InputPath,
		SegmentsChecksum:         srt.SegmentsChecksumHex(segments),
		SourceLanguageCode:       srcLang.Code,
		SourceLanguageName:       srcLang.Name,
		TargetLanguageCode:       tgtLang.Code,
		TargetLanguageName:       tgtLang.Name,
		Model:                    cfg.Model,
		BaseURL:                  cfg.BaseURL,
		MaxTokens:                cfg.MaxTokens,
		ContextSize:              cfg.ContextSize,
		ChunkSize:                cfg.ChunkSize,
		SentenceAwareChunks:      cfg.SentenceAwareChunks,
		MinChunkSize:             cfg.MinChunkSize,
		MaxChunkSize:             cfg.MaxChunkSize,
		ChunkBoundaryPlanner:     cfg.ChunkBoundaryPlanner,
		ThesisRounds:             cfg.PhraseAnchorThesisRounds,
		SynthesisVotes:           cfg.PhraseAnchorVotes,
		QuoteFilterBatchSize:     cfg.PhraseAnchorQuoteFilterBatchSize,
		ProperFilterRuns:         cfg.PhraseAnchorProperFilterRuns,
		ProperFilterWindowChunks: cfg.PhraseAnchorProperFilterWindowChunks,
		ArtifactDir:              cfg.PhraseAnchorsArtifactsDir,
	})
}

func phraseGuidanceFromArtifact(artifact phraseanchor.Artifact, mapping map[string]string) []translator.PhraseGuidance {
	guidance := make([]translator.PhraseGuidance, 0, len(artifact.Entries))
	for _, entry := range artifact.Entries {
		if phraseAnchorSuppressedByMapping(entry.SourceQuote, mapping) {
			continue
		}
		guidance = append(guidance, translator.PhraseGuidance{
			SegmentID:   entry.SegmentID,
			SourceText:  entry.SourceText,
			Type:        entry.Type,
			SourceQuote: entry.SourceQuote,
			Rendering:   entry.Rendering,
		})
	}
	return guidance
}

func phraseAnchorSuppressedByMapping(sourceQuote string, mapping map[string]string) bool {
	if len(mapping) == 0 {
		return false
	}
	quote := strings.TrimSpace(sourceQuote)
	if quote == "" {
		return false
	}
	for src := range mapping {
		if strings.TrimSpace(src) == quote {
			return true
		}
	}
	return false
}
