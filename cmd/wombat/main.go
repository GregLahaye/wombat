package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/GregLahaye/wombat/internal/cli"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/tui"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root := &cobra.Command{
		Use:           "wombat",
		Short:         "Manage Claude Code skills, agents, plugins, and permissions.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.ConfigPath())
			if err != nil {
				return err
			}

			m := tui.New(cfg)
			p := tea.NewProgram(m, tea.WithAltScreen())
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}

			final, ok := finalModel.(tui.Model)
			if !ok {
				return fmt.Errorf("unexpected TUI model type: %T", finalModel)
			}

			if final.ShouldApply() || final.ShouldUpdate() {
				modified := final.Config()
				if err := modified.Save(config.ConfigPath()); err != nil {
					return fmt.Errorf("saving config: %w", err)
				}
				if final.ShouldUpdate() {
					cli.PullSources(modified)
				}
				return cli.RunApplyWithPrev(modified, final.OriginalConfig())
			}

			return nil
		},
	}

	root.AddGroup(
		&cobra.Group{ID: "core", Title: "Core Commands:"},
		&cobra.Group{ID: "manage", Title: "Manage:"},
		&cobra.Group{ID: "maintain", Title: "Maintain:"},
	)

	root.AddCommand(
		cli.ApplyCmd(),
		cli.PullCmd(),
		cli.InitCmd(),
		cli.ItemCmd("skill"),
		cli.ItemCmd("agent"),
		cli.ScopeCmd(),
		cli.SourceCmd(),
		cli.DoctorCmd(),
		cli.TidyCmd(),
	)

	return root.Execute()
}
