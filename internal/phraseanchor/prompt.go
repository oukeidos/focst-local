package phraseanchor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/oukeidos/focst-local/internal/translation"
)

const candidateDiscoveryPromptTemplate = `Create a pre-translation review table for %[1]s-to-%[2]s subtitle translation.

Task:
Flag source phrases that need pre-translation review because casual %[2]s rendering could change meaning, tone, or consistency.

Include sensory or evaluative expressions when a neutral %[2]s rendering would lose warmth, criticism, irony, intimacy, or emotional judgment.

When a target segment consists only of a repeated local keyword already selected from a nearby target segment, include that standalone segment as a separate candidate.
Do not add every later repetition unless the current segment creates a new %[2]s rendering decision.

Selection Criteria:
- Include source phrases where %[2]s wording requires a deliberate choice.
- Include short abbreviations, loanwords, coined terms, or common category labels when %[2]s has competing natural renderings beyond simple transliteration.
- Prefer phrases whose issue can be explained from the local source context.
- Do not force rows if there are no useful phrases.

Scope:
- Work only from the provided %[1]s source segments.
- Use context_before and context_after only to understand local context.
- Create rows only for Segment ID values inside the target array.
- Prioritize the focus ID range when it contains useful candidates, but do not force rows.

Exclusion Criteria:
- Exclude proper nouns, personal names, place names, organization names, work titles, name explanations, and ordinary/common nouns unconditionally.

Type Definitions:
- ambiguity: the source phrase has more than one plausible %[2]s rendering, or the correct subject, object, referent, scope, or sense must be resolved from local context.
- idiom: the source phrase is idiomatic, slang-like, metaphorical, conventional, compressed, or not safely translatable by literal meaning alone.
- wordplay: the source phrase uses sound, spelling, script, repetition, double meaning, meta-language, or deliberate wording as a joke, pun, or linguistic effect.

Output Format:
Return a Markdown table with exactly these columns:

| Segment ID | Source text | Type | Source quote | %[5]s |

Field Rules:
- Segment ID must be one of the provided target segment IDs.
- Source text must exactly equal the source_text of Segment ID.
- Source quote must appear exactly in Source text.
- Source quote must be the shortest exact substring of Source text that still contains the translation decision.
- Do not use the entire Source text as Source quote unless the whole line is itself an indivisible idiom, callback, or wordplay.
- Type must be one of: ambiguity, idiom, wordplay.
- %[5]s must be a concrete %[2]s translation/rendering proposal, not an explanation.
- Each rendering field must contain exactly one concrete %[2]s rendering. Do not use separators such as "/" to list multiple rendering options.
- If there are no useful rows, output only the header row.

Focus ID range: %[3]s

Source JSON:
%[4]s`

const candidateAddPromptTemplate = `Add only newly found useful source phrase rows that were missed before.

Do not repeat rows already present in previous tables.

Return a Markdown table with exactly this header:
| Segment ID | Source text | Type | Source quote | %[2]s |

Source quote rules:
- Source quote must be the shortest exact substring of Source text that still contains the translation decision.
- Do not use the entire Source text as Source quote unless the whole line is itself an indivisible idiom, callback, or wordplay.

Each rendering field must contain exactly one concrete %[1]s rendering. Do not use separators such as "/" to list multiple rendering options.

If there are no new useful rows, output only the header row.`

const alternativePromptTemplate = `For each row in the provided table, write one Alternative Rendering that can serve as option B in a later A/B vote against the existing %[1]s rendering as option A.

A/B vote means that a later step will compare:
- A: the existing %[1]s rendering
- B: your Alternative Rendering

Alternative Rendering must be one concrete %[1]s rendering of Source quote, not a translation of the full Source text. Use only the Source text in the input table to understand the quote.

Choose the alternative according to Type:
- ambiguity: choose a different plausible meaning, referent, scope, or attachment for Source quote.
- idiom: choose a different plausible handling of the expression, such as idiomatic vs literal, direct vs naturalized, or compact vs explanatory.
- terminology: choose a different translation policy for the term, such as loanword vs %[1]s gloss, abbreviation vs expansion, or fixed label vs descriptive rendering.
- tone: keep the same basic meaning, but choose a clearly different tone, register, stance, emotional force, or scene-specific connotation.
- wordplay: choose a different way to handle the linguistic effect, such as sound, script, repetition, double meaning, meta-language, or joke effect.

The provided table is the only row set to process.
Do not add rows.
Do not omit rows.
Do not change Segment ID, Source text, Type, Source quote, or %[2]s.

Each rendering field must contain exactly one concrete %[1]s rendering. Do not use separators such as "/" to list multiple rendering options.

Return a Markdown table with exactly this header:
| Segment ID | Source text | Type | Source quote | %[2]s | Alternative Rendering |

Input table:
%[3]s`

