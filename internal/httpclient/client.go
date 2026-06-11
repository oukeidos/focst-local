package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultTimeout is the default timeout for the centralized HTTP client.
	// We use 10 minutes to allow ample time for AI models to generate responses
	// while still preventing indefinite hangs.
	DefaultTimeout = 10 * time.Minute
	// MaxResponseBytes caps HTTP response bodies to prevent memory spikes.
	MaxResponseBytes = 8 * 1024 * 1024
	// Transport tuning for stable, long-lived connections.
	MaxIdleConns          = 100
	MaxIdleConnsPerHost   = 20
	IdleConnTimeout       = 120 * time.Second
	TLSHandshakeTimeout   = 30 * time.Second
	ExpectContinueTimeout = 2 * time.Second
)

var (
	defaultClient     *http.Client
	defaultClientOnce sync.Once
	overrideClient    *http.Client
)

// NewClient returns a new http.Client with the specified timeout.
func NewClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:          MaxIdleConns,
		MaxIdleConnsPerHost:   MaxIdleConnsPerHost,
		IdleConnTimeout:       IdleConnTimeout,
		TLSHandshakeTimeout:   TLSHandshakeTimeout,
		ExpectContinueTimeout: ExpectContinueTimeout,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// GetDefaultClient returns a standardized http.Client for use across the application.
func GetDefaultClient() *http.Client {
	if overrideClient != nil {
		return overrideClient
	}
	defaultClientOnce.Do(func() {
		defaultClient = NewClient(DefaultTimeout)
	})
	return defaultClient
}

// SetDefaultClientForTesting overrides the singleton client for tests.
// It returns a restore function to reset the previous client.
func SetDefaultClientForTesting(client *http.Client) func() {
	prevOverride := overrideClient
	overrideClient = client
	return func() {
		overrideClient = prevOverride
	}
}

// DoAndRead performs an HTTP request, reads the entire response body,
// ensures the body is closed, and returns the body content and the response object.
// This prevents resource leaks by always closing the response body.
func DoAndRead(client *http.Client, req *http.Request) ([]byte, *http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.ContentLength > MaxResponseBytes {
		return nil, resp, fmt.Errorf("response body too large (limit %d bytes)", MaxResponseBytes)
	}

	limited := &io.LimitedReader{R: resp.Body, N: MaxResponseBytes + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, resp, fmt.Errorf("failed to read response body: %w", err)
	}
	if int64(len(body)) > MaxResponseBytes {
		return nil, resp, fmt.Errorf("response body too large (limit %d bytes)", MaxResponseBytes)
	}

	return body, resp, nil
}
