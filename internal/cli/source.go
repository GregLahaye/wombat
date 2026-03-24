package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/source"
	"github.com/spf13/cobra"
)

func SourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "source",
		Short:   "Manage sources",
		GroupID: "manage",
	}
	cmd.AddCommand(sourceAddCmd())
	cmd.AddCommand(sourceListCmd())
	cmd.AddCommand(sourceRemoveCmd())
	return cmd
}

func sourceAddCmd() *cobra.Command {
	var name string
	var defaultScope []string
	cmd := &cobra.Command{
		Use:   "add <git-url>",
		Short: "Add a source repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gitURL := args[0]
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if name == "" {
				name = sourceNameFromURL(gitURL)
			}
			if _, ok := cfg.Sources[name]; ok {
				return fmt.Errorf("source %q already exists", name)
			}

			for _, s := range defaultScope {
				if _, ok := cfg.Scopes[s]; !ok {
					return fmt.Errorf("unknown scope %q", s)
				}
			}

			srcDir := filepath.Join(config.SourcesDir(), name)
			fmt.Printf("Cloning %s...\n", name)
			if err := source.Clone(gitURL, srcDir); err != nil {
				return fmt.Errorf("cloning %s: %w", name, err)
			}

			src := config.Source{Git: gitURL, DefaultScope: defaultScope}
			skillPaths, agentPath := detectLayout(srcDir)
			if hasNonRootPath(skillPaths) {
				src.SkillPaths = skillPaths
			}
			if agentPath != "" {
				src.AgentPath = agentPath
			}
			cfg.Sources[name] = src

			if err := cfg.Save(config.ConfigPath()); err != nil {
				return err
			}
			fmt.Printf("Added source %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Source name (auto-detected from URL)")
	cmd.Flags().StringSliceVar(&defaultScope, "default-scope", nil, "Default scopes for items from this source")
	return cmd
}

func sourceRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Sources[name]; !ok {
				return fmt.Errorf("source %q not found", name)
			}

			// Check for references.
			var refs []string
			for itemName, item := range cfg.Skills {
				if item.Source == name {
					refs = append(refs, "skill:"+itemName)
				}
			}
			for itemName, item := range cfg.Agents {
				if item.Source == name {
					refs = append(refs, "agent:"+itemName)
				}
			}
			slices.Sort(refs)
			if len(refs) > 0 {
				return fmt.Errorf("source %q is referenced by: %s", name, strings.Join(refs, ", "))
			}

			delete(cfg.Sources, name)
			if err := cfg.Save(config.ConfigPath()); err != nil {
				return err
			}
			fmt.Printf("Removed source %q from config\n", name)
			fmt.Printf("Note: cloned data remains at %s\n", filepath.Join(config.SourcesDir(), name))
			return nil
		},
	}
}

func sourceListCmd() *cobra.Command {
	var checkUpdates bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if len(cfg.Sources) == 0 {
				fmt.Println("No sources configured. Add one with 'wombat source add <git-url>'.")
				return nil
			}

			sourcesDir := config.SourcesDir()

			for _, name := range cfg.SortedSourceNames() {
				src := cfg.Sources[name]
				dir := filepath.Join(sourcesDir, name)

				fmt.Printf("%s:\n", name)
				fmt.Printf("  git: %s\n", src.Git)
				if len(src.DefaultScope) > 0 {
					fmt.Printf("  default_scope: [%s]\n", strings.Join(src.DefaultScope, ", "))
				}
				if len(src.SkillPaths) > 0 {
					fmt.Printf("  skill_paths: [%s]\n", strings.Join(src.SkillPaths, ", "))
				}
				if src.AgentPath != "" {
					fmt.Printf("  agent_path: %s\n", src.AgentPath)
				}
				fmt.Printf("  path: %s\n", dir)

				if _, err := os.Stat(dir); os.IsNotExist(err) {
					fmt.Println("  status: not cloned")
				} else if checkUpdates {
					hasUpdates, err := source.HasUpdates(dir)
					if err != nil {
						fmt.Printf("  status: error: %v\n", err)
					} else if hasUpdates {
						fmt.Println("  status: updates available")
					} else {
						fmt.Println("  status: up to date")
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&checkUpdates, "check-updates", "c", false, "Check for remote updates")
	return cmd
}
