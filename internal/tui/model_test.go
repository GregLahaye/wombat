package tui

import (
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func testConfig() *config.Config {
	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: "/work/.claude", SettingsFile: "settings.local.json"},
			"global": {Path: "/global/.claude", SettingsFile: "settings.json"},
		},
		Plugins: map[string]config.ScopeSet{
			"plugin-a": {Enabled: []string{"global"}},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
				{Rule: "Write", Scopes: []string{"global"}},
			},
			Deny: []config.PermissionRule{
				{Rule: "Bash(rm:*)", Scopes: []string{"global"}},
			},
		},
	}
	cfg.EnsureMaps()
	return cfg
}

func TestNew_InitialState(t *testing.T) {
	m := New(testConfig())

	if m.activeTab != tabPlugins {
		t.Errorf("expected initial tab=Plugins, got %d", m.activeTab)
	}
	if m.dirty {
		t.Error("new model should not be dirty")
	}
	if m.shouldApply || m.shouldUpdate {
		t.Error("new model should not have pending actions")
	}
	if len(m.scopeNames) != 2 {
		t.Errorf("expected 2 scope names, got %d", len(m.scopeNames))
	}
	// Global should be last.
	if m.scopeNames[len(m.scopeNames)-1] != "global" {
		t.Errorf("expected global last, got %v", m.scopeNames)
	}
}

func pressKey(m Model, key string) Model {
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return result.(Model)
}

func pressSpecial(m Model, keyType tea.KeyType) Model {
	result, _ := m.Update(tea.KeyMsg{Type: keyType})
	return result.(Model)
}

func TestTabNavigation(t *testing.T) {
	m := New(testConfig())

	// Tab cycles forward.
	m = pressSpecial(m, tea.KeyTab)
	if m.activeTab != tabSkills {
		t.Errorf("expected Skills tab after tab, got %d", m.activeTab)
	}

	// Number keys jump directly.
	m = pressKey(m, "4")
	if m.activeTab != tabPermissions {
		t.Errorf("expected Permissions tab after '4', got %d", m.activeTab)
	}

	m = pressKey(m, "1")
	if m.activeTab != tabPlugins {
		t.Errorf("expected Plugins tab after '1', got %d", m.activeTab)
	}
}

func TestQuit(t *testing.T) {
	m := New(testConfig())
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	final := result.(Model)

	if final.ShouldApply() || final.ShouldUpdate() {
		t.Error("quit should not set apply or update")
	}
	if cmd == nil {
		t.Error("quit should return a command")
	}
}

func TestApplyAction(t *testing.T) {
	m := New(testConfig())
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	final := result.(Model)

	if !final.ShouldApply() {
		t.Error("'a' should set shouldApply")
	}
	if cmd == nil {
		t.Error("'a' should return quit command")
	}
}

func TestPullAction(t *testing.T) {
	m := New(testConfig())
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	final := result.(Model)

	if !final.ShouldUpdate() {
		t.Error("'p' should set shouldUpdate")
	}
}

func TestScopeCursorNavigation(t *testing.T) {
	m := New(testConfig())

	// Start at scope 0 (work, since global is last).
	if m.scopeCursor[m.activeTab] != 0 {
		t.Fatalf("expected initial scope cursor at 0")
	}

	// Move right.
	m = pressKey(m, "l")
	if m.scopeCursor[m.activeTab] != 1 {
		t.Error("expected scope cursor at 1 after 'l'")
	}

	// Can't go past the end.
	m = pressKey(m, "l")
	if m.scopeCursor[m.activeTab] != 1 {
		t.Error("scope cursor should not go past end")
	}

	// Move left.
	m = pressKey(m, "h")
	if m.scopeCursor[m.activeTab] != 0 {
		t.Error("expected scope cursor at 0 after 'h'")
	}

	// Can't go below 0.
	m = pressKey(m, "h")
	if m.scopeCursor[m.activeTab] != 0 {
		t.Error("scope cursor should not go below 0")
	}
}

func TestToggleScope_Plugin(t *testing.T) {
	m := New(testConfig())
	// Tab 1 is Plugins. plugin-a is enabled in [global].
	// Scope cursor starts at 0 (work).

	// Toggle work scope on for plugin-a.
	m = pressKey(m, " ")

	if !m.dirty {
		t.Error("toggling scope should make model dirty")
	}
	// Plugin should now be enabled in both work and... wait, global exclusivity.
	// Actually plugin-a was enabled in [global]. Enabling work should disable global.
	plugin := m.cfg.Plugins["plugin-a"]
	if len(plugin.Enabled) != 1 || plugin.Enabled[0] != "work" {
		t.Errorf("expected plugin enabled in [work] after toggling work (global exclusivity), got %v", plugin.Enabled)
	}
}

