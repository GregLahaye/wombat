package tui

import (
	"fmt"
	"strings"

	"github.com/GregLahaye/wombat/internal/doctor"
	"github.com/mattn/go-runewidth"
)

const nameWidth = 30

func (m Model) render() string {
	var b strings.Builder

	title := "wombat"
	if m.dirty {
		title += m.styles.Dirty.Render(" *")
	}
	b.WriteString(m.styles.Title.Render(title))
	b.WriteString("\n")

	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		style := m.styles.Tab
		if i == m.activeTab {
			style = m.styles.ActiveTab
		}
		tabs = append(tabs, style.Render(label))
	}
	b.WriteString(m.styles.TabBar.Render(strings.Join(tabs, "  ")))
	b.WriteString("\n")

	// Status bar.
	if m.checkingRemote {
		if len(m.findings) > 0 {
			summary := doctor.Summary(m.findings)
			bar := "! " + summary + " — checking for updates…"
			style := m.styles.StatusBar
			if doctor.HasErrors(m.findings) {
				style = m.styles.StatusBarError
			}
			b.WriteString(style.Render(bar))
		} else {
			b.WriteString(m.styles.Dimmed.Render("  checking for updates…"))
		}
		b.WriteString("\n")
	} else if len(m.findings) > 0 {
		summary := doctor.Summary(m.findings)
		hint := m.statusHint()
		bar := "! " + summary + " — " + hint
		style := m.styles.StatusBar
		if doctor.HasErrors(m.findings) {
			style = m.styles.StatusBarError
		}
		b.WriteString(style.Render(bar))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

	// Scope headers.
	var headers []string
	headers = append(headers, padRight("", nameWidth))
	for i, name := range m.scopeNames {
		style := m.styles.ScopeHeader
		if i == m.scopeCursor[m.activeTab] {
			style = m.styles.CheckCursor
		}
		// Pad before styling so ANSI codes don't break width calculation.
		headers = append(headers, style.Render(padCenter(name, 8)))
	}
	b.WriteString(strings.Join(headers, ""))
	b.WriteString("\n")

	// Items.
	vis := m.visibleItems()
	viewHeight := m.viewHeight()
	start := m.viewOffset[m.activeTab]
	end := start + viewHeight
	if end > len(vis) {
		end = len(vis)
	}

	if len(vis) == 0 {
		b.WriteString(m.styles.Dimmed.Render("  No items"))
		b.WriteString("\n")
	}
	for viewIdx := start; viewIdx < end; viewIdx++ {
		idx := vis[viewIdx]
		item := m.items[m.activeTab][idx]
		selected := viewIdx == m.cursor[m.activeTab]

		b.WriteString(m.renderItem(item, selected))
		b.WriteString("\n")
	}

	if m.filtering {
		b.WriteString(m.styles.FilterPrompt.Render("/") + m.styles.FilterText.Render(m.filterText))
		b.WriteString("\n")
	} else if m.addingRule {
		prefix := "allow: "
		if m.addRuleDeny {
			prefix = "deny: "
		}
		b.WriteString(m.styles.FilterPrompt.Render(prefix) + m.styles.FilterText.Render(m.addRuleText))
		b.WriteString("\n")
	} else if m.filterText != "" {
		b.WriteString(m.styles.Dimmed.Render("filter: "+m.filterText+"  ") + m.styles.Footer.Render("esc to clear"))
		b.WriteString("\n")
	}

	footer := "q quit  a apply  p pull  spc toggle  / filter  g/G jump"
	if m.activeTab == tabSkills || m.activeTab == tabAgents {
		footer += "  r reset"
	}
	if m.activeTab == tabPermissions {
		footer += "  n/N add  d del"
	}
	b.WriteString(m.styles.Footer.Render(footer))

	return m.styles.Border.Render(b.String())
}

func (m Model) renderItem(item listItem, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	if item.IsHeader {
		if item.IsCollapsible {
			arrow := "▶"
			if !m.collapsed[item.Name] {
				arrow = "▼"
			}
			name := fmt.Sprintf("%s%s %s (%d)", cursor, arrow, item.Name, item.ChildCount)
			return m.styles.Header.Render(name)
		}
		if item.Section == "allow" {
			return m.styles.SectionAllow.Render(cursor + item.Name)
		}
		return m.styles.SectionDeny.Render(cursor + item.Name)
	}

	name := padRight(cursor+item.Name, nameWidth)
	style := m.styles.Normal
	if item.IsInherited {
		style = m.styles.Dimmed
	}
	if selected {
		style = m.styles.Selected
	}

	var line strings.Builder
	line.WriteString(style.Render(name))

	hasGlobal := item.Scopes["global"]
	for i, scopeName := range m.scopeNames {
		enabled := item.Scopes[scopeName]
		locked := hasGlobal && scopeName != "global"

		var check string
		if locked {
			check = " * "
		} else if enabled {
			check = " ✓ "
		} else {
			check = " · "
		}

		checkStyle := m.styles.CheckOff
		if enabled || locked {
			checkStyle = m.styles.CheckOn
		}
		if i == m.scopeCursor[m.activeTab] && selected {
			checkStyle = m.styles.CheckCursor
		}

		// Pad before styling so ANSI codes don't break width calculation.
		line.WriteString(checkStyle.Render(padCenter(check, 8)))
	}

	return line.String()
}

// statusHint returns the action hint for the status bar based on finding types.
func (m Model) statusHint() string {
	hasLocal, hasRemote := false, false
	for _, f := range m.findings {
		if strings.HasSuffix(f.Message, "updates available") {
			hasRemote = true
		} else {
			hasLocal = true
		}
	}
	switch {
	case hasLocal && hasRemote:
		return "p to pull"
	case hasRemote:
		return "p to pull"
	default:
		return "a to apply"
	}
}

func padRight(s string, width int) string {
	w := runewidth.StringWidth(s)
	if w >= width {
		return runewidth.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}

func padCenter(s string, width int) string {
	w := runewidth.StringWidth(s)
	if w >= width {
		return s
	}
	left := (width - w) / 2
	right := width - w - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}
