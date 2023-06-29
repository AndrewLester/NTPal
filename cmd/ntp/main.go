package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/AndrewLester/ntp/pkg/ntp"

	"github.com/charmbracelet/bubbles/progress"
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

	textStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render
)

const (
	padding  = 2
	maxWidth = 80
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
		m := model{system: system, address: query}
		m.resetProgress()

		if _, err := tea.NewProgram(m).Run(); err != nil {
			fmt.Println("could not run program:", err)
			os.Exit(1)
		}

	} else {
		system.Start()
	}
}

func ntpQueryCommand(system *ntp.NTPSystem, address string) tea.Cmd {
	return func() tea.Msg {
		offset, delay := system.Query(address)
		addr, _ := net.ResolveIPAddr("ip", address)
		return ntpQueryMessage(fmt.Sprint(offset, " +/- ", delay, " ", address, " ", addr.String()))
	}
}

var percentage float64 = 0
var result string

type ntpQueryMessage string

type progressUpdateMessage struct {
}

type model struct {
	index    int
	progress progress.Model
	system   *ntp.NTPSystem
	address  string
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		percentage += 0.25
		return m, filterListenCommand(m)
	case ntpQueryMessage:
		result = string(msg)
		return m, tea.Quit
	default:
		return m, nil
	}
}

func filterListenCommand(m model) tea.Cmd {
	return func() tea.Msg {
		<-m.system.ProgressFiltered
		return progressUpdateMessage{}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(ntpQueryCommand(m.system, m.address), filterListenCommand(m))
}

func (m *model) resetProgress() {
	m.progress = progress.New(progress.WithScaledGradient("#68b1b1", "#6ea4ff"))
}

func (m model) View() (s string) {
	if result != "" {
		s += result + "\n"
	} else {
		s += textStyle("NTPal - Query\n")
		s += " " + m.progress.ViewAs(percentage) + "\n"
		s += helpStyle("q: exit\n")
	}
	return
}
