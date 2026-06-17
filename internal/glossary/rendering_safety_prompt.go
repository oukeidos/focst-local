package glossary

import (
	"fmt"
	"strings"

	"github.com/oukeidos/focst-local/internal/language"
)

type renderingSafetyRow struct {
	Row         int
	Source      string
	Rendering   string
	Occurrences []Segment
}

func RenderRenderingSafetyPrompt(source, target language.Language, rows []renderingSafetyRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review proposed glossary entries for %s-to-%s subtitle translation.\n\n", source.Name, target.Name)
	b.WriteString("Each input row is a generated global glossary entry. First decide the expected rendering strategy for the source expression in this subtitle context. Then judge whether the proposed rendering fits that expected strategy.\n\n")
	b.WriteString("Choose exactly one Expected strategy:\n")
	fmt.Fprintf(&b, "- name_form: the source identifies a specific person, character, place, organization, product, work, or other unique entity, and should be rendered as that entity's natural %s name.\n", target.Name)
	b.WriteString("- meaning_translation: the source does not identify a unique entity; its meaning, description, role, nickname, or wording should be translated rather than sounded out.\n")
	fmt.Fprintf(&b, "- conventional_term: the source is a fixed public, cultural, or specialized term with a recognized %s equivalent; ordinary words and routine dialogue are not this category.\n", target.Name)
	b.WriteString("- context_dependent: the source should not be globally fixed because the same source text can require different valid renderings in different subtitle contexts.\n\n")
	b.WriteString("Choose exactly one Fit:\n")
	b.WriteString("- fits: the proposed rendering fits the expected strategy.\n")
	b.WriteString("- uncertain: the fit is uncertain.\n")
	b.WriteString("- mismatch: the proposed rendering clearly does not fit the expected strategy.\n\n")
	b.WriteString("Choose exactly one Decision:\n")
	b.WriteString("- keep: the proposed rendering fits well enough for global glossary injection.\n")
	b.WriteString("- review: uncertain, or potentially useful but not safe to trust blindly.\n")
	b.WriteString("- reject: the proposed rendering is clearly unsafe as a global glossary entry.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Expected strategy describes the source expression, not the proposed rendering.\n")
	b.WriteString("- Fit compares the proposed rendering against the expected strategy.\n")
	b.WriteString("- Judge only the proposed rendering. Do not create replacement renderings.\n")
	b.WriteString("- Return only a Markdown table with exactly this header:\n\n")
	b.WriteString("| Row | Expected strategy | Fit | Decision |\n\n")
	b.WriteString("Input rows:\n\n")
	b.WriteString("| Row | Source | Proposed rendering | Occurrence snippets |\n")
	b.WriteString("| ---: | --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(
			&b,
			"| %d | %s | %s | %s |\n",
			row.Row,
			markdownCell(row.Source),
			markdownCell(row.Rendering),
			markdownCell(renderOccurrenceSnippets(row.Occurrences)),
		)
	}
	return b.String()
}

func renderOccurrenceSnippets(segments []Segment) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		parts = append(parts, fmt.Sprintf("%d: %s", segment.ID, segment.SourceText))
	}
	return strings.Join(parts, "; ")
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.TrimSpace(value)
}
