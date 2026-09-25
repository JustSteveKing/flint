// Package tui is flint's walkthrough.
//
// It is built as a wizard rather than a dashboard because flashing firmware is
// a procedure, not a state to observe. Someone doing it has a cable in one
// hand and a key held down with the other, and needs to be told the next thing
// to do, then told whether it worked.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JustSteveKing/flint/internal/board"
	"github.com/JustSteveKing/flint/internal/flash"
	"github.com/JustSteveKing/flint/internal/usb"
)

type step int

const (
	stepMenu step = iota
	stepBoard
	stepFirmware
	stepPrepare
	stepConfirm
	stepFlashing
	stepResult
	stepIdentify
	stepInspect
)

// maxLog bounds the output pane. Flashing produces a few hundred lines at
// most, but a flasher stuck in a retry loop should not eat memory.
const maxLog = 500

// Options configures a run. Anything set here is a step the walkthrough can
// skip, which is how the command-line form stays useful.
type Options struct {
	Firmware string
	DryRun   bool
	Board    *board.Board
}

// Model is the bubbletea model.
type Model struct {
	opts Options

	ctx    context.Context
	cancel context.CancelFunc

	step     step
	prevStep step

	menu   chooser
	boards chooser
	picker filepicker.Model
	spin   spinner.Model
	prog   progress.Model
	out    viewport.Model

	events  <-chan usb.Event
	updates <-chan flash.Update

	target   *board.Board
	firmware string

	found      *usb.Device
	bootloader board.Bootloader
	candidates []board.Board

	// The checklist during stepPrepare. Watching the board leave is what makes
	// the wait feel like progress rather than a hang.
	boardSeen bool
	boardGone bool

	connected []usb.Device
	log       []string
	percent   int
	err       error
	result    string
	learned   string

	width  int
	height int
}

// New builds a model ready for tea.NewProgram.
func New(parent context.Context, opts Options) *Model {
	ctx, cancel := context.WithCancel(parent)

	connected, _ := usb.List()

	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 48

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = dimStyle

	picker := filepicker.New()
	picker.AllowedTypes = []string{".bin", ".hex", ".dfu", ".uf2"}
	picker.CurrentDirectory = firmwareDir()
	picker.ShowSize = true
	picker.ShowPermissions = false

	m := &Model{
		opts:      opts,
		ctx:       ctx,
		cancel:    cancel,
		menu:      mainMenu(),
		boards:    boardMenu(connected),
		picker:    picker,
		spin:      spin,
		prog:      prog,
		out:       viewport.New(72, 12),
		target:    opts.Board,
		firmware:  opts.Firmware,
		connected: connected,
		percent:   -1,
		width:     80,
		height:    24,
	}

	m.step = m.entryStep()
	return m
}

// entryStep starts as far along as the flags allow, so `flint flash x.bin`
// drops straight into the part the person actually needs help with.
func (m *Model) entryStep() step {
	switch {
	case m.firmware != "" && m.target != nil:
		return stepPrepare
	case m.firmware != "":
		return stepBoard
	default:
		return stepMenu
	}
}

// firmwareDir is where firmware usually lands.
func firmwareDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	downloads := filepath.Join(home, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return downloads
	}
	return home
}

func mainMenu() chooser {
	return newChooser(
		choice{Label: "Flash firmware from a file", Note: "pick a board, pick a file, and be walked through it", Value: stepBoard},
		choice{Label: "Identify a board", Note: "find out what a keyboard's bootloader is, without flashing", Value: stepIdentify},
		choice{Label: "Show connected devices", Note: "what flint can see right now", Value: stepInspect},
		choice{Label: "Quit", Value: nil},
	)
}

func boardMenu(connected []usb.Device) chooser {
	present := make(map[usb.ID]bool, len(connected))
	for _, d := range connected {
		present[d.ID] = true
	}

	choices := make([]choice, 0, len(board.Known)+1)
	for i := range board.Known {
		b := board.Known[i]
		note := "not connected"
		if present[b.Normal] {
			note = "connected now"
		}
		choices = append(choices, choice{Label: b.Name, Note: note, Value: &b})
	}
	choices = append(choices, choice{
		Label: "Not listed, or not sure",
		Note:  "flint will accept any bootloader it recognises, and ask harder before writing",
		Value: (*board.Board)(nil),
	})
	return newChooser(choices...)
}

// Err reports a failure so the caller can set an exit code.
func (m *Model) Err() error { return m.err }

