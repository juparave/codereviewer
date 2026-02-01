package main

import (
	"fmt"
	"os"

	"github.com/juparave/codereviewer/internal/app"
	"github.com/juparave/codereviewer/internal/config"
	"github.com/spf13/cobra"
)

var (
	version  = "0.1.0"
	rootPath string
	cfgFile  string
	dryRun   bool
	verbose  bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "review",
		Short:   "Code Review Agent - Your personal senior engineer",
		Long:    `CRA performs nightly code reviews across Git repositories, identifying meaningful issues and delivering a concise daily report.`,
		Version: version,
		RunE:    run,
	}

	rootCmd.Flags().StringVarP(&rootPath, "root", "r", "", "Root path to scan for repositories (default: ~/projects)")
	rootCmd.Flags().StringVarP(&cfgFile, "config", "c", "", "Path to config file (default: ~/.config/cra/config.yaml)")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Scan repositories but don't send email")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with CLI flags
	if rootPath != "" {
		cfg.RootPath = rootPath
	}
	if dryRun {
		cfg.Email.Enabled = false
	}
	cfg.Verbose = verbose

	// Run the review
	runner := app.NewRunner(cfg)
	return runner.Run(cmd.Context())
}
