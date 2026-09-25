package usb

import (
	"context"
	"time"
)

// DefaultInterval is fast enough that a board entering bootloader mode feels
// instant, and slow enough that the poll is invisible in top.
const DefaultInterval = 250 * time.Millisecond

// EventKind says whether a device arrived or left.
type EventKind int

const (
	Added EventKind = iota
	Removed
)

func (k EventKind) String() string {
	if k == Added {
		return "added"
	}
	return "removed"
}

// Event is one change to the set of connected devices.
type Event struct {
	Kind   EventKind
	Device Device
	Err    error
}

// Watch polls sysfs and reports devices appearing and disappearing until the
// context is cancelled. The first poll is treated as the baseline and does not
// emit Added events, so callers see changes rather than what was already there;
// use List for the initial state.
//
// Polling rather than a netlink uevent socket is deliberate. Netlink needs the
// process to keep up with every uevent on the system or it drops them silently,
// and a quarter-second poll of a directory of symlinks costs nothing.
func Watch(ctx context.Context, interval time.Duration) <-chan Event {
	if interval <= 0 {
		interval = DefaultInterval
	}
	events := make(chan Event)

	go func() {
		defer close(events)

		known := make(map[string]Device)
		if initial, err := List(); err == nil {
			for _, d := range initial {
				known[d.SysPath] = d
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			current, err := List()
			if err != nil {
				select {
				case events <- Event{Err: err}:
				case <-ctx.Done():
					return
				}
				continue
			}

			seen := make(map[string]bool, len(current))
			for _, d := range current {
				seen[d.SysPath] = true
				if _, ok := known[d.SysPath]; ok {
					continue
				}
				known[d.SysPath] = d
				select {
				case events <- Event{Kind: Added, Device: d}:
				case <-ctx.Done():
					return
				}
			}

			for path, d := range known {
				if seen[path] {
					continue
				}
				delete(known, path)
				select {
				case events <- Event{Kind: Removed, Device: d}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return events
}
