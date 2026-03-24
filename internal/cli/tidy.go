package cli

import (
	"fmt"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/tidy"
	"github.com/spf13/cobra"
)

func TidyCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "tidy",
		Short:   "Consolidate project permissions into scopes",
		GroupID: "maintain",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			result, err := tidy.Scan(cfg)
			if err != nil {
				return err
			}

			if len(result.Recommendations) == 0 {
				fmt.Printf("Scanned %d project settings files. No consolidation needed.\n", result.Scanned)
				return nil
			}

			fmt.Printf("Scanned %d project settings files. Found %d recommendations:\n\n",
				result.Scanned, len(result.Recommendations))

			for _, rec := range result.Recommendations {
				fmt.Printf("  %s %s → scope %q\n", rec.Type, rec.Rule, rec.TargetScope)
				fmt.Printf("    %s\n", rec.Reason)
			}

			if !yes {
				fmt.Print("\nApply recommendations? [y/N] ")
				var answer string
				fmt.Scanln(&answer)
				if answer != "y" && answer != "Y" {
					return nil
				}
			}

			if err := tidy.ApplyRecommendations(cfg, result.Recommendations); err != nil {
				return err
			}

			if err := cfg.Save(config.ConfigPath()); err != nil {
				return err
			}

			r, err := apply.Apply(cfg, nil)
			if err != nil {
				return err
			}
			fmt.Print(apply.PrintResult(r))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Apply recommendations without prompting")
	return cmd
}
