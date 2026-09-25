package usb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeDevice writes the sysfs attributes flint reads.
func fakeDevice(t *testing.T, root, name string, attrs map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for k, v := range attrs {
		if err := os.WriteFile(filepath.Join(dir, k), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func withFakeSysfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	old := SysfsRoot
	SysfsRoot = root
	t.Cleanup(func() { SysfsRoot = old })
	return root
}

func TestParseID(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    ID
		wantErr bool
	}{
		{in: "3434:0b10", want: ID{0x3434, 0x0b10}},
		{in: "0483:df11", want: ID{0x0483, 0xdf11}},
		{in: "0x3434:0x0b10", want: ID{0x3434, 0x0b10}},
		{in: "3434", wantErr: true},
		{in: "zzzz:0b10", wantErr: true},
		{in: "3434:zzzz", wantErr: true},
	} {
		got, err := ParseID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseID(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseID(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestIDString(t *testing.T) {
	if got := (ID{0x3434, 0x0b10}).String(); got != "3434:0b10" {
		t.Errorf("String() = %q, want 3434:0b10", got)
	}
}

func TestListSkipsHubsAndInterfaces(t *testing.T) {
	root := withFakeSysfs(t)

	fakeDevice(t, root, "usb1", map[string]string{
		"idVendor": "1d6b", "idProduct": "0002", "product": "xHCI Host Controller",
	})
	fakeDevice(t, root, "1-1", map[string]string{
		"idVendor": "3434", "idProduct": "0b10",
		"manufacturer": "Keychron", "product": "Keychron Q1 HE",
	})
	// An interface directory: no idVendor, so it must be ignored.
	fakeDevice(t, root, "1-1:1.0", map[string]string{"bInterfaceClass": "03"})

	devices, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1: %+v", len(devices), devices)
	}
	if devices[0].ID != (ID{0x3434, 0x0b10}) {
		t.Errorf("ID = %v, want 3434:0b10", devices[0].ID)
	}
}

func TestLabelDoesNotRepeatManufacturer(t *testing.T) {
	// Keychron sets product to "Keychron Q1 HE" and manufacturer to
	// "Keychron", which naively concatenated reads "Keychron Keychron Q1 HE".
	d := Device{Manufacturer: "Keychron", Product: "Keychron Q1 HE"}
	if got := d.Label(); got != "Keychron Q1 HE" {
		t.Errorf("Label() = %q, want %q", got, "Keychron Q1 HE")
	}

	d = Device{Manufacturer: "ZSA", Product: "Moonlander"}
	if got := d.Label(); got != "ZSA Moonlander" {
		t.Errorf("Label() = %q, want %q", got, "ZSA Moonlander")
	}

	d = Device{ID: ID{0x0483, 0xdf11}}
	if got := d.Label(); got != "0483:df11" {
		t.Errorf("Label() = %q, want the ID", got)
	}
}

func TestWatchReportsAddAndRemove(t *testing.T) {
	root := withFakeSysfs(t)
	fakeDevice(t, root, "1-1", map[string]string{"idVendor": "3434", "idProduct": "0b10"})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := Watch(ctx, 10*time.Millisecond)

	// The device present at startup is the baseline, not an event. Adding the
	// bootloader afterwards is what must be reported.
	time.Sleep(30 * time.Millisecond)
	dir := fakeDevice(t, root, "1-2", map[string]string{"idVendor": "0483", "idProduct": "df11"})

	ev := next(t, ctx, events)
	if ev.Kind != Added || ev.Device.ID != (ID{0x0483, 0xdf11}) {
		t.Fatalf("got %v %v, want added 0483:df11", ev.Kind, ev.Device.ID)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	ev = next(t, ctx, events)
	if ev.Kind != Removed || ev.Device.ID != (ID{0x0483, 0xdf11}) {
		t.Fatalf("got %v %v, want removed 0483:df11", ev.Kind, ev.Device.ID)
	}
}

func next(t *testing.T, ctx context.Context, ch <-chan Event) Event {
	t.Helper()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("watcher closed early")
			}
			if ev.Err != nil {
				t.Fatalf("watcher error: %v", ev.Err)
			}
			return ev
		case <-ctx.Done():
			t.Fatal("timed out waiting for a USB event")
		}
	}
}
