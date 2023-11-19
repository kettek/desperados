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

func main() {
	program = tea.NewProgram(initialModel())
	if _, err := program.Run(); err != nil {
		panic(err)
	}

}

func startMulticast(addr string) (*dnet.Multicaster, error) {
	m, err := dnet.NewMulticaster(addr)
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

func startRange(ip net.IP) (*dnet.RangerV4, error) {
	r := dnet.NewRanger(ip)
	results := make(chan dnet.RangerResult, 2)
	r.SetResults(results)
	r.Start(11332)

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
	multicaster, err = startMulticast(defaultAddr)
	if err != nil {
		messages = append(messages, err.Error())
	} else {
		messages = append(messages, systemStyle.Render("Multicast started"))
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
		err   error
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
				m.textarea.Prompt = fmt.Sprintf("%03dt> ", ranger.Current())
			} else {
				m.textarea.Prompt = fmt.Sprintf("%03du> ", ranger.Current())
			}
			m.textarea.SetWidth(80)
		case dnet.RangerDone:
			fmt.Println("got ranger done")
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
				// handle as command
				if strings.HasPrefix(v, "/start") {
					parts := strings.Split(v, " ")
					address := defaultAddr
					if len(parts) < 2 {
						m.messages = append(m.messages, m.systemStyle.Render(fmt.Sprintf("No address provided, using %s.", defaultAddr)))
					} else {
						address = parts[1]
					}
					// start multicast
					multicaster, err = startMulticast(address)
					if err != nil {
						m.messages = append(m.messages, err.Error())
					} else {
						m.messages = append(m.messages, m.systemStyle.Render("Multicast started"))
					}
				} else if v == "/stop" {
					// stop multicast
					multicaster.Close()
					multicaster = nil
					m.messages = append(m.messages, m.systemStyle.Render("Multicast stopped"))
				} else if v == "/range" {
					// Get local ip.
					addrs, err := net.InterfaceAddrs()
					if err != nil {
						m.messages = append(m.messages, m.systemStyle.Render(err.Error()))
						break
					}
					var ip net.IP
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
							if ipnet.IP.To4() != nil {
								ip = ipnet.IP
								break
							}
						}
					}
					ranger, _ = startRange(ip)
					m.messages = append(m.messages, m.systemStyle.Render("Ranger started"))
				} else if v == "/rangestop" {
					if ranger != nil {
						ranger.Close()
						ranger = nil
						m.messages = append(m.messages, m.systemStyle.Render("Ranger stopped"))
					}
				} else {
					m.messages = append(m.messages, m.systemStyle.Render("Unknown command"))
				}
			} else {
				if multicaster == nil || multicaster.Closed() {
					m.messages = append(m.messages, m.systemStyle.Render("Please start multicast first with /start"))
				} else {
					multicaster.Send([]byte(m.textarea.Value()))
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
