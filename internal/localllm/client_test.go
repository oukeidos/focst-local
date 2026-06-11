package localllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/srt"
)

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
