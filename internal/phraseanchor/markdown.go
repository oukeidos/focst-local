package phraseanchor

import (
	"fmt"
	"strconv"
	"strings"
)

type markdownTable struct {
	Header     []string
	Rows       []map[string]string
	CodeFence  bool
	ExtraProse []string
}

func parseMarkdownTable(content string) markdownTable {
	var tableLines []string
	table := markdownTable{}
	inFence := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "```") {
			table.CodeFence = true
			inFence = !inFence
			continue
		}
		if strings.HasPrefix(line, "|") {
			tableLines = append(tableLines, line)
			continue
		}
		if inFence || len(tableLines) > 0 {
			table.ExtraProse = append(table.ExtraProse, line)
		} else {
			table.ExtraProse = append(table.ExtraProse, line)
		}
	}
	if len(tableLines) < 2 {
		return table
	}
	header := splitMarkdownRow(tableLines[0])
	align := splitMarkdownRow(tableLines[1])
	if len(header) == 0 || len(align) != len(header) || !alignmentRow(align) {
		return table
	}
	table.Header = header
	for _, rowLine := range tableLines[2:] {
		cells := splitMarkdownRow(rowLine)
		if len(cells) != len(header) {
			continue
		}
		row := map[string]string{}
		for i, key := range header {
			row[key] = strings.TrimSpace(cells[i])
		}
		table.Rows = append(table.Rows, row)
	}
	return table
}

func splitMarkdownRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var b strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '|' {
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	cells = append(cells, strings.TrimSpace(b.String()))
	return cells
}

func alignmentRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.Trim(strings.TrimSpace(cell), ":")
		if cell == "" {
			return false
		}
		for _, r := range cell {
			if r != '-' {
				return false
			}
		}
	}
	return true
}

