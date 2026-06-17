package localllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/apperrors"
	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/httpclient"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/oukeidos/focst-local/internal/translation"
)

const (
	DefaultBaseURL          = "http://127.0.0.1:8080/v1"
	DefaultModel            = "gemma-4-26b-a4b-qat-q4_0"
	DefaultMaxTokens        = 8192
	DefaultPlannerMaxTokens = 192
	// DefaultTranslationTimeout disables per-request timeout for slow local models.
	DefaultTranslationTimeout time.Duration = 0
)

type Client struct {
	baseURL           string
	model             string
	systemInstruction string
	maxTokens         int
	translationClient *http.Client
}

func NewClient(baseURL, model string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultModel
	}
	return &Client{
		baseURL:           strings.TrimRight(baseURL, "/"),
		model:             model,
		maxTokens:         DefaultMaxTokens,
		translationClient: httpclient.NewClient(DefaultTranslationTimeout),
	}
}

func (c *Client) SetMaxTokens(maxTokens int) {
	if maxTokens > 0 {
		c.maxTokens = maxTokens
	}
}

func (c *Client) SetTranslationTimeout(timeout time.Duration) {
	if timeout >= 0 {
		c.translationClient = httpclient.NewClient(timeout)
	}
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) SetSystemInstruction(prompt string) {
	c.systemInstruction = prompt
}

func (c *Client) Check(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create local LLM health request: %w", err)
	}
	respBody, resp, err := httpclient.DoAndRead(httpclient.GetDefaultClient(), httpReq)
	if err != nil {
		return apperrors.Transient(fmt.Errorf("local LLM health check failed: %w", err))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return apperrors.Validation(fmt.Errorf("local LLM health status=%s body=%s", resp.Status, string(respBody)))
	}
	return nil
}

func (c *Client) Translate(ctx context.Context, request translation.RequestData) (*translation.ResponseData, error) {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: c.systemInstruction},
			{Role: "user", Content: string(requestJSON)},
		},
		MaxTokens: c.maxTokens,
		ResponseFormat: responseFormat{
			Type:   "json_object",
			Schema: exactIDSchema(request.Target),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal local LLM request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create local LLM request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := c.translationClient
	if client == nil {
		client = httpclient.NewClient(DefaultTranslationTimeout)
		c.translationClient = client
	}
	respBody, resp, err := httpclient.DoAndRead(client, httpReq)
	if err != nil {
		return nil, apperrors.Transient(fmt.Errorf("local LLM request failed: %w", err))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apperrors.Validation(fmt.Errorf("local LLM status=%s body=%s", resp.Status, string(respBody)))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, apperrors.Validation(fmt.Errorf("failed to decode local LLM response: %w", err))
	}
	if len(completion.Choices) == 0 {
		return nil, apperrors.Validation(fmt.Errorf("local LLM response had no choices"))
	}

	var responseData translation.ResponseData
	content := completion.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &responseData); err != nil {
		var translations []translation.TranslatedSegment
		if err2 := json.Unmarshal([]byte(content), &translations); err2 == nil {
			responseData.Translations = translations
		} else {
			return nil, apperrors.Validation(fmt.Errorf("failed to unmarshal local LLM content: %w", err))
		}
	}

	responseData.Usage = translation.UsageMetadata{
		PromptTokenCount:     completion.Usage.PromptTokens,
		CandidatesTokenCount: completion.Usage.CompletionTokens,
		TotalTokenCount:      completion.Usage.TotalTokens,
	}
	return &responseData, nil
}

