package translator

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

func TestTranslator_TranslateSRT(t *testing.T) {
	// Setup mock
	mockClient := &translation.MockClient{
		Response: &translation.ResponseData{
			Translations: []translation.TranslatedSegment{
				{ID: 1, SourceText: "テスト甲", Text: "번역-갑"},
				{ID: 2, SourceText: "テスト乙 分割行", Text: "번역-을"},
			},
		},
	}

	segments := []srt.Segment{
		{ID: 1, StartTime: "00:00", EndTime: "00:01", Lines: []string{"テスト甲"}},
		{ID: 2, StartTime: "00:01", EndTime: "00:02", Lines: []string{"テスト乙", "分割行"}},
	}

	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	tr, err := NewTranslator(mockClient, 100, 5, 1, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator fail: %v", err)
	}
	results, failed, err := tr.TranslateSRT(context.Background(), segments, nil)
	if err != nil {
		t.Fatalf("TranslateSRT fail: %v", err)
	}
	if len(failed) > 0 {
		t.Errorf("TranslateSRT() failed chunks: %v", failed)
	}

	expected := []srt.Segment{
		{ID: 1, StartTime: "00:00", EndTime: "00:01", Lines: []string{"번역-갑"}},
		{ID: 2, StartTime: "00:01", EndTime: "00:02", Lines: []string{"번역-을"}},
	}

	if !reflect.DeepEqual(results, expected) {
		t.Errorf("TranslateSRT() = %+v, want %+v", results, expected)
	}
}

func TestTranslator_EmptyTranslation(t *testing.T) {
	mockClient := &translation.MockClient{
		Response: &translation.ResponseData{
			Translations: []translation.TranslatedSegment{
				{ID: 1, SourceText: "テスト空", Text: ""},
			},
		},
	}

	segments := []srt.Segment{
		{ID: 1, StartTime: "00:00", EndTime: "00:01", Lines: []string{"テスト空"}},
	}

	tr, err := NewTranslator(mockClient, 1, 0, 1, language.Language{Name: "Japanese", Code: "ja"}, language.Language{Name: "Korean", Code: "ko"})
	if err != nil {
		t.Fatalf("NewTranslator fail: %v", err)
	}
	_, failed, _ := tr.TranslateSRT(context.Background(), segments, nil)

	if len(failed) == 0 {
		t.Errorf("Expected translation to fail for empty text, but it succeeded")
	}
}
func TestTranslator_MergeResultsStrictValidation(t *testing.T) {
	tr := &Translator{}
	original := []srt.Segment{
		{ID: 1},
		{ID: 2},
	}

	tests := []struct {
		name    string
		resp    *translation.ResponseData
		wantErr string
	}{
		{
			name: "Source echo mismatch",
			resp: &translation.ResponseData{
				Translations: []translation.TranslatedSegment{
					{ID: 1, SourceText: "wrong", Text: "T1"},
					{ID: 2, SourceText: "", Text: "T2"},
				},
			},
			wantErr: "source echo mismatch",
		},
		{
			name: "Duplicate ID",
			resp: &translation.ResponseData{
				Translations: []translation.TranslatedSegment{
					{ID: 1, Text: "T1"},
					{ID: 1, Text: "T1-dup"},
				},
			},
			wantErr: "duplicate translation ID",
		},
		{
			name: "Hallucinated ID",
			resp: &translation.ResponseData{
				Translations: []translation.TranslatedSegment{
					{ID: 1, Text: "T1"},
					{ID: 99, Text: "Ghost"},
				},
			},
			wantErr: "unexpected translation ID",
		},
		{
			name: "Missing ID",
			resp: &translation.ResponseData{
				Translations: []translation.TranslatedSegment{
					{ID: 1, Text: "T1"},
				},
			},
			wantErr: "translation count mismatch",
		},
		{
			name: "Too many IDs (even if valid IDs)",
			resp: &translation.ResponseData{
				Translations: []translation.TranslatedSegment{
					{ID: 1, Text: "T1"},
					{ID: 2, Text: "T2"},
					{ID: 1, Text: "T1-again"}, // This also triggers duplicate check first
				},
			},
			wantErr: "duplicate translation ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tr.mergeResults(original, tt.resp)
			if err == nil {
				t.Errorf("mergeResults() expected error, got nil")
				return
			}
			if tt.wantErr != "" {
				errStr := err.Error()
				if !strings.Contains(errStr, tt.wantErr) {
					t.Errorf("mergeResults() error = %v, want substring %v", errStr, tt.wantErr)
				}
			}
		})
	}
}

