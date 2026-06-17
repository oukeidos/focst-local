package localllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
	"github.com/oukeidos/focst-local/internal/translator"
)

func TestDiagnosticSourceEchoFirstChunk(t *testing.T) {
	if os.Getenv("FOCST_SOURCE_ECHO_DIAG") == "" {
		t.Skip("set FOCST_SOURCE_ECHO_DIAG=1 to run source-echo diagnostics")
	}
	baseURL := strings.TrimRight(os.Getenv("FOCST_LOCAL_LLM_BASE_URL"), "/")
	model := os.Getenv("FOCST_LOCAL_LLM_MODEL")
	samplePath := os.Getenv("FOCST_LOCAL_SAMPLE_VTT")
	runDir := os.Getenv("FOCST_SOURCE_ECHO_RUN_DIR")
	if baseURL == "" || model == "" || samplePath == "" || runDir == "" {
		t.Fatal("set FOCST_LOCAL_LLM_BASE_URL, FOCST_LOCAL_LLM_MODEL, FOCST_LOCAL_SAMPLE_VTT, and FOCST_SOURCE_ECHO_RUN_DIR")
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("failed to create run dir: %v", err)
	}

	chunkSize := envInt("FOCST_SOURCE_ECHO_CHUNK_SIZE", 50)
	startID := envInt("FOCST_SOURCE_ECHO_START_ID", 1)
	contextSize := envInt("FOCST_SOURCE_ECHO_CONTEXT_SIZE", 5)
	attempts := envInt("FOCST_SOURCE_ECHO_ATTEMPTS", 1)
	maxTokens := envInt("FOCST_SOURCE_ECHO_MAX_TOKENS", DefaultMaxTokens)

	loaded, err := srt.Load(samplePath)
	if err != nil {
		t.Fatalf("failed to load sample: %v", err)
	}
	segments, mapping := srt.PreprocessForPathWithMappingOptions(loaded, "ja", samplePath, true)
	if startID < 1 || startID > len(segments) {
		t.Fatalf("start id %d outside segment range 1-%d", startID, len(segments))
	}
	startIndex := startID - 1
	endIndex := min(len(segments), startIndex+chunkSize)
	beforeStart := max(0, startIndex-contextSize)
	afterEnd := min(len(segments), endIndex+contextSize)
	target := segments[startIndex:endIndex]
	req := translation.RequestData{
		ContextBefore: diagnosticSegmentData(segments[beforeStart:startIndex]),
		Target:        diagnosticSegmentData(target),
		ContextAfter:  diagnosticSegmentData(segments[endIndex:afterEnd]),
	}

	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	requestJSON, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request data: %v", err)
	}
	payload := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: translator.GetSystemPrompt(src.Name, tgt.Name)},
			{Role: "user", Content: string(requestJSON)},
		},
		MaxTokens: maxTokens,
		ResponseFormat: responseFormat{
			Type:   "json_object",
			Schema: exactIDSchema(req.Target),
		},
	}

	payloadBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "request.json"), payloadBytes, 0o644); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}
	if err := writeDiagnosticInputs(filepath.Join(runDir, "input-segments.json"), target, mapping[startIndex:endIndex]); err != nil {
		t.Fatalf("failed to write input segments: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	for attempt := 1; attempt <= attempts; attempt++ {
		started := time.Now()
		httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payloadBytes))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(httpReq)
		wall := time.Since(started)
		if err != nil {
			t.Fatalf("attempt %d request failed: %v", attempt, err)
		}
		body := new(bytes.Buffer)
		if _, err := body.ReadFrom(resp.Body); err != nil {
			resp.Body.Close()
			t.Fatalf("attempt %d read failed: %v", attempt, err)
		}
		resp.Body.Close()

		prefix := fmt.Sprintf("attempt-%02d", attempt)
		if err := os.WriteFile(filepath.Join(runDir, prefix+"-raw-response.json"), body.Bytes(), 0o644); err != nil {
			t.Fatalf("failed to write raw response: %v", err)
		}
		report, err := analyzeDiagnosticResponse(body.Bytes(), target, wall, resp.StatusCode)
		if err != nil {
			report = "# Source Echo Diagnostic\n\n" + err.Error() + "\n"
		}
		if err := os.WriteFile(filepath.Join(runDir, prefix+"-echo-diff.md"), []byte(report), 0o644); err != nil {
			t.Fatalf("failed to write diff report: %v", err)
		}
	}
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func diagnosticSegmentData(segments []srt.Segment) []translation.SegmentData {
	data := make([]translation.SegmentData, len(segments))
	for i, segment := range segments {
		data[i] = translation.SegmentData{
			ID:         segment.ID,
			SourceText: translation.SourceTextFromLines(segment.Lines),
		}
	}
	return data
}

