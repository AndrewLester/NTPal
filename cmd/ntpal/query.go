package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/AndrewLester/ntpal/internal/sugar"
	"github.com/AndrewLester/ntpal/pkg/ntp"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func handleQueryCommand(system *ntp.NTPSystem, query string) {
	m := queryCommandModel{system: system, address: query}
	m.resetProgress()

	if _, err := sugar.RunProgramWithErrors(m); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

var (
	textStyle = lipgloss.NewStyle().Inline(true).Bold(true).Foreground(lipgloss.Color("252")).Render
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render
)

const (
	padding  = 10
	maxWidth = 80
)

const messages = 5

var percentage float64 = 0
var result string

type queryCommandModel struct {
	progress progress.Model
	system   *ntp.NTPSystem
	address  string
	err      error
}

type ntpQueryMessage string
type ntpQueryError error
type progressUpdateMessage struct{}

func ntpQueryCommand(system *ntp.NTPSystem, address string) tea.Cmd {
	return func() tea.Msg {
		result, err := system.Query(address, messages)
		if err != nil {
			return ntpQueryError(err)
		}

		offsetString := strconv.FormatFloat(result.Offset, 'G', 5, 64)
		if result.Offset > 0 {
			offsetString = "+" + offsetString
		}
		delayString := strconv.FormatFloat(result.Err, 'G', 5, 64)
		addr, _ := net.ResolveIPAddr("ip", address)
		return ntpQueryMessage(fmt.Sprint(offsetString, " +/- ", delayString, " ", address, " ", addr.String()))
	}
}

func filterListenCommand(m queryCommandModel) tea.Cmd {
	return func() tea.Msg {
		<-m.system.ProgressFiltered
		return progressUpdateMessage{}
	}
}

func (m *queryCommandModel) resetProgress() {
	m.progress = progress.New(progress.WithScaledGradient("#68b1b1", "#6ea4ff"))
}

func (m queryCommandModel) Init() tea.Cmd {
	return tea.Batch(ntpQueryCommand(m.system, m.address), filterListenCommand(m))
}

func (m queryCommandModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - padding*2 - 4
		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}
		return m, nil
	case progressUpdateMessage:
		percentage += 1 / float64(messages)
		return m, filterListenCommand(m)
	case ntpQueryMessage:
		result = string(msg)
		return m, tea.Quit
	case ntpQueryError:
		m.err = msg
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m queryCommandModel) View() (s string) {
	if m.err != nil {
		return
	}

	if result == "" {
		s += textStyle("NTPal - Query") + "\n\n"
		s += m.progress.ViewAs(percentage) + "\n\n"
		s += helpStyle("q: exit\n")
	} else {
		s += result + "\n"
	}
	return
}

func (m queryCommandModel) GetError() error {
	return m.err
}
