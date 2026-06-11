package apperrors

import (
	"errors"
	"testing"
)

func TestPublicMessage_UsesSafeMessage(t *testing.T) {
	sentinel := errors.New("SECRET_VALUE")
	err := New(KindAuth, "safe auth error", sentinel)
	if got := PublicMessage(err); got != "safe auth error" {
		t.Fatalf("PublicMessage() = %q, want %q", got, "safe auth error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped cause to be retained for internal matching")
	}
}

func TestKindOfAndRetryable(t *testing.T) {
	err := New(KindRateLimit, "", errors.New("boom"))
	kind, ok := KindOf(err)
	if !ok || kind != KindRateLimit {
		t.Fatalf("KindOf() = (%q, %v), want (%q, true)", kind, ok, KindRateLimit)
	}
	if !IsRetryable(err) {
		t.Fatalf("expected rate_limit error to be retryable")
	}
}

func TestPublicMessage_NonAppError(t *testing.T) {
	err := errors.New("plain")
	if got := PublicMessage(err); got != "plain" {
		t.Fatalf("PublicMessage() = %q, want %q", got, "plain")
	}
}