const votePromptTemplate = `For each row in the provided table, choose whether option A or option B is the better %[1]s rendering for the source quote in context.

A/B vote means:
- A: the %[1]s rendering in column A
- B: the %[1]s rendering in column B

Choose A or B by judging which option better preserves the source meaning, tone, and translation consistency in the given context.

Rules:
- Process every row in the provided table.
- Do not add rows.
- Do not omit rows.
- Do not change row numbers.
- Choose exactly one option for each row: A or B.
- Do not write %[1]s rendering text.
- Do not write explanations or reasons.

Return a Markdown table with exactly this header:
| Row | Choice |

Input table:
%[2]s

Source JSON:
%[3]s`

func RenderReviewSystemPrompt(sourceName, targetName string) string {
	return fmt.Sprintf("You create pre-translation review tables for %s-to-%s subtitle translation.\nOutput only Markdown tables.\nDo not output explanations, commentary, code fences, or extra prose.", sourceName, targetName)
}

func RenderCandidateDiscoveryPrompt(focusStart, focusEnd int, sourceJSON, sourceName, targetName string) string {
	renderingColumn := renderingColumnName(targetName)
	return fmt.Sprintf(candidateDiscoveryPromptTemplate, sourceName, targetName, focusRangeString(focusStart, focusEnd), sourceJSON, renderingColumn)
}

func RenderCandidateAddPrompt(targetName string) string {
	return fmt.Sprintf(candidateAddPromptTemplate, targetName, renderingColumnName(targetName))
}

func RenderQuoteKindPrompt(rows []Candidate) string {
	var b strings.Builder
	b.WriteString("Classify each source quote by what the quote itself is inside the given source text.\n\n")
	b.WriteString("Use exactly one category:\n\n")
	b.WriteString("- proper_noun examples: unique names or titles for specific people, groups, places, works, products, teams, aliases, or named entities.\n")
	b.WriteString("- common_noun examples: plain ordinary nouns or noun phrases that mainly name a general thing, class, role, item, genre, or concept.\n")
	b.WriteString("- other examples: idioms, wordplay, ambiguous wording, callbacks, evaluative wording, conversational formulas, repeated local keywords, compact labels, abbreviations, loanwords, or coined expressions when their local use matters more than simply naming a general thing.\n\n")
	b.WriteString("Important rules:\n")
	b.WriteString("- Classify the Source quote, not the whole Source text.\n")
	b.WriteString("- Use Source text only as context for deciding what the Source quote is.\n")
	b.WriteString("- Classify the quoted span as a whole.\n")
	b.WriteString("- Choose `proper_noun` only when the quote is mainly a unique name or title.\n")
	b.WriteString("- Choose `common_noun` only when the quote mainly names a general thing without relying on local phrasing.\n")
	b.WriteString("- Choose `other` for repeated local expressions, callbacks, compact labels, abbreviations, or loanwords when the quote's local use affects the classification.\n")
	b.WriteString("- If uncertain between `common_noun` and `other`, choose `other`.\n")
	b.WriteString("- Return only the markdown table.\n\n")
	b.WriteString("Output format:\n\n")
	b.WriteString("| Row | Category |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString("| 1 | proper_noun |\n")
	b.WriteString("| 2 | common_noun |\n")
	b.WriteString("| 3 | other |\n\n")
	b.WriteString("Input rows:\n\n")
	b.WriteString("| Row | Source text | Source quote |\n")
	b.WriteString("| --- | --- | --- |\n")
	for index, row := range rows {
		writeMarkdownRow(&b, []string{
			strconv.Itoa(index + 1),
			row.SourceText,
			row.SourceQuote,
		})
	}
	return b.String()
}

func RenderSourceNamePrompt(sourceName, sourceKind string, segments []translation.SegmentData) (string, error) {
	input := struct {
		Segments []translation.SegmentData `json:"segments"`
	}{Segments: segments}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to marshal source-name prompt input: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Extract %s from the provided %s subtitle script.\n\n", sourceKind, sourceName)
	b.WriteString("Return a Markdown table with exactly this column:\n\n")
	b.WriteString("| Source |\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Use exact source expressions from the script.\n")
	b.WriteString("- Do not repeat the same source expression; output at most one row for each distinct source value.\n")
	b.WriteString("- Do not wrap the table in a code block.\n")
	b.WriteString("- Do not include explanations before or after the table.\n")
	b.WriteString("- If there are no useful entries, output a Markdown table with only the header row.\n\n")
	b.WriteString("Input JSON:\n")
	b.Write(payload)
	return b.String(), nil
}

func RenderAlternativePrompt(table, targetName string) string {
	return fmt.Sprintf(alternativePromptTemplate, targetName, renderingColumnName(targetName), table)
}

func RenderVotePrompt(table, sourceJSON, targetName string) string {
	return fmt.Sprintf(votePromptTemplate, targetName, table, sourceJSON)
}

func renderingColumnName(targetName string) string {
	targetName = strings.TrimSpace(targetName)
	if targetName == "" {
		targetName = "Target"
	}
	return targetName + " rendering"
}

func focusRangeString(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func writeMarkdownRow(b *strings.Builder, cells []string) {
	b.WriteString("|")
	for _, cell := range cells {
		b.WriteString(" ")
		b.WriteString(escapeMarkdownCell(cell))
		b.WriteString(" |")
	}
	b.WriteString("\n")
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}
