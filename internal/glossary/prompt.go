package glossary

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oukeidos/focst-local/internal/language"
)

func RenderingHeader(target language.Language) string {
	return target.Name + " rendering"
}

func RenderUserPrompt(source, target language.Language, segments []Segment) (string, error) {
	input := struct {
		Segments []Segment `json:"segments"`
	}{Segments: segments}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to marshal glossary prompt input: %w", err)
	}
	header := RenderingHeader(target)
	var b strings.Builder
	fmt.Fprintf(&b, "Extract proper nouns from the provided %s subtitle script and provide their %s renderings.\n\n", source.Name, target.Name)
	b.WriteString("Return a Markdown table with exactly these columns:\n\n")
	fmt.Fprintf(&b, "| Source | %s |\n\n", header)
	b.WriteString("Rules:\n")
	b.WriteString("- Use exact source expressions from the script.\n")
	fmt.Fprintf(&b, "- For names, %q must be a %s name form, not a translation of the characters' meanings.\n", header, target.Name)
	fmt.Fprintf(&b, "- For specialized terms, do not translate word by word when an established %s rendering exists; use the conventional %s term instead.\n", target.Name, target.Name)
	b.WriteString("- Do not repeat the same source expression; output at most one row for each distinct source value.\n")
	b.WriteString("- Do not wrap the table in a code block.\n")
	b.WriteString("- Do not include explanations before or after the table.\n")
	b.WriteString("- If there are no useful entries, output a Markdown table with only the header row.\n\n")
	b.WriteString("Input JSON:\n")
	b.Write(payload)
	return b.String(), nil
}
