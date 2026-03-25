package cli

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/source"
	"github.com/spf13/cobra"
)

func InitCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "init",
		Short:   "Initialize wombat from existing Claude Code setup",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}
}

func runInit() error {
	cfgPath := config.ConfigPath()
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("config already exists at %s", cfgPath)
	}

	scanner := bufio.NewScanner(os.Stdin)
	cfg := &config.Config{}
	cfg.EnsureMaps()

	fmt.Println("Welcome to wombat! Let's set up your configuration.")
	fmt.Println()

	// Collect scopes.
	fmt.Println("Define your scopes (Claude Code settings directories).")
	fmt.Println()

	for {
		name := promptLine(scanner, "Scope name (empty to finish): ")
		if name == "" {
			break
		}
		path := promptLine(scanner, "Path (e.g., ~/work): ")
		if path == "" {
			continue
		}
		if !strings.HasSuffix(path, ".claude") {
			path = filepath.Join(path, ".claude")
		}
		expanded := config.ExpandPath(path)
		settingsFile := detectSettingsFile(expanded)

		cfg.Scopes[name] = config.Scope{
			Path:         expanded,
			SettingsFile: settingsFile,
		}
		fmt.Printf("  Added scope %q: %s (%s)\n", name, path, settingsFile)
	}

	// Always add global scope.
	if _, ok := cfg.Scopes["global"]; !ok {
		home, _ := os.UserHomeDir()
		globalPath := filepath.Join(home, ".claude")
		cfg.Scopes["global"] = config.Scope{
			Path:         globalPath,
			SettingsFile: detectSettingsFile(globalPath),
		}
		fmt.Println("  Added global scope: ~/.claude")
	}

	// Scan existing setup.
	fmt.Println("\nScanning existing Claude Code setup...")

	type symlink struct {
		scope, name, kind, link, repoURL string
	}
	var allSymlinks []symlink

	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]
		// Import permissions and plugins from settings.
		settingsPath := filepath.Join(scope.Path, scope.SettingsFile)
		fileData, _ := apply.ReadSettings(settingsPath)

		permCount := importPermissions(cfg, fileData, scopeName)
		pluginCount := importPlugins(cfg, fileData, scopeName)
		if permCount > 0 || pluginCount > 0 {
			fmt.Printf("  %s: imported %d permissions, %d plugins\n", scopeName, permCount, pluginCount)
		}

		// Scan symlinks.
		for _, subdir := range []string{"skills", "agents"} {
			dir := filepath.Join(scope.Path, subdir)
			kind := "skill"
			if subdir == "agents" {
				kind = "agent"
			}
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
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				root, err := source.RepoRoot(target)
				if err != nil {
					continue
				}
				url, err := source.RemoteURL(root)
				if err != nil {
					continue
				}
				name := entry.Name()
				// Agent symlinks have .md extension; strip it for the config key
				// so it matches the discovered item name.
				if kind == "agent" {
					name = strings.TrimSuffix(name, ".md")
				}
				allSymlinks = append(allSymlinks, symlink{
					scope: scopeName, name: name, kind: kind,
					link: link, repoURL: strings.TrimSuffix(url, ".git"),
				})
			}
		}
	}

	// Group by repo and create sources.
	if len(allSymlinks) > 0 {
		fmt.Printf("\nFound %d existing symlinks.\n", len(allSymlinks))

		byRepo := make(map[string][]symlink)
		for _, s := range allSymlinks {
			byRepo[s.repoURL] = append(byRepo[s.repoURL], s)
		}

		sourcesDir := config.SourcesDir()
		for _, repoURL := range slices.Sorted(maps.Keys(byRepo)) {
			links := byRepo[repoURL]
			srcName := sourceNameFromURL(repoURL)
			srcDir := filepath.Join(sourcesDir, srcName)

			if _, err := os.Stat(srcDir); os.IsNotExist(err) {
				fmt.Printf("  Cloning %s...\n", srcName)
				if err := source.Clone(repoURL, srcDir); err != nil {
					fmt.Printf("  Warning: could not clone %s: %v\n", repoURL, err)
					continue
				}
			}

			skillPaths, agentPath := detectLayout(srcDir)
			src := config.Source{Git: repoURL}
			if hasNonRootPath(skillPaths) {
				src.SkillPaths = skillPaths
			}
			if agentPath != "" {
				src.AgentPath = agentPath
			}
			cfg.Sources[srcName] = src

			// Import discovered items into config.
			for _, s := range links {
				if s.kind == "skill" {
					if _, exists := cfg.Skills[s.name]; !exists {
						cfg.Skills[s.name] = config.Item{Source: srcName, Enabled: []string{s.scope}}
					}
				} else {
					if _, exists := cfg.Agents[s.name]; !exists {
						cfg.Agents[s.name] = config.Item{Source: srcName, Enabled: []string{s.scope}}
					}
				}
			}

			fmt.Printf("  Added source %q (%d items)\n", srcName, len(links))
		}
	}

	// Add default permissions if none imported.
	if len(cfg.Permissions.Allow) == 0 && len(cfg.Permissions.Deny) == 0 {
		addDefaultPermissions(cfg)
		fmt.Println("\nAdded default permission rules.")
	}

	// Validate before saving.
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("generated config is invalid: %w", err)
	}

	// Save.
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("\nConfiguration saved to %s\n", cfgPath)
	fmt.Println("Run 'wombat apply' to sync your setup.")

	return nil
}

