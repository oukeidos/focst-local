package localllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/httpclient"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

func TestClient_TranslationTimeoutDefaultsToUnlimited(t *testing.T) {
	client := NewClient("http://127.0.0.1:8080/v1", "test-model")
	if client.translationClient == nil {
		t.Fatal("translation client is nil")
	}
	if client.translationClient.Timeout != 0 {
		t.Fatalf("translation timeout = %s, want unlimited", client.translationClient.Timeout)
	}
}

func TestClient_SetTranslationTimeout(t *testing.T) {
	client := NewClient("http://127.0.0.1:8080/v1", "test-model")
	client.SetTranslationTimeout(30 * time.Minute)
	if client.translationClient.Timeout != 30*time.Minute {
		t.Fatalf("translation timeout = %s, want 30m", client.translationClient.Timeout)
	}

	client.SetTranslationTimeout(0)
	if client.translationClient.Timeout != 0 {
		t.Fatalf("translation timeout = %s, want unlimited", client.translationClient.Timeout)
	}
}

func TestClient_TranslateUsesTranslationClientTimeout(t *testing.T) {
	restore := httpclient.SetDefaultClientForTesting(httpclient.NewClient(time.Nanosecond))
	defer restore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "{\"translations\":[{\"id\":1,\"source_text\":\"alpha\",\"text\":\"알파\"}]}"}}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", "test-model")
	client.SetTranslationTimeout(time.Second)
	_, err := client.Translate(context.Background(), translation.RequestData{
		Target: []translation.SegmentData{{ID: 1, SourceText: "alpha"}},
	})
	if err != nil {
		t.Fatalf("Translate should use translation timeout, got error: %v", err)
	}
}

func TestClient_CheckUsesDefaultClientTimeout(t *testing.T) {
	restore := httpclient.SetDefaultClientForTesting(httpclient.NewClient(time.Nanosecond))
	defer restore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", "test-model")
	client.SetTranslationTimeout(time.Second)
	if err := client.Check(context.Background()); err == nil {
		t.Fatalf("Check should use default client timeout")
	}
}

func TestClient_TranslateUsesSourceTextEchoSchema(t *testing.T) {
	var got chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
				"choices": [{"message": {"role": "assistant", "content": "{\"translations\":[{\"id\":7,\"source_text\":\"alpha beta\",\"text\":\"알파 베타\"}]}"}}],
			"usage": {"prompt_tokens": 17, "completion_tokens": 9, "total_tokens": 26}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", "test-model")
	resp, err := client.Translate(context.Background(), translation.RequestData{
		Target: []translation.SegmentData{
			{ID: 7, SourceText: "alpha beta"},
		},
	})
	if err != nil {
		t.Fatalf("Translate failed: %v", err)
	}
	if len(resp.Translations) != 1 {
		t.Fatalf("translations len = %d, want 1", len(resp.Translations))
	}
	if resp.Translations[0].SourceText != "alpha beta" {
		t.Fatalf("source text = %q", resp.Translations[0].SourceText)
	}

	props := got.ResponseFormat.Schema["properties"].(map[string]any)
	translations := props["translations"].(map[string]any)
	prefixItems := translations["prefixItems"].([]any)
	first := prefixItems[0].(map[string]any)
	required := first["required"].([]any)
	if !reflect.DeepEqual(required, []any{"id", "source_text", "text"}) {
		t.Fatalf("required = %+v", required)
	}
	firstProps := first["properties"].(map[string]any)
	sourceText := firstProps["source_text"].(map[string]any)
	if sourceText["const"] != "alpha beta" {
		t.Fatalf("source_text const schema = %+v", sourceText)
	}
}

func TestClient_PlanBoundary(t *testing.T) {
	var got chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "{\"split_after_id\":2}"}}],
			"usage": {"prompt_tokens": 11, "completion_tokens": 3, "total_tokens": 14}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", "test-model")
	decision, err := client.PlanBoundary(context.Background(), chunker.BoundaryRequest{
		Segments: []srt.Segment{
			{ID: 1, Lines: []string{"first"}},
			{ID: 2, Lines: []string{"second"}},
			{ID: 3, Lines: []string{"third"}},
		},
		AllowedSplitAfterIDs: []int{1, 2},
	})
	if err != nil {
		t.Fatalf("PlanBoundary failed: %v", err)
	}
	if decision.SplitAfterID != 2 {
		t.Fatalf("SplitAfterID = %d, want 2", decision.SplitAfterID)
	}
	if decision.PromptTokens != 11 || decision.CompletionTokens != 3 || decision.TotalTokens != 14 {
		t.Fatalf("unexpected usage: %+v", decision)
	}
	if got.Model != "test-model" {
		t.Fatalf("model = %q, want test-model", got.Model)
	}
	if got.MaxTokens != DefaultPlannerMaxTokens {
		t.Fatalf("max tokens = %d, want %d", got.MaxTokens, DefaultPlannerMaxTokens)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
		t.Fatalf("unexpected messages: %+v", got.Messages)
	}
	props, ok := got.ResponseFormat.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %+v", got.ResponseFormat.Schema)
	}
	split, ok := props["split_after_id"].(map[string]any)
	if !ok {
		t.Fatalf("split_after_id schema missing: %+v", props)
	}
	enum, ok := split["enum"].([]any)
	if !ok || len(enum) != 2 {
		t.Fatalf("split_after_id enum = %+v, want two allowed ids", split["enum"])
	}
}