func TestGetSystemPrompt_IncludesSlashRule(t *testing.T) {
	rule := "Do NOT use \"/\" as a line-break substitute in subtitle text."

	prompt := GetSystemPrompt("Japanese", "Korean")
	if !strings.Contains(prompt, rule) {
		t.Fatalf("expected prompt to contain slash rule")
	}
}

func TestGetSystemPrompt_UsesSimpleTextContract(t *testing.T) {
	prompt := GetSystemPrompt("English", "Korean")
	required := []string{
		"'source_text': The exact source_text from the same input target segment.",
		"'text': The translated subtitle segment.",
		"Do not add any other fields.",
		"For each object, first copy the exact target source text into 'source_text'",
		"Do not omit meaningful source_text content.",
		"Use context only to resolve meaning and pronouns.",
		"Adjacent target segments may form one sentence; keep them readable while preserving one output object per target id.",
		"Do not translate, summarize, continue, or copy any content that appears only in 'context_before' or 'context_after'.",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("expected prompt to contain %q", phrase)
		}
	}
}

func TestTranslator_TranslateChunksUsesSavedVariableChunkPlan(t *testing.T) {
	client := &perRequestMockClient{}
	src, _ := language.GetLanguage("en")
	tgt, _ := language.GetLanguage("ko")
	tr, err := NewTranslator(client, 2, 0, 1, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}
	tr.SetChunkPlan(chunker.ChunkPlan{Chunks: []chunker.PlannedChunk{
		{Index: 0, StartIndex: 0, EndIndex: 3, StartID: 1, EndID: 3},
		{Index: 1, StartIndex: 3, EndIndex: 5, StartID: 4, EndID: 5},
	}})

	segments := []srt.Segment{
		{ID: 1, Lines: []string{"one"}},
		{ID: 2, Lines: []string{"two"}},
		{ID: 3, Lines: []string{"three"}},
		{ID: 4, Lines: []string{"four"}},
		{ID: 5, Lines: []string{"five"}},
	}

	got, failed, err := tr.TranslateChunks(context.Background(), segments, []int{1}, nil)
	if err != nil {
		t.Fatalf("TranslateChunks failed: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("failed chunks = %v, want none", failed)
	}
	if got[0].Lines[0] != "one" || got[1].Lines[0] != "two" || got[2].Lines[0] != "three" {
		t.Fatalf("unchanged prefix was modified: %+v", got[:3])
	}
	if got[3].Lines[0] != "T4" || got[4].Lines[0] != "T5" {
		t.Fatalf("saved plan chunk was not merged at indices 3..5: %+v", got)
	}
}

type perRequestMockClient struct {
	translation.Translator
}

func (p *perRequestMockClient) SetSystemInstruction(prompt string) {}

func (p *perRequestMockClient) Translate(ctx context.Context, request translation.RequestData) (*translation.ResponseData, error) {
	resp := &translation.ResponseData{
		Translations: make([]translation.TranslatedSegment, len(request.Target)),
	}
	for i, segment := range request.Target {
		resp.Translations[i] = translation.TranslatedSegment{
			ID:         segment.ID,
			SourceText: segment.SourceText,
			Text:       "T" + strconv.Itoa(segment.ID),
		}
	}
	return resp, nil
}
