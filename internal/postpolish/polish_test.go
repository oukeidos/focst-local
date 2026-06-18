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
		Profile:             ProfileLegacy,
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

func TestRunDefaultsToSegmentLocalAndForcesNoChangeRows(t *testing.T) {
	client := &fakeV2JSONClient{}
	srcLang, _ := language.GetLanguage("ja")
	tgtLang, _ := language.GetLanguage("ko")
	source := []srt.Segment{
		{ID: 1, Lines: []string{"Synthetic Alpha"}},
		{ID: 2, Lines: []string{"Synthetic Beta"}},
	}
	translated := []srt.Segment{
		{ID: 1, Lines: []string{"그대로 둘 문장"}},
		{ID: 2, Lines: []string{"고칠 문장"}},
	}
	result, err := Run(context.Background(), client, source, translated, Config{
		SourceLanguage: srcLang,
		TargetLanguage: tgtLang,
		Model:          "test-model",
		BaseURL:        "http://127.0.0.1:8080/v1",
		ChunkSize:      2,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Artifact.Profile != string(ProfileSegmentLocal) || result.Artifact.PromptVersion != PromptVersionSegmentLocal {
		t.Fatalf("unexpected artifact profile/version: %+v", result.Artifact)
	}
	if result.Artifact.InstructionPromptVersion != PromptVersionSegmentLocal || result.Artifact.ApplicationPromptVersion != PromptVersionSegmentLocal {
		t.Fatalf("unexpected artifact stage prompt versions: %+v", result.Artifact)
	}
	if result.Artifact.Model != "test-model" || result.Artifact.BaseURL != "http://127.0.0.1:8080/v1" {
		t.Fatalf("unexpected artifact model/base URL: %+v", result.Artifact)
	}
	if len(result.Accepted) != 1 {
		t.Fatalf("accepted = %+v, want one correction", result.Accepted)
	}
	if got := result.Accepted[0]; got.ID != 2 || got.After != "자연스럽게 고친 문장" || got.Pass != string(ProfileSegmentLocal) {
		t.Fatalf("unexpected correction: %+v", got)
	}
	if len(result.Artifact.AppliedRows) != 2 || !result.Artifact.AppliedRows[0].ForcedNoChange {
		t.Fatalf("expected forced no-change audit row, got %+v", result.Artifact.AppliedRows)
	}
	out := Apply(translated, result.Accepted)
	if out[0].Lines[0] != "그대로 둘 문장" || out[1].Lines[0] != "자연스럽게 고친 문장" {
		t.Fatalf("unexpected applied output: %+v", out)
	}
	if client.calls != 2 {
		t.Fatalf("client calls = %d, want 2", client.calls)
	}
	if result.Artifact.Stats.RequestOK != 2 || result.Artifact.Stats.RequestError != 0 || result.Artifact.Stats.ChangedSegments != 1 || result.Artifact.Stats.ForcedNoChange != 1 {
		t.Fatalf("unexpected artifact stats: %+v", result.Artifact.Stats)
	}
	if client.lastTemperature == nil || *client.lastTemperature != 0 || client.lastMaxTokens != DefaultV2MaxTokens {
		t.Fatalf("sampler not preserved: temp=%v max=%d", client.lastTemperature, client.lastMaxTokens)
	}
}

func TestV2SchemasUseExactPositionalRows(t *testing.T) {
	segments := []RequestSegment{
		{ID: 7, SourceText: "Synthetic Alpha", Text: "Target Alpha"},
		{ID: 9, SourceText: "Synthetic Beta", Text: "Target Beta"},
	}
	instructionSchema := segmentInstructionSchema(segments)
	instructionProps := instructionSchema["properties"].(map[string]any)
	instructionRows := instructionProps["segments"].(map[string]any)
	if instructionRows["minItems"] != 2 || instructionRows["maxItems"] != 2 {
		t.Fatalf("instruction schema length constraints = %+v", instructionRows)
	}
	schema := applicationSchema(segments)
	props := schema["properties"].(map[string]any)
	rows := props["segments"].(map[string]any)
	if rows["minItems"] != 2 || rows["maxItems"] != 2 {
		t.Fatalf("schema length constraints = %+v", rows)
	}
	prefix := rows["prefixItems"].([]any)
	first := prefix[0].(map[string]any)
	firstProps := first["properties"].(map[string]any)
	if firstProps["id"].(map[string]any)["const"] != 7 {
		t.Fatalf("id const missing: %+v", firstProps["id"])
	}
	if firstProps["source_text"].(map[string]any)["const"] != "Synthetic Alpha" {
		t.Fatalf("source_text const missing: %+v", firstProps["source_text"])
	}
}

func TestRunRejectsInvalidProfile(t *testing.T) {
	srcLang, _ := language.GetLanguage("ja")
	tgtLang, _ := language.GetLanguage("ko")
	_, err := Run(context.Background(), &fakeV2JSONClient{}, []srt.Segment{{ID: 1, Lines: []string{"Synthetic"}}}, []srt.Segment{{ID: 1, Lines: []string{"합성"}}}, Config{
		SourceLanguage: srcLang,
		TargetLanguage: tgtLang,
		Profile:        Profile("auto"),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid post-polish profile: auto") {
		t.Fatalf("expected invalid profile error, got %v", err)
	}
}

func TestValidateApplicationRowsRejectsUnsafeV2Rows(t *testing.T) {
	chunk := v2Chunk{
		Index: 0,
		Segments: []RequestSegment{
			{ID: 1, SourceText: "Synthetic Alpha", Text: "보호어 유지"},
			{ID: 2, SourceText: "Synthetic Beta", Text: "기존 문장"},
			{ID: 3, SourceText: "Synthetic Gamma", Text: "메타 문장"},
		},
	}
	rows, err := validateApplicationRows(chunk, []applicationRow{
		{ID: 1, SourceText: "Synthetic Alpha", Text: "제거된 문장"},
		{ID: 2, SourceText: "Synthetic Beta", Text: ""},
		{ID: 3, SourceText: "Synthetic Gamma", Text: "이 문장은 2번과 연결됨"},
	}, Config{
		ProtectedRenderings: map[string]string{"Synthetic Protected": "보호어"},
	}, ProfileSegmentLocal, InstructionRecord{})
	if err != nil {
		t.Fatalf("validateApplicationRows failed: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want 3", rows)
	}
	wantReasons := []string{"protected_rendering_removed", "empty_text", "meta_text"}
	for i, want := range wantReasons {
		if rows[i].Rejected == nil || rows[i].Rejected.Reason != want {
			t.Fatalf("row %d rejected reason = %+v, want %s", i, rows[i].Rejected, want)
		}
	}
	_, err = validateApplicationRows(chunk, []applicationRow{
		{ID: 1, SourceText: "Different source", Text: "보호어 유지"},
		{ID: 2, SourceText: "Synthetic Beta", Text: "기존 문장"},
		{ID: 3, SourceText: "Synthetic Gamma", Text: "메타 문장"},
	}, Config{}, ProfileSegmentLocal, InstructionRecord{})
	if err == nil || !strings.Contains(err.Error(), "source_text mismatch") {
		t.Fatalf("expected source_text mismatch error, got %v", err)
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

type fakeV2JSONClient struct {
	calls           int
	lastTemperature *float64
	lastMaxTokens   int
}

func (f *fakeV2JSONClient) CompleteJSONWithOptions(_ context.Context, _, userPrompt string, _ map[string]any, opts translation.TextCompletionOptions) (*translation.TextCompletion, error) {
	f.calls++
	f.lastTemperature = opts.Temperature
	f.lastMaxTokens = opts.MaxTokens
	content := `{"segments":[{"id":1,"source_text":"Synthetic Alpha","edit_instruction":"No change needed."},{"id":2,"source_text":"Synthetic Beta","edit_instruction":"Make the current Korean subtitle sound natural."}]}`
	if strings.Contains(userPrompt, "Apply each edit_instruction") {
		content = `{"segments":[{"id":1,"source_text":"Synthetic Alpha","text":"모델이 바꾼 문장"},{"id":2,"source_text":"Synthetic Beta","text":"자연스럽게 고친 문장"}]}`
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
