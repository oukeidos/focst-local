package recovery

import (
	"context"
	"fmt"

	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translator"
)

// Repair function resumes translation for failed chunks.
// resolvedOutputPath should be the absolute path resolved from the log file location.
func Repair(ctx context.Context, tr *translator.Translator, log *SessionLog, resolvedOutputPath string, forceRepair bool, onProgress func(translator.TranslationProgress)) ([]srt.Segment, []int, error) {
	// 1. Load input SRT
	segments, err := srt.Load(log.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load input subtitles: %w", err)
	}

	// Preprocess to match the state during the first run
	if !log.NoPreprocess {
		segments = srt.PreprocessForPathWithOptions(segments, log.SourceLang, log.InputPath, !log.NoLangPreprocess)
	}

	// 2. Load current output SRT (partial success) to preserve previous translations.
	results := make([]srt.Segment, len(segments))
	copy(results, segments)

	currentOutput, parseErr := srt.Load(resolvedOutputPath)
	outputReason := ""
	if parseErr != nil {
		outputReason = fmt.Sprintf("output parse failed: %v", parseErr)
	} else if len(currentOutput) != len(segments) {
		outputReason = fmt.Sprintf("segment count mismatch: expected %d, got %d", len(segments), len(currentOutput))
	} else {
		copy(results, currentOutput)
	}

	// 3. Translate only failed chunks
	targetChunks := log.FailedChunks
	if outputReason != "" {
		if !forceRepair {
			return nil, nil, fmt.Errorf("existing output could not be reused (%s). Use --force-repair to ignore existing output and re-translate", outputReason)
		}
		totalChunks := log.TotalChunks
		if log.ChunkPlan != nil {
			totalChunks = len(log.ChunkPlan.Chunks)
		}
		if totalChunks <= 0 {
			totalChunks = (len(segments) + log.ChunkSize - 1) / log.ChunkSize
		}
		targetChunks = make([]int, totalChunks)
		for i := 0; i < totalChunks; i++ {
			targetChunks[i] = i
		}
	}

	translated, newFailedChunks, err := tr.TranslateChunks(ctx, segments, targetChunks, onProgress)
	if err != nil {
		return nil, nil, err
	}

	// 4. Merge results: for each segment, if it was successfully translated in this run, use it.
	// Actually TranslateChunks already returns a full slice where successful chunks are updated.
	// But we want to preserve previous successes from 'results'.

	// Create a map of chunks that were NOT failed in this run but WERE in the log
	newlySucceeded := make(map[int]bool)
	failedMap := make(map[int]bool)
	for _, idx := range newFailedChunks {
		failedMap[idx] = true
	}
	for _, idx := range targetChunks {
		if !failedMap[idx] {
			newlySucceeded[idx] = true
		}
	}

	// Merge newly succeeded segments into our 'results'
	for chunkIdx := range newlySucceeded {
		startIdx, endIdx, err := chunkBounds(log, chunkIdx, len(segments))
		if err != nil {
			return nil, nil, err
		}
		for i := startIdx; i < endIdx; i++ {
			results[i] = translated[i]
		}
	}

	return results, newFailedChunks, nil
}

func chunkBounds(log *SessionLog, chunkIdx, segmentCount int) (int, int, error) {
	if log.ChunkPlan != nil {
		if chunkIdx < 0 || chunkIdx >= len(log.ChunkPlan.Chunks) {
			return 0, 0, fmt.Errorf("chunk index out of saved plan range: %d", chunkIdx)
		}
		chunk := log.ChunkPlan.Chunks[chunkIdx]
		if chunk.StartIndex < 0 || chunk.EndIndex < chunk.StartIndex || chunk.EndIndex > segmentCount {
			return 0, 0, fmt.Errorf("invalid saved chunk range for chunk %d: %d..%d", chunkIdx, chunk.StartIndex, chunk.EndIndex)
		}
		return chunk.StartIndex, chunk.EndIndex, nil
	}
	startIdx := chunkIdx * log.ChunkSize
	endIdx := startIdx + log.ChunkSize
	if startIdx < 0 || startIdx > segmentCount {
		return 0, 0, fmt.Errorf("chunk index out of range: %d", chunkIdx)
	}
	if endIdx > segmentCount {
		endIdx = segmentCount
	}
	return startIdx, endIdx, nil
}
