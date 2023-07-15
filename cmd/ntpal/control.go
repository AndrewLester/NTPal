package main

import (
	"fmt"
	"log"
	"math"
	"net/rpc"
	"os"
	"strconv"
	"time"

	"github.com/AndrewLester/ntpal/internal/ntp"
	"github.com/AndrewLester/ntpal/internal/ui"
	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func handleNTPalUI(socket string) {
	m := ntpalUIModel{
		socket:           socket,
		associationTable: setupAssociationTable(),
		systemTable:      setupDetailTable(),
		detailTable:      setupDetailTable(),
		paginator:        setupPaginator(),
	}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		log.Fatal(err)
	}
}

const fetchInfoPeriod = time.Second * 5

type ntpalUIModel struct {
	socket string

	associationTable table.Model
	detailTable      table.Model
	systemTable      table.Model
	paginator        paginator.Model
	daemonKillStatus string
	association      *ntp.Association
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
			fmt.Printf("Error getting info from daemon: %v\n", err)
			os.Exit(1)
		}

		err = (<-systemCall.Done).Error
		if err != nil {
			fmt.Printf("Error getting info from daemon: %v\n", err)
			os.Exit(1)
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
			if m.associationTable.Focused() {
				m.associationTable.Blur()
			} else {
				m.associationTable.Focus()
			}
		case "enter":
			if !m.associationTable.Focused() {
				return m, nil
			}

			ipAddr := m.associationTable.SelectedRow()[0]
			for _, association := range m.associations {
				if association.Srcaddr.IP.String() == ipAddr {
					m.association = association
					break
				}
			}

			m.detailTable.SetRows([]table.Row{{"Loading...", ""}})
			m.detailTable.SetHeight(1)

			return m, fetchInfoCommand(m)
		case "right", "left":
			var cmd tea.Cmd
			m.paginator, cmd = m.paginator.Update(msg)
			return m, cmd
		case "stop", "s":
			m.daemonKillStatus = "Stopping ntpald"
			return m, tea.Sequence(stopDaemonCommand(), tea.Quit)
		case "q":
			if m.association != nil {
				m.association = nil
				return m, nil
			}
			fallthrough
		case "ctrl+c":
			return m, tea.Quit
		}

		var cmd tea.Cmd
		if m.association != nil {
			m.detailTable, cmd = m.detailTable.Update(msg)
		} else {
			m.associationTable, cmd = m.associationTable.Update(msg)
		}
		return m, cmd
	case dialSocketMessage:
		client = msg
		return m, tickCommand(0)
	case fetchInfoMessage:
		m.RPCInfo = RPCInfo(msg)

		// Detail table
		if m.association != nil {
			rows := []table.Row{
				{"Hostname", m.association.Hostname},
				{"IP", m.association.Srcaddr.IP.String()},
				{"Offset (ms)", ui.TableFloat(m.association.Offset * 1e3)},
				{"Poll", (time.Duration(ntp.Log2ToDouble(int8(math.Max(float64(m.association.Poll), float64(m.association.Hpoll))))) * time.Second).String()},
				{"Reach", strconv.FormatUint(uint64(m.association.Reach), 2)},
				{"Root delay", ui.TableFloat(m.association.Rootdelay)},
				{"Root dispersion", ui.TableFloat(m.association.Rootdisp)},
				{"Delay", ui.TableFloat(m.association.Delay)},
				{"Dispersion", ui.TableFloat(m.association.Disp)},
				{"initial burst", strconv.FormatBool(m.association.IburstEnabled)},
				{"burst", strconv.FormatBool(m.association.BurstEnabled)},
			}
			m.detailTable.SetRows(rows)
			m.detailTable.SetHeight(len(rows))
		}

		// System table
		peerText := "NONE"
		if m.system.Association != nil {
			peerText = m.system.Association.Srcaddr.String()
		}
		systemRows := []table.Row{
			{"Peer", peerText},
			{"Offset", ui.TableFloat(m.system.Offset)},
			{"Jitter", ui.TableFloat(m.system.Jitter)},
			{"Root delay", ui.TableFloat(m.system.Rootdelay)},
			{"Root dispersion", ui.TableFloat(m.system.Rootdisp)},
			{"Clock time", strconv.FormatUint(m.system.Clock.T, 10)},
			{"Clock offset", ui.TableFloat(m.system.Clock.Offset)},
			{"Clock frequency", ui.TableFloat(m.system.Clock.Freq)},
			{"Clock State", strconv.Itoa(m.system.Clock.State)},
		}
		m.systemTable.SetRows(systemRows)
		m.systemTable.SetHeight(len(systemRows))

		// Association table
		m.associationTable.SetHeight(len(m.associations))
		associationRows := []table.Row{}
		for _, association := range m.associations {
			row := table.Row{
				association.Srcaddr.IP.String(),
				ui.TableFloat(association.Offset * 1e3),
				strconv.FormatUint(uint64(association.Reach), 2),
				ui.TableFloat(association.Jitter * 1e3),
				fmt.Sprintf("%s ago", time.Duration(uint64(float64(m.system.Clock.T)-association.Update))*time.Second),
			}
			associationRows = append(associationRows, row)
		}
		m.associationTable.SetRows(associationRows)

		return m, nil
	case tickMsg:
		return m, tea.Batch(tickCommand(fetchInfoPeriod), fetchInfoCommand(m))
	default:
		return m, nil
	}
}

