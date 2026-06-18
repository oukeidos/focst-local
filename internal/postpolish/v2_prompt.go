package postpolish

import (
	"encoding/json"
	"fmt"
	"strings"
)

type segmentInstructionRequest struct {
	Segments []v2InputSegment `json:"segments"`
}

type chunkInstructionRequest struct {
	SourceText  string `json:"source_text"`
	CurrentText string `json:"current_text"`
}

type segmentApplicationRequest struct {
	Segments []v2InputSegment `json:"segments"`
}

type chunkApplicationRequest struct {
	EditInstruction string           `json:"edit_instruction"`
	Segments        []v2InputSegment `json:"segments"`
}

func segmentLocalInstructionPrompt(sourceLanguage, targetLanguage string, chunk v2Chunk) (string, error) {
	payload := segmentInstructionRequest{Segments: make([]v2InputSegment, 0, len(chunk.Segments))}
	for _, segment := range chunk.Segments {
		payload.Segments = append(payload.Segments, v2InputSegment{
			ID:          segment.ID,
			SourceText:  segment.SourceText,
			CurrentText: segment.Text,
		})
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`You will receive %s subtitle source text and current %s translations.

Task:
Make dialogue and narration sound natural in %s.

Instead of returning revised subtitles, describe how each current translation should be revised.

Return JSON exactly in this shape: {"segments":[{"id":1,"source_text":"...","edit_instruction":"..."}]}.

Rules:
- Return every input segment exactly once.
- Preserve input segment order.
- "id" must be an input segment ID.
- "source_text" must copy the input source_text for that same ID.
- "edit_instruction" must describe what to change in the current %s subtitle text, not the final rewritten subtitle.
- Use the source text to preserve meaning.
- If no edit is needed, write "No change needed."
- Be specific enough that an editor could apply the change.
- Do not include explanations outside JSON or extra fields.

Input JSON:
%s`, sourceLanguage, targetLanguage, targetLanguage, targetLanguage, string(data)), nil
}

func chunkFlowInstructionPrompt(sourceLanguage, targetLanguage string, chunk v2Chunk) (string, error) {
	payload := chunkInstructionRequest{
		SourceText:  mergedSourceText(chunk),
		CurrentText: mergedCurrentText(chunk),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`You will receive %s subtitle source text and current %s translations.

Task:
Make dialogue and narration sound natural in %s.

Instead of returning revised subtitles, write one edit_instruction for this whole chunk.
The instruction may describe multiple concrete fixes if needed.

Return JSON exactly in this shape: {"edit_instruction":"..."}.

Rules:
- The input is a merged chunk. Segment IDs and boundaries are intentionally hidden.
- "edit_instruction" must describe what to change in the current %s subtitle text, not the final rewritten subtitles.
- Use the source text to preserve meaning.
- If no edit is needed anywhere in the chunk, write "No change needed."
- Be specific enough that an editor could apply the change to the relevant subtitle rows.
- Do not include explanations outside JSON or extra fields.
- If you include an example, it must be a short phrase-level example, not a full sentence or full chunk rewrite.

Input JSON:
%s`, sourceLanguage, targetLanguage, targetLanguage, targetLanguage, string(data)), nil
}

func segmentLocalApplicationPrompt(sourceLanguage, targetLanguage string, instructions []SegmentInstruction, chunk v2Chunk) (string, error) {
	byID := make(map[int]SegmentInstruction, len(instructions))
	for _, instruction := range instructions {
		byID[instruction.ID] = instruction
	}
	payload := segmentApplicationRequest{Segments: make([]v2InputSegment, 0, len(chunk.Segments))}
	for _, segment := range chunk.Segments {
		instruction := byID[segment.ID]
		edit := strings.TrimSpace(instruction.EditInstruction)
		if edit == "" {
			edit = "No change needed."
		}
		payload.Segments = append(payload.Segments, v2InputSegment{
			ID:              segment.ID,
			SourceText:      segment.SourceText,
			CurrentText:     segment.Text,
			EditInstruction: edit,
		})
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`You will receive %s subtitle source text, current %s translations, and edit instructions.

Task:
Apply each edit_instruction faithfully to that segment's current_text.

Return JSON exactly in this shape: {"segments":[{"id":1,"source_text":"...","text":"..."}]}.

Rules:
- Return every input segment exactly once.
- Preserve input segment order.
- "id" must be an input segment ID.
- "source_text" must copy the input source_text for that same ID.
- "text" must be the final %s subtitle text after applying edit_instruction.
- Do not decide whether an edit is needed; the edit_instruction already made that decision.
- If edit_instruction is exactly "No change needed.", copy current_text exactly.
- Do not make any change that is not required by edit_instruction.
- Preserve source meaning, names, terms, and amount of information.
- Do not add explanations outside JSON or extra fields.

Input JSON:
%s`, sourceLanguage, targetLanguage, targetLanguage, string(data)), nil
}

func chunkFlowApplicationPrompt(sourceLanguage, targetLanguage, instruction string, chunk v2Chunk) (string, error) {
	payload := chunkApplicationRequest{
		EditInstruction: instruction,
		Segments:        make([]v2InputSegment, 0, len(chunk.Segments)),
	}
	for _, segment := range chunk.Segments {
		payload.Segments = append(payload.Segments, v2InputSegment{
			ID:          segment.ID,
			SourceText:  segment.SourceText,
			CurrentText: segment.Text,
		})
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`You will receive %s subtitle source text, current %s translations, and one edit_instruction for the whole chunk.

Task:
Apply edit_instruction faithfully to the relevant subtitle rows.

Return JSON exactly in this shape: {"segments":[{"id":1,"source_text":"...","text":"..."}]}.

Rules:
- Return every input segment exactly once.
- Preserve input segment order.
- "id" must be an input segment ID.
- "source_text" must copy the input source_text for that same ID.
- "text" must be the final %s subtitle text for that segment.
- If edit_instruction is exactly "No change needed.", copy every current_text exactly.
- If edit_instruction applies only to some rows, leave unrelated rows unchanged.
- Preserve how sentence fragments connect across adjacent segment IDs.
- Do not repeat one full rewritten chunk in multiple segment IDs.
- Preserve source meaning, names, terms, and amount of information.
- First, repair invalid input rows before applying edit_instruction.
- A row whose current_text is an editor note, placeholder, or meta comment is invalid even if edit_instruction does not mention that row.
- For every invalid row, output an actual %s subtitle fragment translated from that row's source_text.
- Do not write text such as "this sentence is connected to another ID", "same as previous", "continues", or similar explanations.
- If an adjacent row already contains the invalid row's meaning, reallocate the wording across the adjacent rows so each row has natural subtitle text and no row is blank.
- A translatable source segment must never become blank.
- Do not add explanations outside JSON or extra fields.

Input JSON:
%s`, sourceLanguage, targetLanguage, targetLanguage, targetLanguage, string(data)), nil
}

func mergedSourceText(chunk v2Chunk) string {
	parts := make([]string, 0, len(chunk.Segments))
	for _, segment := range chunk.Segments {
		if text := strings.TrimSpace(segment.SourceText); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func mergedCurrentText(chunk v2Chunk) string {
	parts := make([]string, 0, len(chunk.Segments))
	for _, segment := range chunk.Segments {
		if text := strings.TrimSpace(segment.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}
