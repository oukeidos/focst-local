package residue

import (
	"context"
	"fmt"
	"testing"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type fakeRepairClient struct {
	content string
	err     error
	opts    translation.TextCompletionOptions
}

func (f *fakeRepairClient) CompleteJSONWithOptions(_ context.Context, _, _ string, _ map[string]any, opts translation.TextCompletionOptions) (*translation.TextCompletion, error) {
	f.opts = opts
	if f.err != nil {
		return nil, f.err
	}
	return &translation.TextCompletion{
		Content: f.content,
		Usage: translation.UsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
			TotalTokenCount:      15,
		},
	}, nil
}

func TestRepairAcceptsNarrowResidueFix(t *testing.T) {
	source := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"今日は になって 話した"}},
	}
	target := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"오늘 になって 말했다"}},
	}
	artifact, err := Detect(source, target, DetectOptions{
		SourceLanguage: language.Languages["ja"],
		TargetLanguage: language.Languages["ko"],
		ScriptSpec:     "hiragana",
	})
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	client := &fakeRepairClient{content: `{"id":1,"source_text":"今日は になって 話した","text":"오늘이 되어서 말했다"}`}
	result, err := Repair(context.Background(), client, source, target, artifact, RepairOptions{
		SourceLanguage: language.Languages["ja"],
		TargetLanguage: language.Languages["ko"],
	})
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected one record, got %d", len(result.Records))
	}
	if result.Records[0].Status != "accepted" {
		t.Fatalf("expected accepted, got %#v", result.Records[0])
	}
	if got := result.Segments[0].Lines[0]; got != "오늘이 되어서 말했다" {
		t.Fatalf("unexpected repaired text: %q", got)
	}
	if client.opts.Temperature == nil || *client.opts.Temperature != 0.0 {
		t.Fatalf("expected temperature 0.0, got %#v", client.opts.Temperature)
	}
	if client.opts.MaxTokens != DefaultRepairMaxTokens {
		t.Fatalf("expected max tokens %d, got %d", DefaultRepairMaxTokens, client.opts.MaxTokens)
	}
}

func TestRepairRejectsProtectedRenderingLoss(t *testing.T) {
	source := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"アリ が になって と言った"}},
	}
	target := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"아리 가 になって 말했다"}},
	}
	artifact, err := Detect(source, target, DetectOptions{
		SourceLanguage: language.Languages["ja"],
		TargetLanguage: language.Languages["ko"],
		ScriptSpec:     "hiragana",
	})
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	client := &fakeRepairClient{content: `{"id":1,"source_text":"アリ が になって と言った","text":"그가 되어서 말했다"}`}
	result, err := Repair(context.Background(), client, source, target, artifact, RepairOptions{
		SourceLanguage:      language.Languages["ja"],
		TargetLanguage:      language.Languages["ko"],
		ProtectedRenderings: map[string]string{"アリ": "아리"},
	})
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected one record, got %d", len(result.Records))
	}
	record := result.Records[0]
	if record.Status != "rejected" || record.Reason != "protected_rendering_removed" {
		t.Fatalf("expected protected rejection, got %#v", record)
	}
	if got := result.Segments[0].Lines[0]; got != "아리 가 になって 말했다" {
		t.Fatalf("expected original text to remain, got %q", got)
	}
}

func TestRepairRecordsRequestError(t *testing.T) {
	source := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"今日は になって 話した"}},
	}
	target := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"오늘 になって 말했다"}},
	}
	artifact, err := Detect(source, target, DetectOptions{
		SourceLanguage: language.Languages["ja"],
		TargetLanguage: language.Languages["ko"],
		ScriptSpec:     "hiragana",
	})
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	result, err := Repair(context.Background(), &fakeRepairClient{err: fmt.Errorf("temporary")}, source, target, artifact, RepairOptions{
		SourceLanguage: language.Languages["ja"],
		TargetLanguage: language.Languages["ko"],
	})
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}
	if len(result.Records) != 1 || result.Records[0].Status != "error" {
		t.Fatalf("expected error record, got %#v", result.Records)
	}
}
