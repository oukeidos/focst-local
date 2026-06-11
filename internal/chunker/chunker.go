package chunker

import "github.com/oukeidos/focst-local/internal/srt"

// Chunk represents a chunk of segments to be translated along with surrounding context.
type Chunk struct {
	Index      int
	StartIndex int
	EndIndex   int
	Target     []srt.Segment
	Context    BeforeAfterContext
}

// BeforeAfterContext holds context segments.
type BeforeAfterContext struct {
	Before []srt.Segment
	After  []srt.Segment
}

// SplitIntoChunks splits a slice of segments into chunks of a given size,
// providing specified amount of context before and after each chunk.
func SplitIntoChunks(segments []srt.Segment, chunkSize, contextSize int) []Chunk {
	var chunks []Chunk
	n := len(segments)

	for i := 0; i < n; i += chunkSize {
		end := i + chunkSize
		if end > n {
			end = n
		}

		target := segments[i:end]

		// Context Before
		beforeStart := i - contextSize
		if beforeStart < 0 {
			beforeStart = 0
		}
		before := segments[beforeStart:i]

		// Context After
		afterEnd := end + contextSize
		if afterEnd > n {
			afterEnd = n
		}
		after := segments[end:afterEnd]

		chunks = append(chunks, Chunk{
			Index:      len(chunks),
			StartIndex: i,
			EndIndex:   end,
			Target:     target,
			Context: BeforeAfterContext{
				Before: before,
				After:  after,
			},
		})
	}

	return chunks
}
