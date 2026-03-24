package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
	"github.com/GregLahaye/wombat/internal/source"
	"github.com/spf13/cobra"
)

// ItemCmd creates a skill or agent command with add/list/remove subcommands.
func ItemCmd(kind string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     kind,
		Short:   fmt.Sprintf("Manage %ss", kind),
		GroupID: "manage",
	}
	cmd.AddCommand(itemAddCmd(kind))
	cmd.AddCommand(itemListCmd(kind))
	cmd.AddCommand(itemRemoveCmd(kind))
	return cmd
}

func itemAddCmd(kind string) *cobra.Command {
	var scope, gitURL string
	cmd := &cobra.Command{
		Use:   "add owner/repo/" + kind,
		Short: fmt.Sprintf("Add a %s", kind),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parts := strings.SplitN(args[0], "/", 3)
			if len(parts) < 3 {
				return fmt.Errorf("expected format: owner/repo/%s", kind)
			}
			owner, repo, name := parts[0], parts[1], parts[2]
			srcName := owner + "-" + repo
			if gitURL == "" {
				gitURL = fmt.Sprintf("https://github.com/%s/%s", owner, repo)
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Scopes[scope]; !ok {
				return fmt.Errorf("scope %q not found", scope)
			}

			srcDir := filepath.Join(config.SourcesDir(), srcName)
			if _, err := os.Stat(srcDir); os.IsNotExist(err) {
				fmt.Printf("Cloning %s...\n", srcName)
				if err := source.Clone(gitURL, srcDir); err != nil {
					return err
				}
			}

			if _, ok := cfg.Sources[srcName]; !ok {
				// Auto-detect layout for new sources.
				src := config.Source{Git: gitURL}
				skillPaths, agentPath := detectLayout(srcDir)
				if hasNonRootPath(skillPaths) {
					src.SkillPaths = skillPaths
				}
				if agentPath != "" {
					src.AgentPath = agentPath
				}
				cfg.Sources[srcName] = src
			}

			found := false
			if kind == "skill" {
				src := cfg.Sources[srcName]
				for _, sp := range src.SkillDirs() {
					items, _ := source.DiscoverSkills(srcDir, sp)
					for _, item := range items {
						if item.Name == name {
							found = true
							break
						}
					}
				}
			} else {
				items, _ := source.DiscoverAgents(srcDir, cfg.Sources[srcName].AgentPath)
				for _, item := range items {
					if item.Name == name {
						found = true
						break
					}
				}
			}
			if !found {
				return fmt.Errorf("%s %q not found in %s", kind, name, srcName)
			}

			items := cfg.Skills
			if kind == "agent" {
				items = cfg.Agents
			}
			if existing, ok := items[name]; ok {
				fmt.Printf("Updating existing %s %q (was source=%s scopes=%v)\n", kind, name, existing.Source, existing.Enabled)
			}
			items[name] = config.Item{Source: srcName, Enabled: []string{scope}}

			if err := cfg.Save(config.ConfigPath()); err != nil {
				return err
			}
			return runApply(cfg, nil)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Scope to enable in (required)")
	cmd.Flags().StringVar(&gitURL, "git", "", "Git URL (defaults to GitHub)")
	_ = cmd.MarkFlagRequired("scope")
	return cmd
}

func itemListCmd(kind string) *cobra.Command {
	var scopeFilter, sourceFilter string
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %ss", kind),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			var skills, agents []resolve.ResolvedItem
			if all {
				skills, agents = apply.ResolveAll(cfg)
			} else {
				skills, agents = apply.Resolve(cfg)
			}
			items := skills
			if kind == "agent" {
				items = agents
			}

			var filtered []resolve.ResolvedItem
			for _, item := range items {
				if scopeFilter != "" && !slices.Contains(item.Scopes, scopeFilter) {
					continue
				}
				if sourceFilter != "" && item.SourceName != sourceFilter {
					continue
				}
				filtered = append(filtered, item)
			}

			if len(filtered) == 0 {
				if all {
					fmt.Printf("No %ss found. Add sources with 'wombat source add'.\n", kind)
				} else {
					fmt.Printf("No %ss enabled. Use --all to see all discovered items.\n", kind)
				}
				return nil
			}

			bySource := make(map[string][]resolve.ResolvedItem)
			for _, item := range filtered {
				bySource[item.SourceName] = append(bySource[item.SourceName], item)
			}

			for _, srcName := range slices.Sorted(maps.Keys(bySource)) {
				fmt.Printf("%s:\n", srcName)
				for _, item := range bySource[srcName] {
					scopes := strings.Join(item.Scopes, ", ")
					if scopes == "" {
						scopes = "(none)"
					}
					fmt.Printf("  %s [%s]\n", item.Name, scopes)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scopeFilter, "scope", "", "Filter by scope")
	cmd.Flags().StringVar(&sourceFilter, "source", "", "Filter by source")
	cmd.Flags().BoolVar(&all, "all", false, "Include items with no scopes")
	return cmd
}

func itemRemoveCmd(kind string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: fmt.Sprintf("Remove a %s", kind),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			delete(cfg.Overrides, name)

			items := cfg.Skills
			if kind == "agent" {
				items = cfg.Agents
			}

			if _, ok := items[name]; ok {
				delete(items, name)
			} else {
				// Auto-discovered: disable by setting empty Enabled.
				found := false
				skills, agents := apply.Resolve(cfg)
				var resolved []resolve.ResolvedItem
				if kind == "skill" {
					resolved = skills
				} else {
					resolved = agents
				}
				for _, r := range resolved {
					if r.Name == name {
						items[name] = config.Item{Source: r.SourceName, Enabled: []string{}}
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("%s %q not found", kind, name)
				}
			}

			if err := cfg.Save(config.ConfigPath()); err != nil {
				return err
			}
			return runApply(cfg, nil)
		},
	}
}
