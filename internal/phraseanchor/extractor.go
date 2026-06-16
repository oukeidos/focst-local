package phraseanchor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type Client interface {
	CompleteText(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (*translation.TextCompletion, error)
	CompleteTextChat(ctx context.Context, messages []localllm.TextChatMessage, maxTokens int) (*translation.TextCompletion, error)
	CompleteTextChatWithSampler(ctx context.Context, messages []localllm.TextChatMessage, maxTokens int, temperature, topP float64, topK int) (*translation.TextCompletion, error)
}

func Extract(ctx context.Context, client Client, segments []srt.Segment, plan chunker.ChunkPlan, cfg ExtractConfig) (Artifact, translation.UsageMetadata, error) {
	cfg = normalizeConfig(cfg)
	windows, err := BuildWindows(segments, plan, cfg.ContextSize)
	if err != nil {
		return Artifact{}, translation.UsageMetadata{}, err
	}
	if cfg.ArtifactDir != "" {
		if err := initArtifactDirs(cfg.ArtifactDir); err != nil {
			return Artifact{}, translation.UsageMetadata{}, err
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "input", "chunk_plan.json"), plan); err != nil {
			return Artifact{}, translation.UsageMetadata{}, err
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "input", "windows.json"), windows); err != nil {
			return Artifact{}, translation.UsageMetadata{}, err
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "config.json"), runConfig(cfg)); err != nil {
			return Artifact{}, translation.UsageMetadata{}, err
		}
	}

	targetName := languageName(cfg.TargetLanguageName, cfg.TargetLanguageCode, "target")
	var usage translation.UsageMetadata
	var rejected []RejectedCandidate
	var allCandidates []Candidate
	for _, window := range windows {
		candidates, usageDelta, rejectedDelta, err := runCandidateDiscovery(ctx, client, window, cfg)
		if err != nil {
			return Artifact{}, usage, err
		}
		usage = addUsage(usage, usageDelta)
		rejected = append(rejected, rejectedDelta...)
		allCandidates = append(allCandidates, candidates...)
	}
	allCandidates = dedupeCandidates(allCandidates)
	sortCandidates(allCandidates)
	if cfg.ArtifactDir != "" {
		if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", "candidates_raw.md"), renderCandidateTable(allCandidates, targetName)+"\n"); err != nil {
			return Artifact{}, usage, err
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "parsed", "candidates_raw.json"), allCandidates); err != nil {
			return Artifact{}, usage, err
		}
	}

	filtered, removed, usageDelta, err := runQuoteKindFilter(ctx, client, allCandidates, cfg)
	if err != nil {
		return Artifact{}, usage, err
	}
	usage = addUsage(usage, usageDelta)
	if cfg.ArtifactDir != "" {
		if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", "quote_kind_kept.md"), renderCandidateTable(filtered, targetName)+"\n"); err != nil {
			return Artifact{}, usage, err
		}
		if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", "quote_kind_removed.md"), renderCandidateTable(removed, targetName)+"\n"); err != nil {
			return Artifact{}, usage, err
		}
	}

	filtered, removedByName, usageDelta, err := runSourceNameFilter(ctx, client, segments, plan, filtered, cfg)
	if err != nil {
		return Artifact{}, usage, err
	}
	usage = addUsage(usage, usageDelta)
	if cfg.ArtifactDir != "" {
		if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", "source_name_kept.md"), renderCandidateTable(filtered, targetName)+"\n"); err != nil {
			return Artifact{}, usage, err
		}
		if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", "source_name_removed.md"), renderCandidateTable(removedByName, targetName)+"\n"); err != nil {
			return Artifact{}, usage, err
		}
	}

	alternatives, usageDelta, rejectedDelta, err := runAlternatives(ctx, client, filtered, cfg)
	if err != nil {
		return Artifact{}, usage, err
	}
	usage = addUsage(usage, usageDelta)
	rejected = append(rejected, rejectedDelta...)

	entries, usageDelta, err := runRenderingVotes(ctx, client, filtered, alternatives, windows, cfg)
	if err != nil {
		return Artifact{}, usage, err
	}
	usage = addUsage(usage, usageDelta)

	artifact := Artifact{
		Version:       1,
		PromptVersion: PromptVersion,
		SourceLang:    cfg.SourceLanguageCode,
		TargetLang:    cfg.TargetLanguageCode,
		CreatedAt:     time.Now().UTC(),
		Input: InputInfo{
			Path:                     cfg.InputPath,
			PreprocessedSegmentCount: len(segments),
			SegmentsChecksum:         cfg.SegmentsChecksum,
		},
		Config:        runConfig(cfg),
		StageMapping:  defaultStageMap(),
		ChunkPlan:     plan,
		Entries:       entries,
		RejectedCount: len(rejected),
	}
	if cfg.ArtifactDir != "" {
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "merged_phrase_anchors.json"), artifact); err != nil {
			return Artifact{}, usage, err
		}
		if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "parsed", "rejected.json"), rejected); err != nil {
			return Artifact{}, usage, err
		}
	}
	return artifact, usage, nil
}

