// Package tui is the watch-and-flash screen.
//
// The whole point of the tool is this loop: sit waiting, notice the board drop
// into bootloader mode, say what it found, and only then write. QMK Toolbox's
// one genuinely useful feature was never the flashing.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JustSteveKing/flint/internal/board"
	"github.com/JustSteveKing/flint/internal/flash"
	"github.com/JustSteveKing/flint/internal/usb"
)

type state int

const (
	stateWaiting state = iota
	stateConfirm
	stateFlashing
	stateDone
	stateFailed
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	warnStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	errStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	okStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	promptStyle = lipgloss.NewStyle().Bold(true)
	logStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Options configures a run.
type Options struct {
	Firmware string
	DryRun   bool
	// Board, when set, restricts flashing to a bootloader that board could be
	// behind. It is how --board turns a guess into a check.
	Board *board.Board
}

// Model is the bubbletea model.
type Model struct {
	opts Options

	ctx    context.Context
	cancel context.CancelFunc

	state    state
	events   <-chan usb.Event
	updates  <-chan flash.Update
	waitErr  func() error
	progress progress.Model

	connected  []usb.Device
	found      *usb.Device
	bootloader board.Bootloader
	candidates []board.Board

	log     []string
	percent int
	err     error
	message string
}

// New builds a model ready for tea.NewProgram.
func New(parent context.Context, opts Options) *Model {
	ctx, cancel := context.WithCancel(parent)
	connected, _ := usb.List()

	p := progress.New(progress.WithDefaultGradient())
	p.Width = 48

	return &Model{
		opts:      opts,
		ctx:       ctx,
		cancel:    cancel,
		state:     stateWaiting,
		connected: connected,
		progress:  p,
		percent:   -1,
	}
}

// Err reports a failure so the caller can set an exit code.
func (m *Model) Err() error { return m.err }

func (m *Model) Init() tea.Cmd {
	m.events = usb.Watch(m.ctx, usb.DefaultInterval)
	return waitForUSB(m.events)
}

type usbEventMsg usb.Event
type flashUpdateMsg flash.Update
type flashDoneMsg struct{ err error }
type channelClosedMsg struct{}

func waitForUSB(ch <-chan usb.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return channelClosedMsg{}
		}
		return usbEventMsg(ev)
	}
}

func waitForFlash(ch <-chan flash.Update) tea.Cmd {
	return func() tea.Msg {
		up, ok := <-ch
		if !ok {
			return channelClosedMsg{}
		}
		return flashUpdateMsg(up)
	}
}

func waitForExit(wait func() error) tea.Cmd {
	return func() tea.Msg { return flashDoneMsg{err: wait()} }
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		if msg.Width > 8 {
			m.progress.Width = min(msg.Width-8, 72)
		}
		return m, nil

	case usbEventMsg:
		cmd := m.handleUSB(usb.Event(msg))
		return m, tea.Batch(cmd, waitForUSB(m.events))

	case flashUpdateMsg:
		m.appendLog(msg.Line)
		if msg.Percent >= 0 {
			m.percent = msg.Percent
			return m, tea.Batch(
				m.progress.SetPercent(float64(msg.Percent)/100),
				waitForFlash(m.updates),
			)
		}
		return m, waitForFlash(m.updates)

	case flashDoneMsg:
		if msg.err != nil {
			m.state = stateFailed
			m.err = msg.err
			return m, nil
		}
		m.state = stateDone
		m.message = "Firmware written. The board should be back as a keyboard."
		return m, m.progress.SetPercent(1)

	case progress.FrameMsg:
		updated, cmd := m.progress.Update(msg)
		m.progress = updated.(progress.Model)
		return m, cmd
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.cancel()
		return m, tea.Quit
	case "y", "enter":
		if m.state == stateConfirm {
			return m, m.startFlash()
		}
	case "n", "esc":
		if m.state == stateConfirm {
			m.state = stateWaiting
			m.found = nil
			m.appendLog("Skipped. Still watching.")
		}
	}
	return m, nil
}

func (m *Model) handleUSB(ev usb.Event) tea.Cmd {
	if ev.Err != nil {
		m.appendLog("usb: " + ev.Err.Error())
		return nil
	}

	if ev.Kind == usb.Removed {
		if b, ok := board.ByNormalID(ev.Device.ID); ok && m.state == stateWaiting {
			m.appendLog(b.Name + " disconnected, waiting for its bootloader")
		}
		return nil
	}

	if m.state != stateWaiting {
		return nil
	}

	bl, ok := board.BootloaderByID(ev.Device.ID)
	if !ok {
		if b, known := board.ByNormalID(ev.Device.ID); known {
			m.appendLog(b.Name + " connected as a keyboard")
		} else {
			// Worth printing: this is how an unknown bootloader gets added to
			// the table rather than silently ignored.
			m.appendLog(fmt.Sprintf("unrecognised device %s (%s)", ev.Device.ID, ev.Device.Label()))
		}
		return nil
	}

	candidates := board.CandidatesFor(bl)
	if m.opts.Board != nil {
		allowed := false
		for _, c := range candidates {
			if c.Normal == m.opts.Board.Normal {
				allowed = true
				break
			}
		}
		if !allowed {
			m.appendLog(fmt.Sprintf("%s appeared, but it cannot be a %s; ignoring", bl.Name, m.opts.Board.Name))
			return nil
		}
		candidates = []board.Board{*m.opts.Board}
	}

	device := ev.Device
	m.found = &device
	m.bootloader = bl
	m.candidates = candidates
	m.state = stateConfirm
	return nil
}

