package chunker

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/srt"
)

const (
	PlannerFixed           = "fixed"
	PlannerPunctuation     = "punctuation"
	PlannerLocalLLM        = "local_llm"
	PlannerFallbackNominal = "fallback_nominal"

	PunctuationStrongTerminal = "strong_terminal"
	PunctuationWeakTerminal   = "weak_terminal"
	PunctuationNonterminal    = "nonterminal"
	PunctuationFinal          = "final"
)

// PlanOptions controls sentence-aware chunk boundary selection.
type PlanOptions struct {
	NominalSize         int
	MinSize             int
	MaxSize             int
	ContextSize         int
	EnableSentenceAware bool
}

// ChunkPlan stores the exact chunk boundaries used for a translation session.
type ChunkPlan struct {
	Chunks []PlannedChunk `json:"chunks"`
}

// PlannedChunk is one target chunk in a reproducible chunk plan.
type PlannedChunk struct {
	Index               int    `json:"index"`
	StartIndex          int    `json:"start_index"`
	EndIndex            int    `json:"end_index"`
	StartID             int    `json:"start_id"`
	EndID               int    `json:"end_id"`
	NominalEndID        int    `json:"nominal_end_id"`
	Planner             string `json:"planner"`
	PunctuationClass    string `json:"punctuation_class,omitempty"`
	SplitReason         string `json:"split_reason,omitempty"`
	Confidence          string `json:"confidence,omitempty"`
	AllowedMinEndID     int    `json:"allowed_min_end_id,omitempty"`
	AllowedMaxEndID     int    `json:"allowed_max_end_id,omitempty"`
	DistanceFromNominal int    `json:"distance_from_nominal"`
}

// BoundaryRequest is sent to a local model when punctuation is not decisive.
type BoundaryRequest struct {
	Segments             []srt.Segment
	AllowedSplitAfterIDs []int
}

// BoundaryDecision is returned by a boundary planner.
type BoundaryDecision struct {
	SplitAfterID     int
	Confidence       string
	Reason           string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	WallSeconds      float64
}

// BoundaryPlanner chooses one split_after_id from BoundaryRequest.
type BoundaryPlanner interface {
	PlanBoundary(ctx context.Context, request BoundaryRequest) (BoundaryDecision, error)
}

// PlanChunks builds chunks using fixed chunking or sentence-aware chunking.
func PlanChunks(ctx context.Context, segments []srt.Segment, opts PlanOptions, planner BoundaryPlanner) ([]Chunk, ChunkPlan, error) {
	if opts.NominalSize <= 0 {
		return nil, ChunkPlan{}, fmt.Errorf("nominal chunk size must be greater than 0")
	}
	if opts.ContextSize < 0 {
		return nil, ChunkPlan{}, fmt.Errorf("context size must be 0 or greater")
	}
	if !opts.EnableSentenceAware {
		chunks := SplitIntoChunks(segments, opts.NominalSize, opts.ContextSize)
		return chunks, PlanFromChunks(chunks, PlannerFixed, opts.NominalSize), nil
	}
	normalized, err := normalizeOptions(opts)
	if err != nil {
		return nil, ChunkPlan{}, err
	}
	plan := buildSentenceAwarePlan(ctx, segments, normalized, planner)
	chunks, err := ChunksFromPlan(segments, normalized.ContextSize, plan)
	if err != nil {
		return nil, ChunkPlan{}, err
	}
	return chunks, plan, nil
}

// PlanFromChunks records a fixed chunk plan.
func PlanFromChunks(chunks []Chunk, planner string, nominalSize int) ChunkPlan {
	plan := ChunkPlan{Chunks: make([]PlannedChunk, 0, len(chunks))}
	for _, chunk := range chunks {
		pc := plannedChunkFromRange(chunk.Index, chunk.StartIndex, chunk.EndIndex, chunk.EndIndex, chunksSegments(chunk), planner, "")
		plan.Chunks = append(plan.Chunks, pc)
	}
	return plan
}

func chunksSegments(chunk Chunk) []srt.Segment {
	return chunk.Target
}

// ChunksFromPlan reconstructs translation chunks and context from a saved plan.
func ChunksFromPlan(segments []srt.Segment, contextSize int, plan ChunkPlan) ([]Chunk, error) {
	if contextSize < 0 {
		return nil, fmt.Errorf("context size must be 0 or greater")
	}
	chunks := make([]Chunk, 0, len(plan.Chunks))
	for i, pc := range plan.Chunks {
		if pc.Index != i {
			return nil, fmt.Errorf("chunk plan index mismatch at %d: got %d", i, pc.Index)
		}
		if pc.StartIndex < 0 || pc.EndIndex < pc.StartIndex || pc.EndIndex > len(segments) {
			return nil, fmt.Errorf("invalid chunk plan range at %d: %d..%d", i, pc.StartIndex, pc.EndIndex)
		}
		if pc.StartIndex == pc.EndIndex {
			return nil, fmt.Errorf("empty chunk plan range at %d", i)
		}
		beforeStart := pc.StartIndex - contextSize
		if beforeStart < 0 {
			beforeStart = 0
		}
		afterEnd := pc.EndIndex + contextSize
		if afterEnd > len(segments) {
			afterEnd = len(segments)
		}
		chunks = append(chunks, Chunk{
			Index:      i,
			StartIndex: pc.StartIndex,
			EndIndex:   pc.EndIndex,
			Target:     segments[pc.StartIndex:pc.EndIndex],
			Context: BeforeAfterContext{
				Before: segments[beforeStart:pc.StartIndex],
				After:  segments[pc.EndIndex:afterEnd],
			},
		})
	}
	return chunks, nil
}

