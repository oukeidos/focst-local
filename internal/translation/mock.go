package translation

import "context"

// MockClient is used by tests that exercise translator behavior.
type MockClient struct {
	Response              *ResponseData
	Error                 error
	LastSystemInstruction string
}

func (m *MockClient) Translate(ctx context.Context, request RequestData) (*ResponseData, error) {
	return m.Response, m.Error
}

func (m *MockClient) SetSystemInstruction(prompt string) {
	m.LastSystemInstruction = prompt
}
