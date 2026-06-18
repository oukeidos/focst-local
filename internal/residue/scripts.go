package residue

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type Script struct {
	Name  string
	Table *unicode.RangeTable
}

var exampleScripts = []string{
	"Latin",
	"Han",
	"Devanagari",
	"Arabic",
	"Bengali",
	"Cyrillic",
	"Hiragana",
	"Katakana",
	"Telugu",
	"Tamil",
	"Hangul",
}

func UnicodeVersion() string {
	return unicode.Version
}

func SupportedScripts() []string {
	names := make([]string, 0, len(unicode.Scripts))
	for name := range unicode.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ExampleScripts() []string {
	return append([]string(nil), exampleScripts...)
}

func ParseScriptList(spec string) ([]Script, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, false, fmt.Errorf("residue scripts are required")
	}
	if strings.EqualFold(spec, AutoScripts) {
		return nil, true, nil
	}
	var scripts []Script
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(spec, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name, table, ok := lookupScript(raw)
		if !ok {
			return nil, false, fmt.Errorf("unsupported residue script %q", raw)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		scripts = append(scripts, Script{Name: name, Table: table})
	}
	if len(scripts) == 0 {
		return nil, false, fmt.Errorf("residue scripts are required")
	}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].Name < scripts[j].Name })
	return scripts, false, nil
}

func lookupScript(input string) (string, *unicode.RangeTable, bool) {
	want := normalizeScriptName(input)
	for name, table := range unicode.Scripts {
		if normalizeScriptName(name) == want {
			return name, table, true
		}
	}
	return "", nil, false
}

func normalizeScriptName(input string) string {
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "-", "_")
	input = strings.ReplaceAll(input, " ", "_")
	return strings.ToLower(input)
}

func NormalizeText(input string) string {
	return norm.NFKC.String(html.UnescapeString(input))
}

func FilterSelectedScripts(input string, scripts []Script) string {
	var b strings.Builder
	for _, r := range NormalizeText(input) {
		if !unicode.IsLetter(r) {
			continue
		}
		for _, script := range scripts {
			if unicode.Is(script.Table, r) {
				b.WriteRune(r)
				break
			}
		}
	}
	return b.String()
}

func ScriptsPresent(input string, scripts []Script) []string {
	seen := map[string]struct{}{}
	for _, r := range NormalizeText(input) {
		if !unicode.IsLetter(r) {
			continue
		}
		for _, script := range scripts {
			if unicode.Is(script.Table, r) {
				seen[script.Name] = struct{}{}
				break
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func AutoSelectScripts(sourceText, targetText string) ([]Script, []ScriptStat, error) {
	sourceCounts, sourceTotal := countScripts(sourceText)
	targetCounts, targetTotal := countScripts(targetText)
	names := SupportedScripts()
	stats := make([]ScriptStat, 0, len(names))
	var selected []Script
	for _, name := range names {
		if name == "Common" || name == "Inherited" {
			continue
		}
		sourceCount := sourceCounts[name]
		targetCount := targetCounts[name]
		sourceShare := share(sourceCount, sourceTotal)
		targetShare := share(targetCount, targetTotal)
		isSelected := targetCount > 0 &&
			sourceCount > 0 &&
			sourceCount >= 5 &&
			targetShare <= 0.02 &&
			sourceShare >= targetShare*5
		stats = append(stats, ScriptStat{
			Name:        name,
			SourceCount: sourceCount,
			TargetCount: targetCount,
			SourceShare: sourceShare,
			TargetShare: targetShare,
			Selected:    isSelected,
		})
		if isSelected {
			table := unicode.Scripts[name]
			selected = append(selected, Script{Name: name, Table: table})
		}
	}
	if len(selected) == 0 {
		return nil, stats, nil
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	return selected, stats, nil
}

func countScripts(input string) (map[string]int, int) {
	counts := map[string]int{}
	total := 0
	for _, r := range NormalizeText(input) {
		if !unicode.IsLetter(r) {
			continue
		}
		for name, table := range unicode.Scripts {
			if name == "Common" || name == "Inherited" {
				continue
			}
			if unicode.Is(table, r) {
				counts[name]++
				total++
				break
			}
		}
	}
	return counts, total
}

func share(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total)
}
