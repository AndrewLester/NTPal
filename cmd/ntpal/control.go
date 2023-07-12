package main

import (
	"fmt"
	"log"
	"net/rpc"
	"strconv"
	"time"

	"github.com/AndrewLester/ntpal/internal/ntp"
	"github.com/AndrewLester/ntpal/internal/ui"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func handleNTPalUI(socket string) {
	m := ntpalUIModel{socket: socket, table: setupTable()}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		log.Fatal(err)
	}
}

const fetchInfoPeriod = time.Second * 5

type ntpalUIModel struct {
	socket string

	table            table.Model
	daemonKillStatus string
	RPCInfo
}

var client *rpc.Client

type RPCInfo struct {
	associations []*ntp.Association
	system       *ntp.System
}

type dialSocketMessage *rpc.Client
type fetchInfoMessage RPCInfo
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
		var associations []*ntp.Association
		assocCall := client.Go("NTPalRPCServer.FetchAssociations", 0, &associations, nil)
		var system *ntp.System
		systemCall := client.Go("NTPalRPCServer.FetchSystem", 0, &system, nil)

		err := (<-assocCall.Done).Error
		if err != nil {
			log.Fatalf("Error getting info from daemon: %v", err)
		}

		err = (<-systemCall.Done).Error
		if err != nil {
			log.Fatalf("Error getting info from daemon: %v", err)
		}

		return fetchInfoMessage(RPCInfo{
			associations: associations,
			system:       system,
		})
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
		m.RPCInfo = RPCInfo(msg)
		rows := []table.Row{}
		for _, association := range m.associations {
			row := table.Row{
				association.Srcaddr.IP.String(),
				strconv.FormatFloat(association.Offset*1e3, 'G', 5, 64),
				strconv.FormatUint(uint64(association.Reach), 2),
				strconv.FormatFloat(association.Jitter*1e3, 'G', 5, 64),
				fmt.Sprintf("%s ago", time.Duration(uint64(float64(m.system.Clock.T)-association.Update))*time.Second),
			}
			rows = append(rows, row)
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
	s += ui.Title("NTPal") + "\n"
	s += ui.TableBase(m.table.View()) + "\n\n"
	if m.daemonKillStatus != "" {
		s += m.daemonKillStatus + "\n"
	} else {
		s += ui.Help("q: exit, s: stop daemon") + "\n"
	}
	return
}

func setupTable() table.Model {
	columns := []table.Column{
		{Title: "Address", Width: 20},
		{Title: "Offset (ms)", Width: 15},
		{Title: "Reach", Width: 15},
		{Title: "Error", Width: 15},
		{Title: "Last Update", Width: 25},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(7),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ui.TableGray).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("218")).
		Background(lipgloss.Color("70")).
		Bold(false)
	t.SetStyles(s)

	return t
}
