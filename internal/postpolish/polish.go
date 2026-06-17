package postpolish

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type JSONCompleter interface {
	CompleteJSONWithOptions(ctx context.Context, systemPrompt, userPrompt string, schema map[string]any, opts translation.TextCompletionOptions) (*translation.TextCompletion, error)
}

func Run(ctx context.Context, client JSONCompleter, sourceSegments, translatedSegments []srt.Segment, cfg Config) (Result, error) {
	cfg = normalizeConfig(cfg)
	if err := validateAligned(sourceSegments, translatedSegments); err != nil {
		return Result{}, err
	}
	base := makeInputs(sourceSegments, translatedSegments)
	system := systemPrompt(cfg.TargetLanguage.Name)
	schema := ResponseSchema()
	temperature := DefaultTemperature
	opts := translation.TextCompletionOptions{
		MaxTokens:   cfg.MaxTokens,
		Temperature: &temperature,
	}

	logger.Info("Post-polish started",
		"event", "post_polish_started",
		"source_language", cfg.SourceLanguage.Code,
		"target_language", cfg.TargetLanguage.Code,
		"broad_chunk_size", cfg.BroadChunkSize,
		"repair_chunk_size", cfg.RepairChunkSize,
		"protected_renderings", len(cfg.ProtectedRenderings),
	)
	broad, err := runPass(ctx, client, system, schema, base, cfg, PassBroad, cfg.BroadChunkSize, opts)
	if err != nil {
		return Result{}, err
	}
	repair, err := runPass(ctx, client, system, schema, base, cfg, PassRepair, cfg.RepairChunkSize, opts)
	if err != nil {
		return Result{}, err
	}

	merged := mergeCorrections(broad.accepted, repair.accepted)
	accepted := mapToSortedCorrections(merged)
	rejected := append([]RejectedCorrection{}, broad.rejected...)
	rejected = append(rejected, repair.rejected...)
	usage := addUsage(broad.usage, repair.usage)
	guardRejected := broad.guardRejected + repair.guardRejected
	failedRequests := broad.failedRequests + repair.failedRequests
	artifact := Artifact{
		Version:             1,
		PromptVersion:       PromptVersion,
		SourceLanguage:      cfg.SourceLanguage.Code,
		TargetLanguage:      cfg.TargetLanguage.Code,
		BroadChunkSize:      cfg.BroadChunkSize,
		RepairChunkSize:     cfg.RepairChunkSize,
		MaxTokens:           cfg.MaxTokens,
		Accepted:            accepted,
		Rejected:            rejected,
		GuardRejected:       guardRejected,
		FailedRequests:      failedRequests,
		Usage:               usage,
		ProtectedRenderings: len(cfg.ProtectedRenderings),
	}
	logger.Info("Post-polish completed",
		"event", "post_polish_completed",
		"broad_corrections", len(broad.accepted),
		"repair_corrections", len(repair.accepted),
		"merged_corrections", len(accepted),
		"guard_rejected", guardRejected,
		"failed_requests", failedRequests,
	)
	return Result{
		Accepted:       accepted,
		Rejected:       rejected,
		GuardRejected:  guardRejected,
		FailedRequests: failedRequests,
		Usage:          usage,
		Artifact:       artifact,
	}, nil
}

func Apply(translated []srt.Segment, corrections []Correction) []srt.Segment {
	if len(corrections) == 0 {
		return translated
	}
	byID := map[int]string{}
	for _, correction := range corrections {
		byID[correction.ID] = correction.After
	}
	out := make([]srt.Segment, len(translated))
	copy(out, translated)
	for i := range out {
		if after, ok := byID[out[i].ID]; ok {
			out[i].Lines = []string{after}
		}
	}
	return out
}

type passResult struct {
	accepted       map[int]Correction
	rejected       []RejectedCorrection
	guardRejected  int
	failedRequests int
	usage          translation.UsageMetadata
}

