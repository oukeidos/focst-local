package main

import (
	"bytes"
	"strings"
	"testing"
)

func withEnvStatusStubs(t *testing.T, status bool, envKey string) (*keyStubs, func()) {
	t.Helper()
	stubs := &keyStubs{}

	prevStatus := getStatus
	prevEnv := getEnvKey

	getStatus = func(_ string) bool {
		return status
	}
	getEnvKey = func(_ string) (string, bool) {
		stubs.envCalls++
		if envKey == "" {
			return "", false
		}
		return envKey, true
	}

	restore := func() {
		getStatus = prevStatus
		getEnvKey = prevEnv
	}

	return stubs, restore
}

func executeCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestHandleEnv_StatusKeychain(t *testing.T) {
	_, restore := withEnvStatusStubs(t, true, "example-env-key")
	defer restore()

	out, err := executeCommand(t, "env", "status", "--service", "openai")
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !strings.Contains(out, "Found (source=Keychain)") {
		t.Fatalf("expected keychain source, got: %s", out)
	}
	if strings.Contains(out, "example-env-key") {
		t.Fatalf("output leaked env key")
	}
}

func TestHandleEnv_StatusEnv(t *testing.T) {
	_, restore := withEnvStatusStubs(t, false, "example-env-key")
	defer restore()

	out, err := executeCommand(t, "env", "status", "--service", "openai")
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !strings.Contains(out, "Found (source=Environment Variable") {
		t.Fatalf("expected env source, got: %s", out)
	}
	if strings.Contains(out, "example-env-key") {
		t.Fatalf("output leaked env key")
	}
}

func TestHandleEnv_StatusNotFound(t *testing.T) {
	_, restore := withEnvStatusStubs(t, false, "")
	defer restore()

	out, err := executeCommand(t, "env", "status", "--service", "openai")
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !strings.Contains(out, "Not Found") {
		t.Fatalf("expected not found, got: %s", out)
	}
}

func TestHandleEnvSetup_RejectsPositionalAPIKey(t *testing.T) {
	out, err := executeCommand(t, "env", "setup", "example-positional-key", "--service", "openai")
	if err == nil {
		t.Fatalf("expected setup to reject positional API key argument")
	}
	if !strings.Contains(out, "unknown command") && !strings.Contains(out, "accepts 0 arg(s)") {
		t.Fatalf("expected positional-argument rejection error, got: %s", out)
	}
}

func TestHandleEnv_RejectsUnsupportedService(t *testing.T) {
	out, err := executeCommand(t, "env", "status", "--service", "google")
	if err == nil {
		t.Fatalf("expected unsupported service to be rejected")
	}
	if !strings.Contains(out, "invalid service") {
		t.Fatalf("expected invalid service error, got: %s", out)
	}
}
