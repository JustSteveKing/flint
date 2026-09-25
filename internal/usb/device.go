// Package usb enumerates USB devices by reading sysfs directly.
//
// Every Go USB binding worth using is cgo over libusb or hidapi, and that
// costs the static binary. Reading /sys/bus/usb/devices gives us everything
// flint needs (vendor, product, strings, and when a device appears or goes
// away) with no dependencies at all.
package usb

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SysfsRoot is where the kernel exposes USB devices.
//
// FLINT_SYSFS overrides it, which is how the flash path gets exercised without
// putting real hardware into bootloader mode: point it at a directory, create
// a fake device in it, and flint sees a board appear.
var SysfsRoot = defaultSysfsRoot()

func defaultSysfsRoot() string {
	if root := os.Getenv("FLINT_SYSFS"); root != "" {
		return root
	}
	return "/sys/bus/usb/devices"
}

// rootHubVendor is the kernel's own virtual root hubs, which are never
// anything anyone wants to flash.
const rootHubVendor = 0x1d6b

// ID is a USB vendor/product pair.
type ID struct {
	Vendor  uint16
	Product uint16
}

func (i ID) String() string {
	return fmt.Sprintf("%04x:%04x", i.Vendor, i.Product)
}

// ParseID reads the "3434:0b10" form used by lsusb and by flint's own output.
func ParseID(s string) (ID, error) {
	v, p, ok := strings.Cut(s, ":")
	if !ok {
		return ID{}, fmt.Errorf("want vendor:product, got %q", s)
	}
	vendor, err := strconv.ParseUint(strings.TrimPrefix(v, "0x"), 16, 16)
	if err != nil {
		return ID{}, fmt.Errorf("bad vendor %q: %w", v, err)
	}
	product, err := strconv.ParseUint(strings.TrimPrefix(p, "0x"), 16, 16)
	if err != nil {
		return ID{}, fmt.Errorf("bad product %q: %w", p, err)
	}
	return ID{Vendor: uint16(vendor), Product: uint16(product)}, nil
}

// Device is one USB device as sysfs describes it.
type Device struct {
	ID           ID
	Manufacturer string
	Product      string
	Serial       string
	SysPath      string
}

// Label is the friendliest name the device gave us, falling back to the ID.
func (d Device) Label() string {
	switch {
	case d.Manufacturer != "" && d.Product != "":
		// Keychron sets both to "Keychron Keychron Q1 HE"-ish shapes, so
		// avoid repeating the manufacturer when the product already carries it.
		if strings.HasPrefix(d.Product, d.Manufacturer) {
			return d.Product
		}
		return d.Manufacturer + " " + d.Product
	case d.Product != "":
		return d.Product
	case d.Manufacturer != "":
		return d.Manufacturer
	default:
		return d.ID.String()
	}
}

// List returns every USB device currently enumerated, root hubs excluded.
func List() ([]Device, error) {
	entries, err := os.ReadDir(SysfsRoot)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", SysfsRoot, err)
	}

	var devices []Device
	for _, entry := range entries {
		path := filepath.Join(SysfsRoot, entry.Name())

		// Interfaces live here too, as "1-1:1.0". Only whole devices carry
		// idVendor, so its absence is the filter.
		vendor, ok := readHex(path, "idVendor")
		if !ok {
			continue
		}
		product, ok := readHex(path, "idProduct")
		if !ok {
			continue
		}
		if vendor == rootHubVendor {
			continue
		}

		devices = append(devices, Device{
			ID:           ID{Vendor: vendor, Product: product},
			Manufacturer: readString(path, "manufacturer"),
			Product:      readString(path, "product"),
			Serial:       readString(path, "serial"),
			SysPath:      path,
		})
	}
	return devices, nil
}

func readString(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readHex(dir, name string) (uint16, bool) {
	s := readString(dir, name)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, false
	}
	return uint16(v), true
}
