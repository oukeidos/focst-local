package translator

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"

	"time"

	"github.com/oukeidos/focst-local/internal/apperrors"
	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

// normalizeText keeps the local-provider output as one subtitle text block per
// source segment while tolerating accidental newlines in model output.
func normalizeText(text string) []string {
	normalized := strings.ReplaceAll(text, "\\n", " ")
	normalized = strings.ReplaceAll(normalized, "\n", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized == "" {
		return nil
	}
	return []string{normalized}
}

// GetSystemPrompt generates a language-specific system prompt.
func GetSystemPrompt(sourceName, targetName string) string {
	return fmt.Sprintf(`You are a professional %s to %s translator specializing in subtitles.
Translate the provided %s subtitle segments into %s.

1. Input Structure:
- The input is provided in JSON format with 'context_before', 'target', and 'context_after'.
- 'target': Contains the segments you must translate.
- Each segment has:
  - 'id': The segment ID.
  - 'source_text': The one-line source text for that segment.
- 'context_before' and 'context_after': Provided for context only. Do NOT translate them or include them in the output.

2. Output Structure:
- The output MUST be a JSON object with a 'translations' field, containing an array of objects.
- Each object in the array must have:
  - 'id': The ID from the same input target segment.
  - 'source_text': The exact source_text from the same input target segment.
  - 'text': The translated subtitle segment.
- Return exactly one output object for each target segment.
- Do not add any other fields.
- Respond ONLY with the JSON object.

3. Rules:
- Maintain the original tone and context.
- For each object, first copy the exact target source text into 'source_text', then translate only that same 'source_text' value into 'text'.
- The output 'source_text' field must exactly match the input target segment's 'source_text', with no added or removed text.
- The 'text' field must translate the full meaning of only the 'source_text' field in the same object.
- Do not omit meaningful source_text content.
- Do not move words, clauses, speaker turns, sentence endings, or meaning between IDs.
- Use context only to resolve meaning and pronouns.
- Adjacent target segments may form one sentence; keep them readable while preserving one output object per target id.
- Do not translate, summarize, continue, or copy any content that appears only in 'context_before' or 'context_after'.
- Never let context-only names, numbers, places, events, or clauses enter the output text for target ids.
- Follow **Standard Cinematic Subtitle Punctuation** for %s.
- Write ONLY the %s translation; do not include the %s source text.
- Do NOT use "/" as a line-break substitute in subtitle text.`,
		sourceName, targetName, sourceName, targetName, targetName, targetName, sourceName)
}

// Translator orchestrates the translation process.
type Translator struct {
	client       translation.Translator
	chunkSize    int
	contextSize  int
	concurrency  int
	planOptions  chunker.PlanOptions
	planner      chunker.BoundaryPlanner
	savedPlan    *chunker.ChunkPlan
	lastPlan     chunker.ChunkPlan
	usage        translation.UsageMetadata
	usageMu      sync.Mutex
	namesMapping map[string]string
	srcLang      language.Language
	tgtLang      language.Language
}

// NewTranslator creates a new Translator instance.
func NewTranslator(client translation.Translator, chunkSize, contextSize, concurrency int, srcLang, tgtLang language.Language) (*Translator, error) {
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunkSize must be greater than 0, got %d", chunkSize)
	}
	if concurrency <= 0 {
		return nil, fmt.Errorf("concurrency must be greater than 0, got %d", concurrency)
	}
	return &Translator{
		client:      client,
		chunkSize:   chunkSize,
		contextSize: contextSize,
		concurrency: concurrency,
		planOptions: chunker.PlanOptions{
			NominalSize:         chunkSize,
			MinSize:             chunkSize,
			MaxSize:             chunkSize,
			ContextSize:         contextSize,
			EnableSentenceAware: false,
		},
		srcLang: srcLang,
		tgtLang: tgtLang,
	}, nil
}

// SetChunkPlanning configures sentence-aware chunk planning.
func (t *Translator) SetChunkPlanning(options chunker.PlanOptions, planner chunker.BoundaryPlanner) {
	if options.NominalSize <= 0 {
		options.NominalSize = t.chunkSize
	}
	if options.ContextSize < 0 {
		options.ContextSize = t.contextSize
	}
	t.planOptions = options
	t.planner = planner
}

