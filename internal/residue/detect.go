package residue

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

func DetectFiles(opts DetectOptions) (Artifact, []srt.Segment, []srt.Segment, error) {
	sourceSegments, err := srt.Load(opts.SourcePath)
	if err != nil {
		return Artifact{}, nil, nil, fmt.Errorf("failed to load source subtitle file: %w", err)
	}
	if err := srt.Validate(sourceSegments); err != nil {
		return Artifact{}, nil, nil, fmt.Errorf("invalid source subtitle file: %w", err)
	}
	if !opts.NoPreprocess {
		sourceSegments, _ = srt.PreprocessForPathWithMappingOptions(sourceSegments, opts.SourceLanguage.Code, opts.SourcePath, !opts.NoLangPreprocess)
	}
	translatedSegments, err := srt.Load(opts.TranslatedPath)
	if err != nil {
		return Artifact{}, nil, nil, fmt.Errorf("failed to load translated subtitle file: %w", err)
	}
	if err := srt.Validate(translatedSegments); err != nil {
		return Artifact{}, nil, nil, fmt.Errorf("invalid translated subtitle file: %w", err)
	}
	artifact, err := Detect(sourceSegments, translatedSegments, opts)
	return artifact, sourceSegments, translatedSegments, err
}

func Detect(sourceSegments, translatedSegments []srt.Segment, opts DetectOptions) (Artifact, error) {
	if err := validateAligned(sourceSegments, translatedSegments); err != nil {
		return Artifact{}, err
	}
	sourceText := allSegmentText(sourceSegments)
	targetText := allSegmentText(translatedSegments)
	scripts, auto, err := ParseScriptList(opts.ScriptSpec)
	if err != nil {
		return Artifact{}, err
	}
	var stats []ScriptStat
	if auto {
		scripts, stats, err = AutoSelectScripts(sourceText, targetText)
		if err != nil {
			return Artifact{}, err
		}
	} else {
		stats = explicitScriptStats(sourceText, targetText, scripts)
	}
	selected := scriptNames(scripts)
	artifact := Artifact{
		Version:        ArtifactVersion,
		PromptVersion:  PromptVersion,
		CreatedAt:      time.Now().UTC(),
		SourceLanguage: opts.SourceLanguage.Code,
		TargetLanguage: opts.TargetLanguage.Code,
		Input: InputInfo{
			SourcePath:                     opts.SourcePath,
			TranslatedPath:                 opts.TranslatedPath,
			PreprocessedSourceSegmentCount: len(sourceSegments),
			TranslatedSegmentCount:         len(translatedSegments),
			SourceSegmentsChecksum:         srt.SegmentsChecksumHex(sourceSegments),
			TranslatedSegmentsChecksum:     srt.SegmentsChecksumHex(translatedSegments),
		},
		Config: RunConfig{
			ScriptSpec:      opts.ScriptSpec,
			SelectedScripts: selected,
		},
		ScriptStats: stats,
	}
	if len(scripts) == 0 {
		return artifact, nil
	}
	for i, source := range sourceSegments {
		target := translatedSegments[i]
		sourceText := translation.SourceTextFromLines(source.Lines)
		targetText := translation.SourceTextFromLines(target.Lines)
		filteredSource := FilterSelectedScripts(sourceText, scripts)
		filteredTarget := FilterSelectedScripts(targetText, scripts)
		if filteredTarget == "" || filteredSource == "" {
			continue
		}
		if !strings.Contains(filteredSource, filteredTarget) {
			continue
		}
		artifact.Candidates = append(artifact.Candidates, Candidate{
			ID:                 source.ID,
			StartTime:          source.StartTime,
			EndTime:            source.EndTime,
			SourceText:         sourceText,
			CurrentText:        targetText,
			Scripts:            ScriptsPresent(targetText, scripts),
			FilteredSourceText: filteredSource,
			FilteredTargetText: filteredTarget,
			Residues:           []string{filteredTarget},
		})
	}
	return artifact, nil
}

func validateAligned(sourceSegments, translatedSegments []srt.Segment) error {
	if len(sourceSegments) != len(translatedSegments) {
		return fmt.Errorf("source and translated segment counts differ: source=%d translated=%d", len(sourceSegments), len(translatedSegments))
	}
	for i := range sourceSegments {
		if sourceSegments[i].ID != translatedSegments[i].ID {
			return fmt.Errorf("source and translated segment IDs differ at index %d: source=%d translated=%d", i, sourceSegments[i].ID, translatedSegments[i].ID)
		}
	}
	return nil
}

func allSegmentText(segments []srt.Segment) string {
	var b strings.Builder
	for _, segment := range segments {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(translation.SourceTextFromLines(segment.Lines))
	}
	return b.String()
}

func explicitScriptStats(sourceText, targetText string, scripts []Script) []ScriptStat {
	sourceCounts, sourceTotal := countScripts(sourceText)
	targetCounts, targetTotal := countScripts(targetText)
	stats := make([]ScriptStat, 0, len(scripts))
	for _, script := range scripts {
		sourceCount := sourceCounts[script.Name]
		targetCount := targetCounts[script.Name]
		stats = append(stats, ScriptStat{
			Name:        script.Name,
			SourceCount: sourceCount,
			TargetCount: targetCount,
			SourceShare: share(sourceCount, sourceTotal),
			TargetShare: share(targetCount, targetTotal),
			Selected:    true,
		})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Name < stats[j].Name })
	return stats
}

func scriptNames(scripts []Script) []string {
	names := make([]string, 0, len(scripts))
	for _, script := range scripts {
		names = append(names, script.Name)
	}
	sort.Strings(names)
	return names
}
