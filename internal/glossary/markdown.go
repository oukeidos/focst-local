package glossary

import (
	"fmt"
	"strings"
)

type ParseResult struct {
	Candidates []Candidate
	Rejected   []RejectedCandidate
	Violations []string
}

func ParseMarkdownTable(text, expectedRenderingHeader string, windowIndex, runIndex int) ParseResult {
	lines := strings.Split(text, "\n")
	tableLines := make([]string, 0, len(lines))
	var violations []string
	inFence := false
	usedFence := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			usedFence = true
			continue
		}
		if strings.HasPrefix(line, "|") {
			tableLines = append(tableLines, line)
			continue
		}
		if inFence || len(tableLines) > 0 {
			violations = append(violations, "non-table text in glossary response: "+line)
		} else {
			violations = append(violations, "non-table text before glossary table: "+line)
		}
	}
	if usedFence {
		violations = append(violations, "glossary table was wrapped in a code fence")
	}
	if len(tableLines) == 0 {
		return ParseResult{Violations: append(violations, "missing markdown table")}
	}
	if len(tableLines) < 2 {
		return ParseResult{Violations: append(violations, "missing markdown table alignment row")}
	}

	header, ok := splitTableRow(tableLines[0])
	if !ok || len(header) != 2 {
		return ParseResult{Violations: append(violations, "invalid glossary table header")}
	}
	if header[0] != "Source" || header[1] != expectedRenderingHeader {
		violations = append(violations, fmt.Sprintf("unexpected glossary table header: %q, %q", header[0], header[1]))
		return ParseResult{Violations: violations}
	}
	align, ok := splitTableRow(tableLines[1])
	if !ok || len(align) != 2 || !isAlignmentCell(align[0]) || !isAlignmentCell(align[1]) {
		violations = append(violations, "invalid glossary table alignment row")
		return ParseResult{Violations: violations}
	}

	var candidates []Candidate
	var rejected []RejectedCandidate
	for _, rowLine := range tableLines[2:] {
		row, ok := splitTableRow(rowLine)
		if !ok || len(row) != 2 {
			rejected = append(rejected, RejectedCandidate{
				Reason:      "malformed markdown row",
				WindowIndex: windowIndex,
				RunIndex:    runIndex,
			})
			continue
		}
		source := strings.TrimSpace(row[0])
		rendering := strings.TrimSpace(row[1])
		if source == "" || rendering == "" {
			rejected = append(rejected, RejectedCandidate{
				Source:      source,
				Rendering:   rendering,
				Reason:      "empty source or rendering",
				WindowIndex: windowIndex,
				RunIndex:    runIndex,
			})
			continue
		}
		candidates = append(candidates, Candidate{
			Source:      source,
			Rendering:   rendering,
			WindowIndex: windowIndex,
			RunIndex:    runIndex,
		})
	}
	return ParseResult{Candidates: candidates, Rejected: rejected, Violations: violations}
}

func splitTableRow(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil, false
	}
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells, true
}

func isAlignmentCell(cell string) bool {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return false
	}
	cell = strings.Trim(cell, ":")
	if cell == "" {
		return false
	}
	for _, r := range cell {
		if r != '-' {
			return false
		}
	}
	return true
}
