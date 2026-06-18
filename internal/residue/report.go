package residue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oukeidos/focst-local/internal/files"
)

func SaveMarkdownReport(path string, artifact Artifact, repairs []RepairRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create residue report directory: %w", err)
	}
	return files.AtomicWrite(path, []byte(MarkdownReport(artifact, repairs)), 0600)
}

func MarkdownReport(artifact Artifact, repairs []RepairRecord) string {
	var b strings.Builder
	b.WriteString("# Source Residue Report\n\n")
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "- Source language: `%s`\n", artifact.SourceLanguage)
	fmt.Fprintf(&b, "- Target language: `%s`\n", artifact.TargetLanguage)
	fmt.Fprintf(&b, "- Script spec: `%s`\n", artifact.Config.ScriptSpec)
	fmt.Fprintf(&b, "- Selected scripts: `%s`\n", strings.Join(artifact.Config.SelectedScripts, ", "))
	fmt.Fprintf(&b, "- Candidates: `%d`\n", len(artifact.Candidates))
	if repairs != nil {
		fmt.Fprintf(&b, "- Repair records: `%d`\n", len(repairs))
	}
	b.WriteString("\n")
	b.WriteString("## Script Stats\n\n")
	b.WriteString("| Script | Source Count | Source Share | Target Count | Target Share | Selected |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, stat := range artifact.ScriptStats {
		if stat.SourceCount == 0 && stat.TargetCount == 0 && !stat.Selected {
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %.4f | %d | %.4f | %t |\n",
			escapeMD(stat.Name),
			stat.SourceCount,
			stat.SourceShare,
			stat.TargetCount,
			stat.TargetShare,
			stat.Selected,
		)
	}
	b.WriteString("\n")
	b.WriteString("## Candidates\n\n")
	if len(artifact.Candidates) == 0 {
		b.WriteString("No residue candidates.\n")
	} else {
		b.WriteString("| ID | Time | Scripts | Residue | Source | Before |\n")
		b.WriteString("| ---: | --- | --- | --- | --- | --- |\n")
		for _, c := range artifact.Candidates {
			fmt.Fprintf(&b, "| %d | %s --> %s | %s | %s | %s | %s |\n",
				c.ID,
				escapeMD(c.StartTime),
				escapeMD(c.EndTime),
				escapeMD(strings.Join(c.Scripts, ", ")),
				escapeMD(strings.Join(c.Residues, ", ")),
				escapeMD(c.SourceText),
				escapeMD(c.CurrentText),
			)
		}
	}
	if repairs == nil {
		return b.String()
	}
	b.WriteString("\n## Repairs\n\n")
	if len(repairs) == 0 {
		b.WriteString("No repair records.\n")
		return b.String()
	}
	b.WriteString("| ID | Status | Reason | Residue | Before | Proposed |\n")
	b.WriteString("| ---: | --- | --- | --- | --- | --- |\n")
	for _, r := range repairs {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n",
			r.ID,
			escapeMD(r.Status),
			escapeMD(r.Reason),
			escapeMD(strings.Join(r.Residues, ", ")),
			escapeMD(r.Before),
			escapeMD(r.Proposed),
		)
	}
	return b.String()
}

func escapeMD(input string) string {
	input = strings.ReplaceAll(input, "\n", " ")
	input = strings.ReplaceAll(input, "|", "\\|")
	return strings.TrimSpace(input)
}
