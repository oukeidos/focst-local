package postpolish

import (
	"encoding/json"
	"fmt"
)

const broadInstruction = "This pass is not a terminology or localization pass. Preserve existing term choices for names, named expressions, recurring expressions, loanwords, titles, and domain terms. Only repair broken %s phrasing."

const repairInstruction = "Only fix typo-like or garbled %s text. Do not polish style. Do not change word choice unless the current wording is malformed."

func systemPrompt(targetLanguage string) string {
	return fmt.Sprintf("You are a conservative %s subtitle copyeditor. Return only JSON.", targetLanguage)
}

func broadUserPrompt(sourceLanguage, targetLanguage string, req Request) (string, error) {
	return userPrompt(sourceLanguage, targetLanguage, fmt.Sprintf(broadInstruction, targetLanguage), req)
}

func repairUserPrompt(sourceLanguage, targetLanguage string, req Request) (string, error) {
	return userPrompt(sourceLanguage, targetLanguage, fmt.Sprintf(repairInstruction, targetLanguage), req)
}

func userPrompt(sourceLanguage, targetLanguage, instruction string, req Request) (string, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`Review the provided %s to %s subtitle translations after translation.

Task:
%s

Return JSON exactly in this shape: {"corrections":[{"id":1,"source_text":"...","text":"..."}]}.

Rules:
- Return only segments that clearly need a target-language polish edit.
- "id" must be an input segment ID.
- "source_text" must copy the input source_text for that same ID.
- "text" must be the replacement %s subtitle text.
- Do not include unchanged segments.
- Do not include explanations or extra fields.
- If no changes are needed, return {"corrections":[]}.

Input JSON:
%s`, sourceLanguage, targetLanguage, instruction, targetLanguage, string(payload)), nil
}

func ResponseSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"corrections"},
		"properties": map[string]any{
			"corrections": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"id", "source_text", "text"},
					"properties": map[string]any{
						"id": map[string]any{
							"type": "integer",
						},
						"source_text": map[string]any{
							"type": "string",
						},
						"text": map[string]any{
							"type": "string",
						},
					},
				},
			},
		},
	}
}
