package localllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/oukeidos/focst-local/internal/apperrors"
	"github.com/oukeidos/focst-local/internal/httpclient"
	"github.com/oukeidos/focst-local/internal/translation"
)

type TextChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type textChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	TopP        float64       `json:"top_p"`
	TopK        int           `json:"top_k"`
	MaxTokens   int           `json:"max_tokens"`
}

// CompleteTextChat sends a plain chat-completion request without response_format.
// Phrase-anchor experiments used this exact shape for continuing chat rounds.
func (c *Client) CompleteTextChat(ctx context.Context, messages []TextChatMessage, maxTokens int) (*translation.TextCompletion, error) {
	return c.CompleteTextChatWithSampler(ctx, messages, maxTokens, DefaultTemperature, DefaultTopP, DefaultTopK)
}

// CompleteTextChatWithSampler is used by helper passes that need explicit
// sampling parameters while preserving a plain text response.
func (c *Client) CompleteTextChatWithSampler(ctx context.Context, messages []TextChatMessage, maxTokens int, temperature, topP float64, topK int) (*translation.TextCompletion, error) {
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	converted := make([]chatMessage, 0, len(messages))
	for _, msg := range messages {
		converted = append(converted, chatMessage{Role: msg.Role, Content: msg.Content})
	}
	payload := textChatCompletionRequest{
		Model:       c.model,
		Messages:    converted,
		Temperature: temperature,
		TopP:        topP,
		TopK:        topK,
		MaxTokens:   maxTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal local LLM text-chat request: %w", err)
	}
	return c.doPlainTextCompletion(ctx, body)
}

func (c *Client) doPlainTextCompletion(ctx context.Context, body []byte) (*translation.TextCompletion, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create local LLM text-chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := c.translationClient
	if client == nil {
		client = httpclient.NewClient(DefaultTranslationTimeout)
		c.translationClient = client
	}
	respBody, resp, err := httpclient.DoAndRead(client, httpReq)
	if err != nil {
		return nil, apperrors.Transient(fmt.Errorf("local LLM text-chat request failed: %w", err))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apperrors.Validation(fmt.Errorf("local LLM text-chat status=%s body=%s", resp.Status, string(respBody)))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, apperrors.Validation(fmt.Errorf("failed to decode local LLM text-chat response: %w", err))
	}
	if len(completion.Choices) == 0 {
		return nil, apperrors.Validation(fmt.Errorf("local LLM text-chat response had no choices"))
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
