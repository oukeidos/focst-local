package auth

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	serviceName   = "focst-local"
	openaiAccount = "openai-api-key"
	openaiEnvVar  = "OPENAI_API_KEY"
)

// GetKey retrieves the OpenAI API key.
// If allowEnv is false, environment variables are ignored.
func GetKey(service string, allowEnv bool) (string, string) {
	if service != "openai" {
		return "", ""
	}

	// 1. Try Keychain
	key, err := keyring.Get(serviceName, openaiAccount)
	if err == nil && key != "" {
		return strings.TrimSpace(key), "Keychain"
	}

	if allowEnv {
		// 2. Try Env Var (optional)
		key = os.Getenv(openaiEnvVar)
		if key != "" {
			return strings.TrimSpace(key), "Environment Variable"
		}
	}

	return "", ""
}

// SaveKey saves the key for a specific service to the OS Keychain.
func SaveKey(service, key string) error {
	if service != "openai" {
		return fmt.Errorf("unsupported key service: %s", service)
	}
	return keyring.Set(serviceName, openaiAccount, strings.TrimSpace(key))
}

// DeleteKey removes the key for a specific service from the OS Keychain.
func DeleteKey(service string) error {
	if service != "openai" {
		return fmt.Errorf("unsupported key service: %s", service)
	}
	return keyring.Delete(serviceName, openaiAccount)
}

// GetStatus returns whether a key exists for a specific service in the keychain.
func GetStatus(service string) bool {
	if service != "openai" {
		return false
	}
	key, err := keyring.Get(serviceName, openaiAccount)
	if err != nil || key == "" {
		return false
	}
	return true
}

// PromptForAPIKey securely prompts the user for their API key.
func PromptForAPIKey(prompt string) (string, error) {
	fmt.Print(prompt)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	fmt.Println() // Add newline after password input
	return strings.TrimSpace(string(bytePassword)), nil
}

// GetEnvKey retrieves the key from environment variables only.
func GetEnvKey(service string) (string, bool) {
	if service != "openai" {
		return "", false
	}
	key := strings.TrimSpace(os.Getenv(openaiEnvVar))
	if key == "" {
		return "", false
	}
	return key, true
}
