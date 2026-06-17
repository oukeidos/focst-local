package srt

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreprocess(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		expected []Segment
	}{
		{
			name: "Remove brackets",
			segments: []Segment{
				{
					ID:    1,
					Lines: []string{"Hello (world)", "[Action] Good morning"},
				},
			},
			expected: []Segment{
				{
					ID:    1,
					Lines: []string{"Hello", "Good morning"},
				},
			},
		},
		{
			name: "Remove brackets leaving empty line",
			segments: []Segment{
				{ID: 1, Lines: []string{"(Action)"}},
				{ID: 2, Lines: []string{"Hello"}},
			},
			expected: []Segment{
				{ID: 1, Lines: []string{"Hello"}},
			},
		},
		{
			name: "Filter meaningless segments",
			segments: []Segment{
				{ID: 1, Lines: []string{"Hello"}},
				{ID: 2, Lines: []string{"!!!"}},
				{ID: 3, Lines: []string{"(Action only)"}},
				{ID: 4, Lines: []string{"...?"}},
				{ID: 5, Lines: []string{"World 123"}},
			},
			expected: []Segment{
				{ID: 1, Lines: []string{"Hello"}},
				{ID: 2, Lines: []string{"World 123"}},
			},
		},
		{
			name: "Re-indexing after removal",
			segments: []Segment{
				{ID: 10, Lines: []string{"First"}},
				{ID: 11, Lines: []string{"???"}},
				{ID: 12, Lines: []string{"Second"}},
			},
			expected: []Segment{
				{ID: 1, Lines: []string{"First"}},
				{ID: 2, Lines: []string{"Second"}},
			},
		},
		{
			name: "Remove angle brackets only",
			segments: []Segment{
				{ID: 1, Lines: []string{"<Hello>", "Normal text", "Left<Right"}},
			},
			expected: []Segment{
				{ID: 1, Lines: []string{"Hello", "Normal text", "LeftRight"}},
			},
		},
		{
			name: "Remove fullwidth parentheses and brackets",
			segments: []Segment{
				{ID: 1, Lines: []string{"Hello（world）", "［Action］ Good morning"}},
				{ID: 2, Lines: []string{"（SFX）"}},
				{ID: 3, Lines: []string{"Bye"}},
			},
			expected: []Segment{
				{ID: 1, Lines: []string{"Hello", "Good morning"}},
				{ID: 2, Lines: []string{"Bye"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := Preprocess(tt.segments, "ja")
			if !reflect.DeepEqual(cleaned, tt.expected) {
				t.Errorf("Preprocess() = %+v, want %+v", cleaned, tt.expected)
			}
		})
	}
}

func TestPreprocess_FullwidthBracketsNotAppliedForNonJapanese(t *testing.T) {
	segments := []Segment{
		{ID: 1, Lines: []string{"Hello（world）", "［Action］ Good morning"}},
	}

	cleaned := Preprocess(segments, "en")
	expected := []Segment{
		{ID: 1, Lines: []string{"Hello（world）", "［Action］ Good morning"}},
	}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Errorf("Preprocess() = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessWithMapping(t *testing.T) {
	segments := []Segment{
		{ID: 1, Lines: []string{"(Note) Hello"}},
		{ID: 2, Lines: []string{"World"}},
		{ID: 3, Lines: []string{"[Action]"}},
		{ID: 4, Lines: []string{"Bye"}},
	}

	cleaned, mapping := PreprocessWithMapping(segments, "ja")
	if len(cleaned) != 3 {
		t.Fatalf("expected 3 cleaned segments, got %d", len(cleaned))
	}
	if len(mapping) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(mapping))
	}
	if mapping[0].InternalID != 1 || mapping[0].OriginalID != 1 {
		t.Fatalf("unexpected mapping[0]: %+v", mapping[0])
	}
	if mapping[1].InternalID != 2 || mapping[1].OriginalID != 2 {
		t.Fatalf("unexpected mapping[1]: %+v", mapping[1])
	}
	if mapping[2].InternalID != 3 || mapping[2].OriginalID != 4 {
		t.Fatalf("unexpected mapping[2]: %+v", mapping[2])
	}
}

func TestPreprocessForPathWithMappingOptions_MergeConsecutiveEqualTimestampsInVTT(t *testing.T) {
	segments := []Segment{
		{
			ID:        10,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"first"},
		},
		{
			ID:        11,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"second"},
		},
		{
			ID:        12,
			StartTime: "00:00:04,000",
			EndTime:   "00:00:05,000",
			Lines:     []string{"third"},
		},
	}

	cleaned, mapping := PreprocessForPathWithMappingOptions(segments, "en", "sample.vtt", false)
	expected := []Segment{
		{
			ID:        1,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"first", "second"},
		},
		{
			ID:        2,
			StartTime: "00:00:04,000",
			EndTime:   "00:00:05,000",
			Lines:     []string{"third"},
		},
	}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}

	if len(mapping) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mapping))
	}
	if mapping[0].InternalID != 1 || mapping[0].OriginalID != 10 {
		t.Fatalf("unexpected mapping[0]: %+v", mapping[0])
	}
	if mapping[1].InternalID != 2 || mapping[1].OriginalID != 12 {
		t.Fatalf("unexpected mapping[1]: %+v", mapping[1])
	}
}

