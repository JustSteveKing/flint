package tui

import (
	"fmt"
	"strings"

	"github.com/JustSteveKing/flint/internal/board"
	"github.com/JustSteveKing/flint/internal/flash"
	"github.com/JustSteveKing/flint/internal/usb"
)

// stepNumber maps a step to its place in the flashing sequence, so the header
// can say "Step 2 of 4". The read-only modes are not part of it.
func (m *Model) stepNumber() (n, total int) {
	total = 4
	switch m.step {
	case stepBoard:
		return 1, total
	case stepFirmware:
		return 2, total
	case stepPrepare:
		return 3, total
	case stepConfirm, stepFlashing:
		return 4, total
	}
	return 0, total
}

func (m *Model) View() string {
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteString("\n")

	switch m.step {
	case stepMenu:
		b.WriteString(m.viewMenu())
	case stepBoard:
		b.WriteString(m.viewBoard())
	case stepFirmware:
		b.WriteString(m.viewFirmware())
	case stepPrepare:
		b.WriteString(m.viewPrepare())
	case stepConfirm:
		b.WriteString(m.viewConfirm())
	case stepFlashing:
		b.WriteString(m.viewFlashing())
	case stepResult:
		b.WriteString(m.viewResult())
	case stepIdentify:
		b.WriteString(m.viewIdentify())
	case stepInspect:
		b.WriteString(m.viewInspect())
	}

	b.WriteString("\n" + dimStyle.Render(m.hints()) + "\n")
	return b.String()
}

func (m *Model) header() string {
	line := titleStyle.Render("flint")
	if m.opts.DryRun {
		line += " " + warnStyle.Render("[dry run]")
	}
	if n, total := m.stepNumber(); n > 0 {
		line += "  " + stepStyle.Render(fmt.Sprintf("step %d of %d", n, total))
	}

	var context []string
	if m.target != nil && m.step > stepBoard {
		context = append(context, m.target.Name)
	}
	if m.firmware != "" && m.step > stepFirmware {
		context = append(context, m.firmware)
	}
	if len(context) > 0 {
		line += "\n" + dimStyle.Render(strings.Join(context, "  ·  "))
	}
	return line + "\n"
}

func (m *Model) viewMenu() string {
	return "\n" + m.menu.View()
}

func (m *Model) viewBoard() string {
	var b strings.Builder
	b.WriteString("\n" + headingStyle.Render("Which keyboard?") + "\n\n")
	b.WriteString(m.boards.View())

	if sel := m.boards.Selected(); sel != nil {
		if target, _ := sel.Value.(*board.Board); target != nil && target.Firmware != "" {
			b.WriteString("\n" + dimStyle.Render("Firmware for this board: "+target.Firmware) + "\n")
		}
	}
	return b.String()
}

func (m *Model) viewFirmware() string {
	var b strings.Builder
	b.WriteString("\n" + headingStyle.Render("Which firmware file?") + "\n")
	b.WriteString(dimStyle.Render("Only .bin, .hex, .dfu and .uf2 can be picked.") + "\n\n")
	b.WriteString(m.picker.View() + "\n")

	if len(m.log) > 0 {
		b.WriteString("\n" + errStyle.Render(m.log[len(m.log)-1]) + "\n")
	}
	return b.String()
}

func (m *Model) viewPrepare() string {
	var b strings.Builder

	name := "the keyboard"
	if m.target != nil {
		name = m.target.Name
	}

	b.WriteString("\n" + headingStyle.Render("Put "+name+" into bootloader mode") + "\n\n")
	for i, s := range m.steps() {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, s))
	}

	b.WriteString("\n" + m.checklist() + "\n")

	if len(m.log) > 0 {
		b.WriteString("\n" + logStyle.Render("  "+m.log[len(m.log)-1]) + "\n")
	}
	return b.String()
}

// checklist is what turns a blocking wait into visible progress. Watching the
// board disappear is the confirmation that step 3 worked, and it arrives
// seconds before the bootloader does.
func (m *Model) checklist() string {
	var b strings.Builder

	if m.target != nil {
		b.WriteString("  " + mark(m.boardSeen) + " " + m.target.Name + " seen\n")
		b.WriteString("  " + mark(m.boardGone) + " unplugged\n")
	}
	b.WriteString("  " + m.spin.View() + " waiting for a bootloader to appear\n")
	return b.String()
}

func mark(done bool) string {
	if done {
		return okStyle.Render("✓")
	}
	return dimStyle.Render("·")
}

