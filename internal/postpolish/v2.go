package postpolish

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type v2Chunk struct {
	Index    int
	Segments []RequestSegment
}

type v2InputSegment struct {
	ID              int    `json:"id"`
	SourceText      string `json:"source_text"`
	CurrentText     string `json:"current_text"`
	EditInstruction string `json:"edit_instruction,omitempty"`
}

type segmentInstructionResponse struct {
	Segments []SegmentInstruction `json:"segments"`
}

type chunkInstructionResponse struct {
	EditInstruction string `json:"edit_instruction"`
}

type applicationResponse struct {
	Segments []applicationRow `json:"segments"`
}

type applicationRow struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
	Text       string `json:"text"`
}

func runSegmentLocal(ctx context.Context, client JSONCompleter, sourceSegments, translatedSegments []srt.Segment, cfg Config) (Result, error) {
	return runV2(ctx, client, sourceSegments, translatedSegments, cfg, ProfileSegmentLocal)
}

func runChunkFlow(ctx context.Context, client JSONCompleter, sourceSegments, translatedSegments []srt.Segment, cfg Config) (Result, error) {
	return runV2(ctx, client, sourceSegments, translatedSegments, cfg, ProfileChunkFlow)
}

func runV2(ctx context.Context, client JSONCompleter, sourceSegments, translatedSegments []srt.Segment, cfg Config, profile Profile) (Result, error) {
	if err := validateAligned(sourceSegments, translatedSegments); err != nil {
		return Result{}, err
	}
	base := makeInputs(sourceSegments, translatedSegments)
	chunks, plan, err := v2ChunksFromPlan(base, cfg.ChunkPlan, cfg.ChunkSize)
	if err != nil {
		return Result{}, err
	}
	temperature := DefaultTemperature
	opts := translation.TextCompletionOptions{
		MaxTokens:   cfg.MaxTokens,
		Temperature: &temperature,
	}
	system := v2SystemPrompt(cfg.TargetLanguage.Name, cfg.ProtectedRenderings)
	var usage translation.UsageMetadata
	var accepted []Correction
	var rejected []RejectedCorrection
	var instructions []InstructionRecord
	var appliedRows []AppliedRowRecord
	guardRejected := 0
	failedRequests := 0
	requestOK := 0

	logger.Info("Post-polish v2 started",
		"event", "post_polish_started",
		"profile", profile,
		"source_language", cfg.SourceLanguage.Code,
		"target_language", cfg.TargetLanguage.Code,
		"chunks", len(chunks),
		"chunk_size", cfg.ChunkSize,
		"protected_renderings", len(cfg.ProtectedRenderings),
	)

	for _, chunk := range chunks {
		var instructionRecord InstructionRecord
		var applicationPrompt string
		if profile == ProfileChunkFlow {
			record, usageDelta, err := generateChunkFlowInstruction(ctx, client, system, chunk, cfg, opts)
			usage = addUsage(usage, usageDelta)
			if err != nil {
				failedRequests++
				rejected = append(rejected, RejectedCorrection{Pass: string(profile), ChunkIndex: chunk.Index, Reason: "instruction_failed: " + err.Error()})
				logger.Warn("Post-polish v2 instruction failed", "event", "post_polish_chunk_failed", "profile", profile, "chunk", chunk.Index, "error", err)
				continue
			}
			requestOK++
			instructionRecord = record
			applicationPrompt, err = chunkFlowApplicationPrompt(cfg.SourceLanguage.Name, cfg.TargetLanguage.Name, record.Instruction, chunk)
			if err != nil {
				return Result{}, err
			}
		} else {
			record, usageDelta, err := generateSegmentLocalInstructions(ctx, client, system, chunk, cfg, opts)
			usage = addUsage(usage, usageDelta)
			if err != nil {
				failedRequests++
				rejected = append(rejected, RejectedCorrection{Pass: string(profile), ChunkIndex: chunk.Index, Reason: "instruction_failed: " + err.Error()})
				logger.Warn("Post-polish v2 instruction failed", "event", "post_polish_chunk_failed", "profile", profile, "chunk", chunk.Index, "error", err)
				continue
			}
			requestOK++
			instructionRecord = record
			applicationPrompt, err = segmentLocalApplicationPrompt(cfg.SourceLanguage.Name, cfg.TargetLanguage.Name, record.SegmentInstructions, chunk)
			if err != nil {
				return Result{}, err
			}
		}
		instructions = append(instructions, instructionRecord)
		if cfg.ArtifactDir != "" {
			writeV2InstructionArtifacts(cfg.ArtifactDir, string(profile), chunk.Index, instructionRecord)
		}
		rows, usageDelta, err := applyV2Instructions(ctx, client, chunk, cfg, profile, applicationPrompt, opts, instructionRecord)
		usage = addUsage(usage, usageDelta)
		if err != nil {
			failedRequests++
			rejected = append(rejected, RejectedCorrection{Pass: string(profile), ChunkIndex: chunk.Index, Reason: "application_failed: " + err.Error()})
			logger.Warn("Post-polish v2 application failed", "event", "post_polish_chunk_failed", "profile", profile, "chunk", chunk.Index, "error", err)
			continue
		}
		requestOK++
		if cfg.ArtifactDir != "" {
			writeV2ApplicationArtifacts(cfg.ArtifactDir, string(profile), chunk.Index, applicationPrompt, rows)
		}
		for _, row := range rows {
			appliedRows = append(appliedRows, row.Record)
			if row.Rejected != nil {
				rejected = append(rejected, *row.Rejected)
				if row.Rejected.Reason == "protected_rendering_removed" {
					guardRejected++
				}
				continue
			}
			if row.Correction != nil {
				accepted = append(accepted, *row.Correction)
			}
		}
		logger.Info("Post-polish v2 chunk completed",
			"event", "post_polish_chunk_completed",
			"profile", profile,
			"chunk", chunk.Index,
			"accepted", countAcceptedRows(rows),
			"rejected", countRejectedRows(rows),
		)
	}

	sort.Slice(accepted, func(i, j int) bool {
		return accepted[i].ID < accepted[j].ID
	})
	artifact := Artifact{
		Version:                  2,
		PromptVersion:            promptVersionForProfile(profile),
		InstructionPromptVersion: promptVersionForProfile(profile),
		ApplicationPromptVersion: promptVersionForProfile(profile),
		Profile:                  string(profile),
		Model:                    cfg.Model,
		BaseURL:                  cfg.BaseURL,
		SourceLanguage:           cfg.SourceLanguage.Code,
		TargetLanguage:           cfg.TargetLanguage.Code,
		ChunkSize:                cfg.ChunkSize,
		ChunkPlan:                plan,
		MaxTokens:                cfg.MaxTokens,
		Accepted:                 accepted,
		Rejected:                 rejected,
		GuardRejected:            guardRejected,
		FailedRequests:           failedRequests,
		Usage:                    usage,
		ProtectedRenderings:      len(cfg.ProtectedRenderings),
		Instructions:             instructions,
		AppliedRows:              appliedRows,
		Stats:                    buildArtifactStats(requestOK, failedRequests, accepted, rejected, appliedRows),
	}
	logger.Info("Post-polish v2 completed",
		"event", "post_polish_completed",
		"profile", profile,
		"accepted", len(accepted),
		"rejected", len(rejected),
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

func generateSegmentLocalInstructions(ctx context.Context, client JSONCompleter, system string, chunk v2Chunk, cfg Config, opts translation.TextCompletionOptions) (InstructionRecord, translation.UsageMetadata, error) {
	prompt, err := segmentLocalInstructionPrompt(cfg.SourceLanguage.Name, cfg.TargetLanguage.Name, chunk)
	if err != nil {
		return InstructionRecord{}, translation.UsageMetadata{}, err
	}
	if cfg.ArtifactDir != "" {
		_ = writeText(filepath.Join(cfg.ArtifactDir, string(ProfileSegmentLocal), fmt.Sprintf("chunk_%03d", chunk.Index), "instruction_prompt.txt"), prompt)
	}
	completion, err := client.CompleteJSONWithOptions(ctx, system, prompt, segmentInstructionSchema(chunk.Segments), opts)
	if err != nil {
		return InstructionRecord{}, translation.UsageMetadata{}, err
	}
	var response segmentInstructionResponse
	if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
		return InstructionRecord{}, completion.Usage, err
	}
	instructions, err := validateSegmentInstructions(chunk, response.Segments)
	if err != nil {
		return InstructionRecord{}, completion.Usage, err
	}
	record := InstructionRecord{
		ChunkIndex:          chunk.Index,
		StartID:             chunk.Segments[0].ID,
		EndID:               chunk.Segments[len(chunk.Segments)-1].ID,
		SegmentInstructions: instructions,
	}
	if cfg.ArtifactDir != "" {
		dir := filepath.Join(cfg.ArtifactDir, string(ProfileSegmentLocal), fmt.Sprintf("chunk_%03d", chunk.Index))
		_ = writeText(filepath.Join(dir, "instruction_response.json"), completion.Content)
		_ = writeJSON(filepath.Join(dir, "instruction_parsed.json"), response)
	}
	return record, completion.Usage, nil
}

func generateChunkFlowInstruction(ctx context.Context, client JSONCompleter, system string, chunk v2Chunk, cfg Config, opts translation.TextCompletionOptions) (InstructionRecord, translation.UsageMetadata, error) {
	prompt, err := chunkFlowInstructionPrompt(cfg.SourceLanguage.Name, cfg.TargetLanguage.Name, chunk)
	if err != nil {
		return InstructionRecord{}, translation.UsageMetadata{}, err
	}
	if cfg.ArtifactDir != "" {
		_ = writeText(filepath.Join(cfg.ArtifactDir, string(ProfileChunkFlow), fmt.Sprintf("chunk_%03d", chunk.Index), "instruction_prompt.txt"), prompt)
	}
	completion, err := client.CompleteJSONWithOptions(ctx, system, prompt, chunkInstructionSchema(), opts)
	if err != nil {
		return InstructionRecord{}, translation.UsageMetadata{}, err
	}
	var response chunkInstructionResponse
	if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
		return InstructionRecord{}, completion.Usage, err
	}
	instruction := strings.TrimSpace(response.EditInstruction)
	if instruction == "" {
		instruction = "No change needed."
	}
	record := InstructionRecord{
		ChunkIndex:  chunk.Index,
		StartID:     chunk.Segments[0].ID,
		EndID:       chunk.Segments[len(chunk.Segments)-1].ID,
		Instruction: instruction,
	}
	if cfg.ArtifactDir != "" {
		dir := filepath.Join(cfg.ArtifactDir, string(ProfileChunkFlow), fmt.Sprintf("chunk_%03d", chunk.Index))
		_ = writeText(filepath.Join(dir, "instruction_response.json"), completion.Content)
		_ = writeJSON(filepath.Join(dir, "instruction_parsed.json"), response)
	}
	return record, completion.Usage, nil
}