// SetChunkPlan reuses a previously saved chunk plan, typically during repair.
func (t *Translator) SetChunkPlan(plan chunker.ChunkPlan) {
	copied := chunker.ChunkPlan{Chunks: append([]chunker.PlannedChunk(nil), plan.Chunks...)}
	t.savedPlan = &copied
}

// ChunkPlan returns the chunk plan used by the most recent translation run.
func (t *Translator) ChunkPlan() chunker.ChunkPlan {
	return chunker.ChunkPlan{Chunks: append([]chunker.PlannedChunk(nil), t.lastPlan.Chunks...)}
}

// SetNamesMapping sets the character name dictionary.
func (t *Translator) SetNamesMapping(mapping map[string]string) {
	t.namesMapping = mapping
}

// TranslationState represents the current state of a chunk translation.
type TranslationState int

const (
	StateStarted TranslationState = iota
	StateInProgress
	StateCompleted
	StateCanceled
)

var defaultQPS = 3
var defaultRampUp = 2 * time.Second

// TranslationProgress represents the current state of the translation process.
type TranslationProgress struct {
	ChunkIndex  int
	TotalChunks int
	Attempt     int
	State       TranslationState
	Error       error
	Duration    time.Duration
	Usage       translation.UsageMetadata
}

func (t *Translator) setSystemInstruction() {
	prompt := GetSystemPrompt(t.srcLang.Name, t.tgtLang.Name)

	// Inject Names Mapping if present
	if len(t.namesMapping) > 0 {
		mappingStr := "\n\nCRITICAL: The following character names MUST be translated as specified:\n"
		keys := make([]string, 0, len(t.namesMapping))
		for src := range t.namesMapping {
			keys = append(keys, src)
		}
		sort.Strings(keys)
		for _, src := range keys {
			tgt := t.namesMapping[src]
			mappingStr += fmt.Sprintf("- %s -> %s\n", src, tgt)
		}
		prompt += mappingStr
	}

	if sc, ok := t.client.(interface{ SetSystemInstruction(string) }); ok {
		sc.SetSystemInstruction(prompt)
	}
}

func (t *Translator) translateEngine(ctx context.Context, segments []srt.Segment, chunkIndices []int, onProgress func(TranslationProgress)) ([]chunker.Chunk, [][]srt.Segment, []bool, error) {
	t.setSystemInstruction()

	chunks, plan, err := t.planChunks(ctx, segments)
	if err != nil {
		return nil, nil, nil, err
	}
	t.lastPlan = plan
	translatedChunks := make([][]srt.Segment, len(chunks))
	failedMarks := make([]bool, len(chunks))
	processed := make([]bool, len(chunks))

	toTranslate := make(map[int]bool, len(chunks))
	if chunkIndices == nil {
		for i := range chunks {
			toTranslate[i] = true
		}
	} else {
		for _, idx := range chunkIndices {
			if idx >= 0 && idx < len(chunks) {
				toTranslate[idx] = true
			}
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	rateCh, stopRate := newRateLimiter(defaultQPS)
	defer stopRate()

	jobs := make(chan int, len(chunks))
	for i := range chunks {
		if toTranslate[i] {
			jobs <- i
		}
	}
	close(jobs)

	for w := 0; w < t.concurrency; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			if delay := rampDelay(worker, t.concurrency, defaultRampUp); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			for i := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if rateCh != nil {
					select {
					case <-ctx.Done():
						return
					case <-rateCh:
					}
				}
				chunk := chunks[i]

				var resp *translation.ResponseData
				var err error
				const maxAttempts = 3
				attemptsUsed := 0

				for attempt := 1; attempt <= maxAttempts; attempt++ {
					attemptsUsed = attempt
					if onProgress != nil {
						state := StateStarted
						if attempt > 1 {
							state = StateInProgress
						}
						onProgress(TranslationProgress{
							ChunkIndex:  i,
							TotalChunks: len(chunks),
							Attempt:     attempt,
							State:       state,
							Error:       err,
						})
					}

					req := t.prepareRequest(chunk)
					attemptStarted := time.Now()
					resp, err = t.client.Translate(ctx, req)
					if err == nil {
						t.usageMu.Lock()
						t.usage.PromptTokenCount += resp.Usage.PromptTokenCount
						t.usage.CandidatesTokenCount += resp.Usage.CandidatesTokenCount
						t.usage.TotalTokenCount += resp.Usage.TotalTokenCount
						t.usageMu.Unlock()

						if err == nil {
							var translated []srt.Segment
							translated, err = t.mergeResults(chunk.Target, resp)
							if err != nil {
								err = apperrors.Validation(err)
							}
							if err == nil {
								mu.Lock()
								translatedChunks[i] = translated
								processed[i] = true
								mu.Unlock()
							}
						}
					}

					if err == nil {
						duration := time.Since(attemptStarted)
						seconds := duration.Seconds()
						outputTokS := 0.0
						totalTokS := 0.0
						if seconds > 0 {
							outputTokS = float64(resp.Usage.CandidatesTokenCount) / seconds
							totalTokS = float64(resp.Usage.TotalTokenCount) / seconds
						}
						logger.Info("Chunk completed",
							"event", "translation_chunk_completed",
							"index", i,
							"total", len(chunks),
							"attempt", attempt,
							"duration_ms", duration.Milliseconds(),
							"prompt_tokens", resp.Usage.PromptTokenCount,
							"completion_tokens", resp.Usage.CandidatesTokenCount,
							"total_tokens", resp.Usage.TotalTokenCount,
							"output_tok_s", outputTokS,
							"total_tok_s", totalTokS,
						)
						if onProgress != nil {
							onProgress(TranslationProgress{
								ChunkIndex:  i,
								TotalChunks: len(chunks),
								Attempt:     attempt,
								State:       StateCompleted,
								Duration:    duration,
								Usage:       resp.Usage,
							})
						}
						break
					}

					retry, backoff := retryDecision(ctx, err, attempt, maxAttempts)
					if !retry {
						break
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(backoff):
					}
				}

				if err != nil {
					mu.Lock()
					failedMarks[i] = true
					mu.Unlock()
					if attemptsUsed >= maxAttempts && apperrors.IsRetryable(err) {
						logger.Error("Chunk failed after maximum retries", "index", i, "attempts", attemptsUsed, "error", err)
					} else {
						logger.Error("Chunk failed without retry", "index", i, "attempts", attemptsUsed, "error", err)
					}
				}
			}
		}(w)
	}

	wg.Wait()
	if ctx.Err() != nil && onProgress != nil {
		onProgress(TranslationProgress{
			ChunkIndex:  -1,
			TotalChunks: len(chunks),
			State:       StateCanceled,
			Error:       ctx.Err(),
		})
	}
	for idx := range toTranslate {
		if idx >= 0 && idx < len(processed) && !processed[idx] {
			failedMarks[idx] = true
		}
	}

	return chunks, translatedChunks, failedMarks, nil
}

