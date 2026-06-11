package llamaserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/oukeidos/focst-local/internal/httpclient"
)

type modelsResponse struct {
	Models []modelEntry `json:"models"`
	Data   []modelEntry `json:"data"`
}

type modelEntry struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Model   string   `json:"model"`
	Aliases []string `json:"aliases"`
	Meta    struct {
		NCtx int `json:"n_ctx"`
	} `json:"meta"`
}

func Probe(ctx context.Context, baseURL string) (Status, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return Status{}, err
	}
	body, resp, err := httpclient.DoAndRead(httpclient.GetDefaultClient(), req)
	if err != nil {
		return Status{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Status{}, fmt.Errorf("llama server models status=%s body=%s", resp.Status, string(body))
	}
	var decoded modelsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Status{}, fmt.Errorf("failed to decode llama server models response: %w", err)
	}
	entries := decoded.Data
	if len(entries) == 0 {
		entries = decoded.Models
	}
	status := Status{BaseURL: baseURL}
	for _, entry := range entries {
		info := ModelInfo{
			ID:      entry.ID,
			Name:    entry.Name,
			Model:   entry.Model,
			Aliases: append([]string(nil), entry.Aliases...),
			NCtx:    entry.Meta.NCtx,
		}
		if info.NCtx > status.NCtx {
			status.NCtx = info.NCtx
		}
		status.Models = append(status.Models, info)
	}
	return status, nil
}

func (s Status) HasModel(alias string) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return true
	}
	for _, model := range s.Models {
		if model.matches(alias) {
			return true
		}
	}
	return false
}

func (s Status) ModelLabels() []string {
	labels := make([]string, 0, len(s.Models))
	for _, model := range s.Models {
		label := firstNonEmpty(model.ID, model.Name, model.Model)
		if label == "" {
			label = "(unnamed)"
		}
		labels = append(labels, label)
	}
	return labels
}

func (m ModelInfo) matches(alias string) bool {
	if alias == m.ID || alias == m.Name || alias == m.Model {
		return true
	}
	for _, item := range m.Aliases {
		if alias == item {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