type v2RowResult struct {
	Record     AppliedRowRecord
	Correction *Correction
	Rejected   *RejectedCorrection
}

func applyV2Instructions(ctx context.Context, client JSONCompleter, chunk v2Chunk, cfg Config, profile Profile, prompt string, opts translation.TextCompletionOptions, instructionRecord InstructionRecord) ([]v2RowResult, translation.UsageMetadata, error) {
	completion, err := client.CompleteJSONWithOptions(ctx, v2ApplicationSystemPrompt(cfg.TargetLanguage.Name), prompt, applicationSchema(chunk.Segments), opts)
	if err != nil {
		return nil, translation.UsageMetadata{}, err
	}
	var response applicationResponse
	if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
		return nil, completion.Usage, err
	}
	rows, err := validateApplicationRows(chunk, response.Segments, cfg, profile, instructionRecord)
	if err != nil {
		return nil, completion.Usage, err
	}
	if cfg.ArtifactDir != "" {
		dir := filepath.Join(cfg.ArtifactDir, string(profile), fmt.Sprintf("chunk_%03d", chunk.Index))
		_ = writeText(filepath.Join(dir, "application_response.json"), completion.Content)
		_ = writeJSON(filepath.Join(dir, "application_parsed.json"), response)
	}
	return rows, completion.Usage, nil
}

