// Package apply synchronizes filesystem state to match the config.
package apply

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
	"github.com/GregLahaye/wombat/internal/source"
)

// Result tracks what changed during apply.
type Result struct {
	Created []string
	Removed []string
	Updated []string
	Errors  []error
}

// Apply syncs the filesystem to match cfg. prevCfg enables delta detection for cleanup.
func Apply(cfg, prevCfg *config.Config) (*Result, error) {
	r := &Result{}

	discovered := DiscoverAll(cfg)
	skills, agents := resolve.Items(cfg, discovered, false)

	syncSymlinks(cfg, skills, "skills", r)
	syncSymlinks(cfg, agents, "agents", r)

	if err := syncSettings(cfg, prevCfg, r); err != nil {
		return r, err
	}

	return r, nil
}

// Resolve returns resolved skills and agents without mutations.
func Resolve(cfg *config.Config) (skills, agents []resolve.ResolvedItem) {
	discovered := DiscoverAll(cfg)
	return resolve.Items(cfg, discovered, false)
}

// ResolveAll returns all items including those with no scopes.
func ResolveAll(cfg *config.Config) (skills, agents []resolve.ResolvedItem) {
	discovered := DiscoverAll(cfg)
	return resolve.Items(cfg, discovered, true)
}

// DiscoverAll scans all sources and returns discovered skills and agents.
func DiscoverAll(cfg *config.Config) map[string][]source.Discovered {
	discovered := make(map[string][]source.Discovered)
	sourcesDir := config.SourcesDir()

	for _, srcName := range slices.Sorted(maps.Keys(cfg.Sources)) {
		src := cfg.Sources[srcName]
		srcDir := filepath.Join(sourcesDir, srcName)

		var items []source.Discovered
		for _, sp := range src.SkillDirs() {
			skills, err := source.DiscoverSkills(srcDir, sp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: discovering skills in %s/%s: %v\n", srcName, sp, err)
				continue
			}
			items = append(items, skills...)
		}
		agents, err := source.DiscoverAgents(srcDir, src.AgentPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: discovering agents in %s: %v\n", srcName, err)
		} else {
			items = append(items, agents...)
		}

		discovered[srcName] = items
	}
	return discovered
}

// PrintResult formats the apply result as a human-readable string.
func PrintResult(r *Result) string {
	slices.Sort(r.Created)
	slices.Sort(r.Removed)
	slices.Sort(r.Updated)

	var b strings.Builder
	for _, path := range r.Created {
		fmt.Fprintf(&b, "  + %s\n", path)
	}
	for _, path := range r.Removed {
		fmt.Fprintf(&b, "  - %s\n", path)
	}
	for _, path := range r.Updated {
		fmt.Fprintf(&b, "  ~ %s\n", path)
	}
	for _, err := range r.Errors {
		fmt.Fprintf(&b, "  ! %s\n", err)
	}
	if len(r.Created) == 0 && len(r.Removed) == 0 && len(r.Updated) == 0 && len(r.Errors) == 0 {
		fmt.Fprintln(&b, "Everything up to date.")
	} else {
		fmt.Fprintf(&b, "Done: %d created, %d removed, %d updated, %d errors\n",
			len(r.Created), len(r.Removed), len(r.Updated), len(r.Errors))
	}
	return b.String()
}

