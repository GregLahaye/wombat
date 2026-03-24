package tui

import (
	"maps"
	"slices"
	"strings"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
	"github.com/GregLahaye/wombat/internal/source"
)

func (m *Model) rebuildItems() {
	m.items[tabPlugins] = m.buildPluginItems()
	m.items[tabSkills] = m.buildSourcedItems("skill")
	m.items[tabAgents] = m.buildSourcedItems("agent")
	m.items[tabPermissions] = m.buildPermissionItems()
	m.items[tabDefaults] = m.buildDefaultItems()

	// Rebuild filter indices.
	for tab := 0; tab < numTabs; tab++ {
		if m.filterText != "" {
			filter := strings.ToLower(m.filterText)
			m.filterItems[tab] = nil
			for i, item := range m.items[tab] {
				if item.IsHeader {
					// Include header only if a subsequent child matches.
					hasMatch := false
					for j := i + 1; j < len(m.items[tab]); j++ {
						child := m.items[tab][j]
						if child.IsHeader {
							break
						}
						if strings.Contains(strings.ToLower(child.Name), filter) {
							hasMatch = true
							break
						}
					}
					if hasMatch {
						m.filterItems[tab] = append(m.filterItems[tab], i)
					}
				} else if strings.Contains(strings.ToLower(item.Name), filter) {
					m.filterItems[tab] = append(m.filterItems[tab], i)
				}
			}
		} else {
			m.filterItems[tab] = nil
		}
	}

	m.dirty = !m.cfg.Equal(m.original)
	m.clampCursors()
}

func (m *Model) clampCursors() {
	for tab := 0; tab < numTabs; tab++ {
		vis := m.visibleItemsForTab(tab)
		if len(vis) == 0 {
			m.cursor[tab] = 0
			m.viewOffset[tab] = 0
			continue
		}
		if m.cursor[tab] >= len(vis) {
			m.cursor[tab] = len(vis) - 1
		}
		if m.viewOffset[tab] >= len(vis) {
			m.viewOffset[tab] = len(vis) - 1
		}
	}
}

func (m *Model) buildPluginItems() []listItem {
	names := slices.Sorted(maps.Keys(m.cfg.Plugins))
	var items []listItem
	for _, name := range names {
		plugin := m.cfg.Plugins[name]
		scopes := scopeSliceToMap(plugin.Enabled)
		if override, ok := m.cfg.Overrides[name]; ok {
			scopes = scopeSliceToMap(override.Enabled)
		}
		items = append(items, listItem{Name: name, Scopes: scopes})
	}
	return items
}

func (m *Model) buildSourcedItems(kind string) []listItem {
	var items []listItem

	for _, srcName := range slices.Sorted(maps.Keys(m.cfg.Sources)) {
		src := m.cfg.Sources[srcName]

		var discovered []source.Discovered
		for _, d := range m.discovered[srcName] {
			if d.Kind == kind {
				discovered = append(discovered, d)
			}
		}

		if len(discovered) == 0 {
			continue
		}

		var sourceItems []listItem
		for _, d := range discovered {
			scopes, inherited := resolve.EffectiveScopes(m.cfg, d.Name, srcName, src.DefaultScope, kind)
			scopeMap := scopeSliceToMap(scopes)
			sourceItems = append(sourceItems, listItem{
				Name:        d.Name,
				Source:      srcName,
				Scopes:      scopeMap,
				IsInherited: inherited,
			})
		}

		items = append(items, listItem{
			Name:          srcName,
			IsHeader:      true,
			IsCollapsible: true,
			ChildCount:    len(sourceItems),
		})
		items = append(items, sourceItems...)
	}
	return items
}

func (m *Model) buildPermissionItems() []listItem {
	var items []listItem
	for _, section := range []struct {
		label string
		key   string
		rules []config.PermissionRule
	}{
		{"Allow", "allow", m.cfg.Permissions.Allow},
		{"Deny", "deny", m.cfg.Permissions.Deny},
	} {
		items = append(items, listItem{Name: section.label, IsHeader: true, Section: section.key})
		for i, rule := range section.rules {
			items = append(items, listItem{
				Name:      rule.Rule,
				Scopes:    scopeSliceToMap(rule.Scopes),
				Section:   section.key,
				RuleIndex: i,
			})
		}
	}
	return items
}

func (m *Model) buildDefaultItems() []listItem {
	var items []listItem
	for _, name := range slices.Sorted(maps.Keys(m.cfg.Sources)) {
		src := m.cfg.Sources[name]
		items = append(items, listItem{
			Name:   name,
			Source: name,
			Scopes: scopeSliceToMap(src.DefaultScope),
		})
	}
	return items
}

func (m *Model) visibleItems() []int {
	return m.visibleItemsForTab(m.activeTab)
}

func (m *Model) visibleItemsForTab(tab int) []int {
	var indices []int
	if m.filterText != "" && m.filterItems[tab] != nil {
		indices = m.filterItems[tab]
	} else {
		indices = make([]int, len(m.items[tab]))
		for i := range indices {
			indices[i] = i
		}
	}

	if tab == tabSkills || tab == tabAgents {
		indices = m.filterCollapsed(tab, indices)
	}

	return indices
}

func scopeSliceToMap(scopes []string) map[string]bool {
	m := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		m[s] = true
	}
	return m
}

func (m *Model) filterCollapsed(tab int, indices []int) []int {
	if m.filterText != "" {
		return indices
	}

	var result []int
	var currentSource string
	collapsed := false

	for _, idx := range indices {
		item := m.items[tab][idx]
		if item.IsCollapsible {
			currentSource = item.Name
			collapsed = m.collapsed[currentSource]
			result = append(result, idx)
			continue
		}
		if collapsed && item.Source == currentSource {
			continue
		}
		result = append(result, idx)
	}
	return result
}

