package hosting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProviderConfigStore manages saved provider configurations on disk.
// Each provider's config is stored as a separate JSON file with restrictive
// permissions (0600) since the files contain API keys.
type ProviderConfigStore struct {
	configDir string
}

// NewConfigStore creates a new store rooted at configDir. The directory is
// created on first write if it does not already exist.
func NewConfigStore(configDir string) *ProviderConfigStore {
	return &ProviderConfigStore{configDir: configDir}
}

// configPath returns the file path for a provider's config file.
func (s *ProviderConfigStore) configPath(providerName string) string {
	return filepath.Join(s.configDir, providerName+".json")
}

// ensureDir creates the config directory if it does not exist.
func (s *ProviderConfigStore) ensureDir() error {
	return os.MkdirAll(s.configDir, 0700)
}

// Save writes a provider configuration to disk. It overwrites any existing
// config for the same provider.
func (s *ProviderConfigStore) Save(providerName string, cfg ProviderConfig) error {
	if providerName == "" {
		return fmt.Errorf("hosting: provider name must not be empty")
	}
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("hosting: create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("hosting: marshal config for %s: %w", providerName, err)
	}

	path := s.configPath(providerName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("hosting: write config for %s: %w", providerName, err)
	}
	return nil
}

// Load reads a provider configuration from disk.
func (s *ProviderConfigStore) Load(providerName string) (*ProviderConfig, error) {
	path := s.configPath(providerName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("hosting: no config found for provider %q", providerName)
		}
		return nil, fmt.Errorf("hosting: read config for %s: %w", providerName, err)
	}

	var cfg ProviderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("hosting: parse config for %s: %w", providerName, err)
	}
	return &cfg, nil
}

// Delete removes a provider's saved configuration.
func (s *ProviderConfigStore) Delete(providerName string) error {
	path := s.configPath(providerName)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // already gone
		}
		return fmt.Errorf("hosting: delete config for %s: %w", providerName, err)
	}
	return nil
}

// ListConfigured returns the names of all providers that have saved configs.
func (s *ProviderConfigStore) ListConfigured() ([]string, error) {
	entries, err := os.ReadDir(s.configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no directory means no configs
		}
		return nil, fmt.Errorf("hosting: list configs: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			names = append(names, strings.TrimSuffix(name, ".json"))
		}
	}
	return names, nil
}
