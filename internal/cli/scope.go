package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/spf13/cobra"
)

func ScopeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "scope",
		Short:   "Manage scopes",
		GroupID: "manage",
	}
	cmd.AddCommand(scopeAddCmd())
	cmd.AddCommand(scopeListCmd())
	cmd.AddCommand(scopeRemoveCmd())
	return cmd
}

func scopeAddCmd() *cobra.Command {
	var settingsFile string
	cmd := &cobra.Command{
		Use:   "add <name> <path>",
		Short: "Add a scope",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, scopePath := args[0], args[1]

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Scopes[name]; ok {
				return fmt.Errorf("scope %q already exists", name)
			}

			scopePath = filepath.Clean(scopePath)
			if !strings.HasSuffix(scopePath, ".claude") {
				scopePath = filepath.Join(scopePath, ".claude")
			}

			expanded := config.ExpandPath(scopePath)
			if _, err := os.Stat(filepath.Dir(expanded)); os.IsNotExist(err) {
				fmt.Printf("Warning: parent directory %s does not exist\n", filepath.Dir(expanded))
			}

			sf := settingsFile
			if !cmd.Flags().Changed("settings-file") {
				sf = detectSettingsFile(expanded)
			}
			cfg.Scopes[name] = config.Scope{
				Path:         expanded,
				SettingsFile: sf,
			}

			if err := cfg.Save(config.ConfigPath()); err != nil {
				return err
			}
			fmt.Printf("Added scope %q: %s (%s)\n", name, config.ContractPath(expanded), sf)
			return nil
		},
	}
	cmd.Flags().StringVar(&settingsFile, "settings-file", "settings.local.json", "Settings file name")
	return cmd
}

func scopeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List scopes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			for _, name := range cfg.ScopeNames() {
				scope := cfg.Scopes[name]
				fmt.Printf("%s: %s (%s)\n", name, config.ContractPath(scope.Path), scope.SettingsFile)
			}
			return nil
		},
	}
}

func scopeRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a scope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Scopes[name]; !ok {
				return fmt.Errorf("scope %q not found", name)
			}

			if refs := cfg.ScopeRefs(name); len(refs) > 0 {
				return fmt.Errorf("scope %q is referenced by: %s", name, strings.Join(refs, ", "))
			}

			delete(cfg.Scopes, name)
			if err := cfg.Save(config.ConfigPath()); err != nil {
				return err
			}
			fmt.Printf("Removed scope %q\n", name)
			return nil
		},
	}
}