func runPass(ctx context.Context, client JSONCompleter, system string, schema map[string]any, base []RequestSegment, cfg Config, pass string, chunkSize int, opts translation.TextCompletionOptions) (passResult, error) {
	result := passResult{accepted: map[int]Correction{}}
	chunks := chunkInputs(base, chunkSize)
	logger.Info("Post-polish pass started", "event", "post_polish_pass_started", "pass", pass, "chunks", len(chunks), "chunk_size", chunkSize)
	for i, chunk := range chunks {
		req := Request{Segments: chunk}
		var user string
		var err error
		if pass == PassRepair {
			user, err = repairUserPrompt(cfg.SourceLanguage.Name, cfg.TargetLanguage.Name, req)
		} else {
			user, err = broadUserPrompt(cfg.SourceLanguage.Name, cfg.TargetLanguage.Name, req)
		}
		if err != nil {
			return passResult{}, fmt.Errorf("failed to build post-polish prompt: %w", err)
		}
		if cfg.ArtifactDir != "" {
			writeRequestArtifacts(cfg.ArtifactDir, pass, i, user, nil, nil, nil)
		}
		completion, err := client.CompleteJSONWithOptions(ctx, system, user, schema, opts)
		if err != nil {
			result.failedRequests++
			reject := RejectedCorrection{Pass: pass, ChunkIndex: i, Reason: "request_failed: " + err.Error()}
			result.rejected = append(result.rejected, reject)
			logger.Warn("Post-polish chunk failed", "event", "post_polish_chunk_failed", "pass", pass, "chunk", i, "error", err)
			continue
		}
		result.usage = addUsage(result.usage, completion.Usage)
		var response Response
		parseErr := json.Unmarshal([]byte(completion.Content), &response)
		if cfg.ArtifactDir != "" {
			var parsed any
			if parseErr == nil {
				parsed = response
			}
			writeRequestArtifacts(cfg.ArtifactDir, pass, i, user, &completion.Content, parsed, nil)
		}
		if parseErr != nil {
			result.failedRequests++
			result.rejected = append(result.rejected, RejectedCorrection{Pass: pass, ChunkIndex: i, Reason: "parse_failed: " + parseErr.Error()})
			logger.Warn("Post-polish parse failed", "event", "post_polish_chunk_failed", "pass", pass, "chunk", i, "error", parseErr)
			continue
		}
		accepted, rejected, guardRejected := validateCorrections(chunk, response.Corrections, cfg.ProtectedRenderings, pass, i)
		for _, correction := range accepted {
			result.accepted[correction.ID] = correction
		}
		result.rejected = append(result.rejected, rejected...)
		result.guardRejected += guardRejected
		if cfg.ArtifactDir != "" {
			writeRequestArtifacts(cfg.ArtifactDir, pass, i, user, &completion.Content, response, accepted)
		}
		logger.Info("Post-polish chunk completed",
			"event", "post_polish_chunk_completed",
			"pass", pass,
			"chunk", i,
			"accepted", len(accepted),
			"rejected", len(rejected),
		)
	}
	return result, nil
}