// TranslateSRT translates a slice of SRT segments with retries and concurrency.
func (t *Translator) TranslateSRT(ctx context.Context, segments []srt.Segment, onProgress func(TranslationProgress)) ([]srt.Segment, []int, error) {
	chunks, translatedChunks, failedMarks, err := t.translateEngine(ctx, segments, nil, onProgress)
	if err != nil {
		return nil, nil, err
	}
	for i, tc := range translatedChunks {
		if tc == nil {
			failedMarks[i] = true
			translatedChunks[i] = chunks[i].Target
		}
	}
	var failedChunkIndices []int
	for i, failed := range failedMarks {
		if failed {
			failedChunkIndices = append(failedChunkIndices, i)
		}
	}

	var allTranslated []srt.Segment
	for _, tc := range translatedChunks {
		allTranslated = append(allTranslated, tc...)
	}

	return allTranslated, failedChunkIndices, nil
}

// TranslateChunks translates a list of specific chunks concurrently.
func (t *Translator) TranslateChunks(ctx context.Context, segments []srt.Segment, chunkIndices []int, onProgress func(TranslationProgress)) ([]srt.Segment, []int, error) {
	chunks, translatedChunks, failedMarks, err := t.translateEngine(ctx, segments, chunkIndices, onProgress)
	if err != nil {
		return nil, nil, err
	}

	translatedSegments := make([]srt.Segment, len(segments))
	copy(translatedSegments, segments)
	for i, translated := range translatedChunks {
		if translated == nil {
			continue
		}
		startIdx := chunks[i].StartIndex
		for j, seg := range translated {
			if startIdx+j < len(translatedSegments) {
				translatedSegments[startIdx+j] = seg
			}
		}
	}
	var failedChunkIndices []int
	for i, failed := range failedMarks {
		if failed {
			failedChunkIndices = append(failedChunkIndices, i)
		}
	}
	return translatedSegments, failedChunkIndices, nil
}