func (m *Model) viewConfirm() string {
	var b strings.Builder
	b.WriteString("\n" + warnStyle.Render("Bootloader detected") + "\n\n")
	b.WriteString("  Device:   " + m.bootloader.Name + " " + dimStyle.Render("("+m.found.ID.String()+")") + "\n")

	switch len(m.candidates) {
	case 0:
		b.WriteString("  Board:    " + errStyle.Render("unknown") + "\n")
	case 1:
		b.WriteString("  Board:    " + m.candidates[0].Name + "\n")
	default:
		names := make([]string, len(m.candidates))
		for i, c := range m.candidates {
			names[i] = c.Name
		}
		b.WriteString("  Board:    " + warnStyle.Render("could be any of: "+strings.Join(names, ", ")) + "\n")
	}

	b.WriteString("  Firmware: " + m.firmware + "\n")
	b.WriteString("  Command:  " + codeStyle.Render(flash.Describe(m.bootloader, m.firmware)) + "\n")

	var warnings []string
	if !m.bootloader.Verified {
		warnings = append(warnings,
			"These flasher arguments have never been run against real hardware.",
			"Check them against the vendor's own instructions before saying yes.")
	}
	if len(m.candidates) > 1 {
		warnings = append(warnings,
			"More than one known board could be behind this bootloader.",
			"Make sure this firmware is for the one you just unplugged.")
	}
	if len(warnings) > 0 {
		b.WriteString("\n" + boxStyle.Render(warnStyle.Render(strings.Join(warnings, "\n"))) + "\n")
	}

	b.WriteString("\n" + headingStyle.Render("Write this firmware? (y/n)") + "\n")
	return b.String()
}

func (m *Model) viewFlashing() string {
	var b strings.Builder
	b.WriteString("\n" + headingStyle.Render("Flashing") + "  " + errStyle.Render("Do not unplug the keyboard.") + "\n\n")
	pct := 0.0
	if m.percent > 0 {
		pct = float64(m.percent) / 100
	}
	b.WriteString("  " + m.prog.ViewAs(pct) + "\n\n")
	b.WriteString(boxStyle.Render(m.out.View()) + "\n")
	return b.String()
}

func (m *Model) viewResult() string {
	var b strings.Builder
	b.WriteString("\n")

	if m.err != nil {
		b.WriteString(errStyle.Render("Failed") + "\n\n")
		b.WriteString("  " + m.err.Error() + "\n")
		b.WriteString("\n" + dimStyle.Render("The board is probably still in bootloader mode, so you can try again") + "\n")
		b.WriteString(dimStyle.Render("without repeating the unplug dance.") + "\n")
	} else {
		b.WriteString(okStyle.Render("Done") + "\n\n")
		b.WriteString("  " + m.result + "\n")
	}

	if m.learned != "" {
		b.WriteString("\n" + boxStyle.Render(
			headingStyle.Render("Worth writing down")+"\n"+
				m.learned+"\n"+
				dimStyle.Render("Narrow this board's Bootloaders list in internal/board/board.go."),
		) + "\n")
	}

	if tail := m.logTail(12); tail != "" {
		b.WriteString("\n" + boxStyle.Render(tail) + "\n")
	}
	return b.String()
}

// logTail renders the last n lines without a viewport's padding, for the
// screens where the output has stopped growing.
func (m *Model) logTail(n int) string {
	if len(m.log) == 0 {
		return ""
	}
	from := len(m.log) - n
	if from < 0 {
		from = 0
	}
	return strings.Join(m.log[from:], "\n")
}

func (m *Model) viewIdentify() string {
	var b strings.Builder
	b.WriteString("\n" + headingStyle.Render("Identify a board") + "\n")
	b.WriteString(dimStyle.Render("Nothing is written in this mode.") + "\n\n")

	for i, s := range board.GenericSteps {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, s))
	}
	b.WriteString("\n  " + m.spin.View() + " watching USB\n\n")

	if tail := m.logTail(12); tail == "" {
		b.WriteString(dimStyle.Render("  Nothing has changed yet.") + "\n")
	} else {
		b.WriteString(boxStyle.Render(tail) + "\n")
	}

	if m.learned != "" {
		b.WriteString("\n" + okStyle.Render("Latest arrival: ") + m.learned + "\n")
	}
	return b.String()
}

func (m *Model) viewInspect() string {
	var b strings.Builder
	b.WriteString("\n" + headingStyle.Render("Connected devices") + "\n\n")

	shown := 0
	for _, d := range m.connected {
		switch {
		case isBootloaderID(d.ID):
			bl, _ := board.BootloaderByID(d.ID)
			b.WriteString("  " + warnStyle.Render(d.ID.String()) + "  bootloader: " + bl.Name + "\n")
		case isKnownBoardID(d.ID):
			known, _ := board.ByNormalID(d.ID)
			b.WriteString("  " + okStyle.Render(d.ID.String()) + "  " + known.Name + "\n")
		default:
			continue
		}
		shown++
	}

	if shown == 0 {
		b.WriteString(dimStyle.Render("  Nothing flint recognises is connected.") + "\n")
	}
	return b.String()
}

func isBootloaderID(id usb.ID) bool {
	_, ok := board.BootloaderByID(id)
	return ok
}

func isKnownBoardID(id usb.ID) bool {
	_, ok := board.ByNormalID(id)
	return ok
}

func (m *Model) hints() string {
	switch m.step {
	case stepMenu:
		return "↑↓ move · enter select · q quit"
	case stepBoard:
		return "↑↓ move · enter select · esc back · q quit"
	case stepFirmware:
		return "↑↓ move · enter open or select · esc back · q quit"
	case stepPrepare:
		return "waiting · esc back · q quit"
	case stepConfirm:
		return "y write · n skip · esc back · q quit"
	case stepFlashing:
		return "do not unplug · ctrl-c abort"
	case stepResult:
		return "enter start again · q quit"
	default:
		return "esc back · q quit"
	}
}
