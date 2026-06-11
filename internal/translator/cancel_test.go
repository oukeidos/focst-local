package translator

import (
	"context"
	"testing"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

type blockingClient struct {
	started chan struct{}
}

func (c *blockingClient) Translate(ctx context.Context, request translation.RequestData) (*translation.ResponseData, error) {
	select {
	case <-c.started:
	default:
		close(c.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c *blockingClient) SetSystemInstruction(prompt string) {}

func TestTranslateSRT_CancelPartialSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go func() {
		<-started
		cancel()
	}()

	client := &blockingClient{started: started}
	src, _ := language.GetLanguage("en")
	tgt, _ := language.GetLanguage("ko")
	tr, err := NewTranslator(client, 1, 0, 1, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}

	segments := []srt.Segment{
		{ID: 1, Lines: []string{"hello"}},
		{ID: 2, Lines: []string{"world"}},
	}

	translated, failed, err := tr.TranslateSRT(ctx, segments, nil)
	if err != nil {
		t.Fatalf("TranslateSRT failed: %v", err)
	}
	if len(translated) != len(segments) {
		t.Fatalf("expected %d segments, got %d", len(segments), len(translated))
	}
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed chunks, got %d", len(failed))
	}
}
