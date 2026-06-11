package llamaserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeModelsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "gemma-test",
				"aliases": ["gemma-alias"],
				"meta": {"n_ctx": 16384}
			}]
		}`))
	}))
	defer server.Close()

	status, err := Probe(context.Background(), server.URL+"/v1")
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if !status.HasModel("gemma-alias") || !status.HasModel("gemma-test") {
		t.Fatalf("expected model id and alias to match: %#v", status)
	}
	if status.NCtx != 16384 {
		t.Fatalf("n_ctx = %d, want 16384", status.NCtx)
	}
}

func TestProbeModelsFallbackModelsField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"models": [{
				"name": "gemma-name",
				"model": "gemma-model",
				"meta": {"n_ctx": 8192}
			}]
		}`))
	}))
	defer server.Close()

	status, err := Probe(context.Background(), server.URL+"/v1")
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if !status.HasModel("gemma-name") || !status.HasModel("gemma-model") {
		t.Fatalf("expected model name/model to match: %#v", status)
	}
	if status.NCtx != 8192 {
		t.Fatalf("n_ctx = %d, want 8192", status.NCtx)
	}
}