func BuildWindows(segments []srt.Segment, plan chunker.ChunkPlan, contextSize int) ([]Window, error) {
	if len(plan.Chunks) == 0 {
		return nil, fmt.Errorf("phrase anchor extraction requires a non-empty chunk plan")
	}
	var windows []Window
	for _, pc := range plan.Chunks {
		if pc.StartIndex < 0 || pc.EndIndex > len(segments) || pc.StartIndex >= pc.EndIndex {
			return nil, fmt.Errorf("invalid phrase anchor chunk range: %d..%d", pc.StartIndex, pc.EndIndex)
		}
		targetLen := pc.EndIndex - pc.StartIndex
		if targetLen < 2 {
			windows = append(windows, buildWindow(len(windows), pc.Index, segments, pc.StartIndex, pc.EndIndex, contextSize))
			continue
		}
		mid := pc.StartIndex + targetLen/2
		windows = append(windows, buildWindow(len(windows), pc.Index, segments, pc.StartIndex, mid, contextSize))
		windows = append(windows, buildWindow(len(windows), pc.Index, segments, mid, pc.EndIndex, contextSize))
	}
	return windows, nil
}

func buildWindow(index, chunkIndex int, segments []srt.Segment, start, end, contextSize int) Window {
	beforeStart := start - contextSize
	if beforeStart < 0 {
		beforeStart = 0
	}
	afterEnd := end + contextSize
	if afterEnd > len(segments) {
		afterEnd = len(segments)
	}
	target := toSegmentData(segments[start:end])
	return Window{
		Index:                 index,
		TranslationChunkIndex: chunkIndex,
		StartIndex:            start,
		EndIndex:              end,
		StartID:               target[0].ID,
		EndID:                 target[len(target)-1].ID,
		ContextBefore:         toSegmentData(segments[beforeStart:start]),
		Target:                target,
		ContextAfter:          toSegmentData(segments[end:afterEnd]),
	}
}