// CompleteText sends a plain text chat-completion request without JSON schema
// forcing. It is used for local helper passes where experiments showed that
// plaintext Markdown is more reliable than structured JSON.
func (c *Client) CompleteText(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (*translation.TextCompletion, error) {
	return c.CompleteTextWithOptions(ctx, systemPrompt, userPrompt, translation.TextCompletionOptions{
		MaxTokens: maxTokens,
	})
}

// CompleteTextWithOptions sends a plain text chat-completion request with
// explicit sampler settings.
func (c *Client) CompleteTextWithOptions(ctx context.Context, systemPrompt, userPrompt string, opts translation.TextCompletionOptions) (*translation.TextCompletion, error) {
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: opts.Temperature,
		MaxTokens:   maxTokens,
		ResponseFormat: responseFormat{
			Type: "text",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal local LLM text request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create local LLM text request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := c.translationClient
	if client == nil {
		client = httpclient.NewClient(DefaultTranslationTimeout)
		c.translationClient = client
	}
	respBody, resp, err := httpclient.DoAndRead(client, httpReq)
	if err != nil {
		return nil, apperrors.Transient(fmt.Errorf("local LLM text request failed: %w", err))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apperrors.Validation(fmt.Errorf("local LLM text status=%s body=%s", resp.Status, string(respBody)))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, apperrors.Validation(fmt.Errorf("failed to decode local LLM text response: %w", err))
	}
	if len(completion.Choices) == 0 {
		return nil, apperrors.Validation(fmt.Errorf("local LLM text response had no choices"))
	}
	return &translation.TextCompletion{
		Content: completion.Choices[0].Message.Content,
		Usage: translation.UsageMetadata{
			PromptTokenCount:     completion.Usage.PromptTokens,
			CandidatesTokenCount: completion.Usage.CompletionTokens,
			TotalTokenCount:      completion.Usage.TotalTokens,
		},
	}, nil
}

// CompleteJSONWithOptions sends a chat-completion request with JSON response
// forcing and an explicit schema. It is used by helper passes whose experiments
// were run with structured JSON output rather than plaintext.
func (c *Client) CompleteJSONWithOptions(ctx context.Context, systemPrompt, userPrompt string, schema map[string]any, opts translation.TextCompletionOptions) (*translation.TextCompletion, error) {
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: opts.Temperature,
		MaxTokens:   maxTokens,
		ResponseFormat: responseFormat{
			Type:   "json_object",
			Schema: schema,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal local LLM JSON request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create local LLM JSON request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := c.translationClient
	if client == nil {
		client = httpclient.NewClient(DefaultTranslationTimeout)
		c.translationClient = client
	}
	respBody, resp, err := httpclient.DoAndRead(client, httpReq)
	if err != nil {
		return nil, apperrors.Transient(fmt.Errorf("local LLM JSON request failed: %w", err))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apperrors.Validation(fmt.Errorf("local LLM JSON status=%s body=%s", resp.Status, string(respBody)))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, apperrors.Validation(fmt.Errorf("failed to decode local LLM JSON response: %w", err))
	}
	if len(completion.Choices) == 0 {
		return nil, apperrors.Validation(fmt.Errorf("local LLM JSON response had no choices"))
	}
	return &translation.TextCompletion{
		Content: completion.Choices[0].Message.Content,
		Usage: translation.UsageMetadata{
			PromptTokenCount:     completion.Usage.PromptTokens,
			CandidatesTokenCount: completion.Usage.CompletionTokens,
			TotalTokenCount:      completion.Usage.TotalTokens,
		},
	}, nil
}

// PlanBoundary asks the local model to choose one split_after_id for a subtitle chunk boundary.
func (c *Client) PlanBoundary(ctx context.Context, request chunker.BoundaryRequest) (chunker.BoundaryDecision, error) {
	if len(request.Segments) == 0 {
		return chunker.BoundaryDecision{}, fmt.Errorf("boundary planner request has no segments")
	}
	if len(request.AllowedSplitAfterIDs) == 0 {
		return chunker.BoundaryDecision{}, fmt.Errorf("boundary planner request has no allowed split ids")
	}

	userPayload := boundaryPlannerPayload{
		Task:       "choose_one_split_after_id",
		Segments:   toBoundarySegments(request.Segments),
		Rule:       "Choose the segment id after which the viewing flow is least interrupted and the previous chunk has completed its current thought.",
		Constraint: "Choose one split_after_id from the listed segment ids, except the final listed id.",
		Output: map[string]string{
			"split_after_id": "the segment id after which to split",
		},
	}
	requestJSON, err := json.Marshal(userPayload)
	if err != nil {
		return chunker.BoundaryDecision{}, fmt.Errorf("failed to marshal boundary planner request: %w", err)
	}

	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: "You are a subtitle editor choosing a chunk boundary. Return only JSON."},
			{Role: "user", Content: string(requestJSON)},
		},
		MaxTokens: DefaultPlannerMaxTokens,
		ResponseFormat: responseFormat{
			Type:   "json_object",
			Schema: boundarySchema(request.AllowedSplitAfterIDs),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return chunker.BoundaryDecision{}, fmt.Errorf("failed to marshal boundary planner payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chunker.BoundaryDecision{}, fmt.Errorf("failed to create boundary planner request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	started := time.Now()
	respBody, resp, err := httpclient.DoAndRead(httpclient.GetDefaultClient(), httpReq)
	wallSeconds := time.Since(started).Seconds()
	if err != nil {
		return chunker.BoundaryDecision{}, apperrors.Transient(fmt.Errorf("boundary planner request failed: %w", err))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return chunker.BoundaryDecision{}, apperrors.Validation(fmt.Errorf("boundary planner status=%s body=%s", resp.Status, string(respBody)))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return chunker.BoundaryDecision{}, apperrors.Validation(fmt.Errorf("failed to decode boundary planner response: %w", err))
	}
	if len(completion.Choices) == 0 {
		return chunker.BoundaryDecision{}, apperrors.Validation(fmt.Errorf("boundary planner response had no choices"))
	}

	var response boundaryPlannerResponse
	content := completion.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return chunker.BoundaryDecision{}, apperrors.Validation(fmt.Errorf("failed to unmarshal boundary planner content: %w", err))
	}
	return chunker.BoundaryDecision{
		SplitAfterID:     response.SplitAfterID,
		PromptTokens:     completion.Usage.PromptTokens,
		CompletionTokens: completion.Usage.CompletionTokens,
		TotalTokens:      completion.Usage.TotalTokens,
		WallSeconds:      wallSeconds,
	}, nil
}

func exactIDSchema(target []translation.SegmentData) map[string]any {
	prefixItems := make([]any, 0, len(target))
	for _, segment := range target {
		prefixItems = append(prefixItems, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"id", "source_text", "text"},
			"properties": map[string]any{
				"id": map[string]any{
					"type":  "integer",
					"const": segment.ID,
				},
				"source_text": map[string]any{
					"type":  "string",
					"const": segment.SourceText,
				},
				"text": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
			},
		})
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"translations"},
		"properties": map[string]any{
			"translations": map[string]any{
				"type":        "array",
				"minItems":    len(target),
				"maxItems":    len(target),
				"prefixItems": prefixItems,
			},
		},
	}
}

