package tui

import (
	"maps"
	"slices"
	"unicode"

	"github.com/GregLahaye/wombat/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "a":
		m.shouldApply = true
		return m, tea.Quit

	case "p":
		m.shouldUpdate = true
		return m, tea.Quit

	case "tab", "shift+tab":
		if msg.String() == "tab" {
			m.activeTab = (m.activeTab + 1) % numTabs
		} else {
			m.activeTab = (m.activeTab + 4) % numTabs
		}
		m.scrollToCursor()

	case "1", "2", "3", "4", "5":
		m.activeTab = int(msg.String()[0]-'0') - 1
		m.scrollToCursor()

	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "g", "home":
		m.moveCursorTo(0)
	case "G", "end":
		vis := m.visibleItems()
		if len(vis) > 0 {
			m.moveCursorTo(len(vis) - 1)
		}
	case "ctrl+d":
		m.moveCursor(m.viewHeight() / 2)
	case "ctrl+u":
		m.moveCursor(-m.viewHeight() / 2)

	case "h", "left":
		if m.scopeCursor[m.activeTab] > 0 {
			m.scopeCursor[m.activeTab]--
		}
	case "l", "right":
		if m.scopeCursor[m.activeTab] < len(m.scopeNames)-1 {
			m.scopeCursor[m.activeTab]++
		}

	case " ":
		m.toggleScope()

	case "r":
		m.resetOverride()

	case "/":
		m.filtering = true
		m.filterText = ""

	case "n":
		if m.activeTab == tabPermissions {
			m.addingRule = true
			m.addRuleText = ""
			m.addRuleDeny = false
		}

	case "N":
		if m.activeTab == tabPermissions {
			m.addingRule = true
			m.addRuleText = ""
			m.addRuleDeny = true
		}

	case "d":
		m.deletePermissionRule()

	case "?":
		return m, m.startDig()

	case "esc":
		if m.filterText != "" {
			m.filterText = ""
			m.rebuildItems()
		}
	}

	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filterText = ""
		m.rebuildItems()
	case "enter":
		m.filtering = false
	case "backspace":
		if len(m.filterText) > 0 {
			runes := []rune(m.filterText)
			m.filterText = string(runes[:len(runes)-1])
			m.rebuildItems()
		}
	default:
		r := msg.Runes
		if len(r) == 1 && unicode.IsPrint(r[0]) {
			m.filterText += string(r)
			m.rebuildItems()
		}
	}
	return m, nil
}

func (m Model) updateAddRule(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addingRule = false
	case "enter":
		if m.addRuleText != "" {
			m.addPermissionRule(m.addRuleText, m.addRuleDeny)
		}
		m.addingRule = false
	case "backspace":
		if len(m.addRuleText) > 0 {
			runes := []rune(m.addRuleText)
			m.addRuleText = string(runes[:len(runes)-1])
		}
	default:
		r := msg.Runes
		if len(r) == 1 && unicode.IsPrint(r[0]) {
			m.addRuleText += string(r)
		}
	}
	return m, nil
}

