package configs

import (
	"testing"

	"gopkg.in/yaml.v2"
)

// TestOverlay tests overlaying a nested map into the Config struct.
func TestOverlay(t *testing.T) {
	// Start with a default config.
	cfg := Config{
		Validation: Validation{
			NameRejectRegex: "test",
		},
	}

	newValues := map[string]any{
		"Validation": map[string]any{
			"NameRejectRegex": "test-changed",
		},
	}

	if err := cfg.OverlayOverrides(newValues); err != nil {
		t.Fatalf("Overlay failed: %v", err)
	}

	if cfg.Validation.NameRejectRegex != "test-changed" {
		t.Errorf("Expected NameRejectRegex to be \"test-changed\", got \"%s\"", cfg.Validation.NameRejectRegex)
	}
}

// TestOverlayDotMap tests overlaying a configuration using dot-syntax keys.
func TestOverlayDotMap(t *testing.T) {
	// Start with a default config.
	cfg := Config{
		Validation: Validation{
			NameRejectRegex: "test",
		},
	}

	dotValues := map[string]any{
		"Validation.NameRejectRegex": "test-changed",
	}

	if err := cfg.OverlayOverrides(dotValues); err != nil {
		t.Fatalf("OverlayDotMap failed: %v", err)
	}

	if cfg.Validation.NameRejectRegex != "test-changed" {
		t.Errorf("Expected LeaderboardSize to be \"test-changed\", got \"%s\"", cfg.Validation.NameRejectRegex)
	}
}

// TestOverlayDotMapMultipleFields demonstrates overlaying multiple fields using dot-syntax.
// Here, we extend the configuration to have an additional field.
func TestOverlayDotMapMultipleFields(t *testing.T) {
	// Define an extended configuration.
	type ExtendedStatistics struct {
		LeaderboardSize int    `yaml:"LeaderboardSize"`
		SomeField       string `yaml:"SomeField"`
	}

	type ExtendedConfig struct {
		Statistics ExtendedStatistics `yaml:"Statistics"`
	}

	cfg := ExtendedConfig{
		Statistics: ExtendedStatistics{
			LeaderboardSize: 5,
			SomeField:       "default",
		},
	}

	dotValues := map[string]any{
		"Statistics.LeaderboardSize": 25,
		"Statistics.SomeField":       "updated",
	}

	// Unflatten the dot-syntax map.
	nestedMap := unflattenMap(dotValues)
	// Marshal to YAML and then unmarshal into the extended config.
	b, err := yaml.Marshal(nestedMap)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Statistics.LeaderboardSize != 25 {
		t.Errorf("Expected LeaderboardSize to be 25, got %d", cfg.Statistics.LeaderboardSize)
	}
	if cfg.Statistics.SomeField != "updated" {
		t.Errorf("Expected SomeField to be 'updated', got '%s'", cfg.Statistics.SomeField)
	}
}