func renderCandidateTable(rows []Candidate, targetName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Segment ID | Source text | Type | Source quote | %s |\n", renderingColumnName(targetName))
	b.WriteString("| ---: | --- | --- | --- | --- |\n")
	for _, row := range rows {
		writeMarkdownRow(&b, []string{
			strconv.Itoa(row.SegmentID),
			row.SourceText,
			row.Type,
			row.SourceQuote,
			row.Rendering,
		})
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderAlternativeInputTable(row Candidate, targetName string) string {
	return renderCandidateTable([]Candidate{row}, targetName)
}

func renderVoteInputTable(candidates []Candidate, alternatives []Alternative) ([]VoteRow, string) {
	alternativeByKey := map[string]Alternative{}
	for _, alt := range alternatives {
		alternativeByKey[candidateKey(alt.Candidate)] = alt
	}
	var rows []VoteRow
	for _, candidate := range candidates {
		alt, ok := alternativeByKey[candidateKey(candidate)]
		if !ok || strings.TrimSpace(alt.AlternativeRendering) == "" {
			continue
		}
		row := VoteRow{
			Row:       len(rows) + 1,
			Candidate: candidate,
			A:         candidate.Rendering,
			B:         alt.AlternativeRendering,
		}
		rows = append(rows, row)
	}
	return renderVoteTableRows(rows)
}

func renderVoteTableRows(inputRows []VoteRow) ([]VoteRow, string) {
	rows := make([]VoteRow, 0, len(inputRows))
	var b strings.Builder
	b.WriteString("| Row No | Segment ID | Source text | Type | Source quote | A | B |\n")
	b.WriteString("| ---: | ---: | --- | --- | --- | --- | --- |\n")
	for _, input := range inputRows {
		row := input
		row.Row = len(rows) + 1
		rows = append(rows, row)
		writeMarkdownRow(&b, []string{
			strconv.Itoa(row.Row),
			strconv.Itoa(row.Candidate.SegmentID),
			row.Candidate.SourceText,
			row.Candidate.Type,
			row.Candidate.SourceQuote,
			row.A,
			row.B,
		})
	}
	return rows, strings.TrimRight(b.String(), "\n")
}

func parseCandidateTable(content string, window Window, stage, targetName string) ([]Candidate, []RejectedCandidate) {
	table := parseMarkdownTable(content)
	var rejected []RejectedCandidate
	renderingColumn := renderingColumnName(targetName)
	if !candidateHeaderOK(table.Header, renderingColumn) {
		return nil, []RejectedCandidate{{Reason: fmt.Sprintf("unexpected %s table header: %v", stage, table.Header), PhraseWindowIndex: window.Index, Stage: stage}}
	}
	sourceByID := map[int]string{}
	targetIDs := map[int]bool{}
	for _, seg := range window.Target {
		sourceByID[seg.ID] = seg.SourceText
		targetIDs[seg.ID] = true
	}
	repairCandidateRows(table.Rows, sourceByID, targetIDs)
	var out []Candidate
	seen := map[string]bool{}
	for _, row := range table.Rows {
		candidate, ok, reason := candidateFromRow(row, window, stage, sourceByID, targetIDs, renderingColumn)
		if !ok {
			rejected = append(rejected, rejectFromRow(row, window.Index, stage, reason, renderingColumn))
			continue
		}
		key := candidateKey(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out, rejected
}

func parseAlternativeTable(content string, input Candidate, targetName string) (Alternative, []RejectedCandidate) {
	table := parseMarkdownTable(content)
	renderingColumn := renderingColumnName(targetName)
	if !alternativeHeaderOK(table.Header, renderingColumn) {
		return Alternative{}, []RejectedCandidate{{Reason: fmt.Sprintf("unexpected alternative table header: %v", table.Header), SegmentID: input.SegmentID, SourceQuote: input.SourceQuote, Stage: AlternativeName}}
	}
	repairAlternativeRows(table.Rows, input, renderingColumn)
	for _, row := range table.Rows {
		candidate, ok, reason := candidateFromRow(row, Window{
			Index:                 input.PhraseWindowIndex,
			TranslationChunkIndex: input.TranslationChunkIndex,
		}, AlternativeName, map[int]string{input.SegmentID: input.SourceText}, map[int]bool{input.SegmentID: true}, renderingColumn)
		if !ok {
			return Alternative{}, []RejectedCandidate{rejectFromRow(row, input.PhraseWindowIndex, AlternativeName, reason, renderingColumn)}
		}
		if candidateKey(candidate) != candidateKey(input) {
			return Alternative{}, []RejectedCandidate{{Reason: "alternative row does not match input candidate", SegmentID: input.SegmentID, SourceQuote: input.SourceQuote, Stage: AlternativeName}}
		}
		alt := strings.TrimSpace(row["Alternative Rendering"])
		if alt == "" || alt == "-" || strings.Contains(alt, "/") {
			return Alternative{}, []RejectedCandidate{{Reason: "empty or multi-option alternative rendering", SegmentID: input.SegmentID, SourceQuote: input.SourceQuote, Stage: AlternativeName}}
		}
		return Alternative{Candidate: candidate, AlternativeRendering: alt}, nil
	}
	return Alternative{}, []RejectedCandidate{{Reason: "missing alternative row", SegmentID: input.SegmentID, SourceQuote: input.SourceQuote, Stage: AlternativeName}}
}

func parseVoteChoices(content string) map[int]string {
	table := parseMarkdownTable(content)
	out := map[int]string{}
	if !sameHeader(table.Header, []string{"Row", "Choice"}) {
		return out
	}
	for _, row := range table.Rows {
		n, err := strconv.Atoi(strings.TrimSpace(row["Row"]))
		if err != nil || n <= 0 {
			continue
		}
		choice := strings.ToUpper(strings.TrimSpace(row["Choice"]))
		if choice == "A" || choice == "B" {
			out[n] = choice
		}
	}
	return out
}

func parseQuoteKindChoices(content string) map[int]string {
	table := parseMarkdownTable(content)
	out := map[int]string{}
	if !sameHeader(table.Header, []string{"Row", "Category"}) {
		return out
	}
	for _, row := range table.Rows {
		n, err := strconv.Atoi(strings.TrimSpace(row["Row"]))
		if err != nil || n <= 0 {
			continue
		}
		out[n] = normalizeCategory(row["Category"])
	}
	return out
}

func parseSourceNameTable(content string) []string {
	table := parseMarkdownTable(content)
	if !sameHeader(table.Header, []string{"Source"}) {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, row := range table.Rows {
		source := collapseSpaces(row["Source"])
		if source == "" || source == "-" || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	return out
}

func candidateHeaderOK(header []string, renderingColumn string) bool {
	return sameHeader(header, []string{"Segment ID", "Source text", "Type", "Source quote", renderingColumn})
}

func alternativeHeaderOK(header []string, renderingColumn string) bool {
	return sameHeader(header, []string{"Segment ID", "Source text", "Type", "Source quote", renderingColumn, "Alternative Rendering"})
}

func sameHeader(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func renderingFromRow(row map[string]string, renderingColumn string) string {
	return strings.TrimSpace(row[renderingColumn])
}

func repairCandidateRows(rows []map[string]string, sourceByID map[int]string, targetIDs map[int]bool) {
	textTargetIDs := sourceTextTargetIDs(sourceByID, targetIDs)
	for _, row := range rows {
		repairSegmentIDFromExactSourceText(row, sourceByID, targetIDs, textTargetIDs)
		repairSourceTextFromSegmentID(row, sourceByID, targetIDs)
	}
}

func repairAlternativeRows(rows []map[string]string, input Candidate, renderingColumn string) {
	for _, row := range rows {
		row["Segment ID"] = strconv.Itoa(input.SegmentID)
		row["Source text"] = input.SourceText
		row["Type"] = input.Type
		row["Source quote"] = input.SourceQuote
		row[renderingColumn] = input.Rendering
	}
}

func repairSegmentIDFromExactSourceText(row map[string]string, sourceByID map[int]string, targetIDs map[int]bool, textTargetIDs map[string][]int) bool {
	sourceText := strings.TrimSpace(row["Source text"])
	quote := strings.TrimSpace(row["Source quote"])
	if sourceText == "" || quote == "" || !strings.Contains(sourceText, quote) {
		return false
	}
	matches := textTargetIDs[sourceText]
	if len(matches) != 1 {
		return false
	}
	currentID, err := strconv.Atoi(strings.TrimSpace(row["Segment ID"]))
	matchID := matches[0]
	if err == nil && currentID == matchID {
		return false
	}
	if err == nil && targetIDs[currentID] && sourceByID[currentID] == sourceText {
		return false
	}
	row["Segment ID"] = strconv.Itoa(matchID)
	return true
}

func repairSourceTextFromSegmentID(row map[string]string, sourceByID map[int]string, targetIDs map[int]bool) bool {
	segID, err := strconv.Atoi(strings.TrimSpace(row["Segment ID"]))
	if err != nil || !targetIDs[segID] {
		return false
	}
	canonical := sourceByID[segID]
	quote := strings.TrimSpace(row["Source quote"])
	if canonical == "" || quote == "" || !strings.Contains(canonical, quote) || row["Source text"] == canonical {
		return false
	}
	row["Source text"] = canonical
	return true
}

func sourceTextTargetIDs(sourceByID map[int]string, targetIDs map[int]bool) map[string][]int {
	result := map[string][]int{}
	for id := range targetIDs {
		text := sourceByID[id]
		if text == "" {
			continue
		}
		result[text] = append(result[text], id)
	}
	return result
}

func normalizeCategory(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "`", "")))
	switch value {
	case CategoryProperNoun, CategoryCommonNoun, CategoryOther:
		return value
	}
	if strings.Contains(value, "proper") {
		return CategoryProperNoun
	}
	if strings.Contains(value, "common") {
		return CategoryCommonNoun
	}
	if strings.Contains(value, "other") {
		return CategoryOther
	}
	return CategoryUnclassified
}

func candidateFromRow(row map[string]string, window Window, stage string, sourceByID map[int]string, targetIDs map[int]bool, renderingColumn string) (Candidate, bool, string) {
	id, err := strconv.Atoi(strings.TrimSpace(row["Segment ID"]))
	if err != nil || id <= 0 {
		return Candidate{}, false, "invalid segment ID"
	}
	sourceText := strings.TrimSpace(row["Source text"])
	typ := strings.TrimSpace(row["Type"])
	quote := strings.TrimSpace(row["Source quote"])
	rendering := renderingFromRow(row, renderingColumn)
	if !targetIDs[id] {
		return Candidate{}, false, "segment ID outside target window"
	}
	if sourceByID[id] != sourceText {
		return Candidate{}, false, "source text mismatch"
	}
	if !allowedType(typ) {
		return Candidate{}, false, "disallowed type"
	}
	if quote == "" || !strings.Contains(sourceText, quote) {
		return Candidate{}, false, "source quote not found in source text"
	}
	if rendering == "" || rendering == "-" || strings.Contains(rendering, "/") {
		return Candidate{}, false, "empty or multi-option rendering"
	}
	return Candidate{
		SegmentID:             id,
		SourceText:            sourceText,
		Type:                  typ,
		SourceQuote:           quote,
		Rendering:             rendering,
		TranslationChunkIndex: window.TranslationChunkIndex,
		PhraseWindowIndex:     window.Index,
		Stage:                 stage,
	}, true, ""
}

func rejectFromRow(row map[string]string, windowIndex int, stage, reason, renderingColumn string) RejectedCandidate {
	id, _ := strconv.Atoi(strings.TrimSpace(row["Segment ID"]))
	return RejectedCandidate{
		SegmentID:         id,
		SourceText:        strings.TrimSpace(row["Source text"]),
		Type:              strings.TrimSpace(row["Type"]),
		SourceQuote:       strings.TrimSpace(row["Source quote"]),
		Rendering:         renderingFromRow(row, renderingColumn),
		Reason:            reason,
		PhraseWindowIndex: windowIndex,
		Stage:             stage,
	}
}

func allowedType(typ string) bool {
	switch typ {
	case TypeAmbiguity, TypeIdiom, TypeWordplay:
		return true
	default:
		return false
	}
}

func candidateKey(candidate Candidate) string {
	return strconv.Itoa(candidate.SegmentID) + "\x00" + candidate.SourceText + "\x00" + candidate.SourceQuote
}

func voteRowKey(row VoteRow) string {
	return candidateKey(row.Candidate) + "\x00" + row.A + "\x00" + row.B
}
