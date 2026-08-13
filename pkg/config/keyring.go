// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const keyringService = "gz-git"

// TokenStore abstracts OS keychain access for forge API tokens.
type TokenStore interface {
	Set(provider, token string) error
	Get(provider string) (string, error)
	Delete(provider string) error
	Available() bool
}

// KeyringTokenStore uses the OS keychain via go-keyring.
// When the backend is unavailable (headless Linux without Secret Service),
// methods return ErrKeyringUnavailable and Available returns false.
type KeyringTokenStore struct {
	mu          sync.Mutex
	unavailable bool
}

// ErrKeyringUnavailable is returned when the OS keychain cannot be used.
var ErrKeyringUnavailable = fmt.Errorf("keyring unavailable")

// DefaultTokenStore is the process-wide token store (overridable in tests).
var DefaultTokenStore TokenStore = &KeyringTokenStore{}

// SetTokenStore replaces the default store (tests).
func SetTokenStore(s TokenStore) {
	if s == nil {
		DefaultTokenStore = &KeyringTokenStore{}
		return
	}
	DefaultTokenStore = s
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// Set stores a token for the provider in the OS keychain.
func (s *KeyringTokenStore) Set(provider, token string) error {
	provider = normalizeProvider(provider)
	if provider == "" {
		return fmt.Errorf("provider is required")
	}
	if token == "" {
		return fmt.Errorf("token is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := keyring.Set(keyringService, provider, token); err != nil {
		s.unavailable = true
		return fmt.Errorf("%w: %w", ErrKeyringUnavailable, err)
	}
	return nil
}

// Get retrieves a token for the provider from the OS keychain.
func (s *KeyringTokenStore) Get(provider string) (string, error) {
	provider = normalizeProvider(provider)
	if provider == "" {
		return "", fmt.Errorf("provider is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := keyring.Get(keyringService, provider)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		s.unavailable = true
		return "", fmt.Errorf("%w: %w", ErrKeyringUnavailable, err)
	}
	return tok, nil
}

// Delete removes a token for the provider from the OS keychain.
func (s *KeyringTokenStore) Delete(provider string) error {
	provider = normalizeProvider(provider)
	if provider == "" {
		return fmt.Errorf("provider is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := keyring.Delete(keyringService, provider); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		s.unavailable = true
		return fmt.Errorf("%w: %w", ErrKeyringUnavailable, err)
	}
	return nil
}

// Available reports whether the last keyring operation succeeded.
func (s *KeyringTokenStore) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.unavailable
}

// MemoryTokenStore is an in-memory TokenStore for tests.
type MemoryTokenStore struct {
	mu    sync.Mutex
	data  map[string]string
	avail bool
}

// NewMemoryTokenStore creates a test token store.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{data: make(map[string]string), avail: true}
}

// Set stores a token in memory, or reports ErrKeyringUnavailable while the
// store is switched off by SetAvailable.
func (m *MemoryTokenStore) Set(provider, token string) error {
	provider = normalizeProvider(provider)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.avail {
		return ErrKeyringUnavailable
	}
	m.data[provider] = token
	return nil
}

// Get returns the stored token, or "" with no error when the provider was
// never set — the same "absent is not a failure" contract KeyringTokenStore has.
func (m *MemoryTokenStore) Get(provider string) (string, error) {
	provider = normalizeProvider(provider)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.avail {
		return "", ErrKeyringUnavailable
	}
	return m.data[provider], nil
}

// Delete removes the provider's token, succeeding when there was none.
func (m *MemoryTokenStore) Delete(provider string) error {
	provider = normalizeProvider(provider)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.avail {
		return ErrKeyringUnavailable
	}
	delete(m.data, provider)
	return nil
}

// Available reports the flag SetAvailable last wrote, so a test can simulate a
// headless machine without a keychain.
func (m *MemoryTokenStore) Available() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.avail
}

// SetAvailable toggles availability for fallback tests.
func (m *MemoryTokenStore) SetAvailable(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.avail = v
}

// providerEnvVar maps forge provider → conventional env var for CI overrides.
func providerEnvVar(provider string) string {
	switch normalizeProvider(provider) {
	case "github":
		return "GITHUB_TOKEN"
	case "gitlab":
		return "GITLAB_TOKEN"
	case "gitea":
		return "GITEA_TOKEN"
	default:
		return ""
	}
}

// ResolveTokenFromEnv returns a token from environment variables for the provider.
// Checks provider-specific vars first, then GZ_GIT_TOKEN.
func ResolveTokenFromEnv(provider string) (token, source string) {
	if key := providerEnvVar(provider); key != "" {
		if v := os.Getenv(key); v != "" {
			return v, "env:" + key
		}
	}
	if v := os.Getenv("GZ_GIT_TOKEN"); v != "" {
		return v, "env:GZ_GIT_TOKEN"
	}
	return "", ""
}
