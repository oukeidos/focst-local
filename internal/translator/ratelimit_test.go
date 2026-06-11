package translator

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type timeMockClient struct {
	translation.Translator
	mu    sync.Mutex
	times []time.Time
	sleep time.Duration
}

func (m *timeMockClient) SetSystemInstruction(prompt string) {}

func (m *timeMockClient) Translate(ctx context.Context, req translation.RequestData) (*translation.ResponseData, error) {
	if m.sleep > 0 {
		time.Sleep(m.sleep)
	}
	m.mu.Lock()
	m.times = append(m.times, time.Now())
	m.mu.Unlock()

	translations := make([]translation.TranslatedSegment, len(req.Target))
	for i, s := range req.Target {
		translations[i] = translation.TranslatedSegment{ID: s.ID, SourceText: s.SourceText, Text: "translated"}
	}
	return &translation.ResponseData{Translations: translations}, nil
}

func TestTranslator_RateLimiter(t *testing.T) {
	oldQPS := defaultQPS
	oldRamp := defaultRampUp
	defaultQPS = 2
	defaultRampUp = 0
	defer func() {
		defaultQPS = oldQPS
		defaultRampUp = oldRamp
	}()

	client := &timeMockClient{}
	src, _ := language.GetLanguage("en")
	tgt, _ := language.GetLanguage("ko")
	tr, err := NewTranslator(client, 1, 0, 3, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}

	segments := []srt.Segment{
		{ID: 1, Lines: []string{"a"}},
		{ID: 2, Lines: []string{"b"}},
		{ID: 3, Lines: []string{"c"}},
	}

	_, _, err = tr.TranslateSRT(context.Background(), segments, nil)
	if err != nil {
		t.Fatalf("TranslateSRT failed: %v", err)
	}

	client.mu.Lock()
	times := append([]time.Time(nil), client.times...)
	client.mu.Unlock()
	if len(times) < 3 {
		t.Fatalf("expected 3 requests, got %d", len(times))
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	minDelta := times[1].Sub(times[0])
	if d := times[2].Sub(times[1]); d < minDelta {
		minDelta = d
	}
	if minDelta < 300*time.Millisecond {
		t.Fatalf("rate limiter too fast: min delta %v", minDelta)
	}
}

func TestTranslator_RampUp(t *testing.T) {
	oldQPS := defaultQPS
	oldRamp := defaultRampUp
	defaultQPS = 0
	defaultRampUp = 300 * time.Millisecond
	defer func() {
		defaultQPS = oldQPS
		defaultRampUp = oldRamp
	}()

	client := &timeMockClient{sleep: 400 * time.Millisecond}
	src, _ := language.GetLanguage("en")
	tgt, _ := language.GetLanguage("ko")
	tr, err := NewTranslator(client, 1, 0, 3, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}

	segments := []srt.Segment{
		{ID: 1, Lines: []string{"a"}},
		{ID: 2, Lines: []string{"b"}},
		{ID: 3, Lines: []string{"c"}},
	}

	_, _, err = tr.TranslateSRT(context.Background(), segments, nil)
	if err != nil {
		t.Fatalf("TranslateSRT failed: %v", err)
	}

	client.mu.Lock()
	times := append([]time.Time(nil), client.times...)
	client.mu.Unlock()
	if len(times) < 3 {
		t.Fatalf("expected 3 requests, got %d", len(times))
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	if times[1].Sub(times[0]) < 100*time.Millisecond {
		t.Fatalf("ramp-up not applied: delta %v", times[1].Sub(times[0]))
	}
	if times[2].Sub(times[1]) < 100*time.Millisecond {
		t.Fatalf("ramp-up not applied: delta %v", times[2].Sub(times[1]))
	}
}
