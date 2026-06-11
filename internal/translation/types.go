package translation

import "context"

// SegmentData is one subtitle segment in the JSON request.
type SegmentData struct {
	ID    int      `json:"id"`
	Lines []string `json:"lines"`
}

// RequestData is the full JSON structure sent to the local model.
type RequestData struct {
	ContextBefore []SegmentData `json:"context_before"`
	Target        []SegmentData `json:"target"`
	ContextAfter  []SegmentData `json:"context_after"`
}

// TranslatedSegment is one translated segment in the JSON response.
type TranslatedSegment struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// ResponseData is the full structured response expected from the model.
type ResponseData struct {
	Translations []TranslatedSegment `json:"translations"`
	Usage        UsageMetadata       `json:"-"`
}

// UsageMetadata holds token usage information.
type UsageMetadata struct {
	PromptTokenCount     int
	CandidatesTokenCount int
	TotalTokenCount      int
	WebSearchCount       int
}

// Translator is implemented by translation backends.
type Translator interface {
	Translate(ctx context.Context, request RequestData) (*ResponseData, error)
	SetSystemInstruction(prompt string)
}
