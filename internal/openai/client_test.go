package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Generate_Errors(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		responseBody   string
		expectedErrMsg string
		sensitiveMark  string
	}{
		{
			name:           "429 Too Many Requests",
			status:         http.StatusTooManyRequests,
			responseBody:   `{"error": {"message": "Rate limit reached: SECRET_SUBTITLE_LINE", "type": "rate_limit_error", "code": "rate_limit_exceeded"}}`,
			expectedErrMsg: "OpenAI API rate limit exceeded (429)",
			sensitiveMark:  "SECRET_SUBTITLE_LINE",
		},
		{
			name:           "401 Unauthorized",
			status:         http.StatusUnauthorized,
			responseBody:   `{"error": {"message": "Invalid API Key: SECRET_SUBTITLE_LINE", "type": "auth_error"}}`,
			expectedErrMsg: "OpenAI API authentication/authorization failed (401)",
			sensitiveMark:  "SECRET_SUBTITLE_LINE",
		},
		{
			name:           "500 Internal Server Error",
			status:         http.StatusInternalServerError,
			responseBody:   "server down SECRET_SUBTITLE_LINE",
			expectedErrMsg: "OpenAI server error (500)",
			sensitiveMark:  "SECRET_SUBTITLE_LINE",
		},
		{
			name:           "403 Forbidden",
			status:         http.StatusForbidden,
			responseBody:   "restricted SECRET_SUBTITLE_LINE",
			expectedErrMsg: "OpenAI API authentication/authorization failed (403)",
			sensitiveMark:  "SECRET_SUBTITLE_LINE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.responseBody)
			}))
			defer server.Close()

			client := NewClient("test-key", "test-model")
			client.baseURL = server.URL // Override baseURL for testing

			_, err := client.Generate(context.Background(), RequestData{})
			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error message to contain %q, got %q", tt.expectedErrMsg, err.Error())
			}
			if tt.sensitiveMark != "" && strings.Contains(err.Error(), tt.sensitiveMark) {
				t.Errorf("Expected error message to redact sensitive content, got %q", err.Error())
			}
		})
	}
}
