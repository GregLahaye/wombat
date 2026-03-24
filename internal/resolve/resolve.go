// Package resolve computes effective scopes for skills, agents, and plugins.
// This is pure logic with no I/O — fully testable.
package resolve

import (
	"maps"
	"slices"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/source"
)

// ResolvedItem is a skill or agent with its effective scopes.
type ResolvedItem struct {
	Name        string
	SourceName  string
	SourcePath  string // Relative path within source repo.
	Scopes      []string
	IsInherited bool // True if scopes come from source default_scope.
	Kind        string // "skill" or "agent"
}

// LinkName returns the filename to use for symlinks. Agents use .md extension;
// skills use the bare name (they are directories).
func (ri ResolvedItem) LinkName() string {
	if ri.Kind == "agent" {
		return ri.Name + ".md"
	}
	return ri.Name
}

// Items resolves all discovered skills and agents against the config.
// If includeAll is true, items with no effective scopes are included.
func Items(cfg *config.Config, discovered map[string][]source.Discovered, includeAll bool) (skills, agents []ResolvedItem) {
	resolved := make(map[string]bool)

	// Process sources in sorted order for determinism.
	for _, srcName := range slices.Sorted(maps.Keys(cfg.Sources)) {
		src := cfg.Sources[srcName]
		for _, item := range discovered[srcName] {
			if resolved[item.Name] {
				continue
			}
			resolved[item.Name] = true

			scopes, inherited := EffectiveScopes(cfg, item.Name, srcName, src.DefaultScope, item.Kind)
			if len(scopes) == 0 && !includeAll {
				continue
			}

			ri := ResolvedItem{
				Name:        item.Name,
				SourceName:  srcName,
				SourcePath:  item.Path,
				Scopes:      scopes,
				IsInherited: inherited,
				Kind:        item.Kind,
			}

			switch item.Kind {
			case "skill":
				skills = append(skills, ri)
			case "agent":
				agents = append(agents, ri)
			}
		}
	}

	// Add explicit entries not yet discovered.
	skills = addExplicit(cfg, skills, cfg.Skills, "skill", resolved, includeAll)
	agents = addExplicit(cfg, agents, cfg.Agents, "agent", resolved, includeAll)

	return skills, agents
}

// EffectiveScopes computes scopes for an item using the resolution chain:
// overrides > explicit entries > source default_scope.
func EffectiveScopes(cfg *config.Config, name, srcName string, defaultScope []string, kind string) (scopes []string, inherited bool) {
	// Priority 1: Overrides.
	if override, ok := cfg.Overrides[name]; ok {
		return override.Enabled, false
	}

	// Priority 2: Explicit entries.
	// nil Enabled means "absent" — fall through to default_scope.
	// empty slice means "explicitly disabled" — return immediately.
	switch kind {
	case "skill":
		if skill, ok := cfg.Skills[name]; ok && skill.Enabled != nil {
			return skill.Enabled, false
		}
	case "agent":
		if agent, ok := cfg.Agents[name]; ok && agent.Enabled != nil {
			return agent.Enabled, false
		}
	case "plugin":
		if plugin, ok := cfg.Plugins[name]; ok && plugin.Enabled != nil {
			return plugin.Enabled, false
		}
	}

	// Priority 3: Source default_scope.
	if len(defaultScope) > 0 {
		return defaultScope, true
	}

	return nil, false
}

func addExplicit(cfg *config.Config, resolved []ResolvedItem, items map[string]config.Item, kind string, seen map[string]bool, includeAll bool) []ResolvedItem {
	for _, name := range slices.Sorted(maps.Keys(items)) {
		if seen[name] {
			continue
		}
		seen[name] = true
		item := items[name]

		// Look up source default_scope for fall-through.
		var defaultScope []string
		if src, ok := cfg.Sources[item.Source]; ok {
			defaultScope = src.DefaultScope
		}

		scopes, inherited := EffectiveScopes(cfg, name, item.Source, defaultScope, kind)
		if len(scopes) == 0 && !includeAll {
			continue
		}
		resolved = append(resolved, ResolvedItem{
			Name:        name,
			SourceName:  item.Source,
			Scopes:      scopes,
			IsInherited: inherited,
			Kind:        kind,
		})
	}
	return resolved
}

