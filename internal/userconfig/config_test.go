package userconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSaveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Config{
		LlamaServerBin: "/bin/llama-server",
		ModelPath:      "/models/model.gguf",
		Model:          "gemma-test",
		CtxSize:        16384,
		Parallel:       1,
		ExtraArgs:      []string{"--reasoning", "off"},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.LlamaServerBin != cfg.LlamaServerBin ||
		loaded.ModelPath != cfg.ModelPath ||
		loaded.Model != cfg.Model ||
		loaded.CtxSize != cfg.CtxSize ||
		loaded.Parallel != cfg.Parallel ||
		strings.Join(loaded.ExtraArgs, " ") != strings.Join(cfg.ExtraArgs, " ") {
		t.Fatalf("loaded config mismatch: %#v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestSetUnsetAndArgs(t *testing.T) {
	cfg, err := Set(Config{}, "llama-server-bin", "/x/llama-server")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	cfg, err = Set(cfg, "ctx-size", "8192")
	if err != nil {
		t.Fatalf("Set ctx-size failed: %v", err)
	}
	cfg = AddArg(cfg, "--reasoning")
	cfg = AddArg(cfg, "off")
	if cfg.LlamaServerBin != "/x/llama-server" || cfg.CtxSize != 8192 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if got := strings.Join(cfg.ExtraArgs, " "); got != "--reasoning off" {
		t.Fatalf("extra args = %q", got)
	}
	cfg, err = Unset(cfg, "llama-server-bin")
	if err != nil {
		t.Fatalf("Unset failed: %v", err)
	}
	cfg = ClearArgs(cfg)
	if cfg.LlamaServerBin != "" || len(cfg.ExtraArgs) != 0 {
		t.Fatalf("unset/clear failed: %#v", cfg)
	}
}

func TestInvalidConfigValues(t *testing.T) {
	if _, err := Set(Config{}, "unknown", "x"); err == nil {
		t.Fatalf("expected unknown key error")
	}
	if _, err := Set(Config{}, "parallel", "zero"); err == nil {
		t.Fatalf("expected invalid integer error")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"ctx_size":`), 0600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("expected malformed config error")
	}
}
