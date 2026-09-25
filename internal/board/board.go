// Package board holds what flint knows about keyboards and their bootloaders.
//
// This is deliberately a table rather than detection logic. A keyboard in
// bootloader mode no longer identifies itself as a keyboard: it comes back as
// a generic DFU device from the MCU vendor, so the only way to know what you
// are about to overwrite is to have written it down.
package board

import (
	"strings"

	"github.com/JustSteveKing/flint/internal/usb"
)

// Bootloader is a DFU device flint knows how to drive.
type Bootloader struct {
	// Name is what the user sees.
	Name string
	// ID is how the device enumerates once it is in bootloader mode.
	ID usb.ID
	// Flasher is the executable that does the work.
	Flasher string
	// Args are passed to Flasher, with FilePlaceholder substituted for the
	// firmware path.
	Args []string
	// Verified records whether these arguments have actually been run
	// successfully against hardware, as opposed to transcribed from docs.
	// Anything false prompts harder before it flashes.
	Verified bool
}

// FilePlaceholder is replaced with the firmware path in Bootloader.Args.
const FilePlaceholder = "%FILE%"

// Command returns the flasher and arguments for a given firmware file.
func (b Bootloader) Command(firmware string) (string, []string) {
	args := make([]string, len(b.Args))
	for i, a := range b.Args {
		args[i] = strings.ReplaceAll(a, FilePlaceholder, firmware)
	}
	return b.Flasher, args
}

// STM32DFU is ST's DfuSe bootloader, which is what most ARM-based keyboards
// expose. The 0x08000000 base is the start of STM32 internal flash, and
// ":leave" tells the bootloader to jump to the new firmware when the download
// finishes instead of sitting there waiting for a power cycle.
var STM32DFU = Bootloader{
	Name:     "STM32 DfuSe",
	ID:       usb.ID{Vendor: 0x0483, Product: 0xdf11},
	Flasher:  "dfu-util",
	Args:     []string{"-a", "0", "-s", "0x08000000:leave", "-D", FilePlaceholder},
	Verified: true,
}

// WB32DFU is Westberry's bootloader, used by a lot of newer Keychron boards.
// It speaks something close enough to DFU to look familiar and far enough from
// it that dfu-util will not do. These arguments are transcribed from the
// vendor tool's usage, not yet proven here, hence Verified: false.
var WB32DFU = Bootloader{
	Name:     "WB32 DFU",
	ID:       usb.ID{Vendor: 0x342d, Product: 0xdfa0},
	Flasher:  "wb32-dfu-updater_cli",
	Args:     []string{"-D", FilePlaceholder, "-R"},
	Verified: false,
}

// AtmelDFU covers the older AVR boards, kept because they are everywhere in
// the QMK world even though nothing on this desk uses one.
var AtmelDFU = Bootloader{
	Name:     "Atmel DFU",
	ID:       usb.ID{Vendor: 0x03eb, Product: 0x2ff4},
	Flasher:  "dfu-programmer",
	Args:     []string{"atmega32u4", "flash", FilePlaceholder},
	Verified: false,
}

// Bootloaders is every DFU device flint recognises.
var Bootloaders = []Bootloader{STM32DFU, WB32DFU, AtmelDFU}

// Board is a keyboard in its normal, running state.
type Board struct {
	// Name is what the user sees.
	Name string
	// Normal is how the board enumerates when it is being a keyboard.
	Normal usb.ID
	// Bootloaders are the DFU devices this board might come back as. More
	// than one means flint does not yet know which, and will say so.
	Bootloaders []Bootloader
	// Steps are how a human puts this board into bootloader mode, one
	// instruction per line. Written as things to do in order, because the
	// walkthrough renders them as a numbered list and people follow those
	// while holding a key down with their other hand.
	Steps []string
	// Firmware is where the official firmware for this board comes from.
	Firmware string
}

// Known is the table. Add a board by observing it: `flint devices` prints the
// running ID, and `flint watch` prints the bootloader ID when it appears.
var Known = []Board{
	{
		Name:   "Keychron Q1 HE",
		Normal: usb.ID{Vendor: 0x3434, Product: 0x0b10},
		// Hall-effect board on Keychron's own firmware rather than QMK.
		// Launcher offers "STM32, WB, DFU in FS Mode" without saying which,
		// so both candidates stay until one is seen.
		Bootloaders: []Bootloader{STM32DFU, WB32DFU},
		Steps: []string{
			"Switch the keyboard to wired mode.",
			"Hold down Esc.",
			"Unplug the USB cable. Keep holding Esc.",
			"Plug the cable back in, still holding Esc.",
			"Let go. The board will look dead, with no lighting. That is correct.",
		},
		Firmware: "https://www.keychron.com/pages/firmware-and-json-files-of-the-keychron-he-series-keyboards",
	},
	{
		Name:        "Keychron Q0 Mini 8K",
		Normal:      usb.ID{Vendor: 0x3434, Product: 0x040b},
		Bootloaders: []Bootloader{STM32DFU, WB32DFU},
		Steps: []string{
			"Hold down the top-left key.",
			"Unplug the USB cable. Keep holding.",
			"Plug the cable back in, still holding.",
			"Let go. The board will look dead. That is correct.",
		},
		Firmware: "https://www.keychron.com/pages/firmware",
	},
	{
		Name:        "Keychron Nape Pro",
		Normal:      usb.ID{Vendor: 0x3434, Product: 0x0440},
		Bootloaders: []Bootloader{STM32DFU, WB32DFU},
		Steps: []string{
			"Switch the keyboard to wired mode.",
			"Hold down Esc.",
			"Unplug the USB cable. Keep holding Esc.",
			"Plug the cable back in, still holding Esc.",
			"Let go. The board will look dead. That is correct.",
		},
		Firmware: "https://www.keychron.com/pages/firmware",
	},
}

// GenericSteps is the best guess for a board that is not in the table. It is
// what most QMK and QMK-adjacent boards want, and it is worth showing rather
// than leaving someone with a blank screen.
var GenericSteps = []string{
	"Switch the keyboard to wired mode if it has one.",
	"Hold down Esc, or press the board's reset button twice.",
	"Unplug the USB cable. Keep holding.",
	"Plug the cable back in, still holding.",
	"Let go. The board will look dead. That is correct.",
}

// ByNormalID finds a known board by how it enumerates when running.
func ByNormalID(id usb.ID) (Board, bool) {
	for _, b := range Known {
		if b.Normal == id {
			return b, true
		}
	}
	return Board{}, false
}

// BootloaderByID finds a known bootloader by how it enumerates.
func BootloaderByID(id usb.ID) (Bootloader, bool) {
	for _, b := range Bootloaders {
		if b.ID == id {
			return b, true
		}
	}
	return Bootloader{}, false
}

// CandidatesFor returns the boards that could be sitting behind a given
// bootloader. A DFU device says nothing about which keyboard it is, so this is
// a list, and a list longer than one is the reason flint asks before it writes.
func CandidatesFor(bl Bootloader) []Board {
	var out []Board
	for _, b := range Known {
		for _, candidate := range b.Bootloaders {
			if candidate.ID == bl.ID {
				out = append(out, b)
				break
			}
		}
	}
	return out
}