func (m *Model) startFlash() tea.Cmd {
	if m.opts.DryRun {
		m.state = stateDone
		m.message = "Dry run. Nothing was written."
		m.appendLog("would run: " + flash.Describe(m.bootloader, m.opts.Firmware))
		return nil
	}

	if err := flash.Preflight(m.bootloader, m.opts.Firmware); err != nil {
		m.state = stateFailed
		m.err = err
		return nil
	}

	m.state = stateFlashing
	m.percent = 0
	m.appendLog("running: " + flash.Describe(m.bootloader, m.opts.Firmware))

	updates, wait := flash.Run(m.ctx, m.bootloader, m.opts.Firmware)
	m.updates = updates
	m.waitErr = wait
	return tea.Batch(waitForFlash(updates), waitForExit(wait))
}

func (m *Model) appendLog(line string) {
	m.log = append(m.log, line)
	if len(m.log) > 8 {
		m.log = m.log[1:]
	}
}

func (m *Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("flint"))
	if m.opts.DryRun {
		b.WriteString(" " + warnStyle.Render("[dry run]"))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(m.opts.Firmware))
	b.WriteString("\n\n")

	switch m.state {
	case stateWaiting:
		b.WriteString(m.viewWaiting())
	case stateConfirm:
		b.WriteString(m.viewConfirm())
	case stateFlashing:
		b.WriteString(m.viewFlashing())
	case stateDone:
		b.WriteString(okStyle.Render("Done") + "\n" + m.message + "\n")
	case stateFailed:
		b.WriteString(errStyle.Render("Failed") + "\n" + m.err.Error() + "\n")
	}

	if len(m.log) > 0 {
		b.WriteString("\n")
		for _, line := range m.log {
			b.WriteString(logStyle.Render("  "+line) + "\n")
		}
	}

	b.WriteString("\n" + dimStyle.Render(m.hints()) + "\n")
	return b.String()
}

func (m *Model) viewWaiting() string {
	var b strings.Builder
	b.WriteString("Waiting for a board in bootloader mode.\n\n")

	target := m.opts.Board
	if target == nil {
		for _, d := range m.connected {
			if known, ok := board.ByNormalID(d.ID); ok {
				b.WriteString("  " + known.Name + " " + dimStyle.Render("("+d.ID.String()+")") + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("Put one into bootloader mode. Usually: hold Esc, unplug, keep holding, plug in.") + "\n")
		return b.String()
	}

	b.WriteString("  Target: " + target.Name + "\n\n")
	b.WriteString(promptStyle.Render(target.Enter) + "\n")
	return b.String()
}

func (m *Model) viewConfirm() string {
	var b strings.Builder
	b.WriteString(warnStyle.Render("Bootloader detected") + "\n\n")
	b.WriteString("  Device:  " + m.bootloader.Name + " " + dimStyle.Render("("+m.found.ID.String()+")") + "\n")

	switch len(m.candidates) {
	case 0:
		b.WriteString("  Board:   " + errStyle.Render("unknown") + "\n")
	case 1:
		b.WriteString("  Board:   " + m.candidates[0].Name + "\n")
	default:
		names := make([]string, len(m.candidates))
		for i, c := range m.candidates {
			names[i] = c.Name
		}
		b.WriteString("  Board:   " + warnStyle.Render("could be any of: "+strings.Join(names, ", ")) + "\n")
	}

	b.WriteString("  Command: " + dimStyle.Render(flash.Describe(m.bootloader, m.opts.Firmware)) + "\n")

	if !m.bootloader.Verified {
		b.WriteString("\n" + warnStyle.Render("These flasher arguments have never been run against real hardware.") + "\n")
		b.WriteString(warnStyle.Render("Check them before saying yes.") + "\n")
	}
	if len(m.candidates) > 1 {
		b.WriteString("\n" + warnStyle.Render("More than one board could be behind this bootloader.") + "\n")
		b.WriteString(warnStyle.Render("Make sure the firmware is for the one you just unplugged.") + "\n")
	}

	b.WriteString("\n" + promptStyle.Render("Write this firmware? (y/n)") + "\n")
	return b.String()
}

func (m *Model) viewFlashing() string {
	var b strings.Builder
	b.WriteString("Flashing. " + errStyle.Render("Do not unplug.") + "\n\n")
	b.WriteString("  " + m.progress.View() + "\n")
	if m.percent >= 0 {
		b.WriteString("  " + dimStyle.Render(fmt.Sprintf("%d%%", m.percent)) + "\n")
	}
	return b.String()
}

func (m *Model) hints() string {
	switch m.state {
	case stateConfirm:
		return "y flash · n skip · q quit"
	case stateFlashing:
		return "do not unplug"
	default:
		return "q quit"
	}
}
