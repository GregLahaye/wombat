package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/source"
	"github.com/spf13/cobra"
)

func PullCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "pull",
		Short:   "Pull source updates and apply",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			PullSources(cfg)
			return runApply(cfg, nil)
		},
	}
}

// PullSources fetches updates for all configured sources that are already cloned.
// Missing sources are skipped — they will be cloned during apply.
func PullSources(cfg *config.Config) {
	sourcesDir := config.SourcesDir()
	for _, name := range cfg.SortedSourceNames() {
		dir := filepath.Join(sourcesDir, name)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		hasUpdates, err := source.HasUpdates(dir)
		if err != nil {
			fmt.Printf("  %s: could not check: %v\n", name, err)
			continue
		}
		if hasUpdates {
			fmt.Printf("Updating %s...\n", name)
			if err := source.Update(dir); err != nil {
				fmt.Printf("  %s: update failed: %v\n", name, err)
			}
		} else {
			fmt.Printf("  %s: up to date\n", name)
		}
	}
}
