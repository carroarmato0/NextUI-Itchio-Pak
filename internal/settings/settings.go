package settings

type Config struct {
	APIKey       string `json:"api_key"`
	ROMSelection string `json:"rom_selection"`
}

func Load(path string) (*Config, error)  { return nil, nil }
func (c *Config) Save(path string) error { return nil }