func runCandidateDiscovery(ctx context.Context, client Client, window Window, cfg ExtractConfig) ([]Candidate, translation.UsageMetadata, []RejectedCandidate, error) {
	sourceJSON, err := windowSourceJSON(window)
	if err != nil {
		return nil, translation.UsageMetadata{}, nil, err
	}
	sourceName := languageName(cfg.SourceLanguageName, cfg.SourceLanguageCode, "source")
	targetName := languageName(cfg.TargetLanguageName, cfg.TargetLanguageCode, "target")
	reviewSystemPrompt := RenderReviewSystemPrompt(sourceName, targetName)
	messages := []localllm.TextChatMessage{{Role: "system", Content: reviewSystemPrompt}}
	var usage translation.UsageMetadata
	var candidates []Candidate
	var rejected []RejectedCandidate
	for round := 1; round <= cfg.ThesisRounds; round++ {
		stage := fmt.Sprintf("window_%03d_candidate_round_%02d", window.Index, round)
		prompt := RenderCandidateAddPrompt(targetName)
		if round == 1 {
			prompt = RenderCandidateDiscoveryPrompt(window.StartID, window.EndID, sourceJSON, sourceName, targetName)
		}
		if cfg.ArtifactDir != "" {
			if err := WriteText(filepath.Join(cfg.ArtifactDir, "prompts", stage+".txt"), prompt); err != nil {
				return nil, usage, rejected, err
			}
		}
		requestMessages := append([]localllm.TextChatMessage(nil), messages...)
		requestMessages = append(requestMessages, localllm.TextChatMessage{Role: "user", Content: prompt})
		started := time.Now()
		resp, err := client.CompleteTextChat(ctx, requestMessages, cfg.MaxTokens)
		if err != nil {
			return nil, usage, rejected, fmt.Errorf("%s failed: %w", stage, err)
		}
		usage = addUsage(usage, resp.Usage)
		content := strings.TrimSpace(resp.Content)
		if cfg.ArtifactDir != "" {
			if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", stage+".md"), content+"\n"); err != nil {
				return nil, usage, rejected, err
			}
			if err := WriteJSON(filepath.Join(cfg.ArtifactDir, "usage", stage+".json"), map[string]any{
				"usage":        resp.Usage,
				"wall_seconds": time.Since(started).Seconds(),
			}); err != nil {
				return nil, usage, rejected, err
			}
		}
		parsed, rejectedRows := parseCandidateTable(content, window, CandidateDiscoveryName, targetName)
		candidates = append(candidates, parsed...)
		rejected = append(rejected, rejectedRows...)
		messages = append(requestMessages, localllm.TextChatMessage{Role: "assistant", Content: content})
	}
	return candidates, usage, rejected, nil
}

func runQuoteKindFilter(ctx context.Context, client Client, candidates []Candidate, cfg ExtractConfig) ([]Candidate, []Candidate, translation.UsageMetadata, error) {
	if len(candidates) == 0 {
		return candidates, nil, translation.UsageMetadata{}, nil
	}
	unique := uniqueQuoteKindCandidates(candidates)
	categoryByKey := map[string]string{}
	var usage translation.UsageMetadata
	for start, batchIndex := 0, 1; start < len(unique); start, batchIndex = start+cfg.QuoteFilterBatchSize, batchIndex+1 {
		end := start + cfg.QuoteFilterBatchSize
		if end > len(unique) {
			end = len(unique)
		}
		batch := unique[start:end]
		stage := fmt.Sprintf("quote_kind_batch_%03d", batchIndex)
		prompt := RenderQuoteKindPrompt(batch)
		if cfg.ArtifactDir != "" {
			if err := WriteText(filepath.Join(cfg.ArtifactDir, "prompts", stage+".txt"), prompt); err != nil {
				return nil, nil, usage, err
			}
		}
		resp, err := client.CompleteTextChatWithSampler(ctx, []localllm.TextChatMessage{{Role: "user", Content: prompt}}, QuoteKindFilterMaxTokens, QuoteKindFilterTemperature, localllm.DefaultTopP, localllm.DefaultTopK)
		if err != nil {
			return nil, nil, usage, fmt.Errorf("%s failed: %w", stage, err)
		}
		usage = addUsage(usage, resp.Usage)
		content := strings.TrimSpace(resp.Content)
		if cfg.ArtifactDir != "" {
			if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", stage+".md"), content+"\n"); err != nil {
				return nil, nil, usage, err
			}
		}
		parsed := parseQuoteKindChoices(content)
		for localIndex, candidate := range batch {
			category := parsed[localIndex+1]
			if category == "" {
				category = CategoryUnclassified
			}
			categoryByKey[quoteKindKey(candidate)] = category
		}
	}
	var kept, removed []Candidate
	for _, candidate := range candidates {
		category := categoryByKey[quoteKindKey(candidate)]
		if category == CategoryProperNoun || category == CategoryCommonNoun {
			removed = append(removed, candidate)
			continue
		}
		kept = append(kept, candidate)
	}
	return kept, removed, usage, nil
}

