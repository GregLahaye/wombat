package tui

import (
	"math/rand"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	tea "github.com/charmbracelet/bubbletea"
)

// digTickMsg drives the dig animation.
type digTickMsg time.Time

type digPhase int

const (
	digIdle    digPhase = iota
	digDown             // items disappear bottom-up
	digReveal           // wombat art types in randomly
	digHold             // show complete wombat + fact
	digUp               // items reappear top-down
)

var wombatArt = []string{
	`                          ,.--""""--.._`,
	`                        ."     .'      ` + "`" + `-.`,
	`                       ;      ;           ;`,
	`                      '      ;             )`,
	`                     /     '             . ;`,
	`                    /     ;     ` + "`" + `.        ` + "`" + `;`,
	`                  ,.'     :         .     : )`,
	`                  ;|\'    :      ` + "`" + `./|) \  ;/`,
	`                  ;| \"  -,-   "-./ |;  ).;`,
	`                  /\/              \/   );`,
	`                 :                 \    ;`,
	`                 :     _      _     ;   )`,
	`                 ` + "`" + `.   \;\    /;/    ;  /`,
	`                   !    :   :     ,/  ;`,
	`                    (` + "`" + `. : _ : ,/""   ;`,
	`                     \\\` + "`" + `"^" ` + "`" + ` :    ;`,
	`                              (    )`,
	`                              ////`,
}

var wombatFacts = []string{
	"wombats dig ~3m of tunnel per night",
	"wombat poop is cube-shaped",
	"wombats can run 40km/h",
	"a group of wombats is called a wisdom",
	"wombats have backwards-facing pouches",
	"wombat burrows can be up to 200m long",
	"wombats have rodent-like teeth that never stop growing",
}

func digTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return digTickMsg(t)
	})
}

// contentWidth returns the visual width of item/scope-header lines.
// This is the width the border is pinned to, so all animation lines
// must not exceed it.
func (m Model) contentWidth() int {
	return nameWidth + len(m.scopeNames)*8
}

// startDig initialises the dig animation state and returns the first tick.
func (m *Model) startDig() tea.Cmd {
	m.digPhase = digDown
	m.digProgress = 0
	m.digFact = wombatFacts[rand.Intn(len(wombatFacts))]

	// Snapshot line count: at least enough for art + blank + fact.
	vis := m.visibleItems()
	n := len(vis) - m.viewOffset[m.activeTab]
	if n > m.viewHeight() {
		n = m.viewHeight()
	}
	minHeight := len(wombatArt) + 2
	if n < minHeight {
		n = minHeight
	}
	m.digLines = n

	return digTick()
}

func (m Model) updateDig() (tea.Model, tea.Cmd) {
	switch m.digPhase {
	case digDown:
		m.digProgress++
		if m.digProgress >= m.digLines {
			m.digPhase = digReveal
			m.digProgress = 0
			m.initRevealOrder()
			return m, digTick()
		}
		return m, digTick()
	case digReveal:
		batch := 12
		end := m.digProgress + batch
		if end > len(m.digRevealOrder) {
			end = len(m.digRevealOrder)
		}
		for i := m.digProgress; i < end; i++ {
			m.digRevealed[m.digRevealOrder[i]] = true
		}
		m.digProgress = end
		if m.digProgress >= len(m.digRevealOrder) {
			m.digPhase = digHold
			m.digHoldLeft = 25 // ~2 seconds
			return m, digTick()
		}
		return m, digTick()
	case digHold:
		m.digHoldLeft--
		if m.digHoldLeft <= 0 {
			m.digPhase = digUp
			m.digProgress = m.digLines
			return m, digTick()
		}
		return m, digTick()
	case digUp:
		m.digProgress--
		if m.digProgress <= 0 {
			m.digPhase = digIdle
			return m, nil
		}
		return m, digTick()
	}
	return m, nil
}

// initRevealOrder builds a shuffled list of non-space character indices.
func (m *Model) initRevealOrder() {
	total := 0
	for _, line := range wombatArt {
		total += len([]rune(line))
	}
	m.digRevealed = make([]bool, total)

	var nonSpace []int
	idx := 0
	for _, line := range wombatArt {
		for _, r := range line {
			if r == ' ' {
				m.digRevealed[idx] = true
			} else {
				nonSpace = append(nonSpace, idx)
			}
			idx++
		}
	}
	rand.Shuffle(len(nonSpace), func(i, j int) {
		nonSpace[i], nonSpace[j] = nonSpace[j], nonSpace[i]
	})
	m.digRevealOrder = nonSpace
}

// renderDig writes the animation frame into b.
// All lines are padded to contentWidth so the border stays stable.
func (m Model) renderDig(b *strings.Builder) {
	w := m.contentWidth()
	vis := m.visibleItems()
	start := m.viewOffset[m.activeTab]

	switch m.digPhase {
	case digDown, digUp:
		visible := m.digLines - m.digProgress
		if visible < 0 {
			visible = 0
		}
		for i := 0; i < m.digLines; i++ {
			if i < visible {
				viewIdx := start + i
				if viewIdx < len(vis) {
					idx := vis[viewIdx]
					item := m.items[m.activeTab][idx]
					selected := viewIdx == m.cursor[m.activeTab]
					b.WriteString(m.renderItem(item, selected))
				}
			} else {
				b.WriteString(m.styles.Dimmed.Render(strings.Repeat("░", w)))
			}
			b.WriteString("\n")
		}

	case digReveal:
		idx := 0
		linesWritten := 0
		for _, line := range wombatArt {
			var out []rune
			for _, r := range line {
				if idx < len(m.digRevealed) && m.digRevealed[idx] {
					out = append(out, r)
				} else {
					out = append(out, ' ')
				}
				idx++
			}
			b.WriteString(m.styles.WombatArt.Render(fixedWidth(string(out), w)))
			b.WriteString("\n")
			linesWritten++
		}
		// Blank + placeholder for fact.
		b.WriteString("\n")
		b.WriteString("\n")
		linesWritten += 2
		for i := linesWritten; i < m.digLines; i++ {
			b.WriteString("\n")
		}

	case digHold:
		linesWritten := 0
		for _, line := range wombatArt {
			b.WriteString(m.styles.WombatArt.Render(fixedWidth(line, w)))
			b.WriteString("\n")
			linesWritten++
		}
		b.WriteString("\n")
		b.WriteString(m.styles.WombatFact.Render("  " + m.digFact))
		b.WriteString("\n")
		linesWritten += 2
		for i := linesWritten; i < m.digLines; i++ {
			b.WriteString("\n")
		}
	}
}

// fixedWidth pads s with spaces to width, or truncates if wider.
// This ensures animation lines never exceed content width.
func fixedWidth(s string, width int) string {
	w := runewidth.StringWidth(s)
	if w == width {
		return s
	}
	if w > width {
		return runewidth.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-w)
}
