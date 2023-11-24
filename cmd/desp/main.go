package main

import (
	"fmt"
	"math/rand"
	"net"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kettek/desperados/pkg/dnet"
)

var program *tea.Program
var multicaster *dnet.Multicaster
var ranger *dnet.RangerV4

const defaultAddr = "239.0.0.0:11332"

var commands map[string]func(m model, in string) []string

func init() {
	commands = map[string]func(m model, in string) []string{
		"help": func(m model, in string) (messages []string) {
			for k := range commands {
				messages = append(messages, m.systemStyle.Render(fmt.Sprintf("/%s", k)))
			}
			return
		},
		"mcast": func(m model, in string) (messages []string) {
			var err error
			parts := strings.Split(in, " ")
			if parts[0] == "start" || parts[0] == "" {
				address := ""
				if len(parts) < 2 {
					messages = append(messages, m.systemStyle.Render("No address provided, using default."))
				} else {
					address = parts[1]
				}
				// start multicast
				multicaster, err = startMulticast(defaultAddr, address)
				if err != nil {
					messages = append(messages, err.Error())
				} else {
					messages = append(messages, m.systemStyle.Render(fmt.Sprintf("Multicast started on %s/%s", multicaster.RecvAddr().String(), multicaster.SendAddr().String())))
				}
			} else if parts[0] == "stop" {
				// stop multicast
				multicaster.Close()
				multicaster = nil
				messages = append(messages, m.systemStyle.Render("Multicast stopped"))
			}
			return messages
		},
		"range": func(m model, in string) (messages []string) {
			parts := strings.Split(in, " ")
			if parts[0] == "" || parts[0] == "udp" || parts[0] == "tcp" {
				// Get local ip.
				addrs, err := net.InterfaceAddrs()
				if err != nil {
					messages = append(messages, m.systemStyle.Render(err.Error()))
					return
				}
				var ip net.IP
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && !ipnet.IP.IsLinkLocalUnicast() {
						if ipnet.IP.To4() != nil {
							ip = ipnet.IP
							break
						}
					}
				}
				if parts[0] == "tcp" {
					ranger, _ = startRange(ip, true)
				} else {
					ranger, _ = startRange(ip, false)
				}
				messages = append(messages, m.systemStyle.Render("Ranger started"))
			} else if parts[0] == "stop" {
				if ranger != nil {
					ranger.Close()
					ranger = nil
					messages = append(messages, m.systemStyle.Render("Ranger stopped"))
				}
			}

			return
		},
	}
}

func main() {
	program = tea.NewProgram(initialModel())
	if _, err := program.Run(); err != nil {
		panic(err)
	}
}

func startMulticast(addr string, sendAddr string) (*dnet.Multicaster, error) {
	m, err := dnet.NewMulticaster(addr, sendAddr)
	if err != nil {
		return nil, err
	}

	recvChan := make(chan *dnet.MulticastMessage)
	m.SetRecv(recvChan)

	go func() {
		for {
			select {
			case msg := <-recvChan:
				program.Send(msg)
			default:
			}
			if m.Closed() {
				break
			}
		}
	}()
	return m, nil
}

func startRange(ip net.IP, tcp bool) (*dnet.RangerV4, error) {
	r := dnet.NewRanger(ip)
	results := make(chan dnet.RangerResult, 2)
	r.SetResults(results)
	r.Start(11332, tcp)

	go func() {
		for {
			select {
			case result := <-results:
				program.Send(result)
				if result.Type() == "done" {
					return
				}
			}
		}
	}()
	return r, nil
}

type model struct {
	viewport    viewport.Model
	messages    []string
	textarea    textarea.Model
	systemStyle lipgloss.Style
	selfStyle   lipgloss.Style
	senderStyle lipgloss.Style
	err         error
}

func initialModel() model {
	var err error
	ta := textarea.New()
	ta.Focus()

	ta.Prompt = "DESP> "
	ta.CharLimit = 280

	ta.SetWidth(80)
	ta.SetHeight(1)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	systemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	vp := viewport.New(80, 4)
	var messages []string
	var welcome string
	if rand.Intn(2) == 0 {
		welcome = "Bienvenida, desperada."
	} else {
		welcome = "Bienvenido, desperado."
	}

	messages = append(messages, systemStyle.Render(welcome))

	// Might as well autostart multicast
	multicaster, err = startMulticast(defaultAddr, "")
	if err != nil {
		messages = append(messages, err.Error())
	} else {
		messages = append(messages, systemStyle.Render(fmt.Sprintf("Multicast started on %s/%s", multicaster.RecvAddr().String(), multicaster.SendAddr().String())))
	}

	messages = append(messages, systemStyle.Render("Type a message and press Enter to send."))

	vp.SetContent(strings.Join(messages, "\n"))

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		textarea:    ta,
		messages:    messages,
		viewport:    vp,
		systemStyle: systemStyle,
		selfStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("0")),
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case *dnet.MulticastMessage:
		if msg.Addr.String() == multicaster.SendAddr().String() {
			m.messages = append(m.messages, m.selfStyle.Render(fmt.Sprintf("%s> %s", msg.Addr.String(), string(msg.Data))))
		} else {
			m.messages = append(m.messages, m.senderStyle.Render(fmt.Sprintf("%s> %s", msg.Addr.String(), string(msg.Data))))
		}
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
	case dnet.RangerResult:
		switch result := msg.(type) {
		case dnet.RangerPong:
			m.messages = append(m.messages, m.systemStyle.Render(fmt.Sprintf("%s is a desperados", result.Addr.String())))
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.viewport.GotoBottom()
		case dnet.RangerStep:
			// Refresh UI
			if result.IsTCP {
				m.textarea.Prompt = fmt.Sprintf("%03dt> ", result.Index)
			} else {
				m.textarea.Prompt = fmt.Sprintf("%03du> ", result.Index)
			}
			m.textarea.SetWidth(80)
		case dnet.RangerDone:
			m.messages = append(m.messages, m.systemStyle.Render("Ranging complete"))
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.viewport.GotoBottom()
			m.textarea.Prompt = "DESP> "
			m.textarea.SetWidth(80)
		}
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case tea.KeyEnter:
			v := m.textarea.Value()
			if v == "" {
				return m, nil
			}
			if len(v) > 0 && v[0] == '/' {
				parts := strings.Split(v[1:], " ")
				if cmd, ok := commands[parts[0]]; ok {
					if msgs := cmd(m, strings.Join(parts[1:], " ")); msgs != nil {
						m.messages = append(m.messages, msgs...)
					}
				} else {
					m.messages = append(m.messages, m.systemStyle.Render("Unknown command"))
				}
			} else {
				if multicaster == nil || multicaster.Closed() {
					m.messages = append(m.messages, m.systemStyle.Render("Please start multicast first with /start"))
				} else {
					multicaster.Send([]byte(v))
				}
			}
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.textarea.Reset()
			m.viewport.GotoBottom()
		}

	// We handle errors just like any other message
	case error:
		m.err = msg
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m model) View() string {
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
	)
}