func (m *Model) moveCursorTo(pos int) {
	vis := m.visibleItems()
	if len(vis) == 0 {
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(vis) {
		pos = len(vis) - 1
	}
	// Skip non-interactive headers — search forward first, backward as fallback.
	orig := pos
	for pos < len(vis) {
		item := m.items[m.activeTab][vis[pos]]
		if !item.IsHeader || item.IsCollapsible {
			m.cursor[m.activeTab] = pos
			m.scrollToCursor()
			return
		}
		pos++
	}
	// Forward search failed — try backward from original position.
	for pos = orig - 1; pos >= 0; pos-- {
		item := m.items[m.activeTab][vis[pos]]
		if !item.IsHeader || item.IsCollapsible {
			m.cursor[m.activeTab] = pos
			m.scrollToCursor()
			return
		}
	}
}

func (m *Model) moveCursor(delta int) {
	vis := m.visibleItems()
	if len(vis) == 0 {
		return
	}

	newPos := m.cursor[m.activeTab] + delta
	if newPos < 0 {
		newPos = 0
	}
	if newPos >= len(vis) {
		newPos = len(vis) - 1
	}

	// Skip non-interactive headers (step by ±1, not by delta).
	step := 1
	if delta < 0 {
		step = -1
	}
	for newPos >= 0 && newPos < len(vis) {
		item := m.items[m.activeTab][vis[newPos]]
		if !item.IsHeader || item.IsCollapsible {
			break
		}
		newPos += step
	}
	if newPos < 0 || newPos >= len(vis) {
		return
	}

	m.cursor[m.activeTab] = newPos
	m.scrollToCursor()
}

func (m *Model) scrollToCursor() {
	viewHeight := m.viewHeight()
	if m.cursor[m.activeTab] < m.viewOffset[m.activeTab] {
		m.viewOffset[m.activeTab] = m.cursor[m.activeTab]
	}
	if m.cursor[m.activeTab] >= m.viewOffset[m.activeTab]+viewHeight {
		m.viewOffset[m.activeTab] = m.cursor[m.activeTab] - viewHeight + 1
	}
}

func (m *Model) toggleScope() {
	vis := m.visibleItems()
	if m.cursor[m.activeTab] >= len(vis) {
		return
	}
	idx := vis[m.cursor[m.activeTab]]
	item := &m.items[m.activeTab][idx]

	if item.IsHeader && !item.IsCollapsible {
		return
	}

	if item.IsCollapsible {
		m.collapsed[item.Name] = !m.collapsed[item.Name]
		return
	}

	if len(m.scopeNames) == 0 {
		return
	}
	scopeName := m.scopeNames[m.scopeCursor[m.activeTab]]

	if item.Scopes[scopeName] {
		delete(item.Scopes, scopeName)
	} else {
		item.Scopes[scopeName] = true
	}

	// Global exclusivity: enabling global disables lower scopes;
	// enabling a lower scope disables global.
	if scopeName == "global" && item.Scopes["global"] {
		for _, s := range m.scopeNames {
			if s != "global" {
				delete(item.Scopes, s)
			}
		}
	} else if scopeName != "global" && item.Scopes[scopeName] && item.Scopes["global"] {
		delete(item.Scopes, "global")
	}

	m.syncItemToConfig(m.activeTab, item)
}

func (m *Model) syncItemToConfig(tab int, item *listItem) {
	scopes := scopeMapToSlice(item.Scopes)

	// Clear override when explicitly setting scopes.
	if tab == tabPlugins || tab == tabSkills || tab == tabAgents {
		delete(m.cfg.Overrides, item.Name)
	}

	switch tab {
	case tabPlugins:
		m.cfg.Plugins[item.Name] = config.ScopeSet{Enabled: scopes}

	case tabSkills:
		m.cfg.Skills[item.Name] = config.Item{Source: item.Source, Enabled: scopes}

	case tabAgents:
		m.cfg.Agents[item.Name] = config.Item{Source: item.Source, Enabled: scopes}

	case tabPermissions:
		var rules *[]config.PermissionRule
		if item.Section == "allow" {
			rules = &m.cfg.Permissions.Allow
		} else {
			rules = &m.cfg.Permissions.Deny
		}
		if item.RuleIndex < len(*rules) {
			(*rules)[item.RuleIndex].Scopes = scopes
		}

	case tabDefaults:
		src := m.cfg.Sources[item.Name]
		src.DefaultScope = scopes
		m.cfg.Sources[item.Name] = src
		// Full rebuild: changing default_scope affects Skills/Agents tabs,
		// including their filter indices which would otherwise go stale.
		m.rebuildItems()
		return
	}

	m.dirty = !m.cfg.Equal(m.original)
}

func (m *Model) resetOverride() {
	if m.activeTab != tabSkills && m.activeTab != tabAgents {
		return
	}

	vis := m.visibleItems()
	if m.cursor[m.activeTab] >= len(vis) {
		return
	}
	idx := vis[m.cursor[m.activeTab]]
	item := m.items[m.activeTab][idx]

	if item.IsHeader {
		return
	}

	delete(m.cfg.Overrides, item.Name)
	if m.activeTab == tabSkills {
		delete(m.cfg.Skills, item.Name)
	} else {
		delete(m.cfg.Agents, item.Name)
	}

	m.rebuildItems()
}

func (m *Model) addPermissionRule(text string, deny bool) {
	rules := &m.cfg.Permissions.Allow
	if deny {
		rules = &m.cfg.Permissions.Deny
	}
	for _, r := range *rules {
		if r.Rule == text {
			return
		}
	}
	*rules = append(*rules, config.PermissionRule{Rule: text})
	m.rebuildItems()
}

func (m *Model) deletePermissionRule() {
	if m.activeTab != tabPermissions {
		return
	}

	vis := m.visibleItems()
	if m.cursor[m.activeTab] >= len(vis) {
		return
	}
	idx := vis[m.cursor[m.activeTab]]
	item := m.items[m.activeTab][idx]

	if item.IsHeader {
		return
	}

	rules := &m.cfg.Permissions.Allow
	if item.Section == "deny" {
		rules = &m.cfg.Permissions.Deny
	}
	*rules = slices.Delete(*rules, item.RuleIndex, item.RuleIndex+1)

	m.rebuildItems()
}

func scopeMapToSlice(m map[string]bool) []string {
	if len(m) == 0 {
		return []string{} // Explicitly empty, not nil (nil means "inherit").
	}
	return slices.Sorted(maps.Keys(m))
}
