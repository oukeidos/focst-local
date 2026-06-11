package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/oukeidos/focst-local/internal/apperrors"
	"github.com/oukeidos/focst-local/internal/httpclient"
)

// RequestData represents the request body for OpenAI API
type RequestData struct {
	Model           string            `json:"model"`
	Input           []InputItem       `json:"input"`
	Tools           []Tool            `json:"tools,omitempty"`
	ToolChoice      any               `json:"tool_choice,omitempty"`
	Reasoning       *ReasoningOptions `json:"reasoning,omitempty"`
	Text            *TextOptions      `json:"text,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Include         []string          `json:"include,omitempty"`
}

type ReasoningOptions struct {
	Effort string `json:"effort,omitempty"`
}

type TextOptions struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

type InputItem struct {
	Type    string `json:"type"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ResponseData represents the simplified response body from OpenAI Responses API
type ResponseData struct {
	ID                string             `json:"id"`
	Status            string             `json:"status"`
	IncompleteDetails *IncompleteDetails `json:"incomplete_details,omitempty"`
	Output            []OutputItem       `json:"output"`
	Usage             Usage              `json:"usage"`
	WebSearchCount    int                `json:"-"`
}

type IncompleteDetails struct {
	Reason string `json:"reason"`
}

type OutputItem struct {
	Type    string            `json:"type"`
	Status  string            `json:"status,omitempty"`
	Role    string            `json:"role,omitempty"`
	Content []ResponseContent `json:"content,omitempty"`
}

type ResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Tool struct {
	Type string `json:"type"`
}

type ResponseFormat struct {
	Type   string `json:"type"`
	Name   string `json:"name,omitempty"`   // Required for Responses API structured outputs
	Strict bool   `json:"strict,omitempty"` // Required for Responses API structured outputs
	Schema any    `json:"schema,omitempty"` // Required for Responses API structured outputs
}

type Usage struct {
	InputTokens    int            `json:"input_tokens"`
	OutputTokens   int            `json:"output_tokens"`
	TotalTokens    int            `json:"total_tokens"`
	WebSearchCalls int            `json:"-"`
	InputDetails   *InputDetails  `json:"input_tokens_details,omitempty"`
	OutputDetails  *OutputDetails `json:"output_tokens_details,omitempty"`
}

type InputDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type OutputDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type errorEnvelope struct {
	Error errorDetails `json:"error"`
}

type errorDetails struct {
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Code    interface{} `json:"code"`
}

func (e errorDetails) codeString() string {
	if e.Code == nil {
		return ""
	}
	return fmt.Sprint(e.Code)
}

type Client struct {
	apiKey  string
	model   string
	baseURL string
}

func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.openai.com/v1",
	}
}

// GetModelID returns the configured model identifier.
func (c *Client) GetModelID() string {
	return c.model
}

func (c *Client) Generate(ctx context.Context, req RequestData) (*ResponseData, error) {
	req.Model = c.model

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := httpclient.GetDefaultClient()
	body, resp, err := httpclient.DoAndRead(client, httpReq)
	if err != nil {
		return nil, apperrors.New(
			apperrors.KindTransient,
			"OpenAI request failed due to a temporary network/runtime error.",
			fmt.Errorf("request failed: %w", err),
		)
	}

	if resp.StatusCode != http.StatusOK {
		details := parseErrorDetails(body)
		return nil, classifyOpenAIError(resp.StatusCode, resp.Status, details)
	}

	var result ResponseData
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, apperrors.New(
			apperrors.KindValidation,
			"OpenAI response format was invalid.",
			fmt.Errorf("failed to decode response: %w", err),
		)
	}

	slog.Debug("OpenAI API Response", "status", resp.Status, "usage_total", result.Usage.TotalTokens, "response_id", result.ID)

	// Count web search calls
	for _, item := range result.Output {
		if item.Type == "web_search_call" {
			result.Usage.WebSearchCalls++
		}
	}

	return &result, nil
}

func parseErrorDetails(body []byte) errorDetails {
	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errorDetails{}
	}
	return envelope.Error
}

func classifyOpenAIError(statusCode int, status string, details errorDetails) error {
	code := details.codeString()
	cause := fmt.Errorf("openai status=%s type=%s code=%s message=%s", status, details.Type, code, details.Message)

	switch statusCode {
	case http.StatusTooManyRequests:
		return apperrors.New(
			apperrors.KindRateLimit,
			"OpenAI API rate limit exceeded (429): please try again later. Automatic retry is disabled for cost control.",
			cause,
		)
	case http.StatusUnauthorized, http.StatusForbidden:
		return apperrors.New(
			apperrors.KindAuth,
			fmt.Sprintf("OpenAI API authentication/authorization failed (%d): please verify your API key and permissions.", statusCode),
			cause,
		)
	case http.StatusNotFound:
		if isOpenAIModelNotFound(details) {
			return apperrors.New(
				apperrors.KindBadRequest,
				"The model does not exist or you do not have access to it.",
				cause,
			)
		}
		return apperrors.New(
			apperrors.KindBadRequest,
			"OpenAI resource not found (404).",
			cause,
		)
	default:
		if statusCode >= 500 {
			return apperrors.New(
				apperrors.KindTransient,
				fmt.Sprintf("OpenAI server error (%d): please try again later.", statusCode),
				cause,
			)
		}
		return apperrors.New(
			apperrors.KindBadRequest,
			fmt.Sprintf("OpenAI API error (%d): %s", statusCode, status),
			cause,
		)
	}
}

func isOpenAIModelNotFound(details errorDetails) bool {
	needle := strings.ToLower(details.codeString() + " " + details.Type + " " + details.Message)
	if strings.Contains(needle, "model_not_found") {
		return true
	}
	return strings.Contains(needle, "does not exist or you do not have access to it")
}
