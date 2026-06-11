package names

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/openai"
)

type Extractor struct {
	client *openai.Client
}

func NewExtractor(client *openai.Client) *Extractor {
	return &Extractor{client: client}
}

type CharacterMapping struct {
	Source string
	Target string
}

type ExtractionResult struct {
	Characters []CharacterMapping `json:"characters"`
	Usage      openai.Usage       `json:"-"`
}

func (e *Extractor) Extract(ctx context.Context, workType, title, year string, maxTokens int, sourceCode, targetCode string) ([]CharacterMapping, openai.Usage, error) {
	sourceLang, ok := language.GetLanguage(sourceCode)
	if !ok {
		return nil, openai.Usage{}, fmt.Errorf("unsupported source language: %s", sourceCode)
	}
	targetLang, ok := language.GetLanguage(targetCode)
	if !ok {
		return nil, openai.Usage{}, fmt.Errorf("unsupported target language: %s", targetCode)
	}
	sourceKey := sourceLang.Code
	targetKey := targetLang.Code

	prompt := fmt.Sprintf(`Search for the %s %s titled "%s" released in %s. 
Extract a list of major characters. For each character, provide their name in %s and its standard %s transliteration.
IMPORTANT: Return ONLY the name itself. Do NOT include any URLs, source links, brackets, or explanations.`,
		sourceLang.Name, workType, title, year, sourceLang.Name, targetLang.Name)

	if maxTokens <= 0 {
		maxTokens = 16384 // Default to 16k if not specified
	}

	req := openai.RequestData{
		Input: []openai.InputItem{
			{
				Type:    "message",
				Role:    "user",
				Content: prompt,
			},
		},
		Tools: []openai.Tool{
			{Type: "web_search"},
		},
		ToolChoice: "required", // Force tool use as requested
		Reasoning: &openai.ReasoningOptions{
			Effort: "medium",
		},
		Text: &openai.TextOptions{
			Format: &openai.ResponseFormat{
				Type:   "json_schema",
				Name:   "character_extraction",
				Strict: true,
				Schema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"characters": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									sourceKey: map[string]interface{}{
										"type":        "string",
										"description": "The name of the character in the source language. MUST contain ONLY the name, no URLs or comments.",
									},
									targetKey: map[string]interface{}{
										"type":        "string",
										"description": "Standard transliteration of the name. ONLY the name.",
									},
								},
								"required":             []string{sourceKey, targetKey},
								"additionalProperties": false,
							},
						},
					},
					"required":             []string{"characters"},
					"additionalProperties": false,
				},
			},
		},
		MaxOutputTokens: maxTokens,
	}

	resp, err := e.client.Generate(ctx, req)
	if err != nil {
		return nil, openai.Usage{}, err
	}

	if resp.Status == "incomplete" {
		reason := "unknown"
		if resp.IncompleteDetails != nil {
			reason = resp.IncompleteDetails.Reason
		}
		return nil, resp.Usage, fmt.Errorf("API response is incomplete (reason: %s). Try increasing MaxOutputTokens or reducing reasoning effort.", reason)
	}

	if len(resp.Output) == 0 {
		return nil, openai.Usage{}, fmt.Errorf("no output from API")
	}

	// Find assistant's message text
	var content string
	for _, item := range resp.Output {
		if item.Type == "message" && item.Role == "assistant" {
			for _, c := range item.Content {
				// Responses API uses "output_text" for the assistant's response content
				if c.Type == "output_text" {
					content = c.Text
					break
				}
			}
		}
		if content != "" {
			break
		}
	}

	if content == "" {
		return nil, openai.Usage{}, fmt.Errorf("no assistant text message found in output")
	}

	var raw struct {
		Characters []map[string]string `json:"characters"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, openai.Usage{}, fmt.Errorf("failed to parse character mapping: %w", err)
	}
	result := ExtractionResult{
		Characters: make([]CharacterMapping, 0, len(raw.Characters)),
		Usage:      resp.Usage,
	}

	for _, entry := range raw.Characters {
		srcVal, ok := entry[sourceKey]
		if !ok {
			return nil, openai.Usage{}, fmt.Errorf("missing source field %q in response", sourceKey)
		}
		tgtVal, ok := entry[targetKey]
		if !ok {
			return nil, openai.Usage{}, fmt.Errorf("missing target field %q in response", targetKey)
		}
		result.Characters = append(result.Characters, CharacterMapping{
			Source: cleanName(srcVal),
			Target: cleanName(tgtVal),
		})
	}

	return result.Characters, result.Usage, nil
}

// cleanName removes common noise patterns (URLs, brackets) from names.
func cleanName(name string) string {
	// 1. Remove URLs
	urlRegex := regexp.MustCompile(`https?://[^\s\]\)]+`)
	name = urlRegex.ReplaceAllString(name, "")

	// 2. Remove brackets containing domain-like text (e.g., [tbs.co.jp])
	domainBracketRegex := regexp.MustCompile(`\[[^\]]*\.[a-z]{2,}[^\]]*\]|\([^\)]*\.[a-z]{2,}[^\)]*\)`)
	name = domainBracketRegex.ReplaceAllString(name, "")

	// 3. Remove metadata-like brackets (e.g., (Voice), [Source])
	// Use a simpler heuristic: if it contains "http", "www", "com", "jp", "org", "net"
	// OR generic cleaning: just remove all [...] and (...) if that's acceptable.
	// Given the user concern is specifically about "source link annotations", removal is safer.
	bracketRegex := regexp.MustCompile(`\[.*?\]|\(.*?\)|<.*?>`)
	name = bracketRegex.ReplaceAllString(name, "")

	return strings.TrimSpace(name)
}
