package metadata

import "testing"

func TestOpenAIPricing_Default(t *testing.T) {
	m, ok := OpenAIPricing("unknown-model")
	if ok {
		t.Fatalf("expected default pricing for unknown model")
	}
	if m.InputPerMillion != DefaultOpenAIInputPerMillion || m.OutputPerMillion != DefaultOpenAIOutputPerMillion {
		t.Fatalf("unexpected default openai pricing: %+v", m)
	}
}