func runSourceNameFilter(ctx context.Context, client Client, segments []srt.Segment, plan chunker.ChunkPlan, candidates []Candidate, cfg ExtractConfig) ([]Candidate, []Candidate, translation.UsageMetadata, error) {
	if len(candidates) == 0 {
		return candidates, nil, translation.UsageMetadata{}, nil
	}
	windows, err := buildSourceNameWindows(segments, plan, cfg.ProperFilterWindowChunks)
	if err != nil {
		return nil, nil, translation.UsageMetadata{}, err
	}
	var usage translation.UsageMetadata
	sourceSet := map[string]bool{}
	sourceName := languageName(cfg.SourceLanguageName, cfg.SourceLanguageCode, "source")
	for _, window := range windows {
		for run := 1; run <= cfg.ProperFilterRuns; run++ {
			stage := fmt.Sprintf("source_name_window_%03d_run_%02d", window.Index, run)
			prompt, err := RenderSourceNamePrompt(sourceName, SourceNameFilterSourceKind, window.Target)
			if err != nil {
				return nil, nil, usage, err
			}
			if cfg.ArtifactDir != "" {
				if err := WriteText(filepath.Join(cfg.ArtifactDir, "prompts", stage+".txt"), prompt); err != nil {
					return nil, nil, usage, err
				}
			}
			resp, err := client.CompleteText(ctx, SourceNameFilterSystemPrompt, prompt, cfg.MaxTokens)
			if err != nil {
				return nil, nil, usage, fmt.Errorf("%s failed: %w", stage, err)
			}
			usage = addUsage(usage, resp.Usage)
			content := strings.TrimSpace(resp.Content)
			if cfg.ArtifactDir != "" {
				if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", stage+".md"), content+"\n"); err != nil {
					return nil, nil, usage, err
				}
			}
			for _, source := range parseSourceNameTable(content) {
				sourceSet[normalizeSource(source)] = true
			}
		}
	}
	var kept, removed []Candidate
	for _, candidate := range candidates {
		if sourceSet[normalizeSource(candidate.SourceQuote)] {
			removed = append(removed, candidate)
			continue
		}
		kept = append(kept, candidate)
	}
	return kept, removed, usage, nil
}

func runAlternatives(ctx context.Context, client Client, candidates []Candidate, cfg ExtractConfig) ([]Alternative, translation.UsageMetadata, []RejectedCandidate, error) {
	var usage translation.UsageMetadata
	var alternatives []Alternative
	var rejected []RejectedCandidate
	sourceName := languageName(cfg.SourceLanguageName, cfg.SourceLanguageCode, "source")
	targetName := languageName(cfg.TargetLanguageName, cfg.TargetLanguageCode, "target")
	reviewSystemPrompt := RenderReviewSystemPrompt(sourceName, targetName)
	for i, candidate := range candidates {
		stage := fmt.Sprintf("alternative_%04d", i+1)
		prompt := RenderAlternativePrompt(renderAlternativeInputTable(candidate, targetName), targetName)
		if cfg.ArtifactDir != "" {
			if err := WriteText(filepath.Join(cfg.ArtifactDir, "prompts", stage+".txt"), prompt); err != nil {
				return nil, usage, rejected, err
			}
		}
		resp, err := client.CompleteText(ctx, reviewSystemPrompt, prompt, cfg.MaxTokens)
		if err != nil {
			return nil, usage, rejected, fmt.Errorf("%s failed: %w", stage, err)
		}
		usage = addUsage(usage, resp.Usage)
		content := strings.TrimSpace(resp.Content)
		if cfg.ArtifactDir != "" {
			if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", stage+".md"), content+"\n"); err != nil {
				return nil, usage, rejected, err
			}
		}
		alt, rejectedRows := parseAlternativeTable(content, candidate, targetName)
		rejected = append(rejected, rejectedRows...)
		if alt.AlternativeRendering != "" {
			alternatives = append(alternatives, alt)
		}
	}
	return alternatives, usage, rejected, nil
}

