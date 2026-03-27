// Package doctor provides configuration health checks.
package doctor

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
	"github.com/GregLahaye/wombat/internal/source"
)

// Severity indicates the importance of a finding.
type Severity int

const (
	SevError   Severity = iota // Something is broken.
	SevWarning                 // Something is suboptimal.
)

// Finding is a single health check result.
type Finding struct {
	Severity Severity
	Message  string
	Details  []string // Optional sub-items explaining the finding.
	Hint     string   // Suggested fix, e.g. "run wombat apply".
}

// Check runs all local health checks and returns findings.
// It accepts pre-computed discovery data to avoid redundant DiscoverAll calls.
// Pass nil for discovered to have Check compute it.
func Check(cfg *config.Config, discovered map[string][]source.Discovered) []Finding {
	if discovered == nil {
		discovered = apply.DiscoverAll(cfg)
	}

	var findings []Finding
	sourcesDir := config.SourcesDir()

	// 1. Check sources exist and are git repos, and configured paths are valid.
	for _, name := range cfg.SortedSourceNames() {
		src := cfg.Sources[name]
		dir := filepath.Join(sourcesDir, name)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			findings = append(findings, Finding{Severity: SevError, Message: "source " + name + ": directory missing", Hint: "run wombat apply"})
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
			findings = append(findings, Finding{Severity: SevError, Message: "source " + name + ": not a git repository", Hint: "run wombat apply"})
			continue
		}
		for _, sp := range src.SkillPaths {
			if sp == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, sp)); os.IsNotExist(err) {
				findings = append(findings, Finding{Severity: SevWarning, Message: "source " + name + ": skill_path " + sp + " does not exist", Hint: "check config.yaml"})
			}
		}
		if src.AgentPath != "" {
			if _, err := os.Stat(filepath.Join(dir, src.AgentPath)); os.IsNotExist(err) {
				findings = append(findings, Finding{Severity: SevWarning, Message: "source " + name + ": agent_path " + src.AgentPath + " does not exist", Hint: "check config.yaml"})
			}
		}
	}

	// 2. Check for name collisions across sources.
	nameSource := make(map[string]string)
	for _, srcName := range cfg.SortedSourceNames() {
		for _, item := range discovered[srcName] {
			key := item.Kind + "\x00" + item.Name
			if first, exists := nameSource[key]; exists {
				findings = append(findings, Finding{Severity: SevWarning, Message: item.Kind + " " + item.Name + ": collision between " + first + " and " + srcName, Hint: "rename or remove one"})
			} else {
				nameSource[key] = srcName
			}
		}
	}

	// 3. Check symlinks.
	projDirs := apply.DiscoverAllProjectDirs(cfg)
	skills, agents := resolve.Items(cfg, discovered, false)
	desired := make(map[string]bool)

	addDesired := func(items []resolve.ResolvedItem, subdir string) {
		for _, item := range items {
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
				for _, projDir := range projDirs[scopeName] {
					desired[filepath.Join(projDir, subdir, item.LinkName())] = true
				}
			}
		}
	}
	addDesired(skills, "skills")
	addDesired(agents, "agents")

	for _, link := range slices.Sorted(maps.Keys(desired)) {
		if _, err := os.Lstat(link); os.IsNotExist(err) {
			findings = append(findings, Finding{Severity: SevError, Message: "missing symlink: " + link, Hint: "run wombat apply"})
		} else if _, err := os.Stat(link); os.IsNotExist(err) {
			findings = append(findings, Finding{Severity: SevError, Message: "dangling symlink: " + link, Hint: "run wombat apply"})
		}
	}

	// 4. Check for unmanaged symlinks.
	sourcesPrefix := filepath.Clean(sourcesDir) + string(filepath.Separator)
	checkUnmanaged := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
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
			target = filepath.Clean(target)
			if !strings.HasPrefix(target, sourcesPrefix) {
				continue
			}
			if !desired[link] {
				findings = append(findings, Finding{Severity: SevWarning, Message: "unmanaged symlink: " + link, Hint: "run wombat apply"})
			}
		}
	}

	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]
		for _, subdir := range []string{"skills", "agents"} {
			checkUnmanaged(filepath.Join(scope.Path, subdir))
		}
	}
	for scopeName := range cfg.Scopes {
		if scopeName == "global" {
			continue
		}
		for _, projDir := range projDirs[scopeName] {
			for _, subdir := range []string{"skills", "agents"} {
				checkUnmanaged(filepath.Join(projDir, subdir))
			}
		}
	}

	// 5. Check settings drift.
	drifts, _ := apply.CheckSettings(cfg, projDirs)
	for _, d := range drifts {
		findings = append(findings, Finding{Severity: SevWarning, Message: "scope " + d.Scope + ": settings drift", Details: d.Details, Hint: "run wombat apply"})
	}

	// 6. Check for unmanaged scope permissions.
	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]
		path := filepath.Join(scope.Path, scope.SettingsFile)
		data, err := apply.ReadSettings(path)
		if err != nil {
			continue
		}
		perms, _ := data["permissions"].(map[string]any)
		if perms == nil {
			continue
		}
		var unmanaged []string
		for _, kind := range []string{"allow", "deny"} {
			for _, rule := range apply.ExtractStringSlice(perms, kind) {
				if !isManagedPermission(cfg, kind, rule, scopeName) {
					unmanaged = append(unmanaged, fmt.Sprintf("%s %q", kind, rule))
				}
			}
		}
		slices.Sort(unmanaged)
		if len(unmanaged) > 0 {
			findings = append(findings, Finding{
				Severity: SevWarning,
				Message:  fmt.Sprintf("scope %s: %d unmanaged permission(s)", scopeName, len(unmanaged)),
				Details:  unmanaged,
				Hint:     "run wombat tidy",
			})
		}
	}

	return findings
}

// Summary returns a one-line summary of findings grouped by category.
// Returns empty string if no findings.
func Summary(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}

	categories := map[string]int{}
	for _, f := range findings {
		switch {
		case strings.Contains(f.Message, "directory missing"):
			categories["missing sources"]++
		case strings.HasPrefix(f.Message, "missing symlink:"):
			categories["missing symlinks"]++
		case strings.HasPrefix(f.Message, "dangling symlink:"):
			categories["dangling symlinks"]++
		case strings.HasSuffix(f.Message, "settings drift"):
			categories["settings drift"]++
		case strings.Contains(f.Message, "collision"):
			categories["collisions"]++
		case strings.Contains(f.Message, "unmanaged permission"):
			categories["unmanaged permissions"]++
		case strings.HasPrefix(f.Message, "unmanaged symlink:"):
			categories["unmanaged symlinks"]++
		case strings.HasSuffix(f.Message, "updates available"):
			categories["updates available"]++
		default:
			categories["other issues"]++
		}
	}

	var parts []string
	for _, key := range slices.Sorted(maps.Keys(categories)) {
		parts = append(parts, strconv.Itoa(categories[key])+" "+key)
	}

	return strings.Join(parts, " · ")
}

// HasErrors returns true if any finding is an error (not just a warning).
func HasErrors(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SevError {
			return true
		}
	}
	return false
}

// isManagedPermission returns true if the rule is in wombat's config for the given scope.
func isManagedPermission(cfg *config.Config, kind, rule, scopeName string) bool {
	var rules []config.PermissionRule
	if kind == "allow" {
		rules = cfg.Permissions.Allow
	} else {
		rules = cfg.Permissions.Deny
	}
	for _, r := range rules {
		if r.Rule == rule && slices.Contains(r.Scopes, scopeName) {
			return true
		}
	}
	return false
}
