package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JustSteveKing/flint/internal/board"
	"github.com/JustSteveKing/flint/internal/usb"
)

func firmware(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "q1he.bin")
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func q1he(t *testing.T) *board.Board {
	t.Helper()
	b, ok := board.ByNormalID(usb.ID{Vendor: 0x3434, Product: 0x0b10})
	if !ok {
		t.Fatal("Q1 HE missing from the table")
	}
	return &b
}

func arrived(id usb.ID) usbEventMsg {
	return usbEventMsg(usb.Event{Kind: usb.Added, Device: usb.Device{ID: id, SysPath: "/fake/1-2"}})
}

func removed(id usb.ID) usbEventMsg {
	return usbEventMsg(usb.Event{Kind: usb.Removed, Device: usb.Device{ID: id, SysPath: "/fake/1-1"}})
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestEntryStepSkipsWhatTheFlagsAlreadyAnswered(t *testing.T) {
	fw := firmware(t)

	if got := New(context.Background(), Options{}).step; got != stepMenu {
		t.Errorf("bare invocation started at %v, want stepMenu", got)
	}
	if got := New(context.Background(), Options{Firmware: fw}).step; got != stepBoard {
		t.Errorf("with a file, started at %v, want stepBoard", got)
	}
	if got := New(context.Background(), Options{Firmware: fw, Board: q1he(t)}).step; got != stepPrepare {
		t.Errorf("with file and board, started at %v, want stepPrepare", got)
	}
}

func TestMenuLeadsIntoTheWalkthrough(t *testing.T) {
	m := New(context.Background(), Options{DryRun: true})

	m.Update(key("enter")) // "Flash firmware from a file" is first

	if m.step != stepBoard {
		t.Fatalf("step = %v, want stepBoard", m.step)
	}
	if !strings.Contains(m.View(), "Which keyboard?") {
		t.Errorf("board step does not ask which keyboard:\n%s", m.View())
	}
}

func TestBoardChoiceLeadsToTheFilePicker(t *testing.T) {
	m := New(context.Background(), Options{DryRun: true})
	m.Update(key("enter")) // into board selection
	m.Update(key("enter")) // first board

	if m.step != stepFirmware {
		t.Fatalf("step = %v, want stepFirmware", m.step)
	}
	if m.target == nil || m.target.Name != board.Known[0].Name {
		t.Errorf("target = %v, want %s", m.target, board.Known[0].Name)
	}
}

func TestNotSureChoiceLeavesTargetUnset(t *testing.T) {
	m := New(context.Background(), Options{DryRun: true, Firmware: firmware(t)})

	// "Not listed, or not sure" is the last entry.
	m.Update(key("G"))
	m.Update(key("enter"))

	if m.target != nil {
		t.Fatalf("target = %v, want nil so any bootloader is accepted", m.target)
	}
	if m.step != stepPrepare {
		t.Fatalf("step = %v, want stepPrepare", m.step)
	}
	if !strings.Contains(m.View(), board.GenericSteps[0]) {
		t.Errorf("no generic instructions shown for an unknown board:\n%s", m.View())
	}
}

func TestPrepareShowsNumberedStepsAndTracksTheUnplug(t *testing.T) {
	target := q1he(t)
	m := New(context.Background(), Options{Firmware: firmware(t), Board: target, DryRun: true})

	view := m.View()
	for i, want := range target.Steps {
		if !strings.Contains(view, want) {
			t.Errorf("instruction %d missing from the walkthrough: %q", i+1, want)
		}
	}
	if !strings.Contains(view, "waiting for a bootloader") {
		t.Errorf("no waiting indicator:\n%s", view)
	}

	m.Update(arrived(target.Normal))
	if !m.boardSeen {
		t.Error("board arriving did not tick the connected item")
	}

	m.Update(removed(target.Normal))
	if !m.boardGone {
		t.Error("board leaving did not tick the unplugged item; that tick is the feedback that step 3 worked")
	}

	// The checklist is a record of progress, not live status. Unplugging must
	// not untick "seen", because that reads as the walkthrough going backwards
	// at the exact moment the person did the right thing.
	after := m.View()
	seenLine := lineContaining(t, after, "seen")
	if !strings.Contains(seenLine, "✓") {
		t.Errorf("after unplugging, the seen line reads %q; it should stay ticked", seenLine)
	}
}

func lineContaining(t *testing.T, view, needle string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", needle, view)
	return ""
}

func TestBootloaderArrivalAsksBeforeWriting(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t), DryRun: true})

	m.Update(arrived(board.STM32DFU.ID))

	if m.step != stepConfirm {
		t.Fatalf("step = %v, want stepConfirm; flint must never write unprompted", m.step)
	}
	if !strings.Contains(m.View(), "Write this firmware? (y/n)") {
		t.Errorf("confirm step does not prompt:\n%s", m.View())
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	fw := firmware(t)
	m := New(context.Background(), Options{Firmware: fw, Board: q1he(t), DryRun: true})
	m.Update(arrived(board.STM32DFU.ID))

	m.Update(key("y"))

	if m.step != stepResult {
		t.Fatalf("step = %v, want stepResult", m.step)
	}
	if !strings.Contains(m.result, "Nothing was written") {
		t.Errorf("result = %q, want it to say nothing was written", m.result)
	}
	joined := strings.Join(m.log, "\n")
	if !strings.Contains(joined, "would run: dfu-util") || !strings.Contains(joined, fw) {
		t.Errorf("log = %q, want the command it would have run", joined)
	}
}

