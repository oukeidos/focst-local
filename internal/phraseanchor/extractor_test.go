package phraseanchor

import (
	"testing"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/srt"
)

func TestBuildWindowsSplitsEachTranslationChunkIntoHalves(t *testing.T) {
	segments := []srt.Segment{
		{ID: 1, Lines: []string{"one"}},
		{ID: 2, Lines: []string{"two"}},
		{ID: 3, Lines: []string{"three"}},
		{ID: 4, Lines: []string{"four"}},
	}
	plan := chunker.ChunkPlan{Chunks: []chunker.PlannedChunk{
		{Index: 0, StartIndex: 0, EndIndex: 4, StartID: 1, EndID: 4},
	}}
	windows, err := BuildWindows(segments, plan, 1)
	if err != nil {
		t.Fatalf("BuildWindows failed: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows len = %d, want 2", len(windows))
	}
	if windows[0].TranslationChunkIndex != 0 || windows[1].TranslationChunkIndex != 0 {
		t.Fatalf("translation chunk indexes were not preserved: %+v", windows)
	}
	if windows[0].StartID != 1 || windows[0].EndID != 2 || windows[1].StartID != 3 || windows[1].EndID != 4 {
		t.Fatalf("unexpected window ID ranges: %+v", windows)
	}
	if len(windows[0].ContextAfter) != 1 || windows[0].ContextAfter[0].ID != 3 {
		t.Fatalf("first window context_after = %+v, want ID 3", windows[0].ContextAfter)
	}
	if len(windows[1].ContextBefore) != 1 || windows[1].ContextBefore[0].ID != 2 {
		t.Fatalf("second window context_before = %+v, want ID 2", windows[1].ContextBefore)
	}
}
