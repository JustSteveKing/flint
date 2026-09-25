# flint

QMK Toolbox does not run on Linux, and it is not going to. It is a Windows and macOS app, and the QMK docs answer the question by sending Linux users to the command line.

That answer is not wrong. `dfu-util`, `dfu-programmer` and `wb32-dfu-updater_cli` have been packaged on Linux for years, and they do the actual flashing. What Toolbox adds on top of them is the part people miss: it sits there watching USB, notices the moment your board drops into bootloader mode, works out what it is, and runs the right tool with the right arguments before you have finished letting go of the key.

flint is that part.

## Install

```sh
go install github.com/JustSteveKing/flint@latest
```

You also need whichever flasher your board's bootloader wants:

| Bootloader | Tool | Arch package |
|---|---|---|
| STM32 DfuSe | `dfu-util` | `pacman -S dfu-util` |
| WB32 | `wb32-dfu-updater_cli` | `yay -S wb32-dfu-updater_cli-git` |
| Atmel DFU | `dfu-programmer` | `pacman -S dfu-programmer` |

If one is missing, flint names the package rather than leaving you with "command not found".

## Use it

```sh
flint
```

That is the whole interface. It asks which keyboard, which firmware file, and then tells you how to put that specific board into bootloader mode:

```
flint  step 3 of 4
Keychron Q1 HE  ·  ~/Downloads/q1he_v1.2.bin

Put Keychron Q1 HE into bootloader mode

  1. Switch the keyboard to wired mode.
  2. Hold down Esc.
  3. Unplug the USB cable. Keep holding Esc.
  4. Plug the cable back in, still holding Esc.
  5. Let go. The board will look dead, with no lighting. That is correct.

  ✓ Keychron Q1 HE seen
  ✓ unplugged
  ⣾ waiting for a bootloader to appear
```

The checklist is driven by real USB events. Watching "unplugged" tick is how you know step 3 worked, and it happens a couple of seconds before the bootloader shows up, which is the gap where you would otherwise be wondering whether you had held the key long enough.

Then it shows you exactly what it is about to run and waits for a `y`.

If you already know what you are doing, the flags skip whatever you have answered:

```sh
flint flash ~/Downloads/firmware.bin                    # starts at "which keyboard"
flint flash --board "Q1 HE" ~/Downloads/firmware.bin    # starts at the instructions
flint flash --dry-run ~/Downloads/firmware.bin          # rehearse, write nothing
```

There are three read-only commands too. `flint devices` lists what is plugged in that flint recognises, `flint boards` prints the table, and `flint watch` reports USB arrivals without touching anything.

## What it will not do

flint does not implement DFU. Reimplementing a flash protocol is how you brick a keyboard, and the existing tools are correct. Everything below the confirmation prompt is `exec`.

It also refuses to write when it is unsure:

- Nothing is flashed without an explicit `y`, and the exact command is on screen before you give it.
- A bootloader whose arguments came from vendor documentation rather than from a successful run is marked unverified, and warns harder.
- When more than one known board could be sitting behind a bootloader, it says so and names them. `--board "Q1 HE"` turns that guess into a check.

## The honest bit

I have not flashed anything with it yet.

The state machine has 35 tests and they cover every transition, but a test cannot tell you whether `wb32-dfu-updater_cli -D file -R` is the right incantation, and those arguments are transcribed from the vendor's own usage rather than proven against hardware. Only the STM32 path is marked verified.

The bootloader IDs are the other gap. A keyboard in bootloader mode stops identifying itself as a keyboard: it comes back as a generic DFU device from the MCU vendor, so the only way to know what you are about to overwrite is to have written it down first. Every board in the table currently lists both candidates, which is why the ambiguity warning fires on all of them.

Narrowing it takes about thirty seconds per board. Run `flint`, pick **Identify a board**, do the unplug dance, and read the ID it prints.

If the vendor ships a working updater for your board, use that instead. flint is for the ones where the updater is a Windows binary.

## Adding a board

1. `flint devices` gives you the running ID.
2. `flint watch`, then bootloader mode. The ID that appears is the bootloader.
3. Add both to `Known` in `internal/board/board.go`, with `Steps` for getting into bootloader mode.

Write the steps as things to do in order. The walkthrough renders them as a numbered list and people follow them while holding a key down with their other hand.

Once you have confirmed a flasher's arguments work, set `Verified: true` and narrow that board's `Bootloaders` to the one it actually uses.

## Why it reads sysfs

Every Go USB binding worth using is cgo over libusb or hidapi, and that costs the static binary. `/sys/bus/usb/devices` gives vendor, product, the descriptor strings and hotplug for nothing, with no dependencies at all.

Polling rather than a netlink uevent socket is deliberate too. Netlink drops events silently when the reader falls behind, and a quarter-second poll of a directory of symlinks costs nothing worth measuring.

## Tests

```sh
go test ./...
```

`FLINT_SYSFS` points the enumerator at a directory instead of the real thing, which is how the flash path is exercised without hardware:

```sh
mkdir -p /tmp/fake/1-2
printf '0483\n' > /tmp/fake/1-2/idVendor
printf 'df11\n' > /tmp/fake/1-2/idProduct
FLINT_SYSFS=/tmp/fake flint watch
```

## Licence

MIT. See [LICENSE](LICENSE).
