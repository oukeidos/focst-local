package llamaserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oukeidos/focst-local/internal/userconfig"
)

func TestResolveConfigUsesEnvBeforeUserConfig(t *testing.T) {
	dir := t.TempDir()
	envServer := writeExecutable(t, filepath.Join(dir, "env-llama-server"))
	cfgServer := writeExecutable(t, filepath.Join(dir, "cfg-llama-server"))
	envModel := writeFile(t, filepath.Join(dir, "env-model.gguf"))
	cfgModel := writeFile(t, filepath.Join(dir, "cfg-model.gguf"))
	t.Setenv(userconfig.EnvLlamaServerBin, envServer)
	t.Setenv(userconfig.EnvModelPath, envModel)

	resolved, err := ResolveConfig(LaunchConfig{
		Mode:       ModeStart,
		ModelAlias: "gemma-test",
	}, userconfig.Config{
		LlamaServerBin: cfgServer,
		ModelPath:      cfgModel,
	})
	if err != nil {
		t.Fatalf("ResolveConfig failed: %v", err)
	}
	if resolved.ServerBin != envServer {
		t.Fatalf("server bin = %q, want env %q", resolved.ServerBin, envServer)
	}
	if resolved.ModelPath != envModel {
		t.Fatalf("model path = %q, want env %q", resolved.ModelPath, envModel)
	}
}

func TestResolveConfigRequiresModelPathInStartMode(t *testing.T) {
	server := writeExecutable(t, filepath.Join(t.TempDir(), "llama-server"))
	_, err := ResolveConfig(LaunchConfig{
		Mode:       ModeStart,
		ServerBin:  server,
		ModelAlias: "gemma-test",
	}, userconfig.Config{})
	if err == nil {
		t.Fatalf("expected missing model path error")
	}
}

func TestResolveConfigExternalDoesNotNeedPaths(t *testing.T) {
	_, err := ResolveConfig(LaunchConfig{
		Mode:       ModeExternal,
		BaseURL:    "http://127.0.0.1:8080/v1",
		ModelAlias: "gemma-test",
	}, userconfig.Config{})
	if err != nil {
		t.Fatalf("external mode should not require paths: %v", err)
	}
}

func writeExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs executable: %v", err)
	}
	return abs
}

func writeFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("model"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs file: %v", err)
	}
	return abs
}
