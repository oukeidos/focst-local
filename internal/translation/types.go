package translation

import (
	"context"
	"strings"
)

// SegmentData is one subtitle segment in the JSON request.
type SegmentData struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
}

// RequestData is the full JSON structure sent to the local model.
type RequestData struct {
	ContextBefore []SegmentData `json:"context_before"`
	Target        []SegmentData `json:"target"`
	ContextAfter  []SegmentData `json:"context_after"`
}

// TranslatedSegment is one translated segment in the JSON response.
type TranslatedSegment struct {
	ID         int    `json:"id"`
	SourceText string `json:"source_text"`
	Text       string `json:"text"`
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

// TextCompletion is a raw text chat-completion result used by non-translation
// helper passes such as local glossary extraction.
type TextCompletion struct {
	Content string
	Usage   UsageMetadata
}

// TextCompleter is implemented by local chat-completion backends that can
// return plain text without structured JSON schema forcing.
type TextCompleter interface {
	CompleteText(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (*TextCompletion, error)
}

// SourceTextFromLines normalizes one subtitle segment into the single text
// block sent to local models.
func SourceTextFromLines(lines []string) string {
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}

// Translator is implemented by translation backends.
type Translator interface {
	Translate(ctx context.Context, request RequestData) (*ResponseData, error)
	SetSystemInstruction(prompt string)
}
