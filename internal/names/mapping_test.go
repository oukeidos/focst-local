package names

import (
	"strings"
	"testing"
)

func TestEncodeDecodeMappings_NormalizesZh(t *testing.T) {
	in := []CharacterMapping{{Source: "Alice", Target: "爱丽丝"}}
	data, err := EncodeMappings(in, "zh", "en")
	if err != nil {
		t.Fatalf("EncodeMappings failed: %v", err)
	}
	if !strings.Contains(string(data), `"zh-Hans"`) {
		t.Fatalf("expected zh-Hans key in output, got: %s", string(data))
	}
	out, err := DecodeMappings(data, "zh", "en")
	if err != nil {
		t.Fatalf("DecodeMappings failed: %v", err)
	}
	if len(out) != 1 || out[0].Source != "Alice" || out[0].Target != "爱丽丝" {
		t.Fatalf("unexpected decoded mappings: %+v", out)
	}
}

func TestDecodeMappings_MissingKey(t *testing.T) {
	data := []byte(`[{"en":"Bob"}]`)
	_, err := DecodeMappings(data, "en", "ko")
	if err == nil {
		t.Fatalf("expected error for missing target key")
	}
}