func validateApplicationRows(chunk v2Chunk, rows []applicationRow, cfg Config, profile Profile, instructionRecord InstructionRecord) ([]v2RowResult, error) {
	if len(rows) != len(chunk.Segments) {
		return nil, fmt.Errorf("segment count mismatch: got %d want %d", len(rows), len(chunk.Segments))
	}
	results := make([]v2RowResult, 0, len(rows))
	for index, input := range chunk.Segments {
		row := rows[index]
		if row.ID != input.ID {
			return nil, fmt.Errorf("id mismatch at row %d: got %d want %d", index, row.ID, input.ID)
		}
		if row.SourceText != input.SourceText {
			return nil, fmt.Errorf("source_text mismatch at row %d", index)
		}
		text := strings.TrimSpace(row.Text)
		forced := false
		if shouldForceNoChange(profile, instructionRecord, input.ID) {
			text = input.Text
			forced = true
		}
		record := AppliedRowRecord{
			ChunkIndex:     chunk.Index,
			ID:             input.ID,
			SourceText:     input.SourceText,
			Before:         input.Text,
			After:          text,
			ForcedNoChange: forced,
		}
		result := v2RowResult{Record: record}
		rejectBase := RejectedCorrection{
			ID:         input.ID,
			SourceText: input.SourceText,
			Before:     input.Text,
			After:      text,
			Pass:       string(profile),
			ChunkIndex: chunk.Index,
		}
		switch {
		case strings.TrimSpace(text) == "":
			rejectBase.Reason = "empty_text"
			result.Rejected = &rejectBase
		case isMetaOutput(text):
			rejectBase.Reason = "meta_text"
			result.Rejected = &rejectBase
		case text == input.Text:
			// No correction is needed; keep the audit row but do not emit a
			// rejected correction for unchanged rows.
		default:
			if hits := protectedRenderingLosses(input.Text, text, cfg.ProtectedRenderings); len(hits) > 0 {
				rejectBase.Reason = "protected_rendering_removed"
				rejectBase.GuardHits = hits
				result.Rejected = &rejectBase
				break
			}
			result.Correction = &Correction{
				ID:         input.ID,
				SourceText: input.SourceText,
				Before:     input.Text,
				After:      text,
				Pass:       string(profile),
				ChunkIndex: chunk.Index,
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func v2ChunksFromPlan(base []RequestSegment, plan chunker.ChunkPlan, size int) ([]v2Chunk, chunker.ChunkPlan, error) {
	if len(base) == 0 {
		return nil, chunker.ChunkPlan{}, fmt.Errorf("post-polish input has no segments")
	}
	if len(plan.Chunks) == 0 {
		if size <= 0 {
			size = DefaultV2ChunkSize
		}
		var chunks []v2Chunk
		generated := chunker.ChunkPlan{}
		for start := 0; start < len(base); start += size {
			end := start + size
			if end > len(base) {
				end = len(base)
			}
			index := len(chunks)
			chunks = append(chunks, v2Chunk{Index: index, Segments: base[start:end]})
			generated.Chunks = append(generated.Chunks, chunker.PlannedChunk{
				Index:        index,
				StartIndex:   start,
				EndIndex:     end,
				StartID:      base[start].ID,
				EndID:        base[end-1].ID,
				NominalEndID: base[end-1].ID,
				Planner:      chunker.PlannerFixed,
			})
		}
		return chunks, generated, nil
	}
	chunks := make([]v2Chunk, 0, len(plan.Chunks))
	for index, pc := range plan.Chunks {
		if pc.Index != index {
			return nil, chunker.ChunkPlan{}, fmt.Errorf("chunk plan index mismatch at %d: got %d", index, pc.Index)
		}
		if pc.StartIndex < 0 || pc.EndIndex > len(base) || pc.StartIndex >= pc.EndIndex {
			return nil, chunker.ChunkPlan{}, fmt.Errorf("invalid chunk plan range at %d: %d..%d", index, pc.StartIndex, pc.EndIndex)
		}
		chunks = append(chunks, v2Chunk{Index: pc.Index, Segments: base[pc.StartIndex:pc.EndIndex]})
	}
	return chunks, plan, nil
}

func v2SystemPrompt(targetLanguage string, protected map[string]string) string {
	base := fmt.Sprintf("You identify how to revise %s subtitle translations. Return only JSON.", targetLanguage)
	if len(protected) == 0 {
		return base
	}
	keys := make([]string, 0, len(protected))
	for source := range protected {
		keys = append(keys, source)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\nCRITICAL: The following character names MUST be translated as specified:\n")
	for _, source := range keys {
		target := strings.TrimSpace(protected[source])
		if target == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(source)
		b.WriteString(" -> ")
		b.WriteString(target)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func v2ApplicationSystemPrompt(targetLanguage string) string {
	return fmt.Sprintf("You apply edit instructions to %s subtitle translations. Return only JSON.", targetLanguage)
}

func promptVersionForProfile(profile Profile) string {
	switch profile {
	case ProfileChunkFlow:
		return PromptVersionChunkFlow
	case ProfileSegmentLocal:
		return PromptVersionSegmentLocal
	case ProfileLegacy:
		return PromptVersionLegacy
	default:
		return string(profile)
	}
}

func isNoChangeInstruction(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimSuffix(normalized, ".")
	return normalized == "no change needed"
}

func shouldForceNoChange(profile Profile, record InstructionRecord, id int) bool {
	if profile == ProfileChunkFlow {
		return isNoChangeInstruction(record.Instruction)
	}
	for _, row := range record.SegmentInstructions {
		if row.ID == id {
			return isNoChangeInstruction(row.EditInstruction)
		}
	}
	return false
}

func isMetaOutput(text string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	patterns := []string{
		"이 문장",
		"문장",
		"연결됨",
		"same as previous",
		"connected to",
		"continues from",
		"source_text",
		"edit_instruction",
	}
	if strings.Contains(normalized, "번과 연결") {
		return true
	}
	for _, pattern := range patterns {
		if strings.Contains(normalized, pattern) && (strings.Contains(normalized, "연결") || strings.Contains(normalized, "segment") || strings.Contains(normalized, "id")) {
			return true
		}
	}
	return false
}

func countAcceptedRows(rows []v2RowResult) int {
	total := 0
	for _, row := range rows {
		if row.Correction != nil {
			total++
		}
	}
	return total
}

func countRejectedRows(rows []v2RowResult) int {
	total := 0
	for _, row := range rows {
		if row.Rejected != nil {
			total++
		}
	}
	return total
}

func writeV2InstructionArtifacts(root, profile string, chunkIndex int, record InstructionRecord) {
	dir := filepath.Join(root, profile, fmt.Sprintf("chunk_%03d", chunkIndex))
	_ = writeJSON(filepath.Join(dir, "instruction_record.json"), record)
}

func writeV2ApplicationArtifacts(root, profile string, chunkIndex int, prompt string, rows []v2RowResult) {
	dir := filepath.Join(root, profile, fmt.Sprintf("chunk_%03d", chunkIndex))
	_ = writeText(filepath.Join(dir, "application_prompt.txt"), prompt)
	_ = writeJSON(filepath.Join(dir, "application_rows.json"), rows)
}
