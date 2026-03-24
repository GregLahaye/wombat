package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

type styles struct {
	Border       lipgloss.Style
	Title        lipgloss.Style
	TabBar       lipgloss.Style
	ActiveTab    lipgloss.Style
	Tab          lipgloss.Style
	Footer       lipgloss.Style
	Header       lipgloss.Style
	Selected     lipgloss.Style
	Normal       lipgloss.Style
	Dimmed       lipgloss.Style
	ScopeHeader  lipgloss.Style
	CheckOn      lipgloss.Style
	CheckOff     lipgloss.Style
	CheckCursor  lipgloss.Style
	Dirty        lipgloss.Style
	FilterPrompt lipgloss.Style
	FilterText   lipgloss.Style
	SectionAllow lipgloss.Style
	SectionDeny  lipgloss.Style
}

func newStyles() styles {
	nc := os.Getenv("NO_COLOR") != ""

	s := styles{
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		Title:        lipgloss.NewStyle().Bold(true).Padding(0, 1),
		TabBar:       lipgloss.NewStyle().Padding(0, 1),
		ActiveTab:    lipgloss.NewStyle().Bold(true).Underline(true),
		Tab:          lipgloss.NewStyle(),
		Footer:       lipgloss.NewStyle().Padding(0, 1),
		Header:       lipgloss.NewStyle().Bold(true),
		Selected:     lipgloss.NewStyle().Bold(true),
		Normal:       lipgloss.NewStyle(),
		Dimmed:       lipgloss.NewStyle(),
		ScopeHeader:  lipgloss.NewStyle().Bold(true),
		CheckOn:      lipgloss.NewStyle(),
		CheckOff:     lipgloss.NewStyle(),
		CheckCursor:  lipgloss.NewStyle(),
		Dirty:        lipgloss.NewStyle().Bold(true),
		FilterPrompt: lipgloss.NewStyle(),
		FilterText:   lipgloss.NewStyle(),
		SectionAllow: lipgloss.NewStyle().Bold(true),
		SectionDeny:  lipgloss.NewStyle().Bold(true),
	}

	if !nc {
		subtle := lipgloss.Color("241")
		highlight := lipgloss.Color("212")
		green := lipgloss.Color("10")
		red := lipgloss.Color("9")
		yellow := lipgloss.Color("11")
		cyan := lipgloss.Color("14")
		dim := lipgloss.Color("240")

		s.Border = s.Border.BorderForeground(subtle)
		s.Title = s.Title.Foreground(highlight)
		s.ActiveTab = s.ActiveTab.Foreground(highlight)
		s.Tab = s.Tab.Foreground(subtle)
		s.Footer = s.Footer.Foreground(subtle)
		s.Dimmed = s.Dimmed.Foreground(dim)
		s.CheckOn = s.CheckOn.Foreground(green)
		s.CheckOff = s.CheckOff.Foreground(subtle)
		s.CheckCursor = s.CheckCursor.Foreground(cyan)
		s.Dirty = s.Dirty.Foreground(yellow)
		s.FilterPrompt = s.FilterPrompt.Foreground(highlight)
		s.SectionAllow = s.SectionAllow.Foreground(green)
		s.SectionDeny = s.SectionDeny.Foreground(red)
	}

	return s
}
