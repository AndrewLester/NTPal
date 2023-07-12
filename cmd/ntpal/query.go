package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/AndrewLester/ntpal/internal/sugar"
	"github.com/AndrewLester/ntpal/internal/ui"
	"github.com/AndrewLester/ntpal/pkg/ntpal"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func handleQueryCommand(system *ntp.NTPalSystem, query string, messages int) {
	m := queryCommandModel{system: system, address: query, messages: messages}
	m.progress = progress.New(progress.WithScaledGradient("#68b1b1", "#6ea4ff"))

	if _, err := sugar.RunProgramWithErrors(m); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

const (
	padding  = 10
	maxWidth = 80
)

var percentage float64 = 0
var result string

type queryCommandModel struct {
	progress progress.Model
	system   *ntp.NTPSystem
	messages int
	address  string
	err      error
}

type ntpQueryMessage string
type ntpQueryError error
type progressUpdateMessage struct{}

func ntpQueryCommand(m queryCommandModel) tea.Cmd {
	return func() tea.Msg {
		result, err := m.system.Query(m.address, m.messages)
		if err != nil {
			return ntpQueryError(err)
		}

		offsetString := strconv.FormatFloat(result.Offset, 'G', 5, 64)
		if result.Offset > 0 {
			offsetString = "+" + offsetString
		}
		delayString := strconv.FormatFloat(result.Err, 'G', 5, 64)
		addr, _ := net.ResolveIPAddr("ip", m.address)
		return ntpQueryMessage(fmt.Sprint(offsetString, " +/- ", delayString, " ", m.address, " ", addr.String()))
	}
}

func filterListenCommand(m queryCommandModel) tea.Cmd {
	return func() tea.Msg {
		<-m.system.FilteredProgress
		return progressUpdateMessage{}
	}
}

func (m queryCommandModel) Init() tea.Cmd {
	return tea.Batch(ntpQueryCommand(m), filterListenCommand(m))
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
		s += ui.Title("NTPal - Query") + "\n"
		s += m.progress.ViewAs(percentage) + "\n\n"
		s += ui.Help("q: exit") + "\n"
	} else {
		s += result + "\n"
	}
	return
}

func (m queryCommandModel) GetError() error {
	return m.err
}