func runRenderingVotes(ctx context.Context, client Client, candidates []Candidate, alternatives []Alternative, windows []Window, cfg ExtractConfig) ([]Entry, translation.UsageMetadata, error) {
	voteRows, _ := renderVoteInputTable(candidates, alternatives)
	if len(voteRows) == 0 {
		return entriesFromCandidates(candidates, nil), translation.UsageMetadata{}, nil
	}
	sourceJSONByWindow := map[int]string{}
	for _, window := range windows {
		src, err := windowSourceJSON(window)
		if err != nil {
			return nil, translation.UsageMetadata{}, err
		}
		sourceJSONByWindow[window.Index] = src
	}
	rowsByWindow := map[int][]VoteRow{}
	indexByKey := map[string]int{}
	for index, row := range voteRows {
		rowsByWindow[row.Candidate.PhraseWindowIndex] = append(rowsByWindow[row.Candidate.PhraseWindowIndex], row)
		indexByKey[voteRowKey(row)] = index
	}
	tallies := make([]VoteTally, len(voteRows))
	var usage translation.UsageMetadata
	sourceName := languageName(cfg.SourceLanguageName, cfg.SourceLanguageCode, "source")
	targetName := languageName(cfg.TargetLanguageName, cfg.TargetLanguageCode, "target")
	reviewSystemPrompt := RenderReviewSystemPrompt(sourceName, targetName)
	windowIndexes := make([]int, 0, len(rowsByWindow))
	for windowIndex := range rowsByWindow {
		windowIndexes = append(windowIndexes, windowIndex)
	}
	sort.Ints(windowIndexes)
	for _, windowIndex := range windowIndexes {
		localRows, voteTable := renderVoteTableRows(rowsByWindow[windowIndex])
		sourceJSON := sourceJSONByWindow[windowIndex]
		for vote := 1; vote <= cfg.SynthesisVotes; vote++ {
			stage := fmt.Sprintf("rendering_vote_window_%03d_%02d", windowIndex, vote)
			prompt := RenderVotePrompt(voteTable, sourceJSON, targetName)
			if cfg.ArtifactDir != "" {
				if err := WriteText(filepath.Join(cfg.ArtifactDir, "prompts", stage+".txt"), prompt); err != nil {
					return nil, usage, err
				}
			}
			resp, err := client.CompleteText(ctx, reviewSystemPrompt, prompt, cfg.MaxTokens)
			if err != nil {
				return nil, usage, fmt.Errorf("%s failed: %w", stage, err)
			}
			usage = addUsage(usage, resp.Usage)
			content := strings.TrimSpace(resp.Content)
			if cfg.ArtifactDir != "" {
				if err := WriteText(filepath.Join(cfg.ArtifactDir, "contents", stage+".md"), content+"\n"); err != nil {
					return nil, usage, err
				}
			}
			choices := parseVoteChoices(content)
			for _, localRow := range localRows {
				globalIndex := indexByKey[voteRowKey(localRow)]
				choice := choices[localRow.Row]
				switch choice {
				case "A":
					tallies[globalIndex].CountA++
				case "B":
					tallies[globalIndex].CountB++
				default:
					tallies[globalIndex].InvalidVotes++
				}
			}
		}
	}
	entries := entriesFromVoteRows(voteRows, tallies)
	voted := map[string]bool{}
	for _, row := range voteRows {
		voted[candidateKey(row.Candidate)] = true
	}
	for _, candidate := range candidates {
		if voted[candidateKey(candidate)] {
			continue
		}
		entries = append(entries, Entry{
			SegmentID:             candidate.SegmentID,
			SourceText:            candidate.SourceText,
			Type:                  candidate.Type,
			SourceQuote:           candidate.SourceQuote,
			Rendering:             candidate.Rendering,
			TranslationChunkIndex: candidate.TranslationChunkIndex,
			PhraseWindowIndex:     candidate.PhraseWindowIndex,
			Origin:                "thesis",
		})
	}
	sortEntries(entries)
	return entries, usage, nil
}

