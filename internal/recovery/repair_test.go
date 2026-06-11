package recovery

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/translation"
	"github.com/oukeidos/focst-local/internal/translator"
)

type mockClient struct{}

func (m *mockClient) Translate(ctx context.Context, req translation.RequestData) (*translation.ResponseData, error) {
	// Return a response that matches the target segments
	resp := &translation.ResponseData{
		Translations: make([]translation.TranslatedSegment, len(req.Target)),
	}
	for i, seg := range req.Target {
		resp.Translations[i] = translation.TranslatedSegment{
			ID:   seg.ID,
			Text: "번역됨: " + strings.Join(seg.Lines, " "),
		}
	}
	return resp, nil
}

func (m *mockClient) SetSystemInstruction(prompt string) {}

func TestRepairToggles(t *testing.T) {
	// 1. Create a temporary SRT file
	srtContent := `1
00:00:01,000 --> 00:00:02,000
(Note) [Action] Hello
`
	tmpSRT, err := os.CreateTemp("", "test_*.srt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpSRT.Name())
	tmpSRT.WriteString(srtContent)
	tmpSRT.Close()

	tmpOut, err := os.CreateTemp("", "test_out_*.srt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpOut.Name())
	tmpOut.Close()

	ctx := context.Background()
	mockG := &mockClient{}
	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	tr, err := translator.NewTranslator(mockG, 10, 5, 1, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}

	t.Run("Repair with Preprocessing (Default)", func(t *testing.T) {
		log := &SessionLog{
			LogVersion:   CurrentLogVersion,
			InputPath:    tmpSRT.Name(),
			OutputPath:   tmpOut.Name(),
			NoPreprocess: false,
			SourceLang:   "ja",
			TargetLang:   "ko",
			FailedChunks: []int{0},
			ChunkSize:    10,
		}

		results, _, err := Repair(ctx, tr, log, tmpOut.Name(), true, nil)
		if err != nil {
			t.Fatalf("Repair failed: %v", err)
		}

		// Preprocessing should remove "(Note) [Action] "
		if len(results) > 0 {
			line := results[0].Lines[0]
			// The Repair logic loads input, preprocesses it, and THEN translates.
			// In our mock, it will translate the preprocessed text.
			// srt.Preprocess removes brackets.
			expected := "번역됨: Hello"
			if line != expected {
				t.Errorf("Expected '%s' (preprocessed), got '%s'", expected, line)
			}
		}
	})

	t.Run("Repair without Preprocessing", func(t *testing.T) {
		log := &SessionLog{
			LogVersion:   CurrentLogVersion,
			InputPath:    tmpSRT.Name(),
			OutputPath:   tmpOut.Name(),
			NoPreprocess: true,
			SourceLang:   "ja",
			TargetLang:   "ko",
			FailedChunks: []int{0},
			ChunkSize:    10,
		}

		results, _, err := Repair(ctx, tr, log, tmpOut.Name(), true, nil)
		if err != nil {
			t.Fatalf("Repair failed: %v", err)
		}

		// Preprocessing skipped, text should remain
		if len(results) > 0 {
			line := results[0].Lines[0]
			expected := "번역됨: (Note) [Action] Hello"
			if line != expected {
				t.Errorf("Expected '%s' (raw), got '%s'", expected, line)
			}
		}
	})

	t.Run("Repair without force rejects unusable output", func(t *testing.T) {
		log := &SessionLog{
			LogVersion:   CurrentLogVersion,
			InputPath:    tmpSRT.Name(),
			OutputPath:   tmpOut.Name(),
			NoPreprocess: true,
			SourceLang:   "ja",
			TargetLang:   "ko",
			FailedChunks: []int{0},
			ChunkSize:    10,
		}

		_, _, err := Repair(ctx, tr, log, tmpOut.Name(), false, nil)
		if err == nil || !strings.Contains(err.Error(), "existing output could not be reused") {
			t.Fatalf("expected output reuse error, got: %v", err)
		}
	})
}

func TestRepairUsesSavedVariableChunkPlanWhenMerging(t *testing.T) {
	input := `1
00:00:01,000 --> 00:00:02,000
one

2
00:00:02,000 --> 00:00:03,000
two

3
00:00:03,000 --> 00:00:04,000
three

4
00:00:04,000 --> 00:00:05,000
four

5
00:00:05,000 --> 00:00:06,000
five
`
	output := `1
00:00:01,000 --> 00:00:02,000
old one

2
00:00:02,000 --> 00:00:03,000
old two

3
00:00:03,000 --> 00:00:04,000
old three

4
00:00:04,000 --> 00:00:05,000
old four

5
00:00:05,000 --> 00:00:06,000
old five
`
	tmpIn, err := os.CreateTemp("", "repair_plan_input_*.srt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpIn.Name())
	if _, err := tmpIn.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := tmpIn.Close(); err != nil {
		t.Fatal(err)
	}

	tmpOut, err := os.CreateTemp("", "repair_plan_output_*.srt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpOut.Name())
	if _, err := tmpOut.WriteString(output); err != nil {
		t.Fatal(err)
	}
	if err := tmpOut.Close(); err != nil {
		t.Fatal(err)
	}

	src, _ := language.GetLanguage("en")
	tgt, _ := language.GetLanguage("ko")
	tr, err := translator.NewTranslator(&mockClient{}, 2, 0, 1, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}
	plan := chunker.ChunkPlan{Chunks: []chunker.PlannedChunk{
		{Index: 0, StartIndex: 0, EndIndex: 3, StartID: 1, EndID: 3},
		{Index: 1, StartIndex: 3, EndIndex: 5, StartID: 4, EndID: 5},
	}}
	tr.SetChunkPlan(plan)

	log := &SessionLog{
		LogVersion:   CurrentLogVersion,
		InputPath:    tmpIn.Name(),
		OutputPath:   tmpOut.Name(),
		NoPreprocess: true,
		SourceLang:   "en",
		TargetLang:   "ko",
		FailedChunks: []int{1},
		ChunkSize:    2,
		TotalChunks:  2,
		ChunkPlan:    &plan,
	}

	results, failed, err := Repair(context.Background(), tr, log, tmpOut.Name(), false, nil)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("failed chunks = %v, want none", failed)
	}
	if got := results[2].Lines[0]; got != "old three" {
		t.Fatalf("segment 3 should remain from existing output, got %q", got)
	}
	if got := results[3].Lines[0]; got != "번역됨: four" {
		t.Fatalf("segment 4 was not repaired using saved plan, got %q", got)
	}
	if got := results[4].Lines[0]; got != "번역됨: five" {
		t.Fatalf("segment 5 was not repaired using saved plan, got %q", got)
	}
}