func TestDeclineReturnsToTheWait(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t), DryRun: true})
	m.Update(arrived(board.STM32DFU.ID))

	m.Update(key("n"))

	if m.step != stepPrepare {
		t.Fatalf("step = %v, want stepPrepare after declining", m.step)
	}
	if m.found != nil {
		t.Error("declined device was not cleared")
	}
}

func TestEscapeWalksBackwards(t *testing.T) {
	m := New(context.Background(), Options{DryRun: true})
	m.Update(key("enter")) // board
	m.Update(key("enter")) // firmware

	m.Update(key("esc"))
	if m.step != stepBoard {
		t.Fatalf("step = %v, want stepBoard", m.step)
	}
	m.Update(key("esc"))
	if m.step != stepMenu {
		t.Fatalf("step = %v, want stepMenu", m.step)
	}
}

func TestQuitIsDisabledWhileWriting(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t)})
	m.step = stepFlashing

	_, cmd := m.Update(key("q"))

	if cmd != nil {
		t.Fatal("q quit during a flash; the one moment interrupting actually breaks something")
	}
	if err := m.ctx.Err(); err != nil {
		t.Error("q cancelled the context during a flash")
	}
	if m.step != stepFlashing {
		t.Errorf("step = %v, want stepFlashing", m.step)
	}
}

func TestEscapeIsDisabledWhileWriting(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t)})
	m.step = stepFlashing

	m.Update(key("esc"))

	if m.step != stepFlashing {
		t.Errorf("step = %v, want stepFlashing; esc must not abandon a running flasher", m.step)
	}
}

func TestBoardRestrictionIgnoresBootloadersItCannotBe(t *testing.T) {
	unrelated := board.Bootloader{
		Name:    "Unrelated DFU",
		ID:      usb.ID{Vendor: 0x1209, Product: 0x0001},
		Flasher: "dfu-util",
	}
	old := board.Bootloaders
	board.Bootloaders = append(append([]board.Bootloader{}, old...), unrelated)
	t.Cleanup(func() { board.Bootloaders = old })

	m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t), DryRun: true})
	m.Update(arrived(unrelated.ID))

	if m.step != stepPrepare {
		t.Fatalf("step = %v, want stepPrepare; a board restriction must reject what it cannot be", m.step)
	}
	if !strings.Contains(strings.Join(m.log, "\n"), "cannot be behind it") {
		t.Errorf("log = %q, want the mismatch explained", m.log)
	}
}

