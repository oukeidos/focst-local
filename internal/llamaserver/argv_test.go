package llamaserver

import (
	"strings"
	"testing"
)

func TestBuildArgs_NoHiddenModelSpecificFlags(t *testing.T) {
	args, err := BuildArgs(LaunchConfig{
		ModelPath:  "/models/model.gguf",
		ModelAlias: "gemma-test",
		Host:       "127.0.0.1",
		Port:       18080,
		CtxSize:    16384,
		Parallel:   1,
	})
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--model /models/model.gguf",
		"--alias gemma-test",
		"--host 127.0.0.1",
		"--port 18080",
		"--ctx-size 16384",
		"--parallel 1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	for _, forbidden := range []string{"--no-kv-offload", "--no-mmproj-offload", "--json-schema-file", "--grammar-file"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("args contain hidden/default flag %q: %q", forbidden, joined)
		}
	}
}

func TestBuildArgs_AppendsExplicitExtraArgs(t *testing.T) {
	args, err := BuildArgs(LaunchConfig{
		ModelPath:  "/models/model.gguf",
		ModelAlias: "gemma-test",
		Host:       "127.0.0.1",
		Port:       18080,
		CtxSize:    16384,
		Parallel:   1,
		ExtraArgs:  []string{"--reasoning", "off"},
	})
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--reasoning off") {
		t.Fatalf("args %q missing explicit reasoning arg", joined)
	}
}
