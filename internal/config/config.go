package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	RootPath string        `yaml:"root_path"`
	Email    EmailConfig   `yaml:"email"`
	Review   ReviewConfig  `yaml:"review"`
	Reports  ReportsConfig `yaml:"reports"`
	Verbose  bool          `yaml:"-"` // Set via CLI only
}

// EmailConfig holds email delivery settings
type EmailConfig struct {
	Enabled      bool   `yaml:"enabled"`
	SendAt       string `yaml:"send_at"`
	SMTPHost     string `yaml:"smtp_host"`
	SMTPPort     int    `yaml:"smtp_port"`
	SMTPUser     string `yaml:"smtp_user"`
	SMTPPassword string `yaml:"smtp_password"`
	FromAddress  string `yaml:"from_address"`
	FromName     string `yaml:"from_name"`
	ToAddress    string `yaml:"to_address"`
}

// ReviewConfig holds LLM review settings
type ReviewConfig struct {
	Strictness string `yaml:"strictness"` // low, medium, high
	Provider   string `yaml:"provider"`   // openai, googleai, vertexai, ollama
	Model      string `yaml:"model"`
	APIKey     string `yaml:"api_key"`
	BaseURL    string `yaml:"base_url"` // Custom API endpoint (for Zhipu AI, etc.)
}

// ReportsConfig holds report storage settings
type ReportsConfig struct {
	OutputDir string `yaml:"output_dir"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		RootPath: filepath.Join(homeDir, "projects"),
		Email: EmailConfig{
			Enabled:  true,
			SendAt:   "08:00",
			SMTPPort: 587,
			FromName: "Code Review Agent",
		},
		Review: ReviewConfig{
			Strictness: "medium",
			Provider:   "openai",
			Model:      "glm-4.7",
			BaseURL:    "https://api.z.ai/api/paas/v4",
		},
		Reports: ReportsConfig{
			OutputDir: "reports",
		},
	}
}

// Load reads configuration from file and merges with defaults
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	// Determine config file path
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return cfg, nil // Use defaults if can't find home
		}
		path = filepath.Join(homeDir, ".config", "cra", "config.yaml")
	}

	// Expand ~ in path
	path = expandPath(path)

	// Read config file if it exists
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults if file doesn't exist
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Expand paths
	cfg.RootPath = expandPath(cfg.RootPath)
	cfg.Reports.OutputDir = expandPath(cfg.Reports.OutputDir)

	return cfg, nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(homeDir, path[2:])
		}
	}
	return path
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.RootPath == "" {
		return fmt.Errorf("root_path is required")
	}

	if _, err := os.Stat(c.RootPath); os.IsNotExist(err) {
		return fmt.Errorf("root_path does not exist: %s", c.RootPath)
	}

	if c.Email.Enabled {
		if c.Email.SMTPHost == "" {
			return fmt.Errorf("smtp_host is required when email is enabled")
		}
		if c.Email.ToAddress == "" {
			return fmt.Errorf("to_address is required when email is enabled")
		}
	}

	if c.Review.APIKey == "" {
		// Check environment variable
		if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
			c.Review.APIKey = key
		} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			c.Review.APIKey = key
		}
	}

	return nil
}
