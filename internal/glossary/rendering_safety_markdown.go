package glossary

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	ExpectedStrategyNameForm           = "name_form"
	ExpectedStrategyMeaningTranslation = "meaning_translation"
	ExpectedStrategyConventionalTerm   = "conventional_term"
	ExpectedStrategyContextDependent   = "context_dependent"

	FitFits      = "fits"
	FitUncertain = "uncertain"
	FitMismatch  = "mismatch"

	DecisionKeep   = "keep"
	DecisionReview = "review"
	DecisionReject = "reject"

	SourcePolicyKeep = "keep"
	SourcePolicyDrop = "drop"

	SourcePolicyReasonKeepNameForm      = "keep_name_form"
	SourcePolicyReasonDropNonNameReject = "drop_non_name_reject"
	SourcePolicyReasonKeepNonReject     = "keep_non_reject"
)

var (
	renderingSafetyStrategies = map[string]bool{
		ExpectedStrategyNameForm:           true,
		ExpectedStrategyMeaningTranslation: true,
		ExpectedStrategyConventionalTerm:   true,
		ExpectedStrategyContextDependent:   true,
	}
	renderingSafetyFits = map[string]bool{
		FitFits:      true,
		FitUncertain: true,
		FitMismatch:  true,
	}
	renderingSafetyDecisions = map[string]bool{
		DecisionKeep:   true,
		DecisionReview: true,
		DecisionReject: true,
	}
)

func ParseRenderingSafetyTable(content string, expectedRows []renderingSafetyRow) ([]RenderingSafetyJudgment, []string) {
	tableLines := collectMarkdownTableLines(content)
	var violations []string
	if len(tableLines) == 0 {
		return nil, []string{"missing rendering safety markdown table"}
	}
	if len(tableLines) < 2 {
		return nil, []string{"missing rendering safety markdown table alignment row"}
	}
	header, ok := splitTableRow(tableLines[0])
	if !ok || len(header) != 4 {
		return nil, []string{"invalid rendering safety table header"}
	}
	expectedHeader := []string{"Row", "Expected strategy", "Fit", "Decision"}
	for i := range expectedHeader {
		if header[i] != expectedHeader[i] {
			return nil, []string{fmt.Sprintf("unexpected rendering safety table header: %v", header)}
		}
	}
	align, ok := splitTableRow(tableLines[1])
	if !ok || len(align) != 4 {
		return nil, []string{"invalid rendering safety table alignment row"}
	}
	for _, cell := range align {
		if !isAlignmentCell(cell) {
			return nil, []string{"invalid rendering safety table alignment row"}
		}
	}

	expected := make(map[int]renderingSafetyRow, len(expectedRows))
	for _, row := range expectedRows {
		expected[row.Row] = row
	}
	seen := make(map[int]bool, len(expectedRows))
	judgments := make([]RenderingSafetyJudgment, 0, len(expectedRows))
	for _, line := range tableLines[2:] {
		cells, ok := splitTableRow(line)
		if !ok || len(cells) != 4 {
			violations = append(violations, "malformed rendering safety row: "+line)
			continue
		}
		rowID, err := strconv.Atoi(strings.TrimSpace(cells[0]))
		if err != nil {
			violations = append(violations, "bad rendering safety row number: "+cells[0])
			continue
		}
		input, ok := expected[rowID]
		if !ok {
			violations = append(violations, fmt.Sprintf("unexpected rendering safety row: %d", rowID))
			continue
		}
		if seen[rowID] {
			violations = append(violations, fmt.Sprintf("duplicate rendering safety row: %d", rowID))
			continue
		}
		seen[rowID] = true
		strategy := normalizeRenderingSafetyCell(cells[1])
		fit := normalizeRenderingSafetyCell(cells[2])
		decision := normalizeRenderingSafetyCell(cells[3])
		if !renderingSafetyStrategies[strategy] {
			violations = append(violations, fmt.Sprintf("bad expected_strategy row %d: %s", rowID, strategy))
		}
		if !renderingSafetyFits[fit] {
			violations = append(violations, fmt.Sprintf("bad fit row %d: %s", rowID, fit))
		}
		if !renderingSafetyDecisions[decision] {
			violations = append(violations, fmt.Sprintf("bad decision row %d: %s", rowID, decision))
		}
		result, reason := RenderingSafetySourcePolicy(strategy, decision)
		judgments = append(judgments, RenderingSafetyJudgment{
			Row:                rowID,
			Source:             input.Source,
			Rendering:          input.Rendering,
			ExpectedStrategy:   strategy,
			Fit:                fit,
			Decision:           decision,
			SourcePolicyResult: result,
			SourcePolicyReason: reason,
		})
	}
	for _, rowID := range sortedRenderingSafetyRows(expected) {
		if !seen[rowID] {
			violations = append(violations, fmt.Sprintf("missing rendering safety row: %d", rowID))
		}
	}
	if len(violations) > 0 {
		return judgments, violations
	}
	sort.Slice(judgments, func(i, j int) bool {
		return judgments[i].Row < judgments[j].Row
	})
	return judgments, nil
}

func collectMarkdownTableLines(content string) []string {
	lines := strings.Split(content, "\n")
	tableLines := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "|") {
			tableLines = append(tableLines, line)
		}
	}
	return tableLines
}

func normalizeRenderingSafetyCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), " ", "_"), "-", "_")
}

func sortedRenderingSafetyRows(rows map[int]renderingSafetyRow) []int {
	keys := make([]int, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func RenderingSafetySourcePolicy(expectedStrategy, decision string) (string, string) {
	if expectedStrategy == ExpectedStrategyNameForm {
		return SourcePolicyKeep, SourcePolicyReasonKeepNameForm
	}
	if decision == DecisionReject {
		return SourcePolicyDrop, SourcePolicyReasonDropNonNameReject
	}
	return SourcePolicyKeep, SourcePolicyReasonKeepNonReject
}
