package httpclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetDefaultClient(t *testing.T) {
	client := GetDefaultClient()
	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	expected := 10 * time.Minute
	if client.Timeout != expected {
		t.Errorf("Expected timeout to be %v, got %v", expected, client.Timeout)
	}

	if GetDefaultClient() != client {
		t.Errorf("Expected singleton client instance")
	}
}

func TestNewClient(t *testing.T) {
	customTimeout := 5 * time.Second
	client := NewClient(customTimeout)
	if client.Timeout != customTimeout {
		t.Errorf("Expected timeout to be %v, got %v", customTimeout, client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("Expected transport to be *http.Transport")
	}
	if transport.MaxIdleConns != MaxIdleConns {
		t.Errorf("Expected MaxIdleConns to be %d, got %d", MaxIdleConns, transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != MaxIdleConnsPerHost {
		t.Errorf("Expected MaxIdleConnsPerHost to be %d, got %d", MaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != IdleConnTimeout {
		t.Errorf("Expected IdleConnTimeout to be %v, got %v", IdleConnTimeout, transport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != TLSHandshakeTimeout {
		t.Errorf("Expected TLSHandshakeTimeout to be %v, got %v", TLSHandshakeTimeout, transport.TLSHandshakeTimeout)
	}
	if transport.ExpectContinueTimeout != ExpectContinueTimeout {
		t.Errorf("Expected ExpectContinueTimeout to be %v, got %v", ExpectContinueTimeout, transport.ExpectContinueTimeout)
	}
}

func TestDoAndRead(t *testing.T) {
	expectedBody := "hello world"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, expectedBody)
	}))
	defer server.Close()

	client := GetDefaultClient()
	req, _ := http.NewRequest("GET", server.URL, nil)

	body, resp, err := DoAndRead(client, req)
	if err != nil {
		t.Fatalf("DoAndRead failed: %v", err)
	}

	if string(body) != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
}

func TestDoAndReadError(t *testing.T) {
	// Request to invalid URL
	client := GetDefaultClient()
	req, _ := http.NewRequest("GET", "http://invalid.url.local", nil)

	_, _, err := DoAndRead(client, req)
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestDoAndReadTooLarge(t *testing.T) {
	oversized := make([]byte, MaxResponseBytes+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(oversized)))
		w.WriteHeader(http.StatusOK)
		w.Write(oversized)
	}))
	defer server.Close()

	client := GetDefaultClient()
	req, _ := http.NewRequest("GET", server.URL, nil)

	_, _, err := DoAndRead(client, req)
	if err == nil || !strings.Contains(err.Error(), "response body too large") {
		t.Fatalf("expected response body too large error, got: %v", err)
	}
}

func TestSetDefaultClientForTesting(t *testing.T) {
	custom := &http.Client{Timeout: 3 * time.Second}
	restore := SetDefaultClientForTesting(custom)
	defer restore()

	if GetDefaultClient() != custom {
		t.Fatalf("Expected overridden default client")
	}
}
