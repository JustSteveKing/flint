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

func bootloaderArrived(id usb.ID) usbEventMsg {
	return usbEventMsg(usb.Event{
		Kind:   usb.Added,
		Device: usb.Device{ID: id, SysPath: "/fake/1-2"},
	})
}

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestBootloaderArrivalAsksBeforeWriting(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), DryRun: true})

	m.Update(bootloaderArrived(board.STM32DFU.ID))

	if m.state != stateConfirm {
		t.Fatalf("state = %v, want stateConfirm; flint must never write unprompted", m.state)
	}
	view := m.View()
	if !strings.Contains(view, "Write this firmware?") {
		t.Errorf("view does not prompt:\n%s", view)
	}
	// Both Keychrons share the candidate list, so the ambiguity must be shown.
	if !strings.Contains(view, "could be any of") {
		t.Errorf("view does not warn about ambiguity:\n%s", view)
	}
}

func TestDeclineReturnsToWaiting(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), DryRun: true})
	m.Update(bootloaderArrived(board.STM32DFU.ID))

	m.Update(key("n"))

	if m.state != stateWaiting {
		t.Fatalf("state = %v, want stateWaiting after declining", m.state)
	}
	if m.found != nil {
		t.Error("declined device was not cleared")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	path := firmware(t)
	m := New(context.Background(), Options{Firmware: path, DryRun: true})
	m.Update(bootloaderArrived(board.STM32DFU.ID))

	m.Update(key("y"))

	if m.state != stateDone {
		t.Fatalf("state = %v, want stateDone", m.state)
	}
	if !strings.Contains(m.message, "Nothing was written") {
		t.Errorf("message = %q, want it to say nothing was written", m.message)
	}
	joined := strings.Join(m.log, "\n")
	if !strings.Contains(joined, "would run: dfu-util") {
		t.Errorf("log = %q, want the command it would have run", joined)
	}
	if !strings.Contains(joined, path) {
		t.Errorf("log = %q, want the firmware path", joined)
	}
}

func TestUnknownDeviceDoesNotTriggerConfirm(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), DryRun: true})

	m.Update(bootloaderArrived(usb.ID{Vendor: 0xdead, Product: 0xbeef}))

	if m.state != stateWaiting {
		t.Fatalf("state = %v, want stateWaiting for an unrecognised device", m.state)
	}
	if !strings.Contains(strings.Join(m.log, "\n"), "unrecognised device dead:beef") {
		t.Errorf("unknown device was not logged; that log line is how new boards get added")
	}
}

func TestBoardRestrictionIgnoresBootloadersItCannotBe(t *testing.T) {
	// A bootloader that no known board lists must not satisfy --board.
	unrelated := board.Bootloader{
		Name:    "Unrelated DFU",
		ID:      usb.ID{Vendor: 0x1209, Product: 0x0001},
		Flasher: "dfu-util",
	}
	old := board.Bootloaders
	board.Bootloaders = append(append([]board.Bootloader{}, old...), unrelated)
	t.Cleanup(func() { board.Bootloaders = old })

	target, ok := board.ByNormalID(usb.ID{Vendor: 0x3434, Product: 0x0b10})
	if !ok {
		t.Fatal("Q1 HE missing from the table")
	}

	m := New(context.Background(), Options{Firmware: firmware(t), DryRun: true, Board: &target})
	m.Update(bootloaderArrived(unrelated.ID))

	if m.state != stateWaiting {
		t.Fatalf("state = %v, want stateWaiting; --board must reject bootloaders that board cannot be", m.state)
	}
	if !strings.Contains(strings.Join(m.log, "\n"), "cannot be a Keychron Q1 HE") {
		t.Errorf("log = %q, want the mismatch explained", m.log)
	}
}

func TestQuitCancels(t *testing.T) {
	m := New(context.Background(), Options{Firmware: firmware(t), DryRun: true})
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("q produced no command, expected tea.Quit")
	}
	if err := m.ctx.Err(); err == nil {
		t.Error("q did not cancel the watcher context")
	}
}