func entriesFromVoteRows(rows []VoteRow, tallies []VoteTally) []Entry {
	out := make([]Entry, 0, len(rows))
	for i, row := range rows {
		tally := tallies[i]
		rendering := row.A
		origin := "thesis"
		if tally.CountB > tally.CountA {
			rendering = row.B
			origin = "antithesis"
		}
		out = append(out, Entry{
			SegmentID:             row.Candidate.SegmentID,
			SourceText:            row.Candidate.SourceText,
			Type:                  row.Candidate.Type,
			SourceQuote:           row.Candidate.SourceQuote,
			Rendering:             rendering,
			TranslationChunkIndex: row.Candidate.TranslationChunkIndex,
			PhraseWindowIndex:     row.Candidate.PhraseWindowIndex,
			Origin:                origin,
			Votes: map[string]int{
				"thesis":     tally.CountA,
				"antithesis": tally.CountB,
				"invalid":    tally.InvalidVotes,
			},
		})
	}
	sortEntries(out)
	return out
}

func entriesFromCandidates(candidates []Candidate, votes map[string]VoteTally) []Entry {
	out := make([]Entry, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, Entry{
			SegmentID:             candidate.SegmentID,
			SourceText:            candidate.SourceText,
			Type:                  candidate.Type,
			SourceQuote:           candidate.SourceQuote,
			Rendering:             candidate.Rendering,
			TranslationChunkIndex: candidate.TranslationChunkIndex,
			PhraseWindowIndex:     candidate.PhraseWindowIndex,
			Origin:                "thesis",
		})
	}
	sortEntries(out)
	return out
}

type sourceNameWindow struct {
	Index      int
	ChunkStart int
	ChunkEnd   int
	Target     []translation.SegmentData
}

func buildSourceNameWindows(segments []srt.Segment, plan chunker.ChunkPlan, windowChunks int) ([]sourceNameWindow, error) {
	if windowChunks <= 0 {
		windowChunks = DefaultProperFilterWindowChunks
	}
	if len(plan.Chunks) == 0 {
		return nil, fmt.Errorf("source-name filter requires a non-empty chunk plan")
	}
	var windows []sourceNameWindow
	for i := 0; i < len(plan.Chunks); i += windowChunks {
		endChunk := i + windowChunks
		if endChunk > len(plan.Chunks) {
			endChunk = len(plan.Chunks)
		}
		start := plan.Chunks[i].StartIndex
		end := plan.Chunks[endChunk-1].EndIndex
		if start < 0 || end > len(segments) || start >= end {
			return nil, fmt.Errorf("invalid source-name window range: %d..%d", start, end)
		}
		windows = append(windows, sourceNameWindow{
			Index:      len(windows),
			ChunkStart: i,
			ChunkEnd:   endChunk - 1,
			Target:     toSegmentData(segments[start:end]),
		})
	}
	return windows, nil
}

func windowSourceJSON(window Window) (string, error) {
	payload := struct {
		ContextBefore []translation.SegmentData `json:"context_before"`
		Target        []translation.SegmentData `json:"target"`
		ContextAfter  []translation.SegmentData `json:"context_after"`
	}{
		ContextBefore: window.ContextBefore,
		Target:        window.Target,
		ContextAfter:  window.ContextAfter,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func toSegmentData(segments []srt.Segment) []translation.SegmentData {
	out := make([]translation.SegmentData, len(segments))
	for i, segment := range segments {
		out[i] = translation.SegmentData{
			ID:         segment.ID,
			SourceText: translation.SourceTextFromLines(segment.Lines),
		}
	}
	return out
}

func languageName(name, code, fallback string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	code = strings.TrimSpace(code)
	if code != "" {
		return code
	}
	return fallback
}

func uniqueQuoteKindCandidates(candidates []Candidate) []Candidate {
	seen := map[string]bool{}
	var out []Candidate
	for _, candidate := range candidates {
		key := quoteKindKey(candidate)
		if key == "\x00" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func quoteKindKey(candidate Candidate) string {
	return strings.TrimSpace(candidate.SourceText) + "\x00" + strings.TrimSpace(candidate.SourceQuote)
}

func dedupeCandidates(candidates []Candidate) []Candidate {
	seen := map[string]bool{}
	var out []Candidate
	for _, candidate := range candidates {
		key := candidateKey(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].SegmentID != candidates[j].SegmentID {
			return candidates[i].SegmentID < candidates[j].SegmentID
		}
		if candidates[i].SourceQuote != candidates[j].SourceQuote {
			return candidates[i].SourceQuote < candidates[j].SourceQuote
		}
		return candidates[i].Rendering < candidates[j].Rendering
	})
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].SegmentID != entries[j].SegmentID {
			return entries[i].SegmentID < entries[j].SegmentID
		}
		if entries[i].SourceQuote != entries[j].SourceQuote {
			return entries[i].SourceQuote < entries[j].SourceQuote
		}
		return entries[i].Rendering < entries[j].Rendering
	})
}