func writeDiagnosticInputs(path string, segments []srt.Segment, mapping []srt.IDMap) error {
	type item struct {
		ID         int      `json:"id"`
		OriginalID int      `json:"original_id"`
		StartTime  string   `json:"start_time"`
		EndTime    string   `json:"end_time"`
		Lines      []string `json:"lines"`
		SourceText string   `json:"source_text"`
	}
	out := make([]item, len(segments))
	for i, segment := range segments {
		originalID := 0
		if i < len(mapping) {
			originalID = mapping[i].OriginalID
		}
		out[i] = item{
			ID:         segment.ID,
			OriginalID: originalID,
			StartTime:  segment.StartTime,
			EndTime:    segment.EndTime,
			Lines:      segment.Lines,
			SourceText: translation.SourceTextFromLines(segment.Lines),
		}
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func analyzeDiagnosticResponse(body []byte, target []srt.Segment, wall time.Duration, statusCode int) (string, error) {
	var completion chatCompletionResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return "", fmt.Errorf("failed to decode chat completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("chat completion had no choices")
	}
	content := completion.Choices[0].Message.Content
	var data translation.ResponseData
	parseErr := json.Unmarshal([]byte(content), &data)

	var lines []string
	lines = append(lines, "# Source Echo Diagnostic", "")
	lines = append(lines, fmt.Sprintf("- HTTP status: `%d`", statusCode))
	lines = append(lines, fmt.Sprintf("- Wall seconds: `%.3f`", wall.Seconds()))
	lines = append(lines, fmt.Sprintf("- Prompt tokens: `%d`", completion.Usage.PromptTokens))
	lines = append(lines, fmt.Sprintf("- Completion tokens: `%d`", completion.Usage.CompletionTokens))
	lines = append(lines, fmt.Sprintf("- Total tokens: `%d`", completion.Usage.TotalTokens))
	lines = append(lines, fmt.Sprintf("- Content bytes: `%d`", len(content)))
	lines = append(lines, "")
	if parseErr != nil {
		lines = append(lines, "## Parse Error", "", fmt.Sprintf("`%v`", parseErr), "")
		lines = append(lines, "## Raw Content Prefix", "", "```json")
		lines = append(lines, truncate(content, 4000))
		lines = append(lines, "```", "")
		return strings.Join(lines, "\n"), nil
	}

	expectedByID := make(map[int]srt.Segment, len(target))
	for _, segment := range target {
		expectedByID[segment.ID] = segment
	}
	mismatches := 0
	missing := 0
	seen := make(map[int]bool, len(data.Translations))
	for _, item := range data.Translations {
		expected, ok := expectedByID[item.ID]
		if !ok {
			mismatches++
			lines = append(lines, fmt.Sprintf("## Unexpected ID %d", item.ID), "")
			lines = append(lines, fmt.Sprintf("- Echo: `%s`", escapeMarkdown(item.SourceText)), "")
			continue
		}
		seen[item.ID] = true
		expectedSourceText := translation.SourceTextFromLines(expected.Lines)
		if item.SourceText != expectedSourceText {
			mismatches++
			lines = append(lines, fmt.Sprintf("## ID %d", item.ID), "")
			lines = append(lines, fmt.Sprintf("- Expected line count: `%d`", len(expected.Lines)))
			lines = append(lines, fmt.Sprintf("- Expected: `%s`", escapeMarkdown(expectedSourceText)))
			lines = append(lines, fmt.Sprintf("- Echo: `%s`", escapeMarkdown(item.SourceText)))
			lines = append(lines, fmt.Sprintf("- Diff: `%s`", firstDiff(expectedSourceText, item.SourceText)))
			lines = append(lines, "")
		}
	}
	for _, segment := range target {
		if !seen[segment.ID] {
			missing++
			lines = append(lines, fmt.Sprintf("## Missing ID %d", segment.ID), "")
		}
	}
	lines = append(lines, "## Translation Preview", "")
	itemsByID := make(map[int]translation.TranslatedSegment, len(data.Translations))
	for _, item := range data.Translations {
		itemsByID[item.ID] = item
	}
	for _, segment := range target {
		item, ok := itemsByID[segment.ID]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("### ID %d", segment.ID), "")
		lines = append(lines, fmt.Sprintf("- Source: `%s`", escapeMarkdown(strings.Join(segment.Lines, " / "))))
		lines = append(lines, fmt.Sprintf("- Model source_text: `%s`", escapeMarkdown(translation.SourceTextFromLines(segment.Lines))))
		lines = append(lines, fmt.Sprintf("- Text: `%s`", escapeMarkdown(item.Text)))
		lines = append(lines, "")
	}
	lines = append([]string{
		"# Source Echo Diagnostic",
		"",
		fmt.Sprintf("- HTTP status: `%d`", statusCode),
		fmt.Sprintf("- Wall seconds: `%.3f`", wall.Seconds()),
		fmt.Sprintf("- Prompt tokens: `%d`", completion.Usage.PromptTokens),
		fmt.Sprintf("- Completion tokens: `%d`", completion.Usage.CompletionTokens),
		fmt.Sprintf("- Total tokens: `%d`", completion.Usage.TotalTokens),
		fmt.Sprintf("- Response items: `%d`", len(data.Translations)),
		fmt.Sprintf("- Echo mismatches: `%d`", mismatches),
		fmt.Sprintf("- Missing IDs: `%d`", missing),
		"",
	}, lines[9:]...)
	if mismatches == 0 && missing == 0 {
		lines = append(lines, "All echoed `source_text` values matched the product input exactly.", "")
	}
	return strings.Join(lines, "\n"), nil
}

func escapeMarkdown(value string) string {
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func firstDiff(expected, got string) string {
	expRunes := []rune(expected)
	gotRunes := []rune(got)
	limit := min(len(expRunes), len(gotRunes))
	for i := 0; i < limit; i++ {
		if expRunes[i] != gotRunes[i] {
			return fmt.Sprintf("rune %d: expected %q U+%04X, got %q U+%04X", i, expRunes[i], expRunes[i], gotRunes[i], gotRunes[i])
		}
	}
	if len(expRunes) != len(gotRunes) {
		return fmt.Sprintf("same prefix, expected %d runes, got %d runes", len(expRunes), len(gotRunes))
	}
	return "no difference"
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n... truncated ..."
}