func (m *Model) Init() tea.Cmd {
	m.events = usb.Watch(m.ctx, usb.DefaultInterval)
	cmds := []tea.Cmd{waitForUSB(m.events), m.spin.Tick}
	if m.step == stepFirmware {
		cmds = append(cmds, m.picker.Init())
	}
	return tea.Batch(cmds...)
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
	case tea.WindowSizeMsg:
		m.resize(msg)
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)

	case usbEventMsg:
		cmd := m.handleUSB(usb.Event(msg))
		return m, tea.Batch(cmd, waitForUSB(m.events))

	case flashUpdateMsg:
		m.appendLog(msg.Line)
		if msg.Percent >= 0 {
			m.percent = msg.Percent
		}
		return m, waitForFlash(m.updates)

	case flashDoneMsg:
		return m, m.finish(msg.err)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	}

	if m.step == stepFirmware {
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) resize(msg tea.WindowSizeMsg) {
	m.width, m.height = msg.Width, msg.Height
	if msg.Width > 8 {
		m.prog.Width = min(msg.Width-8, 72)
		m.out.Width = min(msg.Width-4, 100)
	}
	if msg.Height > 14 {
		m.out.Height = min(msg.Height-12, 20)
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "q":
		// Never quit on a bare q mid-write: the whole screen says not to
		// interrupt, and q is next to the keys someone might fumble.
		if m.step != stepFlashing {
			m.cancel()
			return m, tea.Quit
		}
		return m, nil
	case "esc":
		return m, m.back()
	}

	switch m.step {
	case stepMenu:
		if sel := m.menu.Update(msg); sel != nil {
			if sel.Value == nil {
				m.cancel()
				return m, tea.Quit
			}
			return m, m.goTo(sel.Value.(step))
		}

	case stepBoard:
		if sel := m.boards.Update(msg); sel != nil {
			m.target = sel.Value.(*board.Board)
			if m.firmware != "" {
				return m, m.goTo(stepPrepare)
			}
			return m, m.goTo(stepFirmware)
		}

	case stepFirmware:
		// The picker owns its own keys; flint only reacts to a selection.
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		if ok, path := m.picker.DidSelectFile(msg); ok {
			if err := flash.CheckFirmware(path); err != nil {
				m.appendLog(err.Error())
				return m, cmd
			}
			m.firmware = path
			return m, tea.Batch(cmd, m.goTo(stepPrepare))
		}
		if ok, path := m.picker.DidSelectDisabledFile(msg); ok {
			m.appendLog(filepath.Base(path) + " is not firmware (expected .bin, .hex, .dfu or .uf2)")
		}
		return m, cmd

	case stepConfirm:
		switch msg.String() {
		case "y", "enter":
			return m, m.startFlash()
		case "n":
			m.found = nil
			m.boardGone = false
			m.appendLog("Skipped. Still waiting.")
			return m, m.goTo(stepPrepare)
		}

	case stepResult:
		if msg.String() == "enter" {
			return m, m.restart()
		}

	case stepIdentify, stepInspect:
		if msg.String() == "r" && m.step == stepInspect {
			m.connected, _ = usb.List()
		}
	}

	return m, nil
}

// goTo moves forward, remembering where from so esc can undo it.
func (m *Model) goTo(s step) tea.Cmd {
	m.prevStep = m.step
	m.step = s

	switch s {
	case stepFirmware:
		return m.picker.Init()
	case stepInspect:
		m.connected, _ = usb.List()
	case stepIdentify:
		m.log = nil
	}
	return nil
}

// back is the escape hatch out of any step that has not started writing.
func (m *Model) back() tea.Cmd {
	switch m.step {
	case stepFlashing:
		// Not while the flasher holds the device.
		return nil
	case stepMenu:
		m.cancel()
		return tea.Quit
	case stepBoard:
		if m.opts.Firmware != "" {
			m.cancel()
			return tea.Quit
		}
		m.step = stepMenu
	case stepFirmware:
		m.step = stepBoard
	case stepPrepare:
		m.found = nil
		m.boardGone = false
		if m.opts.Firmware != "" {
			m.step = stepBoard
		} else {
			m.step = stepFirmware
		}
	case stepConfirm:
		m.found = nil
		m.step = stepPrepare
	default:
		m.step = stepMenu
	}
	return nil
}

// restart returns to the menu with the transient state cleared, so a second
// board can be flashed without leaving the program.
func (m *Model) restart() tea.Cmd {
	m.found = nil
	m.boardSeen, m.boardGone = false, false
	m.percent = -1
	m.err = nil
	m.result = ""
	m.learned = ""
	m.log = nil
	m.connected, _ = usb.List()
	m.boards = boardMenu(m.connected)
	m.step = stepMenu
	return nil
}