func (t *Translator) planChunks(ctx context.Context, segments []srt.Segment) ([]chunker.Chunk, chunker.ChunkPlan, error) {
	if t.savedPlan != nil {
		chunks, err := chunker.ChunksFromPlan(segments, t.contextSize, *t.savedPlan)
		if err != nil {
			return nil, chunker.ChunkPlan{}, err
		}
		return chunks, *t.savedPlan, nil
	}
	options := t.planOptions
	if options.NominalSize <= 0 {
		options.NominalSize = t.chunkSize
	}
	options.ContextSize = t.contextSize
	return chunker.PlanChunks(ctx, segments, options, t.planner)
}

func (t *Translator) prepareRequest(chunk chunker.Chunk) translation.RequestData {
	return translation.RequestData{
		ContextBefore: toSegmentData(chunk.Context.Before),
		Target:        toSegmentData(chunk.Target),
		ContextAfter:  toSegmentData(chunk.Context.After),
	}
}

func toSegmentData(segments []srt.Segment) []translation.SegmentData {
	data := make([]translation.SegmentData, len(segments))
	for i, s := range segments {
		data[i] = translation.SegmentData{
			ID:         s.ID,
			SourceText: translation.SourceTextFromLines(s.Lines),
		}
	}
	return data
}

func (t *Translator) mergeResults(original []srt.Segment, resp *translation.ResponseData) ([]srt.Segment, error) {
	expectedIDs := make(map[int]bool)
	for _, s := range original {
		expectedIDs[s.ID] = true
	}

	transMap := make(map[int]translation.TranslatedSegment)
	for _, tr := range resp.Translations {
		// Check for duplicate IDs in model output
		if _, exists := transMap[tr.ID]; exists {
			return nil, fmt.Errorf("duplicate translation ID detected in model output: %d", tr.ID)
		}

		// Check for unexpected (hallucinated) IDs
		if !expectedIDs[tr.ID] {
			return nil, fmt.Errorf("unexpected translation ID (hallucination) from model: %d", tr.ID)
		}

		transMap[tr.ID] = tr
	}

	// Check if all requested IDs were returned
	if len(transMap) != len(original) {
		return nil, fmt.Errorf("translation count mismatch: expected %d, got %d", len(original), len(transMap))
	}

	results := make([]srt.Segment, len(original))
	for i, orig := range original {
		tr, ok := transMap[orig.ID]
		if !ok {
			return nil, fmt.Errorf("missing translation for segment ID %d", orig.ID)
		}
		if tr.SourceText != translation.SourceTextFromLines(orig.Lines) {
			return nil, fmt.Errorf("source echo mismatch for segment ID %d", orig.ID)
		}

		// Validation: Ensure translation is not empty if original was not empty
		if strings.TrimSpace(tr.Text) == "" && len(orig.Lines) > 0 {
			return nil, fmt.Errorf("hallucination detected: empty translation for segment ID %d", orig.ID)
		}

		newLines := normalizeText(tr.Text)

		results[i] = srt.Segment{
			ID:        orig.ID,
			StartTime: orig.StartTime,
			EndTime:   orig.EndTime,
			Lines:     newLines,
		}
	}

	return results, nil
}

func retryDecision(ctx context.Context, err error, attempt, maxAttempts int) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}
	if attempt >= maxAttempts {
		return false, 0
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, 0
	}
	if !apperrors.IsRetryable(err) {
		return false, 0
	}
	base := 1 * time.Second
	maxBackoff := 20 * time.Second
	jitterMax := 1 * time.Second

	backoff := base << (attempt - 1)
	if apperrors.IsRateLimit(err) {
		backoff = backoff * 2
	}
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	jitter := time.Duration(rand.Int63n(int64(jitterMax)))
	return true, backoff + jitter
}

func newRateLimiter(qps int) (<-chan time.Time, func()) {
	if qps <= 0 {
		return nil, func() {}
	}
	interval := time.Second / time.Duration(qps)
	ticker := time.NewTicker(interval)
	return ticker.C, ticker.Stop
}

func rampDelay(worker, concurrency int, ramp time.Duration) time.Duration {
	if ramp <= 0 || concurrency <= 1 {
		return 0
	}
	return time.Duration(int64(ramp) * int64(worker) / int64(concurrency-1))
}

// GetUsage returns the total token usage.
func (t *Translator) GetUsage() translation.UsageMetadata {
	t.usageMu.Lock()
	defer t.usageMu.Unlock()
	return t.usage
}