func normalizeConfig(cfg ExtractConfig) ExtractConfig {
	if cfg.ThesisRounds <= 0 {
		cfg.ThesisRounds = DefaultThesisRounds
	}
	if cfg.SynthesisVotes <= 0 {
		cfg.SynthesisVotes = DefaultSynthesisVotes
	}
	if cfg.QuoteFilterBatchSize <= 0 {
		cfg.QuoteFilterBatchSize = DefaultQuoteFilterBatchSize
	}
	if cfg.ProperFilterRuns <= 0 {
		cfg.ProperFilterRuns = DefaultProperFilterRuns
	}
	if cfg.ProperFilterWindowChunks <= 0 {
		cfg.ProperFilterWindowChunks = DefaultProperFilterWindowChunks
	}
	return cfg
}

func defaultStageMap() StageMap {
	return StageMap{
		CandidateDiscovery:   ExperimentCandidateDiscovery,
		QuoteKindFilter:      ExperimentQuoteKindFilter,
		SourceNameFilter:     ExperimentSourceNameFilter,
		AlternativeRendering: ExperimentAlternative,
	}
}

func runConfig(cfg ExtractConfig) RunConfig {
	return RunConfig{
		Model:                    cfg.Model,
		BaseURL:                  cfg.BaseURL,
		MaxTokens:                cfg.MaxTokens,
		TranslationChunkSize:     cfg.ChunkSize,
		ContextSize:              cfg.ContextSize,
		SentenceAwareChunks:      cfg.SentenceAwareChunks,
		MinChunkSize:             cfg.MinChunkSize,
		MaxChunkSize:             cfg.MaxChunkSize,
		ChunkBoundaryPlanner:     cfg.ChunkBoundaryPlanner,
		Temperature:              localllm.DefaultTemperature,
		TopP:                     localllm.DefaultTopP,
		TopK:                     localllm.DefaultTopK,
		ThesisRounds:             cfg.ThesisRounds,
		SynthesisVotes:           cfg.SynthesisVotes,
		QuoteFilterBatchSize:     cfg.QuoteFilterBatchSize,
		QuoteFilterTemperature:   QuoteKindFilterTemperature,
		QuoteFilterMaxTokens:     QuoteKindFilterMaxTokens,
		ProperFilterRuns:         cfg.ProperFilterRuns,
		ProperFilterWindowChunks: cfg.ProperFilterWindowChunks,
	}
}

func initArtifactDirs(root string) error {
	for _, dir := range []string{"input", "prompts", "contents", "parsed", "usage"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return err
		}
	}
	return nil
}

func addUsage(a, b translation.UsageMetadata) translation.UsageMetadata {
	return translation.UsageMetadata{
		PromptTokenCount:     a.PromptTokenCount + b.PromptTokenCount,
		CandidatesTokenCount: a.CandidatesTokenCount + b.CandidatesTokenCount,
		TotalTokenCount:      a.TotalTokenCount + b.TotalTokenCount,
		WebSearchCount:       a.WebSearchCount + b.WebSearchCount,
	}
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func normalizeSource(value string) string {
	return norm.NFKC.String(collapseSpaces(value))
}

func logStage(name string, count int, duration time.Duration, usage translation.UsageMetadata) {
	logger.Info("Phrase anchor stage completed",
		"event", "phrase_anchor_stage_completed",
		"stage", name,
		"count", count,
		"duration_ms", duration.Milliseconds(),
		"prompt_tokens", usage.PromptTokenCount,
		"completion_tokens", usage.CandidatesTokenCount,
	)
}

func _keepStrconvImportedForGeneratedTables(v int) string {
	return strconv.Itoa(v)
}