func validateCorrections(chunk []RequestSegment, candidates []ResponseCorrection, protected map[string]string, pass string, chunkIndex int) ([]Correction, []RejectedCorrection, int) {
	byID := map[int]RequestSegment{}
	for _, segment := range chunk {
		byID[segment.ID] = segment
	}
	var accepted []Correction
	var rejected []RejectedCorrection
	guardRejected := 0
	for _, candidate := range candidates {
		input, ok := byID[candidate.ID]
		rejectBase := RejectedCorrection{
			ID:         candidate.ID,
			SourceText: candidate.SourceText,
			After:      candidate.Text,
			Pass:       pass,
			ChunkIndex: chunkIndex,
		}
		if ok {
			rejectBase.Before = input.Text
		}
		switch {
		case !ok:
			rejectBase.Reason = "unknown_id"
			rejected = append(rejected, rejectBase)
			continue
		case candidate.SourceText != input.SourceText:
			rejectBase.Reason = "source_text_mismatch"
			rejected = append(rejected, rejectBase)
			continue
		case strings.TrimSpace(candidate.Text) == "":
			rejectBase.Reason = "empty_text"
			rejected = append(rejected, rejectBase)
			continue
		case candidate.Text == input.Text:
			rejectBase.Reason = "unchanged_text"
			rejected = append(rejected, rejectBase)
			continue
		}
		if hits := protectedRenderingLosses(input.Text, candidate.Text, protected); len(hits) > 0 {
			rejectBase.Reason = "protected_rendering_removed"
			rejectBase.GuardHits = hits
			rejected = append(rejected, rejectBase)
			guardRejected++
			logger.Info("Post-polish guard rejected", "event", "post_polish_guard_rejected", "pass", pass, "chunk", chunkIndex, "id", candidate.ID, "hits", hits)
			continue
		}
		accepted = append(accepted, Correction{
			ID:         candidate.ID,
			SourceText: candidate.SourceText,
			Before:     input.Text,
			After:      candidate.Text,
			Pass:       pass,
			ChunkIndex: chunkIndex,
		})
	}
	return accepted, rejected, guardRejected
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

func normalizeConfig(cfg Config) Config {
	if cfg.BroadChunkSize <= 0 {
		cfg.BroadChunkSize = DefaultBroadChunkSize
	}
	if cfg.RepairChunkSize <= 0 {
		cfg.RepairChunkSize = DefaultRepairChunkSize
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.ProtectedRenderings == nil {
		cfg.ProtectedRenderings = map[string]string{}
	}
	return cfg
}

func validateAligned(sourceSegments, translatedSegments []srt.Segment) error {
	if len(sourceSegments) != len(translatedSegments) {
		return fmt.Errorf("source and translated segment counts differ: source=%d translated=%d", len(sourceSegments), len(translatedSegments))
	}
	for i := range sourceSegments {
		if sourceSegments[i].ID != translatedSegments[i].ID {
			return fmt.Errorf("source and translated segment IDs differ at index %d: source=%d translated=%d", i, sourceSegments[i].ID, translatedSegments[i].ID)
		}
	}
	return nil
}

func makeInputs(sourceSegments, translatedSegments []srt.Segment) []RequestSegment {
	out := make([]RequestSegment, 0, len(sourceSegments))
	for i, source := range sourceSegments {
		out = append(out, RequestSegment{
			ID:         source.ID,
			SourceText: translation.SourceTextFromLines(source.Lines),
			Text:       translation.SourceTextFromLines(translatedSegments[i].Lines),
		})
	}
	return out
}

func chunkInputs(input []RequestSegment, size int) [][]RequestSegment {
	if size <= 0 {
		size = len(input)
	}
	var chunks [][]RequestSegment
	for start := 0; start < len(input); start += size {
		end := start + size
		if end > len(input) {
			end = len(input)
		}
		chunks = append(chunks, input[start:end])
	}
	return chunks
}

func mergeCorrections(broad, repair map[int]Correction) map[int]Correction {
	out := map[int]Correction{}
	for id, correction := range broad {
		out[id] = correction
	}
	for id, correction := range repair {
		out[id] = correction
	}
	return out
}

func mapToSortedCorrections(input map[int]Correction) []Correction {
	out := make([]Correction, 0, len(input))
	for _, correction := range input {
		out = append(out, correction)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
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

func writeRequestArtifacts(root, pass string, chunkIndex int, prompt string, response *string, parsed any, valid any) {
	dir := filepath.Join(root, pass, fmt.Sprintf("chunk_%03d", chunkIndex))
	_ = writeText(filepath.Join(dir, "prompt.txt"), prompt)
	if response != nil {
		_ = writeText(filepath.Join(dir, "response.json"), *response)
	}
	if parsed != nil {
		_ = writeJSON(filepath.Join(dir, "parsed.json"), parsed)
	}
	if valid != nil {
		_ = writeJSON(filepath.Join(dir, "valid_corrections.json"), valid)
	}
}
