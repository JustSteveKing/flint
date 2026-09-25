// Package flash runs the underlying DFU tools and reports what they are doing.
//
// flint does not implement DFU. dfu-util and wb32-dfu-updater_cli already do
// that correctly, and reimplementing a flash protocol is how you brick a
// keyboard. What was missing on Linux is everything around them, so that is
// what lives here.
package flash

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/JustSteveKing/flint/internal/board"
)

// maxFirmwareSize is a sanity bound, not a hardware limit. Keyboard firmware
// runs to tens of kilobytes; anything past this is a file picked by mistake.
const maxFirmwareSize = 4 << 20 // 4 MiB

// packageFor names the Arch package providing each flasher, because "command
// not found" is a worse error than it needs to be.
var packageFor = map[string]string{
	"dfu-util":             "dfu-util (pacman)",
	"wb32-dfu-updater_cli": "wb32-dfu-updater_cli-git (AUR)",
	"dfu-programmer":       "dfu-programmer (pacman)",
	"avrdude":              "avrdude (pacman)",
}

// Update is one line of progress from the flasher.
type Update struct {
	// Line is the raw output, with carriage-return redraws already split out.
	Line string
	// Percent is -1 unless the line carried a percentage.
	Percent int
}

var percentRe = regexp.MustCompile(`(\d{1,3})%`)

// CheckFirmware rejects a file before anything touches the hardware.
func CheckFirmware(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("firmware: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("firmware: %s is a directory", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("firmware: %s is empty", path)
	}
	if info.Size() > maxFirmwareSize {
		return fmt.Errorf("firmware: %s is %d bytes, which is far too large to be keyboard firmware", path, info.Size())
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bin", ".hex", ".dfu", ".uf2":
	default:
		return fmt.Errorf("firmware: %s does not look like firmware (expected .bin, .hex, .dfu or .uf2)", path)
	}
	return nil
}

// CheckFlasher reports whether the tool a bootloader needs is installed.
func CheckFlasher(bl board.Bootloader) error {
	if _, err := exec.LookPath(bl.Flasher); err != nil {
		if pkg, ok := packageFor[bl.Flasher]; ok {
			return fmt.Errorf("%s is not installed; it comes from %s", bl.Flasher, pkg)
		}
		return fmt.Errorf("%s is not installed", bl.Flasher)
	}
	return nil
}

// Preflight runs every check that does not require touching the device.
func Preflight(bl board.Bootloader, firmware string) error {
	if err := CheckFirmware(firmware); err != nil {
		return err
	}
	return CheckFlasher(bl)
}

// Describe returns the command flint would run, for dry runs and for showing
// the user before they commit to it.
func Describe(bl board.Bootloader, firmware string) string {
	name, args := bl.Command(firmware)
	parts := append([]string{name}, args...)
	for i, p := range parts {
		if strings.ContainsAny(p, " \t") {
			parts[i] = strconv.Quote(p)
		}
	}
	return strings.Join(parts, " ")
}

// Run flashes firmware through the given bootloader, sending each line of
// output on the returned channel. The channel closes when the flasher exits;
// the error is delivered through the returned function, which blocks until
// then.
func Run(ctx context.Context, bl board.Bootloader, firmware string) (<-chan Update, func() error) {
	updates := make(chan Update)

	name, args := bl.Command(firmware)
	cmd := exec.CommandContext(ctx, name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		close(updates)
		return updates, func() error { return fmt.Errorf("stdout: %w", err) }
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		close(updates)
		return updates, func() error { return fmt.Errorf("stderr: %w", err) }
	}

	if err := cmd.Start(); err != nil {
		close(updates)
		return updates, func() error { return fmt.Errorf("starting %s: %w", name, err) }
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var tail []string
	var tailMu sync.Mutex

	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Split(scanLinesAndRedraws)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			tailMu.Lock()
			tail = append(tail, line)
			if len(tail) > 5 {
				tail = tail[1:]
			}
			tailMu.Unlock()

			select {
			case updates <- Update{Line: line, Percent: parsePercent(line)}:
			case <-ctx.Done():
				return
			}
		}
	}

	go scan(stdout)
	go scan(stderr)

	done := make(chan error, 1)
	go func() {
		wg.Wait()
		close(updates)
		err := cmd.Wait()
		if err != nil {
			tailMu.Lock()
			recent := strings.Join(tail, "; ")
			tailMu.Unlock()
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && recent != "" {
				err = fmt.Errorf("%s exited %d: %s", name, exitErr.ExitCode(), recent)
			} else {
				err = fmt.Errorf("%s: %w", name, err)
			}
		}
		done <- err
	}()

	return updates, func() error { return <-done }
}

func parsePercent(line string) int {
	m := percentRe.FindStringSubmatch(line)
	if m == nil {
		return -1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n > 100 {
		return -1
	}
	return n
}

// scanLinesAndRedraws splits on \n and on \r, because dfu-util draws its
// progress bar by returning to the start of the line. Scanning only on \n
// would buffer the entire flash into one enormous token that arrives after it
// is over.
func scanLinesAndRedraws(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		// Consume \r\n as one break rather than emitting an empty token.
		width := 1
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			width = 2
		}
		return i + width, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
