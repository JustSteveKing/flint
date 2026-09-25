package flash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSteveKing/flint/internal/board"
)

func writeFile(t *testing.T, name string, size int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckFirmware(t *testing.T) {
	if err := CheckFirmware(writeFile(t, "q1he.bin", 64*1024)); err != nil {
		t.Errorf("valid firmware rejected: %v", err)
	}
	if err := CheckFirmware(filepath.Join(t.TempDir(), "absent.bin")); err == nil {
		t.Error("missing file accepted")
	}
	if err := CheckFirmware(writeFile(t, "empty.bin", 0)); err == nil {
		t.Error("empty file accepted")
	}
	if err := CheckFirmware(writeFile(t, "huge.bin", maxFirmwareSize+1)); err == nil {
		t.Error("oversized file accepted")
	}
	// The one that actually bites: pointing at the zip you downloaded.
	if err := CheckFirmware(writeFile(t, "firmware.zip", 1024)); err == nil {
		t.Error("archive accepted as firmware")
	}
	if err := CheckFirmware(t.TempDir()); err == nil {
		t.Error("directory accepted as firmware")
	}
}

func TestDescribeQuotesPathsWithSpaces(t *testing.T) {
	got := Describe(board.STM32DFU, "/home/steve/My Firmware/q1he.bin")
	if !strings.Contains(got, `"/home/steve/My Firmware/q1he.bin"`) {
		t.Errorf("Describe() = %q, want the spaced path quoted", got)
	}
}

func TestParsePercent(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{
		{"Download\t[=========       ]  35%        12345 bytes", 35},
		{"Download\t[================] 100%        28672 bytes", 100},
		{"Download done.", -1},
		{"", -1},
		{"error: 900% nonsense", -1},
	} {
		if got := parsePercent(tc.line); got != tc.want {
			t.Errorf("parsePercent(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestScanSplitsOnCarriageReturns(t *testing.T) {
	// dfu-util redraws its progress bar with \r. Splitting only on \n buffers
	// the whole flash into one token that arrives after it has finished.
	input := "Opening DFU capable USB device...\r 10%\r 50%\r100%\nDownload done.\n"
	var tokens []string
	data := []byte(input)
	for len(data) > 0 {
		advance, token, err := scanLinesAndRedraws(data, true)
		if err != nil {
			t.Fatal(err)
		}
		if advance == 0 {
			break
		}
		tokens = append(tokens, strings.TrimSpace(string(token)))
		data = data[advance:]
	}

	want := []string{"Opening DFU capable USB device...", "10%", "50%", "100%", "Download done."}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens %q, want %d", len(tokens), tokens, len(want))
	}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, tokens[i], want[i])
		}
	}
}

func TestCheckFlasherNamesThePackage(t *testing.T) {
	missing := board.Bootloader{Name: "Fake", Flasher: "wb32-dfu-updater_cli"}
	err := CheckFlasher(missing)
	if err == nil {
		t.Skip("wb32-dfu-updater_cli is installed on this machine")
	}
	if !strings.Contains(err.Error(), "AUR") {
		t.Errorf("error = %q, want it to name where the package comes from", err)
	}
}
