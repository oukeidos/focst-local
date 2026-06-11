package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/translation"
)

type keyStubs struct {
	promptCalls int
	keyCalls    int
	envCalls    int
}

func withKeyStubs(t *testing.T, terminal bool, promptVal string, keychainVal string, envVal string) (*keyStubs, func()) {
	t.Helper()
	stubs := &keyStubs{}

	prevIsTerminal := isTerminal
	prevPrompt := promptForKey
	prevGetKey := getKey
	prevGetEnv := getEnvKey

	isTerminal = func(_ int) bool { return terminal }
	promptForKey = func(_ string) (string, error) {
		stubs.promptCalls++
		return promptVal, nil
	}
	getKey = func(_ string, _ bool) (string, string) {
		stubs.keyCalls++
		if keychainVal == "" {
			return "", ""
		}
		return keychainVal, "Keychain"
	}
	getEnvKey = func(_ string) (string, bool) {
		stubs.envCalls++
		if envVal == "" {
			return "", false
		}
		return envVal, true
	}

	restore := func() {
		isTerminal = prevIsTerminal
		promptForKey = prevPrompt
		getKey = prevGetKey
		getEnvKey = prevGetEnv
	}

	return stubs, restore
}

func TestResolveAPIKey_KeychainFallback(t *testing.T) {
	stubs, restore := withKeyStubs(t, true, "", "keychain-key", "env-key")
	defer restore()

	key, source, err := resolveAPIKey("openai", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "keychain-key" || source != "Keychain" {
		t.Fatalf("expected keychain key/source, got key=%q source=%q", key, source)
	}
	if stubs.envCalls != 0 {
		t.Fatalf("expected no env calls, got envCalls=%d", stubs.envCalls)
	}
}

func TestResolveAPIKey_EnvFallbackWhenAllowed(t *testing.T) {
	stubs, restore := withKeyStubs(t, false, "", "", "env-key")
	defer restore()

	key, source, err := resolveAPIKey("openai", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key" || source != "Environment Variable" {
		t.Fatalf("expected env key/source, got key=%q source=%q", key, source)
	}
	if stubs.envCalls == 0 {
		t.Fatalf("expected env call")
	}
}

func TestResolveAPIKey_EnvDisabledError(t *testing.T) {
	stubs, restore := withKeyStubs(t, false, "", "", "env-key")
	defer restore()

	key, source, err := resolveAPIKey("openai", false, false)
	if err == nil {
		t.Fatalf("expected error, got key=%q source=%q", key, source)
	}
	if stubs.envCalls != 0 {
		t.Fatalf("expected no env calls, got envCalls=%d", stubs.envCalls)
	}
}

func TestResolveAPIKey_NonInteractiveError(t *testing.T) {
	stubs, restore := withKeyStubs(t, false, "", "", "")
	defer restore()

	key, source, err := resolveAPIKey("openai", false, false)
	if err == nil {
		t.Fatalf("expected error, got key=%q source=%q", key, source)
	}
	if stubs.promptCalls != 0 {
		t.Fatalf("expected no prompt, got promptCalls=%d", stubs.promptCalls)
	}
}

func TestResolveAPIKey_EnvOnlySuccess(t *testing.T) {
	stubs, restore := withKeyStubs(t, false, "prompt-key", "keychain-key", "env-key")
	defer restore()

	key, source, err := resolveAPIKey("openai", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key" || source != "Environment Variable" {
		t.Fatalf("expected env key/source, got key=%q source=%q", key, source)
	}
	if stubs.promptCalls != 0 || stubs.keyCalls != 0 {
		t.Fatalf("expected no prompt/keychain calls, got promptCalls=%d keyCalls=%d", stubs.promptCalls, stubs.keyCalls)
	}
}

func TestResolveAPIKey_EnvOnlyMissingError(t *testing.T) {
	_, restore := withKeyStubs(t, false, "", "keychain-key", "")
	defer restore()

	_, _, err := resolveAPIKey("openai", false, true)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestResolveAPIKey_EnvOnlyWithAllowEnvFlag(t *testing.T) {
	stubs, restore := withKeyStubs(t, false, "prompt-key", "keychain-key", "env-key")
	defer restore()

	key, source, err := resolveAPIKey("openai", true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key" || source != "Environment Variable" {
		t.Fatalf("expected env key/source, got key=%q source=%q", key, source)
	}
	if stubs.promptCalls != 0 || stubs.keyCalls != 0 {
		t.Fatalf("expected no prompt/keychain calls, got promptCalls=%d keyCalls=%d", stubs.promptCalls, stubs.keyCalls)
	}
}

func TestResolveAPIKey_PromptFallback(t *testing.T) {
	stubs, restore := withKeyStubs(t, true, "prompt-key", "", "")
	defer restore()

	key, source, err := resolveAPIKey("openai", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "prompt-key" || source != "Terminal Prompt" {
		t.Fatalf("expected prompt key/source, got key=%q source=%q", key, source)
	}
	if stubs.keyCalls == 0 {
		t.Fatalf("expected keychain lookup before prompt")
	}
}

func TestPrintUsageStatsSeparatesOutputAndTotalThroughput(t *testing.T) {
	out := captureStdout(t, func() {
		printUsageStats(&translation.UsageMetadata{
			PromptTokenCount:     60,
			CandidatesTokenCount: 40,
			TotalTokenCount:      100,
		}, 2*time.Second, "local-model")
	})

	required := []string{
		"Tokens: In=60, Out=40, Total=100",
		"Output Throughput: 20.00 tok/s (out/run time)",
		"Total Throughput: 50.00 tok/s (in+out/run time)",
	}
	for _, phrase := range required {
		if !strings.Contains(out, phrase) {
			t.Fatalf("expected output to contain %q, got:\n%s", phrase, out)
		}
	}
	if strings.Contains(out, "Average Speed") {
		t.Fatalf("old ambiguous speed label should not be printed, got:\n%s", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		os.Stdout = oldStdout
		t.Fatalf("failed to close stdout writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return buf.String()
}