func normalizeOptions(opts PlanOptions) (PlanOptions, error) {
	if opts.MinSize <= 0 {
		opts.MinSize = opts.NominalSize
	}
	if opts.MaxSize <= 0 {
		opts.MaxSize = opts.NominalSize
	}
	if opts.MinSize > opts.MaxSize {
		return opts, fmt.Errorf("min chunk size %d is greater than max chunk size %d", opts.MinSize, opts.MaxSize)
	}
	if opts.NominalSize < opts.MinSize || opts.NominalSize > opts.MaxSize {
		return opts, fmt.Errorf("nominal chunk size %d must be inside range %d..%d", opts.NominalSize, opts.MinSize, opts.MaxSize)
	}
	return opts, nil
}

func buildSentenceAwarePlan(ctx context.Context, segments []srt.Segment, opts PlanOptions, planner BoundaryPlanner) ChunkPlan {
	plan := ChunkPlan{}
	for start := 0; start < len(segments); {
		remaining := len(segments) - start
		if remaining <= opts.MaxSize {
			end := len(segments)
			pc := plannedChunkFromAbsoluteRange(len(plan.Chunks), start, end, min(start+opts.NominalSize, end), segments, PlannerFixed, PunctuationFinal)
			pc.SplitReason = "final chunk"
			plan.Chunks = append(plan.Chunks, pc)
			break
		}

		minEnd := start + opts.MinSize
		maxEnd := start + opts.MaxSize
		nominalEnd := start + opts.NominalSize

		chosenEnd, punctuationClass, ok := choosePunctuationBoundary(segments, minEnd, maxEnd, nominalEnd)
		plannerName := PlannerPunctuation
		reason := "strong terminal punctuation"
		confidence := "high"
		if !ok {
			chosenEnd, plannerName, punctuationClass, confidence, reason = chooseLLMBoundary(ctx, segments, start, minEnd, maxEnd, nominalEnd, planner)
		}

		pc := plannedChunkFromAbsoluteRange(len(plan.Chunks), start, chosenEnd, nominalEnd, segments, plannerName, punctuationClass)
		pc.AllowedMinEndID = segments[minEnd-1].ID
		pc.AllowedMaxEndID = segments[maxEnd-1].ID
		pc.Confidence = confidence
		pc.SplitReason = reason
		plan.Chunks = append(plan.Chunks, pc)
		logBoundary(pc)
		start = chosenEnd
	}
	return plan
}

func choosePunctuationBoundary(segments []srt.Segment, minEnd, maxEnd, nominalEnd int) (int, string, bool) {
	bestEnd := -1
	bestDistance := 0
	for end := minEnd; end <= maxEnd; end++ {
		class := ClassifyPunctuation(segments[end-1])
		if class != PunctuationStrongTerminal {
			continue
		}
		distance := abs(end - nominalEnd)
		if bestEnd == -1 || distance < bestDistance || (distance == bestDistance && end < bestEnd) {
			bestEnd = end
			bestDistance = distance
		}
	}
	if bestEnd == -1 {
		return 0, "", false
	}
	return bestEnd, PunctuationStrongTerminal, true
}

