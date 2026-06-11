package translator

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type slowMockClient struct {
	translation.Translator
}

func (m *slowMockClient) SetSystemInstruction(prompt string) {}

func (m *slowMockClient) Translate(ctx context.Context, req translation.RequestData) (*translation.ResponseData, error) {
	// Simulate slow API call
	time.Sleep(100 * time.Millisecond)

	translations := make([]translation.TranslatedSegment, len(req.Target))
	for i, s := range req.Target {
		translations[i] = translation.TranslatedSegment{
			ID:   s.ID,
			Text: "translated",
		}
	}

	return &translation.ResponseData{
		Translations: translations,
		Usage:        translation.UsageMetadata{},
	}, nil
}

func TestTranslator_GoroutineLimit(t *testing.T) {
	concurrency := 5
	chunkCount := 100

	client := &slowMockClient{}
	tr, err := NewTranslator(client, 1, 0, concurrency, language.Languages["en"], language.Languages["ko"])
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}

	segments := make([]srt.Segment, chunkCount)
	for i := 0; i < chunkCount; i++ {
		segments[i] = srt.Segment{ID: i + 1, Lines: []string{"test"}}
	}

	initialGoroutines := runtime.NumGoroutine()

	// Run translation in background to check goroutine count during execution
	errChan := make(chan error, 1)
	go func() {
		_, _, err := tr.TranslateSRT(context.Background(), segments, nil)
		errChan <- err
	}()

	// Wait a bit for goroutines to ramp up
	time.Sleep(500 * time.Millisecond)

	currentGoroutines := runtime.NumGoroutine()
	// Total goroutines should be roughly initial + worker count + a few extra for the test runner/TranslateSRT caller
	// Before the fix, it would have been initial + chunkCount
	if currentGoroutines > initialGoroutines+concurrency+10 {
		t.Errorf("Too many goroutines: got %d, initial was %d, concurrency is %d", currentGoroutines, initialGoroutines, concurrency)
	}

	err = <-errChan
	if err != nil {
		t.Errorf("TranslateSRT failed: %v", err)
	}
}