func boundarySchema(allowedIDs []int) map[string]any {
	enum := make([]any, 0, len(allowedIDs))
	for _, id := range allowedIDs {
		enum = append(enum, id)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"split_after_id"},
		"properties": map[string]any{
			"split_after_id": map[string]any{
				"type": "integer",
				"enum": enum,
			},
		},
	}
}

type boundaryPlannerPayload struct {
	Task       string            `json:"task"`
	Segments   []boundarySegment `json:"segments"`
	Rule       string            `json:"rule"`
	Constraint string            `json:"constraint"`
	Output     map[string]string `json:"output"`
}

type boundarySegment struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

type boundaryPlannerResponse struct {
	SplitAfterID int `json:"split_after_id"`
}

func toBoundarySegments(segments []srt.Segment) []boundarySegment {
	data := make([]boundarySegment, len(segments))
	for i, segment := range segments {
		data[i] = boundarySegment{
			ID:   segment.ID,
			Text: strings.Join(segment.Lines, " "),
		}
	}
	return data
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    *float64       `json:"temperature,omitempty"`
	MaxTokens      int            `json:"max_tokens"`
	ResponseFormat responseFormat `json:"response_format"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

var _ translation.Translator = (*Client)(nil)
var _ translation.TextCompleter = (*Client)(nil)
var _ translation.TextCompleterWithOptions = (*Client)(nil)
var _ chunker.BoundaryPlanner = (*Client)(nil)
