Example for GUI Linux and Run Rosetta within it
=======

This example is made by following [Running GUI Linux in a virtual machine on a Mac](https://developer.apple.com/documentation/virtualization/running_gui_linux_in_a_virtual_machine_on_a_mac) and [Running Intel Binaries in Linux VMs with Rosetta](https://developer.apple.com/documentation/virtualization/running_intel_binaries_in_linux_vms_with_rosetta).

You can get knowledge build and codesign process in Makefile.

## Build

```sh
make all
```

## Resources

Some resources will be created in `GUI Linux VM.bundle` directory on your home directory.

## Run

- `INSTALLER_ISO_PATH=/YOUR_INSTALLER_PATH/linux.iso ./virtualization -install` install Linux OS to your VM.
  - If you look up any installers, you can find easily in [Download-Linux](https://github.com/Code-Hex/vz/wiki/Download-Linux) page.
- `./virtualization` run Linux VM from `Disk.img` which is installed in `GUI Linux VM.bundle`.
