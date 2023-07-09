package main

import (
	"log"
	"net/rpc"
	"strconv"
	"time"

	"github.com/AndrewLester/ntpal/pkg/ntp"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func handleNTPalUI(socket string) {
	m := ntpalUIModel{socket: socket, table: setupTable()}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		log.Fatalf("could not run program: %v", err)
	}
}

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

const fetchInfoPeriod = time.Second * 5

type ntpalUIModel struct {
	socket       string
	associations []*ntp.RPCAssociation
	table        table.Model

	daemonKillStatus string
}

var client *rpc.Client

type dialSocketMessage *rpc.Client
type fetchInfoMessage []*ntp.RPCAssociation
type tickMsg time.Time

func dialSocketCommand(m ntpalUIModel) tea.Cmd {
	return func() tea.Msg {
		client, err := rpc.Dial("unix", m.socket)
		if err != nil {
			log.Fatalf("Error connecting to ntpal daemon: %v", err)
		}

		return dialSocketMessage(client)
	}
}

func fetchInfoCommand(m ntpalUIModel) tea.Cmd {
	return func() tea.Msg {
		var reply []*ntp.RPCAssociation
		err := client.Call("RPCServer.FetchInfo", 0, &reply)
		if err != nil {
			log.Fatalf("Error getting info from daemon: %v", err)
		}
		return fetchInfoMessage(reply)
	}
}

func stopDaemonCommand() tea.Cmd {
	return func() tea.Msg {
		killDaemon()
		return nil
	}
}

func tickCommand(duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m ntpalUIModel) Init() tea.Cmd {
	return dialSocketCommand(m)
}

func (m ntpalUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.table.Focused() {
				m.table.Blur()
			} else {
				m.table.Focus()
			}
		case "enter":
			return m, tea.Printf("Let's go to %s!", m.table.SelectedRow()[1])
		case "stop", "s":
			m.daemonKillStatus = "Stopping ntpald"
			return m, tea.Sequence(stopDaemonCommand(), tea.Quit)
		case "ctrl+c", "q":
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	case dialSocketMessage:
		client = msg
		return m, tickCommand(0)
	case fetchInfoMessage:
		m.associations = msg
		rows := []table.Row{}
		for _, association := range m.associations {
			rows = append(rows, table.Row{association.SrcAddr.IP.String(), strconv.FormatFloat(association.Offset, 'G', 5, 64)})
		}
		m.table.SetRows(rows)
		return m, nil
	case tickMsg:
		return m, tea.Batch(tickCommand(fetchInfoPeriod), fetchInfoCommand(m))
	default:
		return m, nil
	}
}

func (m ntpalUIModel) View() (s string) {
	s += textStyle("NTPal") + "\n\n"
	s += baseStyle.Render(m.table.View()) + "\n\n"
	if m.daemonKillStatus != "" {
		s += m.daemonKillStatus + "\n"
	} else {
		s += helpStyle("q: exit\n")
	}
	return
}

func setupTable() table.Model {
	columns := []table.Column{
		{Title: "Address", Width: 20},
		{Title: "Offset", Width: 15},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(7),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return t
}
