package apperrors

import (
	"errors"
	"strings"
)

type Kind string

const (
	KindTransient  Kind = "transient"
	KindRateLimit  Kind = "rate_limit"
	KindAuth       Kind = "auth"
	KindValidation Kind = "validation"
	KindBadRequest Kind = "bad_request"
)

type Error struct {
	Kind Kind
	// SafeMessage is intended for user-facing output and logs.
	SafeMessage string
	// Cause keeps the original internal error for troubleshooting.
	Cause error

	// Deprecated compatibility fields. New code should use SafeMessage/Cause.
	Err error
	Msg string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	msg := strings.TrimSpace(e.SafeMessage)
	if msg == "" {
		msg = strings.TrimSpace(e.Msg)
	}
	if msg != "" {
		return msg
	}
	if cause := e.cause(); cause != nil {
		return cause.Error()
	}
	return "unknown error"
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause()
}

func (e *Error) cause() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Err
}

func defaultSafeMessage(kind Kind) string {
	switch kind {
	case KindTransient:
		return "Temporary upstream error. Please try again."
	case KindRateLimit:
		return "Rate limit exceeded. Please try again later."
	case KindAuth:
		return "Authentication failed. Please verify your API key and permissions."
	case KindValidation:
		return "Response validation failed."
	case KindBadRequest:
		return "Request rejected by upstream API."
	default:
		return "Request failed."
	}
}

func New(kind Kind, safeMessage string, cause error) error {
	msg := strings.TrimSpace(safeMessage)
	if msg == "" {
		msg = defaultSafeMessage(kind)
	}
	return &Error{
		Kind:        kind,
		SafeMessage: msg,
		Cause:       cause,
	}
}

func Transient(err error) error {
	return New(KindTransient, "", err)
}

func RateLimit(err error) error {
	return New(KindRateLimit, "", err)
}

func Auth(err error) error {
	return New(KindAuth, "", err)
}

func Validation(err error) error {
	return New(KindValidation, "", err)
}

func BadRequest(err error) error {
	return New(KindBadRequest, "", err)
}

func KindOf(err error) (Kind, bool) {
	var e *Error
	if !errors.As(err, &e) {
		return "", false
	}
	return e.Kind, true
}

func PublicMessage(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Error()
	}
	return err.Error()
}

func IsRetryable(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	// Transient: server errors, network issues
	// RateLimit: API rate limiting
	// Validation: LLM output quality issues (hallucination, duplicate ID, etc.)
	//             LLM is non-deterministic, so retrying may succeed
	return e.Kind == KindTransient || e.Kind == KindRateLimit || e.Kind == KindValidation
}

func IsRateLimit(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Kind == KindRateLimit
}