func (m ntpalUIModel) View() (s string) {
	pageTitle := "Associations"
	if m.paginator.OnLastPage() {
		pageTitle = "System"
	}
	s += ui.Title(fmt.Sprintf("NTPal - %s", pageTitle)) + "\n"

	if m.association != nil {
		s += ui.TableBase(m.detailTable.View()) + "\n\n"
	} else {
		if m.paginator.OnLastPage() {
			s += ui.TableBase(m.systemTable.View()) + "\n"
		} else {
			s += ui.TableBase(m.associationTable.View()) + "\n"
		}
		s += lipgloss.PlaceHorizontal(85, lipgloss.Center, m.paginator.View()) + "\n\n"
	}

	if m.daemonKillStatus != "" {
		s += m.daemonKillStatus + "\n"
	} else {
		tableAction := "focus table"
		if m.associationTable.Focused() {
			tableAction = "exit table"
		}
		historyAction := "quit"
		if m.association != nil {
			historyAction = "back"
		}
		s += ui.Help(fmt.Sprintf("q: %s, esc: %s, ←/→ page, s: stop daemon", historyAction, tableAction)) + "\n"
	}
	return
}

func setupPaginator() paginator.Model {
	p := paginator.New()
	p.Type = paginator.Dots
	p.PerPage = 1
	p.ActiveDot = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"}).Render("•")
	p.InactiveDot = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "250", Dark: "238"}).Render("•")
	p.SetTotalPages(2)
	return p
}

func setupAssociationTable() table.Model {
	columns := []table.Column{
		{Title: "Address", Width: 20},
		{Title: "Offset (ms)", Width: 15},
		{Title: "Reach", Width: 15},
		{Title: "Error (ms)", Width: 15},
		{Title: "Last Update", Width: 20},
	}
	return setupTable(columns, []table.Option{table.WithFocused(true)}, true)
}

func setupDetailTable() table.Model {
	columns := []table.Column{
		{Title: "Property", Width: 15},
		{Title: "Value", Width: 30},
	}
	return setupTable(columns, nil, false)
}

func setupTable(columns []table.Column, opts []table.Option, selection bool) table.Model {
	t := table.New(
		append(opts, table.WithColumns(columns), table.WithHeight(4))...,
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ui.TableGray).
		BorderBottom(true).
		Bold(true)

	if selection {
		s.Selected = s.Selected.
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("34")).
			Bold(false)
	} else {
		s.Selected = s.Selected.Foreground(lipgloss.Color("white")).Bold(false)
	}

	t.SetStyles(s)

	return t
}
