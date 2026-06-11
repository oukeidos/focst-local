package prompt

import (
	"bytes"
	"testing"
)

func TestConfirmOverwrite_NonInteractive(t *testing.T) {
	c := Confirmer{
		In:            bytes.NewBufferString("y\n"),
		Out:           nil,
		IsInteractive: func() bool { return false },
	}
	ok, err := c.ConfirmOverwrite("out.srt", false)
	if err == nil {
		t.Fatalf("expected error for non-interactive confirm, got ok=%v", ok)
	}
}

func TestConfirmOverwrite_Force(t *testing.T) {
	c := Confirmer{
		In:            bytes.NewBufferString("n\n"),
		Out:           nil,
		IsInteractive: func() bool { return false },
	}
	ok, err := c.ConfirmOverwrite("out.srt", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true for forced overwrite")
	}
}

func TestConfirmOverwrite_Interactive(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		c := Confirmer{
			In:            bytes.NewBufferString("y\n"),
			Out:           nil,
			IsInteractive: func() bool { return true },
		}
		ok, err := c.ConfirmOverwrite("out.srt", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected ok=true")
		}
	})

	t.Run("no", func(t *testing.T) {
		c := Confirmer{
			In:            bytes.NewBufferString("n\n"),
			Out:           nil,
			IsInteractive: func() bool { return true },
		}
		ok, err := c.ConfirmOverwrite("out.srt", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected ok=false")
		}
	})
}
