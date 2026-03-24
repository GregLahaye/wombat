package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/source"
	"github.com/spf13/cobra"
)

func ApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "apply",
		Short:   "Sync filesystem to config",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return runApply(cfg, nil)
		},
	}
}

func RunApplyWithPrev(cfg, prevCfg *config.Config) error {
	return runApply(cfg, prevCfg)
}

func runApply(cfg, prevCfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	sourcesDir := config.SourcesDir()
	for _, name := range cfg.SortedSourceNames() {
		src := cfg.Sources[name]
		dir := filepath.Join(sourcesDir, name)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Printf("Cloning %s...\n", name)
			if err := source.Clone(src.Git, dir); err != nil {
				return fmt.Errorf("cloning %s: %w", name, err)
			}
		}
	}

	r, err := apply.Apply(cfg, prevCfg)
	if err != nil {
		return err
	}
	fmt.Print(apply.PrintResult(r))
	return nil
}

func loadConfig() (*config.Config, error) {
	return config.Load(config.ConfigPath())
}
