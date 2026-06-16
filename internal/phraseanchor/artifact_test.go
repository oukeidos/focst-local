package phraseanchor

import (
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/srt"
)

func TestValidateArtifactForSegments(t *testing.T) {
	segments := []srt.Segment{
		{ID: 1, Lines: []string{"alpha"}},
		{ID: 2, Lines: []string{"beta gamma"}},
	}
	checksum := srt.SegmentsChecksumHex(segments)
	artifact := Artifact{
		Version:       1,
		PromptVersion: PromptVersion,
		SourceLang:    "ja",
		TargetLang:    "ko",
		Input: InputInfo{
			SegmentsChecksum: checksum,
		},
		ChunkPlan: chunker.ChunkPlan{Chunks: []chunker.PlannedChunk{
			{Index: 0, StartIndex: 0, EndIndex: 1, StartID: 1, EndID: 1},
			{Index: 1, StartIndex: 1, EndIndex: 2, StartID: 2, EndID: 2},
		}},
		Entries: []Entry{
			{SegmentID: 2, SourceText: "beta gamma", SourceQuote: "gamma", Rendering: "감마"},
		},
	}
	if err := ValidateArtifactForSegments(artifact, segments, "ja", "ko", checksum); err != nil {
		t.Fatalf("ValidateArtifactForSegments failed: %v", err)
	}

	artifact.Entries[0].SourceQuote = "delta"
	err := ValidateArtifactForSegments(artifact, segments, "ja", "ko", checksum)
	if err == nil || !strings.Contains(err.Error(), "source quote") {
		t.Fatalf("expected source quote validation error, got %v", err)
	}
}

func TestValidateArtifactForSegmentsRejectsChunkPlanMismatch(t *testing.T) {
	segments := []srt.Segment{
		{ID: 1, Lines: []string{"alpha"}},
		{ID: 2, Lines: []string{"beta"}},
	}
	checksum := srt.SegmentsChecksumHex(segments)
	artifact := Artifact{
		Version:       1,
		PromptVersion: PromptVersion,
		SourceLang:    "ja",
		TargetLang:    "ko",
		Input:         InputInfo{SegmentsChecksum: checksum},
		ChunkPlan: chunker.ChunkPlan{Chunks: []chunker.PlannedChunk{
			{Index: 0, StartIndex: 1, EndIndex: 2, StartID: 2, EndID: 2},
		}},
	}
	err := ValidateArtifactForSegments(artifact, segments, "ja", "ko", checksum)
	if err == nil || !strings.Contains(err.Error(), "not continuous") {
		t.Fatalf("expected chunk continuity validation error, got %v", err)
	}
}