func chooseLLMBoundary(ctx context.Context, segments []srt.Segment, start, minEnd, maxEnd, nominalEnd int, planner BoundaryPlanner) (int, string, string, string, string) {
	if planner == nil {
		return nominalEnd, PlannerFallbackNominal, PunctuationNonterminal, "low", "no local LLM boundary planner configured"
	}
	allowedIDs := make([]int, 0, maxEnd-minEnd+1)
	allowedIndexByID := make(map[int]int, maxEnd-minEnd+1)
	for end := minEnd; end <= maxEnd; end++ {
		id := segments[end-1].ID
		allowedIDs = append(allowedIDs, id)
		allowedIndexByID[id] = end
	}
	started := time.Now()
	decision, err := planner.PlanBoundary(ctx, BoundaryRequest{
		Segments:             segments[start : maxEnd+1],
		AllowedSplitAfterIDs: allowedIDs,
	})
	if err != nil {
		logger.Warn("Chunk boundary planner failed",
			"event", "chunk_boundary_planner_failed",
			"error", err,
			"nominal_end_id", segments[nominalEnd-1].ID,
		)
		return nominalEnd, PlannerFallbackNominal, PunctuationNonterminal, "low", "local LLM planner failed"
	}
	end, ok := allowedIndexByID[decision.SplitAfterID]
	if !ok {
		logger.Warn("Chunk boundary planner returned out-of-range id",
			"event", "chunk_boundary_planner_invalid_id",
			"split_after_id", decision.SplitAfterID,
			"nominal_end_id", segments[nominalEnd-1].ID,
		)
		return nominalEnd, PlannerFallbackNominal, PunctuationNonterminal, "low", "local LLM planner returned invalid id"
	}
	class := ClassifyPunctuation(segments[end-1])
	reason := strings.TrimSpace(decision.Reason)
	if reason == "" {
		reason = "local LLM planner selected boundary"
	}
	confidence := strings.TrimSpace(decision.Confidence)
	if confidence == "" {
		confidence = "unknown"
	}
	wallSeconds := decision.WallSeconds
	if wallSeconds <= 0 {
		wallSeconds = time.Since(started).Seconds()
	}
	outputTokS := 0.0
	totalTokS := 0.0
	if wallSeconds > 0 {
		outputTokS = float64(decision.CompletionTokens) / wallSeconds
		totalTokS = float64(decision.TotalTokens) / wallSeconds
	}
	logger.Info("Chunk boundary planner completed",
		"event", "chunk_boundary_planner_completed",
		"split_after_id", decision.SplitAfterID,
		"duration_ms", int64(wallSeconds*1000),
		"prompt_tokens", decision.PromptTokens,
		"completion_tokens", decision.CompletionTokens,
		"total_tokens", decision.TotalTokens,
		"output_tok_s", outputTokS,
		"total_tok_s", totalTokS,
	)
	return end, PlannerLocalLLM, class, confidence, reason
}

func plannedChunkFromRange(index, startIndex, endIndex, nominalEndIndex int, target []srt.Segment, planner, punctuationClass string) PlannedChunk {
	pc := PlannedChunk{
		Index:            index,
		StartIndex:       startIndex,
		EndIndex:         endIndex,
		Planner:          planner,
		PunctuationClass: punctuationClass,
	}
	if len(target) > 0 {
		pc.StartID = target[0].ID
		pc.EndID = target[len(target)-1].ID
		pc.NominalEndID = pc.EndID
	}
	return pc
}

func plannedChunkFromAbsoluteRange(index, startIndex, endIndex, nominalEndIndex int, segments []srt.Segment, planner, punctuationClass string) PlannedChunk {
	target := segments[startIndex:endIndex]
	pc := plannedChunkFromRange(index, startIndex, endIndex, nominalEndIndex, target, planner, punctuationClass)
	if nominalEndIndex > startIndex && nominalEndIndex <= len(segments) {
		pc.NominalEndID = segments[nominalEndIndex-1].ID
		pc.DistanceFromNominal = endIndex - nominalEndIndex
	}
	return pc
}

func logBoundary(pc PlannedChunk) {
	logger.Info("Chunk boundary planned",
		"event", "chunk_boundary_planned",
		"index", pc.Index,
		"start_id", pc.StartID,
		"end_id", pc.EndID,
		"nominal_end_id", pc.NominalEndID,
		"planner", pc.Planner,
		"confidence", pc.Confidence,
		"reason", pc.SplitReason,
		"punctuation_class", pc.PunctuationClass,
		"distance_from_nominal", pc.DistanceFromNominal,
		"allowed_min_end_id", pc.AllowedMinEndID,
		"allowed_max_end_id", pc.AllowedMaxEndID,
	)
}

// ClassifyPunctuation classifies the final punctuation of one subtitle segment.
func ClassifyPunctuation(segment srt.Segment) string {
	text := strings.TrimSpace(strings.Join(segment.Lines, " "))
	if text == "" {
		return PunctuationNonterminal
	}
	return classifyTextPunctuation(text)
}

func classifyTextPunctuation(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return PunctuationNonterminal
	}
	if strings.HasSuffix(text, "...") || strings.HasSuffix(text, "…") {
		return PunctuationWeakTerminal
	}
	text = trimClosingMarks(text)
	if strings.HasSuffix(text, "...") || strings.HasSuffix(text, "…") {
		return PunctuationWeakTerminal
	}
	r, _ := utf8.DecodeLastRuneInString(text)
	switch r {
	case '.', '?', '!', '。', '？', '！':
		return PunctuationStrongTerminal
	case '-', '—', '–':
		return PunctuationWeakTerminal
	default:
		return PunctuationNonterminal
	}
}

func trimClosingMarks(text string) string {
	for {
		text = strings.TrimSpace(text)
		if text == "" {
			return text
		}
		r, size := utf8.DecodeLastRuneInString(text)
		if !isClosingMark(r) {
			return text
		}
		text = text[:len(text)-size]
	}
}

func isClosingMark(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '}', '」', '』', '”', '’':
		return true
	default:
		return false
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
