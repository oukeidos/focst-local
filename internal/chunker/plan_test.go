package chunker

import (
	"context"
	"testing"

	"github.com/oukeidos/focst-local/internal/srt"
)

type fakeBoundaryPlanner struct {
	decision BoundaryDecision
	err      error
	calls    int
}

func (f *fakeBoundaryPlanner) PlanBoundary(ctx context.Context, request BoundaryRequest) (BoundaryDecision, error) {
	f.calls++
	return f.decision, f.err
}

func makeSegments(n int) []srt.Segment {
	segments := make([]srt.Segment, n)
	for i := range segments {
		segments[i] = srt.Segment{
			ID:        i + 1,
			StartTime: "00:00",
			EndTime:   "00:01",
			Lines:     []string{"segment"},
		}
	}
	return segments
}

func TestClassifyPunctuation(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "strong period", text: "This is done.", want: PunctuationStrongTerminal},
		{name: "strong with closing quote", text: `This is done."`, want: PunctuationStrongTerminal},
		{name: "cjk strong", text: "끝났다。", want: PunctuationStrongTerminal},
		{name: "ellipsis weak", text: "Wait...", want: PunctuationWeakTerminal},
		{name: "unicode ellipsis weak", text: "Wait…", want: PunctuationWeakTerminal},
		{name: "dash weak", text: "Wait-", want: PunctuationWeakTerminal},
		{name: "comma nonterminal", text: "Wait,", want: PunctuationNonterminal},
		{name: "plain nonterminal", text: "Wait", want: PunctuationNonterminal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPunctuation(srt.Segment{Lines: []string{tt.text}})
			if got != tt.want {
				t.Fatalf("ClassifyPunctuation(%q) = %s, want %s", tt.text, got, tt.want)
			}
		})
	}
}

func TestPlanChunks_PunctuationShortcutChoosesClosestStrongTerminal(t *testing.T) {
	segments := makeSegments(12)
	segments[3].Lines = []string{"Earlier sentence."}
	segments[5].Lines = []string{"Later sentence."}

	chunks, plan, err := PlanChunks(context.Background(), segments, PlanOptions{
		NominalSize:         5,
		MinSize:             4,
		MaxSize:             6,
		ContextSize:         1,
		EnableSentenceAware: true,
	}, nil)
	if err != nil {
		t.Fatalf("PlanChunks failed: %v", err)
	}
	if got := chunks[0].EndIndex; got != 4 {
		t.Fatalf("first chunk end index = %d, want 4", got)
	}
	if got := plan.Chunks[0].EndID; got != 4 {
		t.Fatalf("first planned end id = %d, want 4", got)
	}
	if got := plan.Chunks[0].Planner; got != PlannerPunctuation {
		t.Fatalf("planner = %s, want %s", got, PlannerPunctuation)
	}
	if got := plan.Chunks[0].PunctuationClass; got != PunctuationStrongTerminal {
		t.Fatalf("punctuation class = %s, want %s", got, PunctuationStrongTerminal)
	}
	if len(chunks[0].Context.After) != 1 || chunks[0].Context.After[0].ID != 5 {
		t.Fatalf("unexpected after context: %+v", chunks[0].Context.After)
	}
}

func TestPlanChunks_UsesLocalLLMFallbackWhenPunctuationIsWeak(t *testing.T) {
	segments := makeSegments(12)
	segments[4].Lines = []string{"This continues..."}
	planner := &fakeBoundaryPlanner{decision: BoundaryDecision{SplitAfterID: 6}}

	chunks, plan, err := PlanChunks(context.Background(), segments, PlanOptions{
		NominalSize:         5,
		MinSize:             4,
		MaxSize:             6,
		ContextSize:         0,
		EnableSentenceAware: true,
	}, planner)
	if err != nil {
		t.Fatalf("PlanChunks failed: %v", err)
	}
	if planner.calls == 0 {
		t.Fatalf("expected local planner to be called")
	}
	if got := chunks[0].EndIndex; got != 6 {
		t.Fatalf("first chunk end index = %d, want 6", got)
	}
	if got := plan.Chunks[0].Planner; got != PlannerLocalLLM {
		t.Fatalf("planner = %s, want %s", got, PlannerLocalLLM)
	}
}

func TestPlanChunks_InvalidLLMFallbackUsesNominalBoundary(t *testing.T) {
	segments := makeSegments(12)
	planner := &fakeBoundaryPlanner{decision: BoundaryDecision{SplitAfterID: 99}}

	chunks, plan, err := PlanChunks(context.Background(), segments, PlanOptions{
		NominalSize:         5,
		MinSize:             4,
		MaxSize:             6,
		ContextSize:         0,
		EnableSentenceAware: true,
	}, planner)
	if err != nil {
		t.Fatalf("PlanChunks failed: %v", err)
	}
	if got := chunks[0].EndIndex; got != 5 {
		t.Fatalf("first chunk end index = %d, want nominal 5", got)
	}
	if got := plan.Chunks[0].Planner; got != PlannerFallbackNominal {
		t.Fatalf("planner = %s, want %s", got, PlannerFallbackNominal)
	}
}

func TestChunksFromPlan_ReconstructsVariableRangesAndContext(t *testing.T) {
	segments := makeSegments(7)
	plan := ChunkPlan{Chunks: []PlannedChunk{
		{Index: 0, StartIndex: 0, EndIndex: 3, StartID: 1, EndID: 3},
		{Index: 1, StartIndex: 3, EndIndex: 7, StartID: 4, EndID: 7},
	}}

	chunks, err := ChunksFromPlan(segments, 1, plan)
	if err != nil {
		t.Fatalf("ChunksFromPlan failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
	if got := chunks[1].StartIndex; got != 3 {
		t.Fatalf("chunk 1 start index = %d, want 3", got)
	}
	if len(chunks[1].Context.Before) != 1 || chunks[1].Context.Before[0].ID != 3 {
		t.Fatalf("unexpected before context: %+v", chunks[1].Context.Before)
	}
}
