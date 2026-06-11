package logger

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestPrettyHandler_Structural(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: LevelDebug}
	h := NewPrettyHandler(&buf, opts, false)
	l := slog.New(h)

	t.Run("WithAttrs", func(t *testing.T) {
		buf.Reset()
		l2 := l.With("request_id", "abc-123")
		l2.Info("test message", "user", "alice")

		output := buf.String()
		if !strings.Contains(output, "request_id=") || !strings.Contains(output, "abc-123") {
			t.Errorf("output missing persistent attr: %q", output)
		}
		if !strings.Contains(output, "user=") || !strings.Contains(output, "alice") {
			t.Errorf("output missing record attr: %q", output)
		}
	})

	t.Run("WithGroup", func(t *testing.T) {
		buf.Reset()
		l2 := l.WithGroup("billing").With("amount", 100)
		l2.Info("payment processing", "currency", "USD")

		output := buf.String()
		if !strings.Contains(output, "billing.amount=") || !strings.Contains(output, "100") {
			t.Errorf("output missing grouped persistent attr: %q", output)
		}
		if !strings.Contains(output, "billing.currency=") || !strings.Contains(output, "USD") {
			t.Errorf("output missing grouped record attr: %q", output)
		}
	})

	t.Run("NestedGroups", func(t *testing.T) {
		buf.Reset()
		l2 := l.WithGroup("outer").WithGroup("inner").With("key", "val")
		l2.Info("msg")

		output := buf.String()
		if !strings.Contains(output, "outer.inner.key=") || !strings.Contains(output, "val") {
			t.Errorf("output missing nested grouped attr: %q", output)
		}
	})
}

func TestRedactAttr(t *testing.T) {
	openAIStyleKey := "sk-" + "1234567890abcdef"

	t.Run("KeyBasedRedaction", func(t *testing.T) {
		attr := slog.String("api_key", openAIStyleKey)
		got := RedactAttr(nil, attr)
		if got.Value.String() != "[REDACTED]" {
			t.Fatalf("expected redaction, got %q", got.Value.String())
		}
	})

	t.Run("ValuePatternRedaction", func(t *testing.T) {
		attr := slog.String("message", "bearer "+openAIStyleKey)
		got := RedactAttr(nil, attr)
		if got.Value.String() != "[REDACTED]" {
			t.Fatalf("expected redaction, got %q", got.Value.String())
		}
	})

	t.Run("NonSensitive", func(t *testing.T) {
		attr := slog.String("user", "alice")
		got := RedactAttr(nil, attr)
		if got.Value.String() != "alice" {
			t.Fatalf("unexpected redaction: %q", got.Value.String())
		}
	})
}

func TestPrettyHandler_NoColorWhenNotTTY(t *testing.T) {
	prevIsTerminal := isTerminal
	isTerminal = func(_ int) bool { return false }
	defer func() { isTerminal = prevIsTerminal }()

	prevStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = prevStderr }()

	Init(LevelInfo, nil)
	Info("test message", "key", "value")

	_ = w.Close()
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "\033[") {
		t.Fatalf("unexpected ANSI codes in output: %q", string(out))
	}
}

func TestPrettyHandler_NoColorWhenLogFileEnabled(t *testing.T) {
	prevIsTerminal := isTerminal
	isTerminal = func(_ int) bool { return true }
	defer func() { isTerminal = prevIsTerminal }()

	prevStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = prevStderr }()

	var logBuf bytes.Buffer
	Init(LevelInfo, &logBuf)
	Info("test message", "key", "value")

	_ = w.Close()
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "\033[") {
		t.Fatalf("unexpected ANSI codes in output: %q", string(out))
	}
}