func TestPreprocessForPathWithMappingOptions_NoMergeForNonVTT(t *testing.T) {
	segments := []Segment{
		{
			ID:        1,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"first"},
		},
		{
			ID:        2,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"second"},
		},
	}

	cleaned, _ := PreprocessForPathWithMappingOptions(segments, "en", "sample.srt", false)
	expected := []Segment{
		{
			ID:        1,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"first"},
		},
		{
			ID:        2,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"second"},
		},
	}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessForPathWithMappingOptions_NoMergeForSeparatedGroups(t *testing.T) {
	segments := []Segment{
		{
			ID:        1,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"first"},
		},
		{
			ID:        2,
			StartTime: "00:00:04,000",
			EndTime:   "00:00:05,000",
			Lines:     []string{"middle"},
		},
		{
			ID:        3,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"last"},
		},
	}

	cleaned, _ := PreprocessForPathWithMappingOptions(segments, "en", "sample.vtt", false)
	expected := []Segment{
		{
			ID:        1,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"first"},
		},
		{
			ID:        2,
			StartTime: "00:00:04,000",
			EndTime:   "00:00:05,000",
			Lines:     []string{"middle"},
		},
		{
			ID:        3,
			StartTime: "00:00:01,000",
			EndTime:   "00:00:03,000",
			Lines:     []string{"last"},
		},
	}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessForPathWithMappingOptions_NormalizesVTTEntitiesBeforeJapaneseAngleCleanup(t *testing.T) {
	segments := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物&gt;"}}}

	cleaned, _ := PreprocessForPathWithMappingOptions(segments, "ja", "sample.vtt", true)
	expected := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物"}}}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessForPathWithMappingOptions_NormalizesSRTEntitiesBeforeJapaneseAngleCleanup(t *testing.T) {
	segments := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物&gt;"}}}

	cleaned, _ := PreprocessForPathWithMappingOptions(segments, "ja", "sample.srt", true)
	expected := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物"}}}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessForPathWithMappingOptions_DoesNotNormalizeEntitiesForOtherFormats(t *testing.T) {
	segments := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物&gt;"}}}

	cleaned, _ := PreprocessForPathWithMappingOptions(segments, "ja", "sample.ass", true)
	expected := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物&gt;"}}}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessWithMappingOptions_DoesNotNormalizeEntitiesWithoutSourcePath(t *testing.T) {
	segments := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物&gt;"}}}

	cleaned, _ := PreprocessWithMappingOptions(segments, "ja", true)
	expected := []Segment{{ID: 1, Lines: []string{"仕事に集中する人物&gt;"}}}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessForPathWithMappingOptions_NormalizesStandardAmpersandEntityOnce(t *testing.T) {
	segments := []Segment{{ID: 1, Lines: []string{"A &amp; B"}}}

	cleaned, _ := PreprocessForPathWithMappingOptions(segments, "en", "sample.srt", false)
	expected := []Segment{{ID: 1, Lines: []string{"A & B"}}}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestPreprocessForPathWithMappingOptions_DocumentsSinglePassDoubleEscapedEntity(t *testing.T) {
	segments := []Segment{{ID: 1, Lines: []string{"A &amp;gt; B"}}}

	cleaned, _ := PreprocessForPathWithMappingOptions(segments, "en", "sample.vtt", false)
	expected := []Segment{{ID: 1, Lines: []string{"A &gt; B"}}}
	if !reflect.DeepEqual(cleaned, expected) {
		t.Fatalf("cleaned = %+v, want %+v", cleaned, expected)
	}
}

func TestLoadThenPreprocessVTTDoesNotExposeEntityResidue(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "synthetic.vtt")
	content := "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n合成人物&gt;\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write synthetic vtt: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	cleaned, _ := PreprocessForPathWithMappingOptions(loaded, "ja", path, true)
	if len(cleaned) != 1 {
		t.Fatalf("cleaned len = %d, want 1: %+v", len(cleaned), cleaned)
	}
	if strings.Contains(cleaned[0].Lines[0], "&gt;") {
		t.Fatalf("entity residue survived preprocessing: %+v", cleaned)
	}
	if cleaned[0].Lines[0] != "合成人物" {
		t.Fatalf("cleaned line = %q, want 合成人物", cleaned[0].Lines[0])
	}
}
