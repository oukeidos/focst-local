package main

import (
	"strings"
	"testing"
)

func TestConfigCommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out, err := executeCommand(t, "config", "path")
	if err != nil {
		t.Fatalf("config path failed: %v", err)
	}
	if !strings.Contains(out, "focst-local/config.json") {
		t.Fatalf("unexpected config path output: %q", out)
	}

	commands := [][]string{
		{"config", "set", "llama-server-bin", "/tmp/llama-server"},
		{"config", "set", "model-path", "/tmp/model.gguf"},
		{"config", "set", "model", "gemma-test"},
		{"config", "set", "ctx-size", "16384"},
		{"config", "set", "parallel", "1"},
		{"config", "add-arg", "--reasoning"},
		{"config", "add-arg", "off"},
	}
	for _, args := range commands {
		if _, err := executeCommand(t, args...); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	}
	out, err = executeCommand(t, "config", "show")
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}
	for _, want := range []string{
		`"llama_server_bin": "/tmp/llama-server"`,
		`"model_path": "/tmp/model.gguf"`,
		`"model": "gemma-test"`,
		`"ctx_size": 16384`,
		`"parallel": 1`,
		`"--reasoning"`,
		`"off"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config show output missing %q: %s", want, out)
		}
	}

	if _, err := executeCommand(t, "config", "unset", "model"); err != nil {
		t.Fatalf("config unset failed: %v", err)
	}
	if _, err := executeCommand(t, "config", "clear-args"); err != nil {
		t.Fatalf("config clear-args failed: %v", err)
	}
	out, err = executeCommand(t, "config", "show")
	if err != nil {
		t.Fatalf("config show after unset failed: %v", err)
	}
	if strings.Contains(out, `"model":`) || strings.Contains(out, "--reasoning") {
		t.Fatalf("unset/clear-args did not update config: %s", out)
	}
}

func TestConfigSetValidatesScalarTypes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := executeCommand(t, "config", "set", "ctx-size", "bad")
	if err == nil {
		t.Fatalf("expected invalid ctx-size error")
	}
}
