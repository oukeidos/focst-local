package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// Level aliases
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

var globalLogger *slog.Logger
var isTerminal = term.IsTerminal

var sensitiveKeys = map[string]bool{
	"api_key":         true,
	"apikey":          true,
	"authorization":   true,
	"bearer":          true,
	"body":            true,
	"content":         true,
	"input":           true,
	"output":          true,
	"password":        true,
	"prompt":          true,
	"secret":          true,
	"session":         true,
	"token":           true,
	"translated_text": true,
}

var sensitiveKeySubstrings = []string{
	"key",
	"token",
	"secret",
	"password",
	"authorization",
	"bearer",
	"api",
	"prompt",
	"content",
	"body",
	"input",
	"output",
	"text",
}

var sensitiveValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]{10,}\b`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z\-_]{10,}\b`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-._~+/]+=*\b`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|secret)\b\s*[:=]\s*\S+`),
}

// RedactAttr is a slog.ReplaceAttr function that redacts sensitive information.
func RedactAttr(_ []string, a slog.Attr) slog.Attr {
	if shouldRedact(a) {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

func shouldRedact(a slog.Attr) bool {
	key := strings.ToLower(a.Key)
	if isSafeMetricKey(key) {
		return false
	}
	if sensitiveKeys[key] {
		return true
	}
	for _, sub := range sensitiveKeySubstrings {
		if strings.Contains(key, sub) {
			return true
		}
	}

	var value string
	switch a.Value.Kind() {
	case slog.KindString:
		value = a.Value.String()
	default:
		value = fmt.Sprint(a.Value.Any())
	}
	if value == "" {
		return false
	}
	for _, re := range sensitiveValuePatterns {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

func isSafeMetricKey(key string) bool {
	return key == "max_tokens" ||
		key == "prompt_tokens" ||
		key == "completion_tokens" ||
		key == "total_tokens" ||
		key == "output_tok_s" ||
		key == "total_tok_s"
}

func init() {
	// Default logger: Info level to Stderr
	Init(LevelInfo, nil)
}

// Init initializes the global logger.
// logLevel sets the minimum level to log.
// logFile is an optional writer for JSONL output (e.g., an os.File).
func Init(level slog.Level, logFile io.Writer) {
	opts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: RedactAttr,
	}

	// Console Handler (Pretty)
	useColor := logFile == nil && isTerminal(int(os.Stderr.Fd()))
	consoleHandler := NewPrettyHandler(os.Stderr, opts, useColor)

	var handler slog.Handler = consoleHandler

	// If logFile is provided, use a multi-handler (simplified as a wrapper here)
	if logFile != nil {
		jsonHandler := slog.NewJSONHandler(logFile, opts)
		handler = &multiHandler{
			handlers: []slog.Handler{consoleHandler, jsonHandler},
		}
	}

	globalLogger = slog.New(handler)
	slog.SetDefault(globalLogger)
}

// Global Logging Functions
func Debug(msg string, args ...any) { globalLogger.Debug(msg, args...) }
func Info(msg string, args ...any)  { globalLogger.Info(msg, args...) }
func Warn(msg string, args ...any)  { globalLogger.Warn(msg, args...) }
func Error(msg string, args ...any) { globalLogger.Error(msg, args...) }

// Fatal logs an error and exits
func Fatal(msg string, args ...any) {
	globalLogger.Error(msg, args...)
	os.Exit(1)
}

// --- Pretty Handler Implementation ---

type PrettyHandler struct {
	w      io.Writer
	opts   *slog.HandlerOptions
	attrs  []slog.Attr
	groups []string
	color  bool
}

func NewPrettyHandler(w io.Writer, opts *slog.HandlerOptions, color bool) *PrettyHandler {
	return &PrettyHandler{w: w, opts: opts, color: color}
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	level := r.Level.String()
	levelColor := ""
	reset := ""

	// Simple ANSI colors
	if h.color {
		switch r.Level {
		case slog.LevelDebug:
			levelColor = "\033[90m" // Gray
		case slog.LevelInfo:
			levelColor = "\033[32m" // Green
		case slog.LevelWarn:
			levelColor = "\033[33m" // Yellow
		case slog.LevelError:
			levelColor = "\033[31m" // Red
		}
		reset = "\033[0m"
	}

	timeStr := r.Time.Format("15:04:05")

	fmt.Fprintf(h.w, "%s %s%-5s%s %s",
		timeStr,
		levelColor, level, reset,
		r.Message,
	)

	// Helper to print attributes with group prefix
	printAttr := func(a slog.Attr, groups []string) {
		if h.opts != nil && h.opts.ReplaceAttr != nil {
			a = h.opts.ReplaceAttr(groups, a)
		}
		if a.Key == "" {
			return
		}

		// Handle groups
		key := a.Key
		for i := len(groups) - 1; i >= 0; i-- {
			key = groups[i] + "." + key
		}

		if h.color {
			fmt.Fprintf(h.w, " \033[90m%s=\033[0m%v", key, a.Value)
			return
		}
		fmt.Fprintf(h.w, " %s=%v", key, a.Value)
	}

	// Print persistent attributes
	for _, a := range h.attrs {
		printAttr(a, h.groups)
	}

	// Print record attributes
	r.Attrs(func(a slog.Attr) bool {
		printAttr(a, h.groups)
		return true
	})

	fmt.Fprintln(h.w)
	return nil
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := *h
	h2.attrs = append(h2.attrs[:len(h2.attrs):len(h2.attrs)], attrs...)
	return &h2
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.groups = append(h2.groups[:len(h2.groups):len(h2.groups)], name)
	return &h2
}

// --- Multi Handler Implementation ---

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if err := h.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}
