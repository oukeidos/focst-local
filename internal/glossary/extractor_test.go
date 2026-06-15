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
	content string
	usage   translation.UsageMetadata
}

func (f fakeTextCompleter) CompleteText(context.Context, string, string, int) (*translation.TextCompletion, error) {
	return &translation.TextCompletion{Content: f.content, Usage: f.usage}, nil
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
	artifact, usage, err := Extract(context.Background(), fakeTextCompleter{
		content: "| Source | Korean rendering |\n| --- | --- |\n| 架空田一郎 | 가공다 이치로 |\n",
		usage:   translation.UsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 7, TotalTokenCount: 12},
	}, segments, plan, ExtractConfig{
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
	if usage.TotalTokenCount != 12 {
		t.Fatalf("usage = %+v", usage)
	}
	if len(artifact.Entries) != 1 || artifact.Entries[0].Source != "架空田一郎" {
		t.Fatalf("entries = %+v", artifact.Entries)
	}
	for _, rel := range []string{
		"glossary_config.json",
		"chunk_plan.json",
		"windows.json",
		"merged_glossary.json",
		"names_compatible.json",
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
