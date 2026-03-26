// Package doctor provides configuration health checks.
package doctor

import (
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
			findings = append(findings, Finding{SevError, "source " + name + ": directory missing"})
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
			findings = append(findings, Finding{SevError, "source " + name + ": not a git repository"})
			continue
		}
		for _, sp := range src.SkillPaths {
			if sp == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, sp)); os.IsNotExist(err) {
				findings = append(findings, Finding{SevWarning, "source " + name + ": skill_path " + sp + " does not exist"})
			}
		}
		if src.AgentPath != "" {
			if _, err := os.Stat(filepath.Join(dir, src.AgentPath)); os.IsNotExist(err) {
				findings = append(findings, Finding{SevWarning, "source " + name + ": agent_path " + src.AgentPath + " does not exist"})
			}
		}
	}

	// 2. Check for name collisions across sources.
	nameSource := make(map[string]string)
	for _, srcName := range cfg.SortedSourceNames() {
		for _, item := range discovered[srcName] {
			key := item.Kind + "\x00" + item.Name
			if first, exists := nameSource[key]; exists {
				findings = append(findings, Finding{SevWarning, item.Kind + " " + item.Name + ": collision between " + first + " and " + srcName})
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
			findings = append(findings, Finding{SevError, "missing symlink: " + link})
		} else if _, err := os.Stat(link); os.IsNotExist(err) {
			findings = append(findings, Finding{SevError, "dangling symlink: " + link})
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
				findings = append(findings, Finding{SevWarning, "unmanaged symlink: " + link})
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
	drifted, _ := apply.CheckSettings(cfg, projDirs)
	slices.Sort(drifted)
	for _, name := range drifted {
		findings = append(findings, Finding{SevWarning, "scope " + name + ": settings drift"})
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
