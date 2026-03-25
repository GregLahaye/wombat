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

func DoctorCmd() *cobra.Command {
	var verbose, offline bool
	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Check configuration health",
		GroupID: "maintain",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			noColor := os.Getenv("NO_COLOR") != ""
			errors, warnings := 0, 0

			report := func(level, msg string) {
				switch level {
				case "error":
					errors++
					if noColor {
						fmt.Printf("[ERROR] %s\n", msg)
					} else {
						fmt.Printf("✗ %s\n", msg)
					}
				case "warning":
					warnings++
					if noColor {
						fmt.Printf("[WARN]  %s\n", msg)
					} else {
						fmt.Printf("⚠ %s\n", msg)
					}
				default:
					if verbose {
						if noColor {
							fmt.Printf("[OK]    %s\n", msg)
						} else {
							fmt.Printf("✓ %s\n", msg)
						}
					}
				}
			}

			sourcesDir := config.SourcesDir()

			// Check sources exist and paths are valid.
			for _, name := range cfg.SortedSourceNames() {
				src := cfg.Sources[name]
				dir := filepath.Join(sourcesDir, name)
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					report("error", fmt.Sprintf("source %q: directory missing (run wombat apply)", name))
					continue
				}
				if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
					report("error", fmt.Sprintf("source %q: not a git repository", name))
					continue
				}
				report("info", fmt.Sprintf("source %q: ok", name))
				// Check configured paths exist within the source repo.
				for _, sp := range src.SkillPaths {
					if sp == "" {
						continue
					}
					if _, err := os.Stat(filepath.Join(dir, sp)); os.IsNotExist(err) {
						report("warning", fmt.Sprintf("source %q: skill_path %q does not exist", name, sp))
					}
				}
				if src.AgentPath != "" {
					if _, err := os.Stat(filepath.Join(dir, src.AgentPath)); os.IsNotExist(err) {
						report("warning", fmt.Sprintf("source %q: agent_path %q does not exist", name, src.AgentPath))
					}
				}
			}

			// Check for remote updates (requires network, skip with --offline).
			if !offline {
				for _, name := range cfg.SortedSourceNames() {
					dir := filepath.Join(sourcesDir, name)
					if _, err := os.Stat(dir); err != nil {
						continue
					}
					hasUpdates, err := source.HasUpdates(dir)
					if err != nil {
						report("warning", fmt.Sprintf("source %q: could not check for updates: %v", name, err))
					} else if hasUpdates {
						report("warning", fmt.Sprintf("source %q: updates available (run wombat pull)", name))
					}
				}
			}

			// Discover all items (once — reused for collision check and symlink check).
			discovered := apply.DiscoverAll(cfg)

			// Check for name collisions across sources (same kind only).
			nameSource := make(map[string]string) // "kind\x00name" -> first source
			for _, srcName := range cfg.SortedSourceNames() {
				for _, item := range discovered[srcName] {
					key := item.Kind + "\x00" + item.Name
					if first, exists := nameSource[key]; exists {
						report("warning", fmt.Sprintf("%s %q found in both %s and %s (using %s)", item.Kind, item.Name, first, srcName, first))
					} else {
						nameSource[key] = srcName
					}
				}
			}

			// Check symlinks.
			skills, agents := resolve.Items(cfg, discovered, false)
			desired := make(map[string]bool)
			addDesired := func(items []resolve.ResolvedItem, subdir string) {
				for _, item := range items {
					// Skip items with no SourcePath (manually configured but not
					// discovered). syncSymlinks also skips these — no symlink exists.
					if item.SourcePath == "" {
						continue
					}
					scopes := item.Scopes
					if slices.Contains(scopes, "global") {
						scopes = []string{"global"}
					}
					for _, scopeName := range scopes {
						scope := cfg.Scopes[scopeName]
						link := filepath.Join(scope.Path, subdir, item.LinkName())
						desired[link] = true
					}
				}
			}
			addDesired(skills, "skills")
			addDesired(agents, "agents")

			for _, link := range slices.Sorted(maps.Keys(desired)) {
				if _, err := os.Lstat(link); os.IsNotExist(err) {
					report("error", fmt.Sprintf("missing symlink: %s", link))
				} else if _, err := os.Stat(link); os.IsNotExist(err) {
					report("error", fmt.Sprintf("dangling symlink: %s", link))
				} else {
					report("info", fmt.Sprintf("symlink ok: %s", link))
				}
			}

			// Check for unmanaged symlinks.
			sourcesPrefix := filepath.Clean(sourcesDir) + string(filepath.Separator)
			for _, scopeName := range cfg.ScopeNames() {
				scope := cfg.Scopes[scopeName]
				for _, subdir := range []string{"skills", "agents"} {
					dir := filepath.Join(scope.Path, subdir)
					entries, err := os.ReadDir(dir)
					if err != nil {
						continue
					}
					for _, entry := range entries {
						link := filepath.Join(dir, entry.Name())
						target, err := os.Readlink(link)
						if err != nil {
							continue
						}
						// Resolve relative targets for comparison.
						if !filepath.IsAbs(target) {
							target = filepath.Join(dir, target)
						}
						target = filepath.Clean(target)
						if !strings.HasPrefix(target, sourcesPrefix) {
							continue
						}
						if !desired[link] {
							report("warning", fmt.Sprintf("unmanaged symlink: %s -> %s", link, target))
						}
					}
				}
			}

			// Check settings drift.
			drifted, _ := apply.CheckSettings(cfg)
			slices.Sort(drifted)
			for _, name := range drifted {
				report("warning", fmt.Sprintf("scope %q: settings drift (run wombat apply)", name))
			}

			if errors > 0 {
				return fmt.Errorf("%d errors, %d warnings", errors, warnings)
			}
			if warnings > 0 {
				fmt.Printf("\n%d warnings\n", warnings)
			} else {
				fmt.Println("\nAll checks passed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show passing checks")
	cmd.Flags().BoolVar(&offline, "offline", false, "Skip network checks (remote updates)")
	return cmd
}
