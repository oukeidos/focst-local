package postpolish

import (
	"context"
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

func TestPromptGoldenUsesDynamicTargetLanguage(t *testing.T) {
	req := Request{Segments: []RequestSegment{{
		ID:         1,
		SourceText: "Synthetic source",
		Text:       "Synthetic target",
	}}}
	got, err := broadUserPrompt("Japanese", "Korean", req)
	if err != nil {
		t.Fatalf("broadUserPrompt failed: %v", err)
	}
	for _, want := range []string{
		"Review the provided Japanese to Korean subtitle translations after translation.",
		"Only repair broken Korean phrasing.",
		`Return JSON exactly in this shape: {"corrections":[{"id":1,"source_text":"...","text":"..."}]}.`,
		`"text" must be the replacement Korean subtitle text.`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	if systemPrompt("Korean") != "You are a conservative Korean subtitle copyeditor. Return only JSON." {
		t.Fatalf("unexpected system prompt: %q", systemPrompt("Korean"))
	}
}

func TestRunMergesRepairOverBroadAndProtectsRenderings(t *testing.T) {
	client := &fakeJSONClient{}
	srcLang, _ := language.GetLanguage("ja")
	tgtLang, _ := language.GetLanguage("ko")
	source := []srt.Segment{
		{ID: 1, Lines: []string{"Synthetic Alpha"}},
		{ID: 2, Lines: []string{"Synthetic Beta"}},
	}
	translated := []srt.Segment{
		{ID: 1, Lines: []string{"보호어 유지"}},
		{ID: 2, Lines: []string{"고장난 문장"}},
	}
	result, err := Run(context.Background(), client, source, translated, Config{
		SourceLanguage:      srcLang,
		TargetLanguage:      tgtLang,
		BroadChunkSize:      2,
		RepairChunkSize:     2,
		MaxTokens:           2048,
		ProtectedRenderings: map[string]string{"Synthetic Protected": "보호어"},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Accepted) != 1 {
		t.Fatalf("accepted = %+v, want one repair correction", result.Accepted)
	}
	got := result.Accepted[0]
	if got.ID != 2 || got.After != "수리된 문장" || got.Pass != PassRepair {
		t.Fatalf("unexpected merged correction: %+v", got)
	}
	if result.GuardRejected != 1 {
		t.Fatalf("guard rejected = %d, want 1", result.GuardRejected)
	}
	out := Apply(translated, result.Accepted)
	if out[0].Lines[0] != "보호어 유지" || out[1].Lines[0] != "수리된 문장" {
		t.Fatalf("unexpected applied output: %+v", out)
	}
	if client.calls != 2 {
		t.Fatalf("client calls = %d, want 2", client.calls)
	}
	if client.lastTemperature == nil || *client.lastTemperature != 0 || client.lastMaxTokens != 2048 {
		t.Fatalf("sampler not preserved: temp=%v max=%d", client.lastTemperature, client.lastMaxTokens)
	}
}

type fakeJSONClient struct {
	calls           int
	lastTemperature *float64
	lastMaxTokens   int
}

func (f *fakeJSONClient) CompleteJSONWithOptions(_ context.Context, _, userPrompt string, _ map[string]any, opts translation.TextCompletionOptions) (*translation.TextCompletion, error) {
	f.calls++
	f.lastTemperature = opts.Temperature
	f.lastMaxTokens = opts.MaxTokens
	content := `{"corrections":[{"id":1,"source_text":"Synthetic Alpha","text":"제거됨"},{"id":2,"source_text":"Synthetic Beta","text":"넓은 교정"}]}`
	if strings.Contains(userPrompt, "Only fix typo-like or garbled") {
		content = `{"corrections":[{"id":2,"source_text":"Synthetic Beta","text":"수리된 문장"}]}`
	}
	return &translation.TextCompletion{
		Content: content,
		Usage: translation.UsageMetadata{
			PromptTokenCount:     1,
			CandidatesTokenCount: 2,
			TotalTokenCount:      3,
		},
	}, nil
}
