package translator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type cancelMockClient struct {
	translation.Translator
	callCount int32
}

func (m *cancelMockClient) SetSystemInstruction(prompt string) {}

func (m *cancelMockClient) Translate(ctx context.Context, req translation.RequestData) (*translation.ResponseData, error) {
	atomic.AddInt32(&m.callCount, 1)
	// Simulate some work
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	translations := make([]translation.TranslatedSegment, len(req.Target))
	for i, s := range req.Target {
		translations[i] = translation.TranslatedSegment{ID: s.ID, Text: "translated"}
	}
	return &translation.ResponseData{Translations: translations}, nil
}

func TestTranslator_Cancellation(t *testing.T) {
	oldQPS := defaultQPS
	oldRamp := defaultRampUp
	defaultQPS = 1000
	defaultRampUp = 0
	defer func() {
		defaultQPS = oldQPS
		defaultRampUp = oldRamp
	}()

	mock := &cancelMockClient{}
	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	tr, _ := NewTranslator(mock, 1, 0, 5, src, tgt)

	// Create 20 segments (20 chunks since chunkSize=1)
	segments := make([]srt.Segment, 20)
	for i := range segments {
		segments[i] = srt.Segment{ID: i + 1, Lines: []string{"test"}}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 250ms (should have started some chunks but not all)
	go func() {
		time.Sleep(250 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _, _ = tr.TranslateSRT(ctx, segments, nil)
	duration := time.Since(start)

	// Should not have finished all 20 chunks
	finalCalls := atomic.LoadInt32(&mock.callCount)
	if finalCalls >= 20 {
		t.Errorf("Expected fewer than 20 calls due to cancellation, got %d", finalCalls)
	}

	// Should have finished much faster than 20 * 100ms / 5 concurrency = 400ms
	// but significantly, it should return shortly after 250ms
	if duration > 600*time.Millisecond {
		t.Errorf("TranslateSRT took too long to return after cancellation: %v", duration)
	}
}
