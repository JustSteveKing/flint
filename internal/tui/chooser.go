package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// choice is one line in a chooser.
type choice struct {
	Label string
	// Note is shown dimmed under the label when the line is selected.
	Note string
	// Value is whatever the caller needs back.
	Value any
}

// chooser is a short vertical menu.
//
// bubbles/list would do this, but it brings pagination, filtering and its own
// help line, none of which a four-item menu wants. Twenty lines here keeps
// every screen in flint looking like the same program.
type chooser struct {
	choices []choice
	cursor  int
}

func newChooser(choices ...choice) chooser {
	return chooser{choices: choices}
}

func (c *chooser) Update(msg tea.KeyMsg) (selected *choice) {
	switch msg.String() {
	case "up", "k":
		if c.cursor > 0 {
			c.cursor--
		}
	case "down", "j":
		if c.cursor < len(c.choices)-1 {
			c.cursor++
		}
	case "home", "g":
		c.cursor = 0
	case "end", "G":
		c.cursor = len(c.choices) - 1
	case "enter", " ":
		if len(c.choices) > 0 {
			return &c.choices[c.cursor]
		}
	}
	return nil
}

func (c chooser) Selected() *choice {
	if len(c.choices) == 0 {
		return nil
	}
	return &c.choices[c.cursor]
}

func (c chooser) View() string {
	var b strings.Builder
	for i, ch := range c.choices {
		if i == c.cursor {
			b.WriteString(cursorStyle.Render("  > "+ch.Label) + "\n")
			if ch.Note != "" {
				b.WriteString(dimStyle.Render("      "+ch.Note) + "\n")
			}
			continue
		}
		b.WriteString("    " + ch.Label + "\n")
	}
	return b.String()
}
