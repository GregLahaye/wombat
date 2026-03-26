package tui

import (
	"os"
	"path/filepath"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/doctor"
	"github.com/GregLahaye/wombat/internal/source"
	tea "github.com/charmbracelet/bubbletea"
)

// remoteCheckMsg carries findings from async remote update checks.
type remoteCheckMsg struct {
	findings []doctor.Finding
}

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
	discovered      map[string][]source.Discovered // cached per source
	findings        []doctor.Finding              // health check results from startup
	checkingRemote bool                          // true while remote update check is in flight
	digPhase       digPhase                      // easter egg animation state
	digProgress    int                           // lines cleared/restored or chars revealed
	digHoldLeft    int                           // ticks remaining in hold phase
	digFact        string                        // random fact to display
	digLines       int                           // snapshotted line count for full animation
	digRevealed    []bool                        // which chars are visible (reveal phase)
	digRevealOrder []int                         // random order to reveal chars
	styles         styles
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
	m.checkingRemote = true

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
	h := m.height - 9 // title + tabs + status bar + scope headers + footer + borders
	if h < 1 {
		return 20
	}
	return h
}

func (m Model) Config() *config.Config        { return m.cfg }
func (m Model) OriginalConfig() *config.Config { return m.original }
func (m Model) ShouldApply() bool              { return m.shouldApply }
func (m Model) ShouldUpdate() bool             { return m.shouldUpdate }
func (m Model) Init() tea.Cmd                  { return m.checkRemoteUpdates() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case remoteCheckMsg:
		m.checkingRemote = false
		m.findings = append(m.findings, msg.findings...)
		return m, nil
	case digTickMsg:
		return m.updateDig()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.digPhase != digIdle {
			return m, nil // swallow keys during animation
		}
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

// checkRemoteUpdates returns a tea.Cmd that checks all sources for remote
// updates in the background. Results arrive as a remoteCheckMsg.
func (m Model) checkRemoteUpdates() tea.Cmd {
	sourcesDir := config.SourcesDir()
	type srcEntry struct {
		name string
		dir  string
	}
	var sources []srcEntry
	for _, name := range m.cfg.SortedSourceNames() {
		dir := filepath.Join(sourcesDir, name)
		if _, err := os.Stat(dir); err == nil {
			sources = append(sources, srcEntry{name, dir})
		}
	}
	if len(sources) == 0 {
		return func() tea.Msg { return remoteCheckMsg{} }
	}
	return func() tea.Msg {
		var findings []doctor.Finding
		for _, src := range sources {
			hasUpdates, err := source.HasUpdates(src.dir)
			if err != nil {
				findings = append(findings, doctor.Finding{
					Severity: doctor.SevWarning,
					Message:  "source " + src.name + ": could not check for updates",
				})
			} else if hasUpdates {
				findings = append(findings, doctor.Finding{
					Severity: doctor.SevWarning,
					Message:  "source " + src.name + ": updates available",
				})
			}
		}
		return remoteCheckMsg{findings}
	}
}
