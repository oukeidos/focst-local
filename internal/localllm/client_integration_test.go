package localllm_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translator"
)

func TestIntegrationL5SChunkWithLocalLlama(t *testing.T) {
	baseURL := os.Getenv("FOCST_LOCAL_LLM_BASE_URL")
	model := os.Getenv("FOCST_LOCAL_LLM_MODEL")
	samplePath := os.Getenv("FOCST_LOCAL_SAMPLE_SRT")
	if baseURL == "" || model == "" || samplePath == "" {
		t.Skip("set FOCST_LOCAL_LLM_BASE_URL, FOCST_LOCAL_LLM_MODEL, and FOCST_LOCAL_SAMPLE_SRT to run")
	}

	segments, err := srt.Load(samplePath)
	if err != nil {
		t.Fatalf("failed to load sample SRT: %v", err)
	}

	client := localllm.NewClient(baseURL, model)
	src, _ := language.GetLanguage("en")
	tgt, _ := language.GetLanguage("ko")
	tr, err := translator.NewTranslator(client, 20, 3, 1, src, tgt)
	if err != nil {
		t.Fatalf("NewTranslator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	translated, failed, err := tr.TranslateChunks(ctx, segments, []int{1}, nil)
	if err != nil {
		t.Fatalf("TranslateChunks failed: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("expected no failed chunks, got %v", failed)
	}

	for id := 21; id <= 40; id++ {
		seg := translated[id-1]
		if seg.ID != id {
			t.Fatalf("segment position %d has ID %d", id, seg.ID)
		}
		text := strings.Join(seg.Lines, " ")
		if strings.TrimSpace(text) == "" {
			t.Fatalf("segment %d translated to empty text", id)
		}
		if strings.TrimSpace(text) == strings.Join(segments[id-1].Lines, " ") {
			t.Fatalf("segment %d was not translated: %q", id, text)
		}
		t.Logf("%d\t%s", id, text)
	}
}