func (m *Model) handleUSB(ev usb.Event) tea.Cmd {
	if ev.Err != nil {
		m.appendLog("usb: " + ev.Err.Error())
		return nil
	}

	if m.step == stepIdentify {
		m.recordIdentify(ev)
		return nil
	}

	if ev.Kind == usb.Removed {
		if m.target != nil && ev.Device.ID == m.target.Normal {
			m.boardGone = true
		}
		return nil
	}

	if m.target != nil && ev.Device.ID == m.target.Normal {
		m.boardSeen = true
		m.boardGone = false
	}

	if m.step != stepPrepare {
		return nil
	}

	bl, ok := board.BootloaderByID(ev.Device.ID)
	if !ok {
		return nil
	}

	candidates := board.CandidatesFor(bl)
	if m.target != nil {
		if !couldBe(*m.target, bl) {
			m.appendLog(fmt.Sprintf("%s appeared, but a %s cannot be behind it; ignoring", bl.Name, m.target.Name))
			return nil
		}
		candidates = []board.Board{*m.target}
		if len(m.target.Bootloaders) > 1 {
			m.learned = fmt.Sprintf("%s uses %s (%s)", m.target.Name, bl.Name, bl.ID)
		}
	}

	device := ev.Device
	m.found = &device
	m.bootloader = bl
	m.candidates = candidates
	m.prevStep = stepPrepare
	m.step = stepConfirm
	return nil
}

// recordIdentify is the read-only mode: report everything, write nothing.
func (m *Model) recordIdentify(ev usb.Event) {
	sign := "+"
	if ev.Kind == usb.Removed {
		sign = "-"
	}

	note := ""
	if bl, ok := board.BootloaderByID(ev.Device.ID); ok {
		note = "  <- bootloader: " + bl.Name
		if ev.Kind == usb.Added {
			m.learned = fmt.Sprintf("%s is %s", ev.Device.ID, bl.Name)
		}
	} else if b, ok := board.ByNormalID(ev.Device.ID); ok {
		note = "  <- " + b.Name + ", running normally"
	} else if ev.Kind == usb.Added {
		// An unrecognised device arriving during the bootloader dance is very
		// likely the bootloader, and is the thing worth writing down.
		note = "  <- unrecognised; if this appeared when you replugged, this is it"
		m.learned = fmt.Sprintf("%s (%s) is not in flint's table", ev.Device.ID, ev.Device.Label())
	}

	// A device with no descriptor strings labels itself with its own ID, and
	// printing that twice reads like a bug.
	label := ev.Device.Label()
	if label == ev.Device.ID.String() {
		label = "(no name)"
	}
	m.appendLog(fmt.Sprintf("%s %s  %s%s", sign, ev.Device.ID, label, note))
}

func couldBe(b board.Board, bl board.Bootloader) bool {
	for _, candidate := range b.Bootloaders {
		if candidate.ID == bl.ID {
			return true
		}
	}
	return false
}

func (m *Model) startFlash() tea.Cmd {
	if m.opts.DryRun {
		m.appendLog("would run: " + flash.Describe(m.bootloader, m.firmware))
		m.result = "Dry run. Nothing was written."
		m.step = stepResult
		return nil
	}

	if err := flash.Preflight(m.bootloader, m.firmware); err != nil {
		m.err = err
		m.step = stepResult
		return nil
	}

	m.step = stepFlashing
	m.percent = 0
	m.log = nil
	m.appendLog("running: " + flash.Describe(m.bootloader, m.firmware))

	updates, wait := flash.Run(m.ctx, m.bootloader, m.firmware)
	m.updates = updates
	return tea.Batch(waitForFlash(updates), waitForExit(wait))
}

func (m *Model) finish(err error) tea.Cmd {
	m.step = stepResult
	if err != nil {
		m.err = err
		return nil
	}
	m.percent = 100
	m.result = "Firmware written. The board should come back as a keyboard."
	return nil
}

func (m *Model) appendLog(line string) {
	m.log = append(m.log, line)
	if len(m.log) > maxLog {
		m.log = m.log[len(m.log)-maxLog:]
	}
	m.out.SetContent(strings.Join(m.log, "\n"))
	m.out.GotoBottom()
}

// steps returns the instructions for whatever board is targeted.
func (m *Model) steps() []string {
	if m.target != nil {
		return m.target.Steps
	}
	return board.GenericSteps
}
