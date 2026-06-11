package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/recovery"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translator"
)

// RunTranslation executes the full translation pipeline.
func RunTranslation(ctx context.Context, cfg Config) (TranslationResult, error) {
	var notes []string
	cfg, notes = cfg.Normalize()
	for _, note := range notes {
		logger.Warn("Config normalized", "detail", note)
	}
	if err := cfg.Validate(); err != nil {
		return TranslationResult{}, fmt.Errorf("invalid configuration: %w", err)
	}

	// 1. Validation & Setup
	absIn, err := filepath.Abs(cfg.InputPath)
	if err != nil {
		return TranslationResult{}, fmt.Errorf("failed to resolve input path: %w", err)
	}
	absOut, err := filepath.Abs(cfg.OutputPath)
	if err != nil {
		return TranslationResult{}, fmt.Errorf("failed to resolve output path: %w", err)
	}
	if absIn == absOut {
		return TranslationResult{}, fmt.Errorf("input and output files are the same (%s)", absIn)
	}
	if inInfo, err := os.Stat(absIn); err == nil {
		if outInfo, err := os.Stat(absOut); err == nil {
			if os.SameFile(inInfo, outInfo) {
				return TranslationResult{}, fmt.Errorf("input and output files are the same (%s)", absIn)
			}
		} else if !os.IsNotExist(err) {
			return TranslationResult{}, fmt.Errorf("failed to stat output path: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return TranslationResult{}, fmt.Errorf("failed to stat input path: %w", err)
	}
	if err := files.RejectSymlinkPath(cfg.OutputPath); err != nil {
		return TranslationResult{}, err
	}
	if cfg.LogPath != "" {
		if err := files.RejectSymlinkPath(cfg.LogPath); err != nil {
			return TranslationResult{}, err
		}
	}

	shouldOverwrite := cfg.Overwrite
	outputExists := false
	if _, err := os.Stat(cfg.OutputPath); err == nil {
		outputExists = true
		if cfg.OnConfirmOverwrite != nil {
			shouldOverwrite = cfg.OnConfirmOverwrite(cfg.OutputPath)
		}
		if !shouldOverwrite {
			logger.Info("Output file exists. Aborted by user.", "path", cfg.OutputPath)
			return TranslationResult{Status: TranslationStatusSkipped}, nil // Not an error, just user cancellation
		}
		logger.Info("Overwriting output file", "path", cfg.OutputPath)
	}

	srcLang, ok := language.GetLanguage(cfg.SourceLang)
	if !ok {
		return TranslationResult{}, fmt.Errorf("unsupported source language: %s", cfg.SourceLang)
	}
	tgtLang, ok := language.GetLanguage(cfg.TargetLang)
	if !ok {
		return TranslationResult{}, fmt.Errorf("unsupported target language: %s", cfg.TargetLang)
	}
	if srcLang.Code == tgtLang.Code {
		return TranslationResult{}, fmt.Errorf("source and target languages must be different (%s)", srcLang.Code)
	}

	// 2. Load and Preprocess
	segments, err := srt.Load(cfg.InputPath)
	if err != nil {
		return TranslationResult{}, fmt.Errorf("failed to load subtitle file: %w", err)
	}
	if err := srt.Validate(segments); err != nil {
		return TranslationResult{}, fmt.Errorf("invalid subtitle file: %w", err)
	}
	logger.Info("Loaded and validated subtitles", "count", len(segments), "path", cfg.InputPath)

	if !cfg.NoPreprocess {
		var idMap []srt.IDMap
		segments, idMap = srt.PreprocessForPathWithMappingOptions(segments, srcLang.Code, cfg.InputPath, !cfg.NoLangPreprocess)
		logger.Info("Preprocessing complete", "count", len(segments))
		if cfg.LogPath != "" && len(idMap) > 0 {
			if err := writeIDMap(cfg.LogPath, idMap); err != nil {
				logger.Warn("Failed to write segment ID mapping", "error", err)
			}
		}
	} else {
		logger.Info("Preprocessing skipped")
	}

	// 3. Initialize Client & Translator
	server, cleanupServer, err := ensureLlamaServer(ctx, cfg, cfg.BaseURL, cfg.Model)
	if err != nil {
		return TranslationResult{}, err
	}
	defer cleanupLlamaServer(cleanupServer)
	cfg.BaseURL = server.BaseURL

	client := localllm.NewClient(cfg.BaseURL, cfg.Model)
	client.SetMaxTokens(cfg.MaxTokens)

	tr, err := translator.NewTranslator(client, cfg.ChunkSize, cfg.ContextSize, cfg.Concurrency, srcLang, tgtLang)
	if err != nil {
		return TranslationResult{}, fmt.Errorf("failed to initialize translator: %w", err)
	}
	var boundaryPlanner chunker.BoundaryPlanner
	if cfg.SentenceAwareChunks && cfg.ChunkBoundaryPlanner == ChunkBoundaryPlannerLocalLLM {
		boundaryPlanner = client
	}
	tr.SetChunkPlanning(cfg.ChunkPlanOptions(), boundaryPlanner)
	if len(cfg.NamesMapping) > 0 {
		tr.SetNamesMapping(cfg.NamesMapping)
		logger.Info("Loaded character name mapping", "count", len(cfg.NamesMapping))
	}

	// 4. Translate
	logger.Info("Starting translation",
		"model", cfg.Model,
		"base_url", cfg.BaseURL,
		"max_tokens", cfg.MaxTokens,
		"sentence_aware_chunks", cfg.SentenceAwareChunks,
		"chunk_boundary_planner", cfg.ChunkBoundaryPlanner,
		"min_chunk_size", cfg.MinChunkSize,
		"max_chunk_size", cfg.MaxChunkSize,
	)
	translated, failed, err := tr.TranslateSRT(ctx, segments, cfg.OnProgress)
	if err != nil {
		return TranslationResult{Usage: tr.GetUsage()}, fmt.Errorf("fatal translation error: %w", err)
	}

	// 5. Handle Results
	chunkPlan := tr.ChunkPlan()
	totalChunks := len(chunkPlan.Chunks)
	if totalChunks == 0 {
		totalChunks = (len(segments) + cfg.ChunkSize - 1) / cfg.ChunkSize
	}
	status := translationStatusFromRecovery(recovery.CalculateStatus(len(failed), totalChunks))
	result := TranslationResult{
		Status:       status,
		Usage:        tr.GetUsage(),
		FailedChunks: len(failed),
		TotalChunks:  totalChunks,
	}
	logger.Info("Translation finished", "status", status)
	canceled := ctx.Err() != nil

	effectiveOutputPath := cfg.OutputPath
	if status == TranslationStatusSuccess || status == TranslationStatusPartialSuccess {
		if !(outputExists && shouldOverwrite) {
			safePath, changed, err := files.SafePath(cfg.OutputPath)
			if err != nil {
				return result, fmt.Errorf("failed to resolve output path: %w", err)
			}
			if changed {
				logger.Warn("Output path adjusted to avoid overwrite", "original", cfg.OutputPath, "effective", safePath)
				effectiveOutputPath = safePath
			}
		}

		outSegments := translated
		if status == TranslationStatusSuccess {
			if !cfg.NoPostprocess {
				logger.Info("Performing post-processing")
				outSegments = srt.PostprocessWithOptions(outSegments, tgtLang.Code, tgtLang.DefaultCPS, !cfg.NoLangPostprocess)
			} else {
				logger.Info("Post-processing skipped")
			}
		} else {
			logger.Info("Skipping post-processing for partial output")
		}

		if err := srt.Save(effectiveOutputPath, outSegments); err != nil {
			return result, fmt.Errorf("failed to save output file: %w", err)
		}
		result.OutputPath = effectiveOutputPath
		logger.Info("Saved results", "path", effectiveOutputPath)
	}

	if status == TranslationStatusPartialSuccess || status == TranslationStatusFailure {
		inputHash, err := recovery.HashFileHex(absIn)
		if err != nil {
			return result, fmt.Errorf("failed to compute input hash for recovery log: %w", err)
		}
		segmentsChecksum := srt.SegmentsChecksumHex(segments)
		logPath := recovery.GenerateRecoveryPath(effectiveOutputPath)

		relativeInputPath, err := recovery.ToRelativeInputPath(logPath, absIn)
		if err != nil {
			return result, fmt.Errorf("failed to convert input path to relative: %w", err)
		}

		// Convert output path to relative (based on log file location).
		relativeOutputPath, err := recovery.ToRelativeOutputPath(logPath, effectiveOutputPath)
		if err != nil {
			return result, fmt.Errorf("failed to convert output path to relative: %w", err)
		}

		relativeNamesPath := ""
		if cfg.NamesPath != "" {
			relativeNamesPath, err = recovery.ToRelativeInputPath(logPath, cfg.NamesPath)
			if err != nil {
				return result, fmt.Errorf("failed to convert names path to relative: %w", err)
			}
		}

		session := &recovery.SessionLog{
			LogVersion:           recovery.CurrentLogVersion,
			InputPath:            relativeInputPath,
			OutputPath:           relativeOutputPath,
			InputHash:            inputHash,
			SegmentsChecksum:     segmentsChecksum,
			Model:                cfg.Model,
			Provider:             "llama.cpp",
			BaseURL:              cfg.BaseURL,
			LlamaServerMode:      string(server.Config.Mode),
			LlamaServerBin:       server.Config.ServerBin,
			LlamaModelPath:       server.Config.ModelPath,
			LlamaCtxSize:         server.Config.CtxSize,
			LlamaParallel:        server.Config.Parallel,
			LlamaExtraArgs:       append([]string(nil), server.Config.ExtraArgs...),
			LlamaLogFile:         server.LogFile,
			MaxTokens:            cfg.MaxTokens,
			NamesPath:            relativeNamesPath,
			ChunkSize:            cfg.ChunkSize,
			ContextSize:          cfg.ContextSize,
			SentenceAwareChunks:  cfg.SentenceAwareChunks,
			MinChunkSize:         cfg.MinChunkSize,
			MaxChunkSize:         cfg.MaxChunkSize,
			ChunkBoundaryPlanner: cfg.ChunkBoundaryPlanner,
			Concurrency:          cfg.Concurrency,
			NoPreprocess:         cfg.NoPreprocess,
			NoPostprocess:        cfg.NoPostprocess,
			NoLangPreprocess:     cfg.NoLangPreprocess,
			NoLangPostprocess:    cfg.NoLangPostprocess,
			SourceLang:           srcLang.Code,
			TargetLang:           tgtLang.Code,
			FailedChunks:         failed,
			TotalChunks:          totalChunks,
			Status:               string(status),
		}
		if len(chunkPlan.Chunks) > 0 {
			session.ChunkPlan = &chunkPlan
		}
		if canceled {
			session.StatusReason = "canceled"
		}
		if err := recovery.SaveSessionLog(logPath, session); err != nil {
			logger.Error("Failed to save recovery log", "error", err)
		} else {
			if status == TranslationStatusPartialSuccess {
				logger.Warn("Partial success - recovery log saved")
			} else {
				logger.Error("Translation failed - recovery log saved")
			}
		}
		result.RecoveryLogPath = logPath
		return result, nil
	}

	return result, nil
}

func writeIDMap(logPath string, mapping []srt.IDMap) error {
	dir := filepath.Dir(logPath)
	base := strings.TrimSuffix(filepath.Base(logPath), filepath.Ext(logPath))
	id := uuid.NewString()
	mapPath := filepath.Join(dir, fmt.Sprintf("%s_idmap_%s.json", base, id))

	data, err := json.Marshal(struct {
		Version int         `json:"version"`
		Mapping []srt.IDMap `json:"mapping"`
	}{
		Version: 1,
		Mapping: mapping,
	})
	if err != nil {
		return err
	}

	if err := files.AtomicWrite(mapPath, data, 0600); err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	logger.Info("Segment ID mapping saved",
		"event", "segment_id_map",
		"mapping_path", mapPath,
		"mapping_count", len(mapping),
		"mapping_hash", "sha256:"+hex.EncodeToString(sum[:]),
	)
	return nil
}
