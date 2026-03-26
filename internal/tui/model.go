package tui

import (
	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/doctor"
	"github.com/GregLahaye/wombat/internal/source"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	tabPlugins     = 0
	tabSkills      = 1
	tabAgents      = 2
	tabPermissions = 3
	tabDefaults    = 4
	numTabs        = 5
)

var tabNames = []string{"Plugins", "Skills", "Agents", "Permissions", "Defaults"}

// listItem represents a row in any tab's list.
type listItem struct {
	Name          string
	Source        string
	Scopes        map[string]bool
	IsHeader      bool
	IsCollapsible bool
	IsInherited   bool
	Section       string // "allow" or "deny" for permissions tab.
	RuleIndex     int    // Index into permissions slice.
	ChildCount    int    // For collapsible headers.
}

// Model is the Bubbletea model for the wombat TUI.
type Model struct {
	cfg          *config.Config
	original     *config.Config
	scopeNames   []string
	activeTab    int
	items        [numTabs][]listItem
	cursor       [numTabs]int
	scopeCursor  [numTabs]int
	dirty        bool
	shouldApply  bool
	shouldUpdate bool
	filtering    bool
	filterText   string
	filterItems  [numTabs][]int
	addingRule   bool
	addRuleText  string
	addRuleDeny  bool
	width        int
	height       int
	viewOffset   [numTabs]int
	collapsed    map[string]bool
	discovered   map[string][]source.Discovered // cached per source
	findings     []doctor.Finding              // health check results from startup
	styles       styles
}

// New creates a TUI model from a config.
func New(cfg *config.Config) Model {
	m := Model{
		cfg:       cfg,
		original:  cfg.Clone(),
		collapsed: make(map[string]bool),
		styles:    newStyles(),
	}

	m.scopeNames = cfg.ScopeNames()
	m.discovered = apply.DiscoverAll(cfg)
	m.findings = doctor.Check(cfg, m.discovered)

	// Collapse sources without default_scope.
	for name, src := range cfg.Sources {
		if len(src.DefaultScope) == 0 {
			m.collapsed[name] = true
		}
	}

	m.rebuildItems()

	// Place cursors on first selectable item.
	for tab := 0; tab < numTabs; tab++ {
		vis := m.visibleItemsForTab(tab)
		for i, idx := range vis {
			if !m.items[tab][idx].IsHeader || m.items[tab][idx].IsCollapsible {
				m.cursor[tab] = i
				break
			}
		}
	}

	return m
}

// viewHeight returns the number of item rows visible in the viewport.
func (m Model) viewHeight() int {
	h := m.height - 8 // title + tabs + scope headers + footer + borders
	if len(m.findings) > 0 {
		h-- // status bar
	}
	if h < 1 {
		return 20
	}
	return h
}

func (m Model) Config() *config.Config        { return m.cfg }
func (m Model) OriginalConfig() *config.Config { return m.original }
func (m Model) ShouldApply() bool              { return m.shouldApply }
func (m Model) ShouldUpdate() bool             { return m.shouldUpdate }
func (m Model) Init() tea.Cmd                  { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.addingRule {
			return m.updateAddRule(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m Model) View() string {
	return m.render()
}
