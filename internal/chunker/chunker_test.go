package chunker

import (
	"testing"

	"github.com/oukeidos/focst-local/internal/srt"
)

func TestSplitIntoChunks(t *testing.T) {
	segments := make([]srt.Segment, 250)
	for i := 0; i < 250; i++ {
		segments[i] = srt.Segment{ID: i + 1}
	}

	chunkSize := 100
	contextSize := 5

	chunks := SplitIntoChunks(segments, chunkSize, contextSize)

	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks, got %d", len(chunks))
	}

	// First chunk
	if len(chunks[0].Target) != 100 {
		t.Errorf("Chunk 0: expected 100 target segments, got %d", len(chunks[0].Target))
	}
	if len(chunks[0].Context.Before) != 0 {
		t.Errorf("Chunk 0: expected 0 before context segments, got %d", len(chunks[0].Context.Before))
	}
	if len(chunks[0].Context.After) != 5 {
		t.Errorf("Chunk 0: expected 5 after context segments, got %d", len(chunks[0].Context.After))
	}
	if chunks[0].Context.After[0].ID != 101 {
		t.Errorf("Chunk 0: first after context ID expected 101, got %d", chunks[0].Context.After[0].ID)
	}

	// Second chunk
	if len(chunks[1].Target) != 100 {
		t.Errorf("Chunk 1: expected 100 target segments, got %d", len(chunks[1].Target))
	}
	if len(chunks[1].Context.Before) != 5 {
		t.Errorf("Chunk 1: expected 5 before context segments, got %d", len(chunks[1].Context.Before))
	}
	if chunks[1].Context.Before[0].ID != 96 {
		t.Errorf("Chunk 1: first before context ID expected 96, got %d", chunks[1].Context.Before[0].ID)
	}
	if len(chunks[1].Context.After) != 5 {
		t.Errorf("Chunk 1: expected 5 after context segments, got %d", len(chunks[1].Context.After))
	}

	// Last chunk
	if len(chunks[2].Target) != 50 {
		t.Errorf("Chunk 2: expected 50 target segments, got %d", len(chunks[2].Target))
	}
	if len(chunks[2].Context.Before) != 5 {
		t.Errorf("Chunk 2: expected 5 before context segments, got %d", len(chunks[2].Context.Before))
	}
	if len(chunks[2].Context.After) != 0 {
		t.Errorf("Chunk 2: expected 0 after context segments, got %d", len(chunks[2].Context.After))
	}
}