func TestFilter(t *testing.T) {
	m := New(testConfig())
	// Switch to permissions tab which has multiple items.
	m = pressKey(m, "4")

	// Enter filter mode.
	m = pressKey(m, "/")
	if !m.filtering {
		t.Fatal("expected filter mode after '/'")
	}

	// Type filter text.
	m = pressKey(m, "R")
	m = pressKey(m, "e")
	m = pressKey(m, "a")
	m = pressKey(m, "d")

	if m.filterText != "Read" {
		t.Errorf("expected filter text 'Read', got %q", m.filterText)
	}

	// Confirm filter.
	m = pressSpecial(m, tea.KeyEnter)
	if m.filtering {
		t.Error("expected filter mode off after enter")
	}
	if m.filterText != "Read" {
		t.Error("filter text should persist after enter")
	}

	// Clear filter with esc.
	m = pressSpecial(m, tea.KeyEscape)
	if m.filterText != "" {
		t.Error("filter text should be cleared after esc")
	}
}

func TestAddPermissionRule(t *testing.T) {
	m := New(testConfig())
	m = pressKey(m, "4") // Permissions tab

	origAllow := len(m.cfg.Permissions.Allow)

	// Press 'n' to add allow rule.
	m = pressKey(m, "n")
	if !m.addingRule {
		t.Fatal("expected add-rule mode after 'n'")
	}
	if m.addRuleDeny {
		t.Error("'n' should add allow rule, not deny")
	}

	// Type rule name.
	m = pressKey(m, "B")
	m = pressKey(m, "a")
	m = pressKey(m, "s")
	m = pressKey(m, "h")

	// Confirm.
	m = pressSpecial(m, tea.KeyEnter)
	if m.addingRule {
		t.Error("expected add-rule mode off after enter")
	}
	if len(m.cfg.Permissions.Allow) != origAllow+1 {
		t.Errorf("expected %d allow rules, got %d", origAllow+1, len(m.cfg.Permissions.Allow))
	}
	if m.cfg.Permissions.Allow[origAllow].Rule != "Bash" {
		t.Errorf("expected new rule 'Bash', got %q", m.cfg.Permissions.Allow[origAllow].Rule)
	}
}

func TestAddDenyRule(t *testing.T) {
	m := New(testConfig())
	m = pressKey(m, "4") // Permissions tab

	origDeny := len(m.cfg.Permissions.Deny)

	// Press 'N' for deny rule.
	m = pressKey(m, "N")
	if !m.addRuleDeny {
		t.Error("'N' should add deny rule")
	}

	m = pressKey(m, "X")
	m = pressSpecial(m, tea.KeyEnter)
	if len(m.cfg.Permissions.Deny) != origDeny+1 {
		t.Errorf("expected %d deny rules, got %d", origDeny+1, len(m.cfg.Permissions.Deny))
	}
}

func TestDeletePermissionRule(t *testing.T) {
	m := New(testConfig())
	m = pressKey(m, "4") // Permissions tab

	// Move cursor to first non-header item (first allow rule).
	m = pressKey(m, "j")

	origAllow := len(m.cfg.Permissions.Allow)
	m = pressKey(m, "d")
	if len(m.cfg.Permissions.Allow) != origAllow-1 {
		t.Errorf("expected %d allow rules after delete, got %d", origAllow-1, len(m.cfg.Permissions.Allow))
	}
}

func TestWindowResize(t *testing.T) {
	m := New(testConfig())
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)

	if m.width != 120 || m.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m.width, m.height)
	}
}

func testConfigWithSource() *config.Config {
	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: "/work/.claude", SettingsFile: "settings.local.json"},
			"global": {Path: "/global/.claude", SettingsFile: "settings.json"},
		},
		Sources: map[string]config.Source{
			"my-source": {Git: "https://example.com/repo", DefaultScope: []string{"work"}},
		},
		Plugins: map[string]config.ScopeSet{
			"plugin-a": {Enabled: []string{"global"}},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
			},
		},
	}
	cfg.EnsureMaps()
	return cfg
}

func TestDefaultsTab_ToggleWithFilterDoesNotPanic(t *testing.T) {
	cfg := testConfigWithSource()
	m := New(cfg)

	// Switch to Skills tab and set a filter.
	m = pressKey(m, "2")
	m = pressKey(m, "/")
	m = pressKey(m, "x") // filter that matches nothing
	m = pressSpecial(m, tea.KeyEnter)

	// Switch to Defaults tab and toggle a scope.
	m = pressKey(m, "5")
	m = pressKey(m, " ") // toggle scope on default_scope

	// Switch back to Skills tab — should not panic despite filter rebuild.
	m = pressKey(m, "2")
	_ = m.View() // This would panic with stale filter indices.
}

func TestRender_DoesNotPanic(t *testing.T) {
	m := New(testConfig())
	// Just ensure rendering doesn't panic with various states.
	_ = m.View()

	// With filter active.
	m = pressKey(m, "/")
	m = pressKey(m, "t")
	_ = m.View()

	// Confirm filter and render.
	m = pressSpecial(m, tea.KeyEnter)
	_ = m.View()

	// Each tab.
	for i := 1; i <= 5; i++ {
		m = pressKey(m, string(rune('0'+i)))
		_ = m.View()
	}
}
