# flint

Flash keyboard firmware on Linux.

QMK Toolbox is Windows and macOS only. The flashing tools it wraps, `dfu-util`
and friends, have always been on Linux. What was missing is everything around
them: noticing a board drop into bootloader mode, knowing what it is, and not
writing the wrong firmware to it.

flint is that part. It does not implement DFU, because reimplementing a flash
protocol is how you brick a keyboard.

## Install

```bash
go install github.com/JustSteveKing/flint@latest
```

You also need the flasher for your board's bootloader:

| Bootloader | Tool | Where |
|---|---|---|
| STM32 DfuSe | `dfu-util` | `pacman -S dfu-util` |
| WB32 | `wb32-dfu-updater_cli` | `yay -S wb32-dfu-updater_cli-git` |
| Atmel DFU | `dfu-programmer` | `pacman -S dfu-programmer` |

flint names the missing package if one is not installed.

## Use

```bash
flint
```

That is the whole interface. flint walks you through it:

1. **Which keyboard**, from the boards it knows, with the connected ones marked.
2. **Which firmware**, from a file browser that only offers real firmware.
3. **How to get into bootloader mode**, as numbered steps for that specific
   board, with a checklist that ticks off as flint watches it happen. Seeing
   "unplugged" tick is how you know step 3 worked, and it arrives seconds
   before the bootloader does.
4. **Confirm**, showing exactly what was detected and the exact command, with
   anything uncertain called out.

Then live output while it writes, and a result screen that tells you what it
learned about your board.

The menu also has an **Identify a board** mode, which runs the same
walkthrough and reports what appears without writing anything. That is how you
find out what a new keyboard's bootloader is.

### Skipping ahead

Anything you already know is a step flint will not ask about:

```bash
flint flash ~/Downloads/firmware.bin                    # starts at "which keyboard"
flint flash --board "Q1 HE" ~/Downloads/firmware.bin    # starts at the instructions
flint flash --dry-run ~/Downloads/firmware.bin          # rehearse, write nothing
```

### Other commands

These are the non-interactive forms, for when you already know what you are
doing:

```bash
flint devices          # what is connected that flint recognises
flint devices --all    # every USB device
flint boards           # the table of known boards and bootloaders
flint watch            # print devices coming and going, flash nothing
```

## Adding a board

A keyboard in bootloader mode no longer identifies itself as a keyboard. It
comes back as a generic DFU device from the MCU vendor, so the only way to know
what is behind it is to have written it down.

1. `flint devices` gives the running ID.
2. `flint watch`, then enter bootloader mode. The ID that appears is the
   bootloader.
3. Add both to `Known` in `internal/board/board.go`, along with `Steps` for
   getting into bootloader mode. Those steps are what the walkthrough renders,
   so write them as things to do in order.

`flint` will also tell you what it learned after a successful flash, on the
result screen.

Bootloaders listed as unverified have arguments transcribed from vendor docs
rather than proven against hardware. flint warns before running one. When you
confirm a set works, set `Verified: true` and narrow that board's `Bootloaders`
to the one it actually uses.

## Why sysfs

Every Go USB binding worth using is cgo over libusb or hidapi, and that costs
the static binary. Reading `/sys/bus/usb/devices` gives vendor, product, the
descriptor strings and hotplug for nothing, with no dependencies.

Polling rather than a netlink uevent socket is also deliberate: netlink drops
events silently if the process cannot keep up, and a quarter-second poll of a
directory of symlinks costs nothing.

## Testing

```bash
go test ./...
```

`FLINT_SYSFS` points the enumerator at a directory instead of the real sysfs,
which is how the flash path is exercised without hardware:

```bash
mkdir -p /tmp/fake/1-2
printf '0483\n' > /tmp/fake/1-2/idVendor
printf 'df11\n' > /tmp/fake/1-2/idProduct
FLINT_SYSFS=/tmp/fake flint watch
```

## Keys

`esc` goes back a step, anywhere except during a write. `q` quits, except
during a write. `ctrl-c` always quits.

Interrupting a flasher mid-write is the one action here that can leave a board
unusable, so the two easy keys are taken away at exactly that moment and
nowhere else.

## Caveat

If the vendor ships a working updater, use it. flint is for the boards where
that updater is a Windows binary.
