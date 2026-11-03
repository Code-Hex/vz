# Native GUI examples

Run these commands from this directory. Use macOS 13 or later, Xcode Command Line Tools, and Go 1.27.3 or later.

```sh
make run FRAMEWORK=mygo
make run FRAMEWORK=fyne
make run FRAMEWORK=wails
make run FRAMEWORK=gio
```

Each command builds and signs the app before it starts. Without an ISO, the window shows an empty VM display and Start is disabled.

## Boot a Linux ISO

Choose a bootable ISO for your Mac's architecture: `aarch64` or `arm64` on Apple silicon, or `x86_64` on Intel.

```sh
make run FRAMEWORK=mygo ARGS='-iso /path/to/linux.iso -autostart'
```

Replace `mygo` with any framework above. The VM boots through EFI with 4 virtual CPUs and 4 GiB of memory. The ISO is read-only. Add `-disk /path/to/disk.raw` to attach an existing writable raw disk image.

`-capture-keys` sends supported system keyboard shortcuts to the guest. `-auto-resize` adjusts the guest resolution when you resize the window and requires macOS 14 or later.

For all flags, run `make run FRAMEWORK=mygo ARGS='-h'`.
