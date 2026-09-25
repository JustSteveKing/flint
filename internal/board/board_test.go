package board

import (
	"strings"
	"testing"

	"github.com/JustSteveKing/flint/internal/usb"
)

func TestCommandSubstitutesFile(t *testing.T) {
	name, args := STM32DFU.Command("/tmp/q1he.bin")
	if name != "dfu-util" {
		t.Errorf("flasher = %q, want dfu-util", name)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "/tmp/q1he.bin") {
		t.Errorf("args = %q, want the firmware path", joined)
	}
	if strings.Contains(joined, FilePlaceholder) {
		t.Errorf("args = %q, placeholder was not substituted", joined)
	}
}

func TestCommandDoesNotMutateTable(t *testing.T) {
	// Command builds a fresh slice; if it wrote through to Bootloader.Args the
	// second flash of a session would target the first flash's file.
	STM32DFU.Command("/tmp/first.bin")
	_, args := STM32DFU.Command("/tmp/second.bin")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "first.bin") {
		t.Fatalf("args = %q, table was mutated by an earlier call", joined)
	}
	if !strings.Contains(joined, "second.bin") {
		t.Fatalf("args = %q, want second.bin", joined)
	}
}

func TestByNormalID(t *testing.T) {
	b, ok := ByNormalID(usb.ID{Vendor: 0x3434, Product: 0x0b10})
	if !ok {
		t.Fatal("Q1 HE not found by its running ID")
	}
	if b.Name != "Keychron Q1 HE" {
		t.Errorf("Name = %q", b.Name)
	}
	if _, ok := ByNormalID(usb.ID{Vendor: 0xdead, Product: 0xbeef}); ok {
		t.Error("unknown ID matched a board")
	}
}

func TestBootloaderByID(t *testing.T) {
	bl, ok := BootloaderByID(usb.ID{Vendor: 0x0483, Product: 0xdf11})
	if !ok {
		t.Fatal("STM32 DFU not found")
	}
	if bl.Flasher != "dfu-util" {
		t.Errorf("Flasher = %q", bl.Flasher)
	}
}

func TestCandidatesForIsAmbiguousUntilProven(t *testing.T) {
	// Every board currently lists both bootloaders because none has been
	// observed yet. When one is confirmed the list shortens, and this test
	// should be updated rather than deleted: it documents why flint asks.
	got := CandidatesFor(STM32DFU)
	if len(got) < 2 {
		t.Skip("bootloaders have been narrowed; ambiguity warning no longer applies")
	}
	for _, b := range got {
		if b.Enter == "" {
			t.Errorf("%s has no instructions for entering bootloader mode", b.Name)
		}
	}
}

func TestEveryKnownBoardIsUsable(t *testing.T) {
	seen := map[usb.ID]string{}
	for _, b := range Known {
		if b.Name == "" {
			t.Error("a board has no name")
		}
		if prev, dup := seen[b.Normal]; dup {
			t.Errorf("%s and %s share the running ID %s", prev, b.Name, b.Normal)
		}
		seen[b.Normal] = b.Name
		if len(b.Bootloaders) == 0 {
			t.Errorf("%s lists no bootloaders, so it can never be flashed", b.Name)
		}
		if b.Enter == "" {
			t.Errorf("%s has no instructions for entering bootloader mode", b.Name)
		}
	}
}
