package ui

import (
	"strconv"

	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	TextWhite = lipgloss.Color("252")
	TextGray  = lipgloss.Color("241")

	TableGray = lipgloss.Color("240")
)

// Text
var (
	Title      = lipgloss.NewStyle().Inline(true).Bold(true).Foreground(TextWhite).Render
	Help       = lipgloss.NewStyle().Inline(true).Foreground(TextGray).Render
	TableFloat = func(num float64) string { return strconv.FormatFloat(num, 'G', 5, 64) }
)

// Models
var (
	TableBase = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(TableGray).Render
)
