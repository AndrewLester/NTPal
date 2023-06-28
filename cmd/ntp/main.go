package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/AndrewLester/ntp/pkg/ntp"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const DEFAULT_CONFIG_PATH = "/etc/ntp.conf"
const DEFAULT_DRIFT_PATH = "/etc/ntp.drift"

var (
	// Available spinners
	spinners = []spinner.Spinner{
		spinner.Line,
		spinner.Dot,
		spinner.MiniDot,
		spinner.Jump,
		spinner.Pulse,
		spinner.Points,
		spinner.Globe,
		spinner.Moon,
		spinner.Monkey,
	}

	textStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render
)

func main() {
	var config string
	var drift string
	var query string
	flag.StringVar(&config, "config", DEFAULT_CONFIG_PATH, "Path to the NTP config file.")
	flag.StringVar(&drift, "drift", DEFAULT_DRIFT_PATH, "Path to the NTP drift file.")
	flag.StringVar(&query, "query", "", "Address to query.")
	flag.StringVar(&query, "q", query, "Address to query.")
	flag.Parse()

	port := os.Getenv("NTP_PORT")
	if port == "" {
		if query != "" {
			port = "0" // System will choose random port
		} else {
			port = "123"
		}
	}
	host := os.Getenv("NTP_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	system := ntp.NewNTPSystem(host, port, config, drift)

	if query != "" {
		m := model{}
		m.resetSpinner()

		if _, err := tea.NewProgram(m).Run(); err != nil {
			fmt.Println("could not run program:", err)
			os.Exit(1)
		}

		offset, delay := system.Query(query)
		addr, _ := net.ResolveIPAddr("ip", query)
		fmt.Println(offset, "+/-", delay, query, addr.String())
	} else {
		system.Start()
	}
}

type model struct {
	index   int
	spinner spinner.Model
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "h", "left":
			m.index--
			if m.index < 0 {
				m.index = len(spinners) - 1
			}
			m.resetSpinner()
			return m, m.spinner.Tick
		case "l", "right":
			m.index++
			if m.index >= len(spinners) {
				m.index = 0
			}
			m.resetSpinner()
			return m, m.spinner.Tick
		default:
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *model) resetSpinner() {
	m.spinner = spinner.New()
	m.spinner.Style = spinnerStyle
	m.spinner.Spinner = spinners[m.index]
}

func (m model) View() (s string) {
	var gap string
	switch m.index {
	case 1:
		gap = ""
	default:
		gap = " "
	}

	s += fmt.Sprintf("\n %s%s%s\n\n", m.spinner.View(), gap, textStyle("Spinning..."))
	s += helpStyle("h/l, ←/→: change spinner • q: exit\n")
	return
}
