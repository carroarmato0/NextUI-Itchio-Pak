package settings

import (
	"encoding/json"
	"os"
)

// ParentalAdvisory holds the parental content filter configuration.
type ParentalAdvisory struct {
	MatureEnabled     bool     `json:"mature_enabled"`
	SensitiveEnabled  bool     `json:"sensitive_enabled"`
	SensitiveDisabled []string `json:"sensitive_disabled"` // tags individually turned off
}

type Config struct {
	APIKey       string           `json:"api_key"`
	ROMSelection string           `json:"rom_selection"`
	Parental     ParentalAdvisory `json:"parental"`
}

func defaults() *Config {
	return &Config{
		APIKey:       "",
		ROMSelection: "auto",
		Parental: ParentalAdvisory{
			MatureEnabled:    true,
			SensitiveEnabled: true,
		},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// File missing → return defaults (not an error)
		return defaults(), nil
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		// Corrupted file → return defaults (not an error)
		return defaults(), nil
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