func TestDetectionRecordsWhatWasLearned(t *testing.T) {
	target := q1he(t)
	if len(target.Bootloaders) < 2 {
		t.Skip("Q1 HE bootloader already narrowed; nothing left to learn")
	}

	m := New(context.Background(), Options{Firmware: firmware(t), Board: target, DryRun: true})
	m.Update(arrived(board.STM32DFU.ID))
	m.Update(key("y"))

	if m.learned == "" {
		t.Fatal("flint did not record which bootloader the board turned out to use")
	}
	if !strings.Contains(m.learned, "STM32") {
		t.Errorf("learned = %q, want it to name the bootloader", m.learned)
	}
	if !strings.Contains(m.View(), "Worth writing down") {
		t.Errorf("the finding is not surfaced on the result screen:\n%s", m.View())
	}
}

func TestIdentifyModeWritesNothingAndReportsEverything(t *testing.T) {
	m := New(context.Background(), Options{})
	m.Update(key("down")) // "Identify a board"
	m.Update(key("enter"))

	if m.step != stepIdentify {
		t.Fatalf("step = %v, want stepIdentify", m.step)
	}

	m.Update(arrived(board.STM32DFU.ID))

	if m.step != stepIdentify {
		t.Fatalf("step = %v; identify mode must never lead to a write", m.step)
	}
	joined := strings.Join(m.log, "\n")
	if !strings.Contains(joined, "0483:df11") || !strings.Contains(joined, "bootloader") {
		t.Errorf("log = %q, want the bootloader reported", joined)
	}
}

func TestIdentifyFlagsDevicesItDoesNotKnow(t *testing.T) {
	m := New(context.Background(), Options{})
	m.Update(key("down"))
	m.Update(key("enter"))

	m.Update(arrived(usb.ID{Vendor: 0xdead, Product: 0xbeef}))

	if !strings.Contains(m.learned, "not in flint's table") {
		t.Errorf("learned = %q, want an unknown arrival called out; that is the point of the mode", m.learned)
	}
}

func TestResultCanStartOver(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t), DryRun: true})
	m.Update(arrived(board.STM32DFU.ID))
	m.Update(key("y"))

	m.Update(key("enter"))

	if m.step != stepMenu {
		t.Fatalf("step = %v, want stepMenu", m.step)
	}
	if m.found != nil || m.learned != "" || len(m.log) != 0 {
		t.Error("starting over left state from the previous run")
	}
}

func TestHeaderShowsTheChoicesAlreadyMade(t *testing.T) {
	fw := firmware(t)
	m := New(context.Background(), Options{Firmware: fw, Board: q1he(t), DryRun: true})

	view := m.View()
	if !strings.Contains(view, "step 3 of 4") {
		t.Errorf("no step counter:\n%s", view)
	}
	if !strings.Contains(view, "Keychron Q1 HE") || !strings.Contains(view, fw) {
		t.Errorf("header does not carry the choices made:\n%s", view)
	}
	if !strings.Contains(view, "[dry run]") {
		t.Errorf("dry run not signalled:\n%s", view)
	}
}

func TestQuitWorksEverywhereElse(t *testing.T) {
	for _, s := range []step{stepMenu, stepBoard, stepFirmware, stepPrepare, stepConfirm, stepResult, stepIdentify, stepInspect} {
		m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t), DryRun: true})
		m.step = s

		_, cmd := m.Update(key("q"))

		if cmd == nil {
			t.Errorf("q did not quit from step %v", s)
		}
		if m.ctx.Err() == nil {
			t.Errorf("q did not cancel the watcher from step %v", s)
		}
	}
}

func TestCtrlCQuitsEvenWhileWriting(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), Board: q1he(t)})
	m.step = stepFlashing

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if cmd == nil {
		t.Fatal("ctrl-c did not quit; it is the documented way out of a stuck flash")
	}
	if m.ctx.Err() == nil {
		t.Error("ctrl-c did not cancel the context, so the flasher would keep running")
	}
}