// hasNonRootPath reports whether paths contains any non-empty entry,
// meaning skills exist in a subdirectory (not just the repo root).
func hasNonRootPath(paths []string) bool {
	for _, p := range paths {
		if p != "" {
			return true
		}
	}
	return false
}

func detectLayout(srcDir string) (skillPaths []string, agentPath string) {
	for _, dir := range []string{"", "skills", "workflows"} {
		items, _ := source.DiscoverSkills(srcDir, dir)
		if len(items) > 0 {
			skillPaths = append(skillPaths, dir)
		}
	}
	for _, dir := range []string{"agents", ""} {
		items, _ := source.DiscoverAgents(srcDir, dir)
		if len(items) > 0 {
			agentPath = dir
			break
		}
	}
	return
}

func addDefaultPermissions(cfg *config.Config) {
	global := []string{"global"}
	cfg.Permissions.Allow = []config.PermissionRule{
		{Rule: "Read", Scopes: global},
		{Rule: "Edit", Scopes: global},
		{Rule: "Write", Scopes: global},
		{Rule: "Bash", Scopes: global},
		{Rule: "WebSearch", Scopes: global},
	}
	cfg.Permissions.Deny = []config.PermissionRule{
		{Rule: "Bash(git push --force:*)", Scopes: global},
		{Rule: "Bash(git reset --hard:*)", Scopes: global},
		{Rule: "Bash(git clean:*)", Scopes: global},
	}
}

func importPermissions(cfg *config.Config, data map[string]any, scopeName string) int {
	permsRaw, _ := data["permissions"].(map[string]any)
	if permsRaw == nil {
		return 0
	}
	count := 0
	for _, kind := range []string{"allow", "deny"} {
		rules := apply.ExtractStringSlice(permsRaw, kind)
		for _, rule := range rules {
			addPermRule(cfg, rule, kind, scopeName)
			count++
		}
	}
	return count
}

func importPlugins(cfg *config.Config, data map[string]any, scopeName string) int {
	plugins, _ := data["enabledPlugins"].(map[string]any)
	if plugins == nil {
		return 0
	}
	count := 0
	for name, v := range plugins {
		if enabled, ok := v.(bool); !ok || !enabled {
			continue
		}
		if p, ok := cfg.Plugins[name]; !ok {
			cfg.Plugins[name] = config.ScopeSet{Enabled: []string{scopeName}}
		} else if !slices.Contains(p.Enabled, scopeName) {
			p.Enabled = append(p.Enabled, scopeName)
			cfg.Plugins[name] = p
		}
		count++
	}
	return count
}

func addPermRule(cfg *config.Config, rule, kind, scopeName string) {
	var rules *[]config.PermissionRule
	if kind == "allow" {
		rules = &cfg.Permissions.Allow
	} else {
		rules = &cfg.Permissions.Deny
	}
	for i, r := range *rules {
		if r.Rule == rule {
			if !slices.Contains(r.Scopes, scopeName) {
				(*rules)[i].Scopes = append((*rules)[i].Scopes, scopeName)
			}
			return
		}
	}
	*rules = append(*rules, config.PermissionRule{Rule: rule, Scopes: []string{scopeName}})
}

func sourceNameFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	// Handle SSH URLs: git@github.com:owner/repo → owner/repo
	if i := strings.Index(url, ":"); i > 0 && !strings.Contains(url[:i], "/") {
		url = url[i+1:]
	}
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "-" + parts[len(parts)-1]
	}
	return parts[len(parts)-1]
}

func detectSettingsFile(scopePath string) string {
	for _, name := range []string{"settings.local.json", "settings.json"} {
		if _, err := os.Stat(filepath.Join(scopePath, name)); err == nil {
			return name
		}
	}
	return "settings.local.json"
}

func promptLine(scanner *bufio.Scanner, prompt string) string {
	fmt.Print(prompt)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}
