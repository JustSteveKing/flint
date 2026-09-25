# AGENTS.md

Guidance for coding agents working in this repository. Read this before changing anything.

## What flint is

A Go TUI that flashes keyboard firmware on Linux. It does not implement DFU. It watches USB for a board entering bootloader mode, identifies it, and shells out to `dfu-util`, `wb32-dfu-updater_cli` or `dfu-programmer`.

The value is the loop around those tools, not the flashing itself.

## Commands

```sh
go build ./...
go vet ./...
gofmt -l .        # must print nothing
go test ./...
```

All four must pass before you finish. There is no separate linter.

## Layout

```
main.go                  entry point, nothing else
cmd/                     cobra commands: root, flash, devices, boards, watch
internal/usb/            sysfs enumeration and the hotplug watcher
internal/board/          the table of known boards and bootloaders
internal/flash/          runs the external flashers, parses their output
internal/tui/            the bubbletea walkthrough
```

## Invariants

Breaking any of these changes what flint is. Do not do it without being asked directly.

**No cgo.** Every Go USB binding worth using is cgo over libusb or hidapi, which costs the static binary. USB enumeration reads `/sys/bus/usb/devices` and nothing else. If you find yourself adding `github.com/google/gousb` or similar, stop.

**Do not implement DFU.** Reimplementing a flash protocol is how a keyboard gets bricked. Everything past the confirmation prompt is `exec.Command`.

**Never write without an explicit confirmation.** `stepConfirm` shows the exact command and waits for `y`. There is no flag that skips it and there should not be.

**`esc` and `q` are disabled during a write, and only during a write.** Interrupting a flasher mid-write is the one action here that can leave a board unusable. `ctrl-c` still works. Tests pin this down; if you change key handling, they will tell you.

**The board table is a table, not detection.** A keyboard in bootloader mode no longer identifies itself as a keyboard, so the only way to know what is behind a DFU device is to have recorded it. Do not add heuristics that guess.

## Testing without hardware

`internal/usb.SysfsRoot` is overridable, and `FLINT_SYSFS` sets it from the environment. Create a directory with `idVendor` and `idProduct` files in it and flint sees a device appear. Every USB and TUI test uses this; none of them need a keyboard.

The TUI tests drive the model directly with messages rather than through a terminal. `internal/tui/wizard_test.go` shows the pattern. Do not reach for a pty harness: bubbletea sends terminal capability queries that nothing answers in a headless environment, and you will spend an afternoon on it.

## Adding a board

Add to `Known` in `internal/board/board.go`. A board needs:

- `Normal`, how it enumerates while running
- `Bootloaders`, the DFU devices it might come back as
- `Steps`, how a human puts it into bootloader mode, as things to do in order
- `Firmware`, where the official firmware comes from

More than one entry in `Bootloaders` means flint does not know which, and it will warn before flashing. Narrowing that list requires observing a real device, so do not guess to silence the warning.

`Verified: false` on a `Bootloader` means its arguments came from vendor documentation and have never been run against hardware. Only flip it to true when someone has actually flashed with it.

## Conventions

Comments explain why, not what. `internal/usb/watcher.go` explains why polling beats netlink; that is the register. Do not annotate code that already reads clearly.

Errors name the fix where there is one. `flash.CheckFlasher` reports which package provides a missing tool rather than letting the caller see "command not found".

Tests are named for the behaviour they protect, and several carry a comment saying what would break without them. Keep that when you touch them.

## Current state

Nothing has been flashed with flint yet. The bootloader IDs for the boards in the table are unobserved, so every entry lists multiple candidates. Only the STM32 DfuSe arguments are verified.

If you are asked to narrow the table, the answer is not to infer it from documentation. It is `flint watch` against a real board.
