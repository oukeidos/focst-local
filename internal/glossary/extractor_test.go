package glossary

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type fakeTextCompleter struct {
	responses []translation.TextCompletion
	options   []translation.TextCompletionOptions
}

func (f *fakeTextCompleter) CompleteText(_ context.Context, _, _ string, _ int) (*translation.TextCompletion, error) {
	if len(f.responses) == 0 {
		return &translation.TextCompletion{}, nil
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return &resp, nil
}

func (f *fakeTextCompleter) CompleteTextWithOptions(_ context.Context, _, _ string, opts translation.TextCompletionOptions) (*translation.TextCompletion, error) {
	f.options = append(f.options, opts)
	if len(f.responses) == 0 {
		return &translation.TextCompletion{}, nil
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return &resp, nil
}

func TestExtractWritesDebugArtifacts(t *testing.T) {
	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	segments := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"架空田一郎です"}},
		{ID: 2, StartTime: "00:00:03,000", EndTime: "00:00:04,000", Lines: []string{"合成都市へ行く"}},
	}
	plan := chunker.ChunkPlan{Chunks: []chunker.PlannedChunk{
		{Index: 0, StartIndex: 0, EndIndex: 2, StartID: 1, EndID: 2},
	}}
	dir := t.TempDir()
	completer := &fakeTextCompleter{responses: []translation.TextCompletion{
		{
			Content: "| Source | Korean rendering |\n| --- | --- |\n| 架空田一郎 | 가공다 이치로 |\n",
			Usage:   translation.UsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 7, TotalTokenCount: 12},
		},
		{
			Content: "| Row | Expected strategy | Fit | Decision |\n| ---: | --- | --- | --- |\n| 1 | name_form | fits | keep |\n",
			Usage:   translation.UsageMetadata{PromptTokenCount: 3, CandidatesTokenCount: 4, TotalTokenCount: 7},
		},
	}}
	artifact, usage, err := Extract(context.Background(), completer, segments, plan, ExtractConfig{
		InputPath:        "synthetic.srt",
		SegmentsChecksum: "sha256:test",
		SourceLang:       src,
		TargetLang:       tgt,
		Model:            "model",
		BaseURL:          "http://127.0.0.1:8080/v1",
		Runs:             1,
		WindowChunks:     3,
		MaxTokens:        8192,
		ArtifactDir:      dir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if usage.TotalTokenCount != 19 {
		t.Fatalf("usage = %+v", usage)
	}
	if len(artifact.Entries) != 1 || artifact.Entries[0].Source != "架空田一郎" {
		t.Fatalf("entries = %+v", artifact.Entries)
	}
	if artifact.RenderingSafetyFilter == nil || !artifact.RenderingSafetyFilter.Applied {
		t.Fatalf("rendering safety filter info missing: %+v", artifact.RenderingSafetyFilter)
	}
	if len(completer.options) != 1 {
		t.Fatalf("filter option calls = %d, want 1", len(completer.options))
	}
	if completer.options[0].Temperature != DefaultRenderingSafetyTemperature {
		t.Fatalf("filter temperature = %v, want %v", completer.options[0].Temperature, DefaultRenderingSafetyTemperature)
	}
	for _, rel := range []string{
		"glossary_config.json",
		"chunk_plan.json",
		"windows.json",
		"merged_glossary.prefilter.json",
		"merged_glossary.json",
		"names_compatible.json",
		"rendering_safety_filter/batch_001_prompt.txt",
		"rendering_safety_filter/batch_001_response.md",
		"rendering_safety_filter/batch_001_judgments.json",
		"rendering_safety_filter/batch_001_usage.json",
		"rendering_safety_filter/judgments.json",
		"rendering_safety_filter/filtered_glossary.json",
		"window_000/run_001_prompt.txt",
		"window_000/run_001_response.md",
		"window_000/run_001_parsed.json",
		"window_000/run_001_rejected.json",
		"window_000/run_001_usage.json",
		"window_000/window_summary.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}
}
