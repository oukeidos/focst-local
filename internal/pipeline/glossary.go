package pipeline

import (
	"context"
	"fmt"
	"os"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/glossary"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type GlossaryExtractionResult struct {
	OutputPath string
	Artifact   glossary.Artifact
	Usage      translation.UsageMetadata
}

func RunGlossaryExtraction(ctx context.Context, cfg Config) (GlossaryExtractionResult, error) {
	var notes []string
	cfg, notes = cfg.Normalize()
	for _, note := range notes {
		logger.Warn("Config normalized", "detail", note)
	}
	if err := cfg.Validate(); err != nil {
		return GlossaryExtractionResult{}, fmt.Errorf("invalid configuration: %w", err)
	}
	if cfg.OutputPath == "" {
		return GlossaryExtractionResult{}, fmt.Errorf("glossary output path is required")
	}
	if err := files.RejectSymlinkPath(cfg.OutputPath); err != nil {
		return GlossaryExtractionResult{}, err
	}
	if cfg.GlossaryArtifactsDir != "" {
		if err := files.RejectSymlinkPath(cfg.GlossaryArtifactsDir); err != nil {
			return GlossaryExtractionResult{}, err
		}
	}

	outputExists := false
	if _, err := os.Stat(cfg.OutputPath); err == nil {
		outputExists = true
		overwrite := cfg.Overwrite
		if cfg.OnConfirmOverwrite != nil {
			overwrite = cfg.OnConfirmOverwrite(cfg.OutputPath)
		}
		if !overwrite {
			return GlossaryExtractionResult{}, fmt.Errorf("glossary output exists: %s", cfg.OutputPath)
		}
	} else if !os.IsNotExist(err) {
		return GlossaryExtractionResult{}, fmt.Errorf("failed to stat glossary output path: %w", err)
	}
	_ = outputExists

	segments, srcLang, tgtLang, err := loadPreprocessedSegments(cfg)
	if err != nil {
		return GlossaryExtractionResult{}, err
	}

	server, cleanupServer, err := ensureLlamaServer(ctx, cfg, cfg.BaseURL, cfg.Model)
	if err != nil {
		return GlossaryExtractionResult{}, err
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
		return GlossaryExtractionResult{}, fmt.Errorf("failed to plan glossary chunks: %w", err)
	}
	artifact, usage, err := extractGlossaryWithClient(ctx, cfg, client, segments, plan, srcLang, tgtLang)
	if err != nil {
		return GlossaryExtractionResult{}, err
	}
	if err := glossary.SaveArtifact(cfg.OutputPath, artifact); err != nil {
		return GlossaryExtractionResult{}, err
	}
	logger.Info("Glossary saved", "event", "glossary_saved", "path", cfg.OutputPath, "entries", len(artifact.Entries), "rejected", artifact.RejectedCount)
	return GlossaryExtractionResult{OutputPath: cfg.OutputPath, Artifact: artifact, Usage: usage}, nil
}

func extractGlossaryWithClient(ctx context.Context, cfg Config, client translation.TextCompleter, segments []srt.Segment, plan chunker.ChunkPlan, srcLang, tgtLang language.Language) (glossary.Artifact, translation.UsageMetadata, error) {
	return glossary.Extract(ctx, client, segments, plan, glossary.ExtractConfig{
		InputPath:        cfg.InputPath,
		SegmentsChecksum: srt.SegmentsChecksumHex(segments),
		SourceLang:       srcLang,
		TargetLang:       tgtLang,
		Model:            cfg.Model,
		BaseURL:          cfg.BaseURL,
		Runs:             cfg.GlossaryRuns,
		WindowChunks:     cfg.GlossaryWindowChunks,
		MaxTokens:        cfg.MaxTokens,
		ArtifactDir:      cfg.GlossaryArtifactsDir,
	})
}

func loadPreprocessedSegments(cfg Config) ([]srt.Segment, language.Language, language.Language, error) {
	srcLang, ok := language.GetLanguage(cfg.SourceLang)
	if !ok {
		return nil, language.Language{}, language.Language{}, fmt.Errorf("unsupported source language: %s", cfg.SourceLang)
	}
	tgtLang, ok := language.GetLanguage(cfg.TargetLang)
	if !ok {
		return nil, language.Language{}, language.Language{}, fmt.Errorf("unsupported target language: %s", cfg.TargetLang)
	}
	if srcLang.Code == tgtLang.Code {
		return nil, language.Language{}, language.Language{}, fmt.Errorf("source and target languages must be different (%s)", srcLang.Code)
	}
	segments, err := srt.Load(cfg.InputPath)
	if err != nil {
		return nil, language.Language{}, language.Language{}, fmt.Errorf("failed to load subtitle file: %w", err)
	}
	if err := srt.Validate(segments); err != nil {
		return nil, language.Language{}, language.Language{}, fmt.Errorf("invalid subtitle file: %w", err)
	}
	logger.Info("Loaded and validated subtitles", "count", len(segments), "path", cfg.InputPath)
	if !cfg.NoPreprocess {
		segments = srt.PreprocessForPathWithOptions(segments, srcLang.Code, cfg.InputPath, !cfg.NoLangPreprocess)
		logger.Info("Preprocessing complete", "count", len(segments))
	} else {
		logger.Info("Preprocessing skipped")
	}
	return segments, srcLang, tgtLang, nil
}
